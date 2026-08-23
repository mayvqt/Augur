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

const (
	expiredSearchMessage       = "That search expired. Run `/request` again."
	expiredPickerMessage       = "That picker expired. Run `/request` again."
	expiredSeasonPickerMessage = "That season picker expired. Run `/request` again."
)

func (b *Bot) handleCommand(s interactionSession, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case commandLink:
		b.handleLink(s, i)
	case commandRequest:
		b.handleRequest(s, i)
	case commandSetup:
		b.handleSetup(s, i)
	}
}

func (b *Bot) handleSetup(s interactionSession, i *discordgo.InteractionCreate) {
	if i.GuildID == "" || !canManageServer(i) {
		b.ephemeral(s, i, "You need Manage Server permission to configure approval messages.")
		return
	}
	enabled := false
	channelID := ""
	for _, option := range i.ApplicationCommandData().Options {
		switch option.Name {
		case "enabled":
			enabled = option.BoolValue()
		case "channel":
			channelID, _ = option.Value.(string)
		}
	}
	if enabled && channelID == "" {
		b.ephemeral(s, i, "Choose a channel when enabling approval messages.")
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 10*time.Second)
	defer cancel()
	if err := b.handler.ConfigureApprovals(ctx, i.GuildID, channelID, enabled); err != nil {
		b.logger.Error("configure approval messages", "guild_id", i.GuildID, "error", err)
		b.ephemeral(s, i, "Could not save the approval settings. Try again.")
		return
	}
	if enabled {
		b.ephemeral(s, i, "Approval messages are enabled in <#"+channelID+">.")
	} else {
		b.ephemeral(s, i, "Approval messages are disabled for this server.")
	}
}

func (b *Bot) handleLink(s interactionSession, i *discordgo.InteractionCreate) {
	userID := interactionUserID(i)
	if userID == "" {
		b.ephemeral(s, i, "Discord did not provide your user ID. Try again.")
		return
	}
	linkURL := b.linkURL(userID)
	content := fmt.Sprintf("Connect Discord to Seerr so requests use your account and quota.\n\nCopy your Discord ID into the Seerr settings page:\n```\n%s\n```\nAfter saving, run `/request` again.", userID)
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
	quota, err := b.handler.Quota(ctx, ownerID)
	if err != nil {
		var userErr interface{ UserMessage() string }
		if errors.As(err, &userErr) {
			b.editLinkRequired(s, i, userErr.UserMessage(), ownerID)
			return
		}
		b.edit(s, i, "Could not verify your Seerr account. Try again in a minute.")
		b.logger.Warn("seer account lookup failed", "error", err)
		return
	}
	cacheID, err := randomID()
	if err != nil {
		b.edit(s, i, "Could not create a secure result picker. Try again.")
		b.logger.Error("generate result picker ID", "error", err)
		return
	}
	b.cache.set(cacheID, ownerID, query, nil)
	b.cache.setQuota(cacheID, ownerID, quota)
	b.searchAndShow(ctx, s, i, cacheID, ownerID, query)
}

func (b *Bot) searchAndShow(ctx context.Context, s interactionSession, i *discordgo.InteractionCreate, cacheID, ownerID, query string) {
	results, err := b.handler.Search(ctx, query)
	if err != nil {
		b.editSearchRetry(s, i, "Seerr search failed. Try again in a minute.", cacheID)
		// Search text is user-controlled and may itself contain private data.
		b.logger.Error("seer search failed", "error", err)
		return
	}
	options := make([]discordgo.SelectMenuOption, 0, 25)
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
	if !b.cache.setResults(cacheID, ownerID, selections) {
		b.edit(s, i, expiredSearchMessage)
		return
	}
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
	case strings.HasPrefix(data.CustomID, componentBack):
		b.handleBack(s, i, data)
	case strings.HasPrefix(data.CustomID, componentRetry):
		b.handleRetry(s, i, data)
	case strings.HasPrefix(data.CustomID, componentSearch):
		b.handleSearchRetry(s, i, data)
	case strings.HasPrefix(data.CustomID, componentApprove):
		b.handleApproval(s, i, data, "approve")
	case strings.HasPrefix(data.CustomID, componentDecline):
		b.handleApproval(s, i, data, "decline")
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
		b.edit(s, i, expiredPickerMessage)
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()
	quota, quotaKnown := b.cache.getQuota(cacheID, ownerID)
	if !quotaKnown {
		b.edit(s, i, expiredPickerMessage)
		return
	}
	if result.MediaInfo != nil && seer.IsMediaAvailable(result.MediaInfo.Status) {
		b.editPreview(s, i, b.mediaPreview(result, quota), b.availableComponents(cacheID, result))
		return
	}
	if result.MediaType != "tv" {
		b.editPreview(s, i, b.mediaPreview(result, quota), previewComponents(cacheID, key))
		return
	}
	seasons, err := b.handler.TVSeasons(ctx, result.ID)
	if err != nil {
		b.logger.Error("seer TV details failed", "media_id", result.ID, "error", err)
		embed := b.mediaPreview(result, quota)
		embed.Footer.Text = "Could not load this show's seasons. Choose another title or try again."
		b.editPreview(s, i, embed, backComponents(cacheID))
		return
	}
	if !b.cache.setAvailableSeasons(cacheID, key, ownerID, seasons) {
		b.edit(s, i, expiredPickerMessage)
		return
	}
	b.editPreview(
		s,
		i,
		b.mediaPreview(result, quota),
		seasonPickerComponents(cacheID, key, seasons, quota, seer.SeasonSelection{}),
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
	b.showSeasonSelection(s, i, cacheID, key, selection)
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
	ownerID := interactionUserID(i)
	if !b.cache.setSeasons(cacheID, key, ownerID, seer.SeasonSelection{All: true}) {
		b.edit(s, i, expiredSeasonPickerMessage)
		return
	}
	b.submitRequest(s, i, cacheID, key)
}

func (b *Bot) showSeasonSelection(
	s interactionSession,
	i *discordgo.InteractionCreate,
	cacheID string,
	key string,
	selection seer.SeasonSelection,
) {
	ownerID := interactionUserID(i)
	result, seasons, _, ok := b.cache.selection(cacheID, key, ownerID)
	if !ok || result.MediaType != "tv" {
		b.edit(s, i, expiredSeasonPickerMessage)
		return
	}
	quota, ok := b.cache.getQuota(cacheID, ownerID)
	if !ok {
		b.edit(s, i, expiredSeasonPickerMessage)
		return
	}
	if err := validatePickerSelection(selection, quota); err != nil {
		b.edit(s, i, err.Error())
		return
	}
	if !b.cache.setSeasons(cacheID, key, ownerID, selection) {
		b.edit(s, i, expiredSeasonPickerMessage)
		return
	}
	embed := b.mediaPreview(result, quota)
	b.editPreview(
		s,
		i,
		embed,
		seasonPickerComponents(cacheID, key, seasons, quota, selection),
	)
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
	b.submitRequest(s, i, cacheID, key)
}

func (b *Bot) submitRequest(s interactionSession, i *discordgo.InteractionCreate, cacheID, key string) {
	ownerID := interactionUserID(i)
	result, _, seasons, ok := b.cache.selection(cacheID, key, ownerID)
	if !ok {
		b.edit(s, i, "That confirmation expired or was already used. Run `/request` again.")
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	req, err := b.handler.Request(ctx, ownerID, result, seasons)
	if err != nil {
		message := "Request failed. Try again in a minute."
		var userErr interface{ UserMessage() string }
		if errors.As(err, &userErr) {
			message = userErr.UserMessage()
		}
		b.editRetry(s, i, truncate(message, 180), cacheID, key)
		b.logger.Error("request media failed", "media_type", result.MediaType, "media_id", result.ID, "error", err)
		return
	}
	b.cache.discard(cacheID, ownerID)
	if seer.IsPendingRequest(req.Status) {
		b.postApproval(ctx, i.GuildID, req.ID, ownerID, result, seasons)
	}
	title := escapeMarkdown(optionLabel(result))
	selection := ""
	if result.MediaType == "tv" {
		selection = "\nSelected: **" + escapeMarkdown(seasonSelectionLabel(seasons)) + "**"
	}
	b.edit(s, i, fmt.Sprintf("Your request for **%s** has been submitted successfully.%s\nYou will receive a direct message when it is fully available.", title, selection))
}

func (b *Bot) postApproval(ctx context.Context, guildID string, requestID int, requesterID string, result seer.SearchResult, seasons seer.SeasonSelection) {
	if guildID == "" || requestID <= 0 {
		return
	}
	channelID, enabled, err := b.handler.ApprovalChannel(ctx, guildID)
	if err != nil || !enabled {
		if err != nil {
			b.logger.Error("load approval channel", "guild_id", guildID, "error", err)
		}
		return
	}
	claimed, err := b.handler.ClaimApproval(ctx, requestID, guildID, channelID)
	if err != nil || !claimed {
		if err != nil {
			b.logger.Error("claim approval message", "request_id", requestID, "error", err)
		}
		return
	}
	embed := b.mediaPreview(result, nil)
	embed.Color = 0xFEE75C
	embed.Fields = []*discordgo.MessageEmbedField{
		{Name: "Requested by", Value: "<@" + requesterID + ">", Inline: true},
		{Name: "Status", Value: "Pending approval", Inline: true},
	}
	if result.MediaType == "tv" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Seasons", Value: seasonSelectionLabel(seasons), Inline: true})
	}
	embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Seerr request #%d", requestID)}
	message, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds:          []*discordgo.MessageEmbed{embed},
		Components:      approvalComponents(requestID),
		AllowedMentions: noMentions(),
	})
	if err != nil {
		_ = b.handler.ReleaseApproval(ctx, requestID, guildID)
		b.logger.Error("send approval message", "request_id", requestID, "channel_id", channelID, "error", err)
		return
	}
	if err := b.handler.FinishApproval(ctx, requestID, guildID, channelID, message.ID); err != nil {
		b.logger.Error("save approval message", "request_id", requestID, "error", err)
	}
}

func approvalComponents(requestID int) []discordgo.MessageComponent {
	id := strconv.Itoa(requestID)
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{CustomID: componentApprove + id, Label: "Approve", Style: discordgo.SuccessButton},
		discordgo.Button{CustomID: componentDecline + id, Label: "Decline", Style: discordgo.DangerButton},
	}}}
}

func (b *Bot) handleApproval(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData, action string) {
	if i.GuildID == "" || !canManageServer(i) {
		b.ephemeral(s, i, "You need Manage Server permission to approve or decline requests.")
		return
	}
	prefix := componentApprove
	if action == "decline" {
		prefix = componentDecline
	}
	requestID, err := strconv.Atoi(strings.TrimPrefix(data.CustomID, prefix))
	if err != nil || requestID <= 0 {
		b.ephemeral(s, i, "That approval button is invalid.")
		return
	}
	if !b.deferComponentUpdate(s, i) {
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	request, err := b.handler.DecideRequest(ctx, requestID, action)
	if err != nil {
		b.logger.Error("update Seerr request", "request_id", requestID, "action", action, "error", err)
		b.editContent(s, i, "Seerr could not update this request. Try again.", approvalComponents(requestID), "restore failed approval")
		return
	}
	status := seer.RequestStatusLabel(request.Status)
	if len(i.Message.Embeds) > 0 {
		embed := *i.Message.Embeds[0]
		embed.Color = 0x57F287
		if status == "Declined" {
			embed.Color = 0xED4245
		}
		for _, field := range embed.Fields {
			if field.Name == "Status" {
				field.Value = status
			}
		}
		embed.Footer = &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("Seerr request #%d · %s by %s", requestID, status, interactionDisplayName(i))}
		empty := ""
		components := []discordgo.MessageComponent{}
		_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &empty, Embeds: &[]*discordgo.MessageEmbed{&embed}, Components: &components, AllowedMentions: noMentions()})
		if err != nil {
			b.logger.Error("update approval message", "request_id", requestID, "error", err)
		}
	}
}

func interactionDisplayName(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		if i.Member.Nick != "" {
			return normalizeInlineText(i.Member.Nick)
		}
		if i.Member.User != nil {
			return normalizeInlineText(i.Member.User.Username)
		}
	}
	return "an administrator"
}

func canManageServer(i *discordgo.InteractionCreate) bool {
	if i == nil || i.Member == nil {
		return false
	}
	permissions := i.Member.Permissions
	return permissions&discordgo.PermissionAdministrator != 0 || permissions&discordgo.PermissionManageServer != 0
}

func (b *Bot) handleBack(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData) {
	if !b.deferComponentUpdate(s, i) {
		return
	}
	cacheID := strings.TrimPrefix(data.CustomID, componentBack)
	ownerID := interactionUserID(i)
	options := b.cache.options(cacheID, ownerID)
	if len(options) == 0 {
		b.edit(s, i, expiredSearchMessage)
		return
	}
	menuOptions := make([]discordgo.SelectMenuOption, 0, len(options))
	for _, option := range options {
		menuOptions = append(menuOptions, discordgo.SelectMenuOption{Label: truncate(optionLabel(option.result), 100), Value: option.key})
	}
	query, _ := b.cache.query(cacheID, ownerID)
	msg := fmt.Sprintf("Results for **%s**. Choose a title.", escapeMarkdown(query))
	b.editContent(s, i, msg, []discordgo.MessageComponent{resultPickerRow(cacheID, "Choose a title", menuOptions)}, "restore search results")
}

func (b *Bot) handleRetry(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData) {
	if !b.deferComponentUpdate(s, i) {
		return
	}
	cacheID, key, ok := componentSelection(data.CustomID, componentRetry)
	if !ok {
		b.edit(s, i, "That retry expired. Run `/request` again.")
		return
	}
	b.submitRequest(s, i, cacheID, key)
}

func (b *Bot) handleSearchRetry(s interactionSession, i *discordgo.InteractionCreate, data discordgo.MessageComponentInteractionData) {
	if !b.deferComponentUpdate(s, i) {
		return
	}
	cacheID := strings.TrimPrefix(data.CustomID, componentSearch)
	ownerID := interactionUserID(i)
	query, ok := b.cache.query(cacheID, ownerID)
	if !ok {
		b.edit(s, i, expiredSearchMessage)
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()
	b.searchAndShow(ctx, s, i, cacheID, ownerID, query)
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
