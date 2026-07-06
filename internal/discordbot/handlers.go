package discordbot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mayvqt/Augur/internal/seer"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch i.ApplicationCommandData().Name {
	case commandLink:
		b.handleLink(s, i)
	case commandRequest:
		b.handleRequest(s, i)
	}
}

func (b *Bot) handleLink(s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID := interactionUserID(i)
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

func (b *Bot) handleRequest(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	cacheID := randomID()
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
	b.cache.setMany(cacheID, selections)
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

func (b *Bot) handleComponent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	if !strings.HasPrefix(data.CustomID, componentPick) || len(data.Values) == 0 {
		return
	}
	if !b.deferInteraction(s, i) {
		return
	}
	cacheID := strings.TrimPrefix(data.CustomID, componentPick)
	result, ok := b.cache.get(cacheID, data.Values[0])
	if !ok {
		b.edit(s, i, "That picker expired. Run `/request` again.")
		return
	}
	ctx, cancel := context.WithTimeout(b.ctx, 30*time.Second)
	defer cancel()
	req, err := b.handler.Request(ctx, interactionUserID(i), result)
	if err != nil {
		b.edit(s, i, "Request failed: "+truncate(err.Error(), 160))
		return
	}
	title := optionLabel(result)
	if req.ID > 0 {
		b.edit(s, i, fmt.Sprintf("Requested **%s**. I will DM you when it is available.", title))
		return
	}
	b.edit(s, i, fmt.Sprintf("Requested **%s**.", title))
}
