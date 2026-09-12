package discordbot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
	"github.com/mayvqt/Augur/internal/storage"
)

type decisionHandler struct {
	Handler
	err          error
	presentation storage.ApprovalDecision
	decision     storage.ApprovalDecision
	changed      bool
	marked       storage.ApprovalMessage
}

func (h *decisionHandler) DecideRequest(_ context.Context, _ int, _ string, d storage.ApprovalDecision) (storage.ApprovalDecision, bool, error) {
	h.presentation = d
	return h.decision, h.changed, h.err
}
func (h *decisionHandler) MarkApprovalDecided(_ context.Context, m storage.ApprovalMessage, _ time.Time) error {
	h.marked = m
	return nil
}
func approvalInteraction() *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{Type: discordgo.InteractionMessageComponent, GuildID: "123", ChannelID: "456", Member: &discordgo.Member{Permissions: discordgo.PermissionManageGuild, User: &discordgo.User{ID: "123456789012345678", Username: "Second moderator"}}, Message: &discordgo.Message{ID: "789", ChannelID: "456", Embeds: []*discordgo.MessageEmbed{{Title: "Arrival", Fields: []*discordgo.MessageEmbedField{{Name: "Status", Value: "Pending approval"}}}}}}}
}
func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestApprovalErrorPreservesCardAndEmptyLegacyCardCanRecover(t *testing.T) {
	h := &decisionHandler{err: errors.New("temporary error"), decision: storage.ApprovalDecision{RequestID: 42, Status: "Approved", Title: "Arrival", Actor: "First moderator"}}
	b := &Bot{handler: h, ctx: context.Background(), logger: quietLogger()}
	s := &fakeInteractionSession{}
	i := approvalInteraction()
	b.handleApproval(s, i, discordgo.MessageComponentInteractionData{CustomID: componentApprove + "42"}, "approve")
	if s.edit == nil || s.edit.Embeds != nil || len(*s.edit.Components) == 0 {
		t.Fatal("failed approval erased the source card")
	}
	h.err = nil
	i.Message.Embeds = nil
	b.handleApproval(s, i, discordgo.MessageComponentInteractionData{CustomID: componentApprove + "42"}, "approve")
	if s.edit == nil || s.edit.Embeds == nil || len(*s.edit.Embeds) != 1 || (*s.edit.Embeds)[0].Title != "Arrival" {
		t.Fatal("legacy empty card did not recover")
	}
	if !strings.Contains((*s.edit.Embeds)[0].Footer.Text, "First moderator") || !strings.Contains(*s.edit.Content, "already approved") {
		t.Fatal("stale click changed decision attribution")
	}
	if h.marked.MessageID != "789" || h.marked.ChannelID != "456" {
		t.Fatal("card acknowledgement was not fenced", h.marked)
	}
}

func TestDeclineModalUsesDecodedComponentsAndAcknowledgesBeforeLookup(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			var data discordgo.ModalSubmitInteractionData
			if err := json.Unmarshal([]byte(`{"custom_id":"augur:decline-reason:42:456:789","components":[{"type":1,"components":[{"type":4,"custom_id":"reason","value":"  Not this week  "}]}]}`), &data); err != nil {
				t.Fatal(err)
			}
			h := &decisionHandler{changed: true, decision: storage.ApprovalDecision{RequestID: 42, Status: "Declined", Title: "Arrival", Actor: "Moderator", Reason: "Not this week"}}
			if fail {
				h.err = errors.New("offline")
			}
			s := &fakeInteractionSession{}
			i := approvalInteraction()
			i.Type = discordgo.InteractionModalSubmit
			i.Data = data
			b := &Bot{handler: h, ctx: context.Background(), logger: quietLogger()}
			b.fetchApprovalMessage = func(context.Context, string, string) (*discordgo.Message, error) {
				if s.response == nil || s.response.Type != discordgo.InteractionResponseDeferredChannelMessageWithSource {
					t.Fatal("modal was not acknowledged before I/O")
				}
				return &discordgo.Message{ID: "789", ChannelID: "456"}, nil
			}
			edited := false
			b.editApprovalMessage = func(_ context.Context, edit *discordgo.MessageEdit) (*discordgo.Message, error) {
				edited = true
				return &discordgo.Message{}, nil
			}
			b.handleDeclineModal(s, i)
			if h.presentation.Reason != "Not this week" {
				t.Fatal("decoded reason was lost", h.presentation)
			}
			if s.edit == nil || s.edit.Components == nil || len(*s.edit.Components) != 0 {
				t.Fatal("orphan approval controls were attached to modal response")
			}
			if !fail && (!edited || *s.edit.Content != "Request declined.") {
				t.Fatal("missing modal completion", s.edit)
			}
		})
	}
}

func TestDecidedEmbedDoesNotMutatePendingMessage(t *testing.T) {
	source := &discordgo.MessageEmbed{Title: "Arrival", Fields: []*discordgo.MessageEmbedField{{Name: "Status", Value: "Pending approval"}}}
	_ = decidedApprovalEmbed(source, 42, "Approved", "Moderator")
	if source.Fields[0].Value != "Pending approval" {
		t.Fatal("source embed was mutated")
	}
}

type maintenanceHandler struct {
	Handler
	store        *storage.Store
	request      seer.Request
	requestCalls int
}

func (h *maintenanceHandler) DueApprovalCleanup(ctx context.Context) ([]storage.ApprovalMessage, error) {
	return h.store.DueApprovalCleanup(ctx, time.Now())
}
func (h *maintenanceHandler) DueApprovalMessages(ctx context.Context, before time.Time) ([]storage.ApprovalMessage, error) {
	return h.store.DueApprovalMessages(ctx, before)
}
func (h *maintenanceHandler) ApprovalMessages(ctx context.Context) ([]storage.ApprovalMessage, error) {
	return h.store.ApprovalMessages(ctx)
}
func (h *maintenanceHandler) ApprovalDecision(ctx context.Context, id int, status string) (storage.ApprovalDecision, bool, error) {
	return h.store.ApprovalDecision(ctx, id, status)
}
func (h *maintenanceHandler) RequestStatus(context.Context, int) (seer.Request, error) {
	h.requestCalls++
	return h.request, nil
}
func (h *maintenanceHandler) MediaDetails(context.Context, string, int) (seer.SearchResult, error) {
	return seer.SearchResult{ID: 99, Title: "Arrival", MediaType: "movie"}, nil
}
func (h *maintenanceHandler) ObserveApprovalDecision(ctx context.Context, request seer.Request, d storage.ApprovalDecision) (storage.ApprovalDecision, error) {
	d.RequestID = request.ID
	d.Status = seer.RequestStatusLabel(request.Status)
	d.RequesterID = 7
	d.Actor = "Seerr"
	return h.store.RecordApprovalDecision(ctx, d)
}
func (h *maintenanceHandler) DeleteApprovalRecord(ctx context.Context, m storage.ApprovalMessage) error {
	return h.store.DeleteApprovalMessage(ctx, m)
}
func (h *maintenanceHandler) MarkApprovalDecided(ctx context.Context, m storage.ApprovalMessage, at time.Time) error {
	return h.store.MarkApprovalMessageDecided(ctx, m, at)
}
func (h *maintenanceHandler) RetryApproval(ctx context.Context, m storage.ApprovalMessage) error {
	return h.store.RetryApprovalMessage(ctx, m, time.Now().Add(time.Minute))
}

func TestBlankApprovalDeliveriesObserveExternalDecisionOnceAcrossGuilds(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, guild := range []string{"one", "two"} {
		store.SetApprovalSettings(ctx, storage.ApprovalSettings{GuildID: guild, ChannelID: "channel", Enabled: true})
		if _, ok, err := store.ClaimApprovalMessage(ctx, storage.ApprovalMessage{RequestID: 42, GuildID: guild, ChannelID: "channel"}, time.Now().Add(-time.Hour)); err != nil || !ok {
			t.Fatal(err)
		}
	}
	h := &maintenanceHandler{store: store, request: seer.Request{ID: 42, Status: 2, RequestedBy: &seer.User{ID: 7}, Media: &seer.Media{TMDBID: 99, MediaType: "movie"}}}
	b := &Bot{handler: h, ctx: ctx, logger: quietLogger()}
	b.fetchApprovalMessage = func(context.Context, string, string) (*discordgo.Message, error) {
		t.Fatal("looked up an empty message ID")
		return nil, nil
	}
	if err := b.MaintainApprovals(ctx); err != nil {
		t.Fatal(err)
	}
	if h.requestCalls != 1 {
		t.Fatal("request fetched once per guild", h.requestCalls)
	}
	if jobs, err := store.DueDecisionJobs(ctx, time.Now(), 25); err != nil || len(jobs) != 1 || jobs[0].Title != "Arrival" {
		t.Fatal("decision lost with failed card send", jobs, err)
	}
	if messages, err := store.ApprovalMessages(ctx); err != nil || len(messages) != 0 {
		t.Fatal("finished blank claims retained", messages, err)
	}
}

func TestCanonicalDecisionRepairsCardWithoutPendingPoll(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.SetApprovalSettings(ctx, storage.ApprovalSettings{GuildID: "g", ChannelID: "c", Enabled: true})
	claim, _, err := store.ClaimApprovalMessage(ctx, storage.ApprovalMessage{RequestID: 42, GuildID: "g", ChannelID: "c"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	claim.MessageID = "m"
	if err := store.FinishApprovalMessage(ctx, claim); err != nil {
		t.Fatal(err)
	}
	store.RecordApprovalDecision(ctx, storage.ApprovalDecision{RequestID: 42, RequesterID: 7, Status: "Declined", Title: "Arrival", Reason: "Not this week", Actor: "First moderator"})
	h := &maintenanceHandler{store: store}
	b := &Bot{handler: h, ctx: ctx, logger: quietLogger(), pendingIDs: map[int]bool{42: true}, pendingAt: time.Now()}
	b.fetchApprovalMessage = func(context.Context, string, string) (*discordgo.Message, error) {
		return &discordgo.Message{ID: "m", ChannelID: "c"}, nil
	}
	attempts := 0
	b.editApprovalMessage = func(_ context.Context, edit *discordgo.MessageEdit) (*discordgo.Message, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("Discord offline")
		}
		embed := (*edit.Embeds)[0]
		if embed.Title != "Arrival" || !strings.Contains(embed.Footer.Text, "First moderator") {
			t.Fatal("lost canonical card content")
		}
		return &discordgo.Message{}, nil
	}
	b.MaintainApprovals(ctx)
	store.RetryApprovalMessage(ctx, claim, time.Now().Add(-time.Second))
	if err := b.MaintainApprovals(ctx); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || h.requestCalls != 0 {
		t.Fatal("card repair depended on Seerr polling", attempts, h.requestCalls)
	}
	if due, err := store.DueApprovalMessages(ctx, time.Now().Add(time.Hour)); err != nil || len(due) != 1 {
		t.Fatal("retention started before successful render", due, err)
	}
}

func TestStartupFailureCancelsAndDrainsAcceptedInteraction(t *testing.T) {
	b, err := New(config.DiscordConfig{Token: "synthetic"}, config.LinkConfig{}, &decisionHandler{}, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	cancelled := make(chan struct{})
	release := make(chan struct{})
	b.openSession = func() error {
		b.lifecycleMu.Lock()
		b.interactions.Add(1)
		b.lifecycleMu.Unlock()
		go func() { defer b.interactions.Done(); <-b.ctx.Done(); close(cancelled); <-release }()
		return nil
	}
	b.closeSession = func() error { return nil }
	b.registerApplicationCommands = func() error { return errors.New("synthetic command registration failure") }
	done := make(chan error, 1)
	go func() { done <- b.Start(context.Background()) }()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("startup failure did not cancel interaction")
	}
	select {
	case <-done:
		t.Fatal("startup returned before interaction persistence finished")
	default:
	}
	close(release)
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "registration failure") {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup failed to drain")
	}
	b.onInteraction(nil, approvalInteraction()) // closed admission must not invoke handlers
}

func (h *maintenanceHandler) CompleteApprovalCleanup(ctx context.Context, m storage.ApprovalMessage) error {
	return h.store.CompleteApprovalCleanup(ctx, m)
}
func (h *maintenanceHandler) RetryApprovalCleanup(ctx context.Context, m storage.ApprovalMessage) error {
	return h.store.RetryApprovalCleanup(ctx, m, time.Now().Add(time.Minute))
}

func TestMaintenanceTimeoutDefersSlowCleanupAndAllowsNextCard(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, id := range []string{"1-slow", "2-healthy"} {
		if _, err := store.QueueUntrackedApprovalCleanup(ctx, storage.ApprovalMessage{ChannelID: "c", MessageID: id}); err != nil {
			t.Fatal(err)
		}
	}
	h := &maintenanceHandler{store: store}
	b := &Bot{handler: h, ctx: ctx, logger: quietLogger()}
	healthyDeleted := false
	b.cleanupApprovalMessage = func(ctx context.Context, channel, id string) error {
		if id == "1-slow" {
			<-ctx.Done()
			return ctx.Err()
		}
		healthyDeleted = true
		return nil
	}
	timed, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	_ = b.MaintainApprovals(timed)
	cancel()
	due, err := store.DueApprovalCleanup(ctx, time.Now())
	if err != nil || len(due) != 1 || due[0].MessageID != "2-healthy" {
		t.Fatal("timed out row did not retain backoff", due, err)
	}
	if err := b.MaintainApprovals(ctx); err != nil {
		t.Fatal(err)
	}
	if !healthyDeleted {
		t.Fatal("slow first row starved healthy cleanup")
	}
}
