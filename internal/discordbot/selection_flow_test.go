package discordbot

import (
	"context"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/mayvqt/Augur/internal/seer"
)

func testSeasons(n int) []seer.Season {
	seasons := make([]seer.Season, n)
	for i := range seasons {
		seasons[i] = seer.Season{SeasonNumber: i + 1}
	}
	return seasons
}

func TestSeasonPagesPreserveSelectionsAndQuota(t *testing.T) {
	var cache selectionCache
	cache.set("s", "owner", "show", []seer.SearchResult{{ID: 42, MediaType: "tv"}})
	quota := &seer.Quota{TV: seer.QuotaUsage{Limit: 5, Used: 2, Remaining: 3}}
	cache.setQuota("s", "owner", quota)
	seasons := testSeasons(51)
	cache.setAvailableSeasons("s", "0", "owner", seasons)
	selectPage := func(page int, numbers ...int) {
		t.Helper()
		if err := cache.selectSeasonPage("s", "0", "owner", page, numbers); err != nil {
			t.Fatal(err)
		}
	}
	menu := func(page int) discordgo.SelectMenu {
		t.Helper()
		_, _, selected, _ := cache.selection("s", "0", "owner")
		return seasonPickerComponents("s", "0", seasons, quota, selected, page)[0].(discordgo.ActionsRow).Components[0].(discordgo.SelectMenu)
	}
	selectPage(0, 1, 2, 3)
	if m := menu(1); !m.Disabled || m.MaxValues != 1 || *m.MinValues != 0 || m.Options[0].Value != "26" {
		t.Fatalf("full quota menu: %#v", m)
	}
	if menu(0).Disabled {
		t.Fatal("cannot clear previous page")
	}
	selectPage(0)
	if menu(1).Disabled {
		t.Fatal("quota not released by clearing selections")
	}
	selectPage(1, 26)
	selectPage(2, 51)
	selectPage(0, 2)
	_, _, selected, _ := cache.selection("s", "0", "owner")
	if !reflect.DeepEqual(selected.Numbers, []int{2, 26, 51}) {
		t.Fatal(selected)
	}
	if err := cache.selectSeasonPage("s", "0", "owner", 0, []int{26}); err == nil {
		t.Fatal("accepted a forged page selection")
	}
	if err := cache.selectSeasonPage("s", "0", "owner", 0, []int{2, 3}); err == nil {
		t.Fatal("exceeded finite quota with restricted=false")
	}
	selectPage(1)
	_, _, selected, _ = cache.selection("s", "0", "owner")
	if !reflect.DeepEqual(selected.Numbers, []int{2, 51}) {
		t.Fatal(selected)
	}
}

func TestSeasonPagePayloadLimits(t *testing.T) {
	for _, n := range []int{0, 1, 25, 26, 50, 51} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			seasons := testSeasons(n)
			for page := 0; page < max(1, (n+24)/25); page++ {
				rows := seasonPickerComponents("s", "0", seasons, &seer.Quota{}, seer.SeasonSelection{}, page)
				if len(rows) > 5 {
					t.Fatal("too many rows")
				}
				for _, row := range rows {
					for _, component := range row.(discordgo.ActionsRow).Components {
						if menu, ok := component.(discordgo.SelectMenu); ok {
							if len(menu.Options) < 1 || len(menu.Options) > 25 || menu.MaxValues < 1 || menu.MaxValues > 25 || *menu.MinValues != 0 {
								t.Fatalf("invalid Discord select: %#v", menu)
							}
						}
					}
				}
			}
		})
	}
}

type requestFlowHandler struct {
	Handler
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
	err     error
}

func (f *requestFlowHandler) Request(context.Context, string, seer.SearchResult, seer.SeasonSelection) (seer.Request, error) {
	f.calls.Add(1)
	if f.entered != nil {
		close(f.entered)
		<-f.release
	}
	return seer.Request{ID: 42, Status: 2}, f.err
}
func componentInteraction(id string) *discordgo.InteractionCreate {
	return &discordgo.InteractionCreate{Interaction: &discordgo.Interaction{Type: discordgo.InteractionMessageComponent, User: &discordgo.User{ID: "owner"}, Data: discordgo.MessageComponentInteractionData{CustomID: id}}}
}

func TestConcurrentConfirmationPostsOnceAndPreservesSuccess(t *testing.T) {
	handler := &requestFlowHandler{entered: make(chan struct{}), release: make(chan struct{})}
	bot := &Bot{handler: handler, ctx: context.Background(), logger: slog.Default()}
	bot.cache.set("s", "owner", "movie", []seer.SearchResult{{ID: 1, MediaType: "movie", Title: "Movie"}})
	first, duplicate := &fakeInteractionSession{}, &fakeInteractionSession{}
	var wg sync.WaitGroup
	wg.Go(func() { bot.handleComponent(first, componentInteraction(componentConfirm+"s:0")) })
	<-handler.entered
	bot.handleComponent(duplicate, componentInteraction(componentConfirm+"s:0"))
	if duplicate.edit != nil || duplicate.response == nil || duplicate.response.Data.Flags != discordgo.MessageFlagsEphemeral {
		t.Fatal("duplicate touched original response")
	}
	close(handler.release)
	wg.Wait()
	bot.handleComponent(duplicate, componentInteraction(componentBack+"s"))
	if handler.calls.Load() != 1 {
		t.Fatal("duplicate POST")
	}
	if first.edit == nil || !strings.Contains(*first.edit.Content, "has been submitted") {
		t.Fatalf("lost success: %#v", first.edit)
	}
	if duplicate.edit != nil {
		t.Fatal("stale click overwrote success")
	}
}

func TestUnknownSubmissionConsumesConfirmationWithoutRetry(t *testing.T) {
	handler := &requestFlowHandler{err: seer.ErrSubmissionUnknown}
	bot := &Bot{handler: handler, ctx: context.Background(), logger: slog.Default()}
	bot.cache.set("s", "owner", "movie", []seer.SearchResult{{ID: 1, MediaType: "movie"}})
	session := &fakeInteractionSession{}
	bot.handleComponent(session, componentInteraction(componentConfirm+"s:0"))
	if session.edit == nil || len(*session.edit.Components) != 0 || !strings.Contains(*session.edit.Content, "may have accepted") {
		t.Fatalf("unknown response: %#v", session.edit)
	}
	bot.handleComponent(&fakeInteractionSession{}, componentInteraction(componentRetry+"s:0"))
	if handler.calls.Load() != 1 {
		t.Fatal("retried unknown submission")
	}
}

func TestSubmissionClaimIsAtomicAndCanRetryDefiniteFailure(t *testing.T) {
	var cache selectionCache
	cache.set("s", "owner", "movie", []seer.SearchResult{{ID: 1, MediaType: "movie"}})
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() {
			if _, _, state := cache.beginSubmission("s", "0", "owner", false); state == "" {
				accepted.Add(1)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal(accepted.Load())
	}
	cache.finishSubmission("s", "owner", false)
	if _, _, state := cache.beginSubmission("s", "0", "owner", false); state != "" {
		t.Fatal(state)
	}
	cache.finishSubmission("s", "owner", true)
	if state := cache.submissionState("s", "owner"); state != "submitted" {
		t.Fatal(state)
	}
}
