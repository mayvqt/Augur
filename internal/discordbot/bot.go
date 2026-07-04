package discordbot

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"augur/internal/config"
	"augur/internal/seer"

	"github.com/bwmarrin/discordgo"
)

const (
	commandLink    = "link"
	commandRequest = "request"
	componentPick  = "augur:pick:"
	discordPath    = "/profile/settings/notifications/discord"
)

type Handler interface {
	Search(ctx context.Context, query string) ([]seer.SearchResult, error)
	Request(ctx context.Context, discordID string, result seer.SearchResult) (seer.Request, error)
}

type Bot struct {
	session *discordgo.Session
	cfg     config.DiscordConfig
	link    config.LinkConfig
	handler Handler
	logger  *slog.Logger
	ctx     context.Context
	cache   selectionCache
}

func New(cfg config.DiscordConfig, link config.LinkConfig, handler Handler, logger *slog.Logger) (*Bot, error) {
	session, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, err
	}
	bot := &Bot{session: session, cfg: cfg, link: link, handler: handler, logger: logger, ctx: context.Background()}
	session.AddHandler(bot.onReady)
	session.AddHandler(bot.onInteraction)
	return bot, nil
}

func (b *Bot) Start(ctx context.Context) error {
	b.ctx = ctx
	b.session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsDirectMessages
	if err := b.session.Open(); err != nil {
		return err
	}
	if err := b.applyPresence(); err != nil {
		b.logger.Error("discord presence update failed", "error", err)
	}
	_, err := b.session.ApplicationCommandBulkOverwrite(b.session.State.User.ID, b.cfg.GuildID, slashCommands())
	if err != nil {
		return err
	}
	b.logger.Info("discord slash commands registered", "guild_id", b.cfg.GuildID)
	return nil
}

func (b *Bot) Close() error {
	b.logger.Info("closing discord session")
	return b.session.Close()
}

func slashCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{Name: commandLink, Description: "Link your Discord account to Seerr."},
		{
			Name:        commandRequest,
			Description: "Search Seerr and request a movie or show.",
			Options: []*discordgo.ApplicationCommandOption{{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "query",
				Description: "Movie or show title",
				Required:    true,
				MinLength:   intPtr(2),
				MaxLength:   100,
			}},
		},
	}
}

func (b *Bot) onReady(_ *discordgo.Session, event *discordgo.Ready) {
	b.logger.Info("discord ready", "user", event.User.String())
	if err := b.applyPresence(); err != nil {
		b.logger.Error("discord presence update failed", "error", err)
	}
}

func (b *Bot) onInteraction(s *discordgo.Session, interaction *discordgo.InteractionCreate) {
	if interaction == nil || interaction.Interaction == nil {
		return
	}
	switch interaction.Type {
	case discordgo.InteractionApplicationCommand:
		b.handleCommand(s, interaction)
	case discordgo.InteractionMessageComponent:
		b.handleComponent(s, interaction)
	}
}

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
	for _, result := range results {
		if len(options) == 25 {
			break
		}
		label := truncate(optionLabel(result), 100)
		description := truncate(optionDescription(result), 100)
		key := strconv.Itoa(len(options))
		b.cache.set(cacheID, key, result)
		options = append(options, discordgo.SelectMenuOption{Label: label, Description: description, Value: key})
	}
	if len(options) == 0 {
		b.edit(s, i, "No movies or shows matched that search.")
		return
	}
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

func (b *Bot) NotifyComplete(ctx context.Context, discordID, title string) error {
	channel, err := b.session.UserChannelCreate(discordID)
	if err != nil {
		return err
	}
	_, err = b.session.ChannelMessageSend(channel.ID, fmt.Sprintf("Your request for **%s** is now available.", title))
	return err
}

func (b *Bot) applyPresence() error {
	if !b.cfg.Presence.Enabled {
		return b.session.UpdateStatusComplex(discordgo.UpdateStatusData{Status: "online"})
	}
	return b.session.UpdateStatusComplex(discordgo.UpdateStatusData{
		Status: b.cfg.Presence.Status,
		Activities: []*discordgo.Activity{{
			Name: b.cfg.Presence.Message,
			Type: presenceActivityType(b.cfg.Presence.Type),
		}},
	})
}

func (b *Bot) linkURL(discordID string) string {
	u, err := url.Parse(b.link.PublicURL)
	if err != nil {
		return b.link.PublicURL
	}
	if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), discordPath) {
		u.Path = strings.TrimRight(u.Path, "/") + discordPath
	}
	q := u.Query()
	q.Set("discord_id", discordID)
	q.Set("discordId", discordID)
	u.RawQuery = q.Encode()
	return u.String()
}

func presenceActivityType(value string) discordgo.ActivityType {
	switch value {
	case "playing":
		return discordgo.ActivityTypeGame
	case "listening":
		return discordgo.ActivityTypeListening
	case "competing":
		return discordgo.ActivityTypeCompeting
	default:
		return discordgo.ActivityTypeWatching
	}
}

func (b *Bot) respond(s *discordgo.Session, i *discordgo.InteractionCreate, data *discordgo.InteractionResponseData) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: data}); err != nil {
		b.logger.Error("respond interaction", "error", err)
	}
}

func (b *Bot) ephemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	b.respond(s, i, &discordgo.InteractionResponseData{Content: msg, Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions()})
}

func (b *Bot) deferInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) bool {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral, AllowedMentions: noMentions()},
	})
	if err != nil {
		b.logger.Error("defer interaction", "error", err)
		return false
	}
	return true
}

func (b *Bot) edit(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	_, err := s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &msg, AllowedMentions: noMentions(), Components: &[]discordgo.MessageComponent{}})
	if err != nil {
		b.logger.Error("edit interaction", "error", err)
	}
}

func optionString(i *discordgo.InteractionCreate, name string) string {
	for _, opt := range i.ApplicationCommandData().Options {
		if opt != nil && opt.Name == name && opt.Type == discordgo.ApplicationCommandOptionString {
			return opt.StringValue()
		}
	}
	return ""
}

func interactionUserID(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	if i.User != nil {
		return i.User.ID
	}
	return ""
}

func optionLabel(result seer.SearchResult) string {
	title := result.Title
	if title == "" {
		title = result.Name
	}
	if title == "" {
		title = strconv.Itoa(result.ID)
	}
	year := releaseYear(result)
	if year != "" {
		return fmt.Sprintf("%s (%s)", title, year)
	}
	return title
}

func optionDescription(result seer.SearchResult) string {
	switch result.MediaType {
	case "movie":
		return "Movie"
	case "tv":
		return "TV show"
	default:
		return result.MediaType
	}
}

func releaseYear(result seer.SearchResult) string {
	date := result.ReleaseDate
	if date == "" {
		date = result.FirstAirDate
	}
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "..."
}

func intPtr(v int) *int {
	return &v
}

func noMentions() *discordgo.MessageAllowedMentions {
	return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
}
