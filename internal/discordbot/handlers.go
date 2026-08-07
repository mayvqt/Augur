package discordbot

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mayvqt/Augur/internal/seer"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleCommand(s interactionSession, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case commandLink:
		b.handleLink(s, i)
	case commandRequest:
		b.handleRequest(s, i)
	}
}

func (b *Bot) handleLink(s interactionSession, i *discordgo.InteractionCreate) {
	userID := interactionUserID(i)
	if userID == "" {
		b.ephemeral(s, i, "Discord did not provide your user ID. Try again.")
		return
	}
	linkURL := b.linkURL(userID)
	content := fmt.Sprintf("Open your Seerr Discord notification settings and paste your Discord ID64.\nDiscord ID64: `%s`\nURL: %s", userID, linkURL)
	data := &discordgo.InteractionResponseData{
		Content:         content,
		Flags:           discordgo.MessageFlagsEphemeral,
		AllowedMentions: noMentions(),
		Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Open link page", Style: discordgo.LinkButton, URL: linkURL},
		}}},
	}
	b.respond(s, i, data)
}

func (b *Bot) handleRequest(s interactionSession, i *discordgo.InteractionCreate) {
	query := strings.TrimSpace(optionString(i, "query"))
	if query == "" {
		b.ephemeral(s, i, "Type a title to search for.")
		return
	}
	ownerID := interactionUserID(i)
	if ownerID == "" {
		b.ephemeral(s, i, "Discord did not provide your user ID. Try again.")
		return
	}
	if !b.deferInteraction(s, i) {
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()
	results, err := b.handler.Search(ctx, query)
	if err != nil {
		b.edit(s, i, "Seerr search failed. Try again in a minute.")
		// Search text is user-controlled and may itself contain private data.
		b.logger.Error("seer search failed", "error", err)
		return
	}
	options := make([]discordgo.SelectMenuOption, 0, 25)
	cacheID, err := randomID()
	if err != nil {
		b.edit(s, i, "Could not create a secure result picker. Try again.")
		b.logger.Error("generate result picker ID", "error", err)
		return
	}
	selections := make([]seer.SearchResult, 0, 25)
	for _, result := range results {
		if len(options) == 25 {
			break
		}
		label := truncate(optionLabel(result), 100)
		key := strconv.Itoa(len(options))
		selections = append(selections, result)
		options = append(options, discordgo.SelectMenuOption{Label: label, Value: key})
	}
	if len(options) == 0 {
		b.edit(s, i, "No movies or shows matched that search.")
		return
	}
	b.cache.set(cacheID, ownerID, selections)
	msg := "Select a result to preview."
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:         &msg,
		Components:      &[]discordgo.MessageComponent{resultPickerRow(cacheID, "Choose a title", options)},
		AllowedMentions: noMentions(),
	})
	if err != nil {
		b.logger.Error("edit request search response", "error", err)
	}
}

func (b *Bot) handleComponent(s interactionSession, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	switch {
	case strings.HasPrefix(data.CustomID, componentPick):
		b.handlePick(s, i, data)
	case strings.HasPrefix(data.CustomID, componentSeasons):
		b.handleSeasons(s, i, data)
	case strings.HasPrefix(data.CustomID, componentAll):
		b.handleAllSeasons(s, i, data)
	case strings.HasPrefix(data.CustomID, componentConfirm):
		b.handleConfirm(s, i, data)
	case strings.HasPrefix(data.CustomID, componentCancel):
		b.handleCancel(s, i, data)
	}
}

func (b *Bot) handlePick(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData) {
	if len(data.Values) != 1 {
		return
	}
	if !b.deferComponentUpdate(s, i) {
		return
	}
	cacheID := strings.TrimPrefix(data.CustomID, componentPick)
	key := data.Values[0]
	ownerID := interactionUserID(i)
	result, ok := b.cache.get(cacheID, key, ownerID)
	if !ok {
		b.edit(s, i, "That picker expired. Run `/request` again.")
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()
	quota, quotaKnown := b.cache.getQuota(cacheID, ownerID)
	var quotaErr error
	if !quotaKnown {
		quota, quotaErr = b.handler.Quota(ctx, ownerID)
		if quotaErr != nil {
			b.logger.Warn("seer quota lookup failed", "error", quotaErr)
		} else if !b.cache.setQuota(cacheID, ownerID, quota) {
			b.edit(s, i, "That picker expired. Run `/request` again.")
			return
		}
	}
	if result.MediaType != "tv" {
		b.editBrowsablePreview(s, i, cacheID, key, b.mediaPreview(result, quota), previewComponents(cacheID, key))
		return
	}
	if quotaErr != nil {
		b.editBrowsablePreview(s, i, cacheID, key, b.mediaPreview(result, nil), cancelComponents(cacheID))
		return
	}
	seasons, err := b.handler.TVSeasons(ctx, result.ID)
	if err != nil {
		b.logger.Error("seer TV details failed", "media_id", result.ID, "error", err)
		embed := b.mediaPreview(result, quota)
		embed.Footer.Text = "Could not load this show's seasons. Choose another title or try again."
		b.editBrowsablePreview(s, i, cacheID, key, embed, cancelComponents(cacheID))
		return
	}
	b.editBrowsablePreview(
		s,
		i,
		cacheID,
		key,
		b.mediaPreview(result, quota),
		seasonPickerComponents(cacheID, key, seasons, quota),
	)
}

func (b *Bot) handleSeasons(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData) {
	if len(data.Values) == 0 {
		return
	}
	if !b.deferComponentUpdate(s, i) {
		return
	}
	cacheID, key, ok := componentSelection(data.CustomID, componentSeasons)
	if !ok {
		b.edit(s, i, "That season picker is invalid. Run `/request` again.")
		return
	}
	selection, err := parseSeasonValues(data.Values)
	if err != nil {
		b.edit(s, i, err.Error())
		return
	}
	b.showSeasonConfirmation(s, i, cacheID, key, selection)
}

func (b *Bot) handleAllSeasons(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData) {
	if !b.deferComponentUpdate(s, i) {
		return
	}
	cacheID, key, ok := componentSelection(data.CustomID, componentAll)
	if !ok {
		b.edit(s, i, "That season selection is invalid. Run `/request` again.")
		return
	}
	b.showSeasonConfirmation(s, i, cacheID, key, seer.SeasonSelection{All: true})
}

func (b *Bot) showSeasonConfirmation(
	s interactionSession,
	i *discordgo.InteractionCreate,
	cacheID string,
	key string,
	selection seer.SeasonSelection,
) {
	ownerID := interactionUserID(i)
	result, ok := b.cache.get(cacheID, key, ownerID)
	if !ok || result.MediaType != "tv" {
		b.edit(s, i, "That season picker expired. Run `/request` again.")
		return
	}
	quota, ok := b.cache.getQuota(cacheID, ownerID)
	if !ok {
		b.edit(s, i, "That season picker expired. Run `/request` again.")
		return
	}
	if err := validatePickerSelection(selection, quota); err != nil {
		b.edit(s, i, err.Error())
		return
	}
	if !b.cache.setSeasons(cacheID, key, ownerID, selection) {
		b.edit(s, i, "That season picker expired. Run `/request` again.")
		return
	}
	embed := b.mediaPreview(result, quota)
	embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
		Name:  "Selected seasons",
		Value: seasonSelectionLabel(selection),
	})
	b.editBrowsablePreview(
		s,
		i,
		cacheID,
		key,
		embed,
		previewComponents(cacheID, key),
	)
}

func (b *Bot) editBrowsablePreview(
	s interactionSession,
	i *discordgo.InteractionCreate,
	cacheID string,
	selectedKey string,
	embed *discordgo.MessageEmbed,
	components []discordgo.MessageComponent,
) {
	components = b.browsableComponents(cacheID, selectedKey, interactionUserID(i), components)
	b.editPreview(s, i, embed, components)
}

func (b *Bot) handleConfirm(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData) {
	if !b.deferComponentUpdate(s, i) {
		return
	}
	cacheID, key, ok := componentSelection(data.CustomID, componentConfirm)
	if !ok {
		b.edit(s, i, "That confirmation is invalid. Run `/request` again.")
		return
	}
	ownerID := interactionUserID(i)
	result, seasons, ok := b.cache.take(cacheID, key, ownerID)
	if !ok {
		b.edit(s, i, "That confirmation expired or was already used. Run `/request` again.")
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	_, err := b.handler.Request(ctx, ownerID, result, seasons)
	if err != nil {
		message := "Request failed. Try again in a minute."
		var userErr interface{ UserMessage() string }
		if errors.As(err, &userErr) {
			message = userErr.UserMessage()
		}
		b.edit(s, i, truncate(message, 180))
		b.logger.Error("request media failed", "media_type", result.MediaType, "media_id", result.ID, "error", err)
		return
	}
	title := escapeMarkdown(optionLabel(result))
	selection := ""
	if result.MediaType == "tv" {
		selection = "\nSelected: **" + escapeMarkdown(seasonSelectionLabel(seasons)) + "**"
	}
	b.edit(s, i, fmt.Sprintf("Your request for **%s** has been submitted successfully.%s\nYou will receive a direct message when it is fully available.", title, selection))
}

func (b *Bot) handleCancel(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData) {
	if !b.deferComponentUpdate(s, i) {
		return
	}
	cacheID := strings.TrimPrefix(data.CustomID, componentCancel)
	b.cache.discard(cacheID, interactionUserID(i))
	b.edit(s, i, "Request cancelled.")
}

func parseSeasonValues(values []string) (seer.SeasonSelection, error) {
	selection := seer.SeasonSelection{}
	seen := make(map[int]struct{}, len(values))
	for _, value := range values {
		number, err := strconv.Atoi(value)
		if err != nil || number < 0 {
			return seer.SeasonSelection{}, errors.New("the selected seasons are invalid; run `/request` again")
		}
		if _, duplicate := seen[number]; duplicate {
			continue
		}
		seen[number] = struct{}{}
		selection.Numbers = append(selection.Numbers, number)
	}
	if len(selection.Numbers) == 0 {
		return seer.SeasonSelection{}, errors.New("select at least one season")
	}
	sort.Ints(selection.Numbers)
	return selection, nil
}

func componentSelection(customID, prefix string) (cacheID, key string, ok bool) {
	value, found := strings.CutPrefix(customID, prefix)
	if !found {
		return "", "", false
	}
	cacheID, key, found = strings.Cut(value, ":")
	return cacheID, key, found && cacheID != "" && key != ""
}

func validatePickerSelection(selection seer.SeasonSelection, quota *seer.Quota) error {
	if selection.All {
		if quota == nil || quota.TV.Restricted {
			return errors.New("all seasons are only available with an unlimited TV request limit")
		}
		return nil
	}
	if quota != nil && quota.TV.Restricted && len(selection.Numbers) > quota.TV.Remaining {
		return fmt.Errorf("you can select up to %d more TV season(s)", quota.TV.Remaining)
	}
	return nil
}
