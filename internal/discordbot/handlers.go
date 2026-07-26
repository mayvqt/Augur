package discordbot

import (
	"context"
	"errors"
	"fmt"
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
	if !b.deferInteraction(s, i) {
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 20*time.Second)
	defer cancel()
	results, err := b.handler.Search(ctx, query)
	if err != nil {
		b.edit(s, i, "Seerr search failed. Try again in a minute.")
		b.logger.Error("seer search failed", "query", query, "error", err)
		return
	}
	options := make([]discordgo.SelectMenuOption, 0, 25)
	cacheID, err := randomID()
	if err != nil {
		b.edit(s, i, "Could not create a secure result picker. Try again.")
		b.logger.Error("generate result picker ID", "error", err)
		return
	}
	selections := make(map[string]seer.SearchResult, 25)
	for _, result := range results {
		if len(options) == 25 {
			break
		}
		label := truncate(optionLabel(result), 100)
		description := truncate(optionDescription(result), 100)
		key := strconv.Itoa(len(options))
		selections[key] = result
		options = append(options, discordgo.SelectMenuOption{Label: label, Description: description, Value: key})
	}
	if len(options) == 0 {
		b.edit(s, i, "No movies or shows matched that search.")
		return
	}
	b.cache.setMany(cacheID, interactionUserID(i), selections)
	msg := "Pick the result to request."
	_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &msg,
		Components: &[]discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    componentPick + cacheID,
				Placeholder: "Choose a title",
				MinValues:   intPtr(1),
				MaxValues:   1,
				Options:     options,
			},
		}}},
		AllowedMentions: noMentions(),
	})
	if err != nil {
		b.logger.Error("edit request search response", "error", err)
	}
}

func (b *Bot) handleComponent(s interactionSession, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if !strings.HasPrefix(data.CustomID, componentPick) || len(data.Values) == 0 {
		return
	}
	if !b.deferInteraction(s, i) {
		return
	}
	cacheID := strings.TrimPrefix(data.CustomID, componentPick)
	result, ok := b.cache.take(cacheID, data.Values[0], interactionUserID(i))
	if !ok {
		b.edit(s, i, "That picker expired. Run `/request` again.")
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	_, err := b.handler.Request(ctx, interactionUserID(i), result)
	if err != nil {
		message := "Request failed. Try again in a minute."
		var userErr interface{ UserMessage() string }
		if errors.As(err, &userErr) {
			message = userErr.UserMessage()
		}
		b.edit(s, i, truncate(message, 180))
		b.logger.Error("request media failed", "discord_id", interactionUserID(i), "media_type", result.MediaType, "media_id", result.ID, "error", err)
		return
	}
	title := escapeMarkdown(optionLabel(result))
	summary := escapeMarkdown(requestSummary(result))
	b.edit(s, i, fmt.Sprintf("Requested **%s**\n%s\nI will DM you when it is fully available.", title, summary))
}
