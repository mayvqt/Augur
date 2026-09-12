package seer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

const userLookupConcurrency = 4

func (c *Client) FindUserByDiscordID(ctx context.Context, discordID string) (User, bool, error) {
	discordID = strings.TrimSpace(discordID)
	if discordID == "" {
		return User{}, false, errors.New("discord ID is required")
	}
	var matched User
	seen := make(map[int]bool)
	for skip := 0; ; skip += 100 {
		values := url.Values{"take": {"100"}, "skip": {strconv.Itoa(skip)}}
		var page struct {
			Results []User `json:"results"`
		}
		if err := c.do(ctx, http.MethodGet, "/api/v1/user?"+values.Encode(), nil, &page); err != nil {
			return User{}, false, err
		}
		if page.Results == nil {
			return User{}, false, errors.New("seerr user page is missing results")
		}
		users := make([]User, 0, len(page.Results))
		for _, user := range page.Results {
			if user.ID <= 0 {
				return User{}, false, errors.New("seerr user page contains an invalid user ID")
			}
			if !seen[user.ID] {
				seen[user.ID] = true
				users = append(users, user)
			}
		}
		matches, err := c.matchDiscordUsers(ctx, users, discordID)
		if err != nil {
			return User{}, false, err
		}
		for _, user := range matches {
			if matched.ID != 0 && matched.ID != user.ID {
				return User{}, false, errors.New("discord ID is linked to multiple seerr users")
			}
			matched = user
		}
		if len(page.Results) < 100 {
			return matched, matched.ID != 0, nil
		}
		if skip > math.MaxInt-100 {
			return User{}, false, errors.New("seerr user pagination overflowed")
		}
		if len(users) == 0 {
			return User{}, false, errors.New("seerr user pagination did not advance")
		}
	}
}

// Each worker owns one result slot at a time. Join every worker before returning,
// including errors, so an incomplete scan can never authorize a partial match.
func (c *Client) matchDiscordUsers(ctx context.Context, users []User, discordID string) ([]User, error) {
	lookupCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int, len(users))
	for i := range users {
		jobs <- i
	}
	close(jobs)
	matched := make([]bool, len(users))
	failures := make([]error, len(users))
	var wg sync.WaitGroup
	for range min(userLookupConcurrency, len(users)) {
		wg.Go(func() {
			for i := range jobs {
				if lookupCtx.Err() != nil {
					return
				}
				settings, err := c.NotificationSettings(lookupCtx, users[i].ID)
				if err != nil {
					failures[i] = fmt.Errorf("verify seerr account link: %w", err)
					cancel()
					return
				}
				matched[i] = settings.HasDiscordID(discordID)
			}
		})
	}
	wg.Wait()
	for _, err := range failures {
		if err != nil && !errors.Is(err, context.Canceled) {
			return nil, err
		}
	}
	if err := lookupCtx.Err(); err != nil {
		return nil, err
	}
	var out []User
	for i, yes := range matched {
		if yes {
			out = append(out, users[i])
		}
	}
	return out, nil
}
