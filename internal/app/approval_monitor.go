package app

import (
	"context"
	"sort"
	"strings"

	"github.com/mayvqt/Augur/internal/config"
	"github.com/mayvqt/Augur/internal/seer"
)

func (r *Runner) reconcileApprovals(ctx context.Context) {
	requests, err := r.seer.PendingRequests(ctx)
	if err != nil {
		if ctx.Err() == nil {
			r.logger.Error("list pending Seerr requests", "error", err)
		}
		return
	}
	approvals := make([]seer.ApprovalRequest, 0, len(requests))
	for _, request := range requests {
		if ctx.Err() != nil {
			return
		}
		approval, ok := r.pendingApproval(ctx, request)
		if ok {
			approvals = append(approvals, approval)
		}
	}
	if err := r.bot.ReconcileApprovals(ctx, approvals); err != nil && ctx.Err() == nil {
		r.logger.Error("reconcile Discord approval messages", "error", err)
	}
}

func (r *Runner) pendingApproval(ctx context.Context, request seer.Request) (seer.ApprovalRequest, bool) {
	needed, err := r.store.NeedsApprovalMessage(ctx, request.ID)
	if err != nil {
		r.logger.Error("check pending request approval coverage", "request_id", request.ID, "error", err)
		return seer.ApprovalRequest{}, false
	}
	if !needed {
		return seer.ApprovalRequest{}, false
	}
	mediaType := request.Type
	if mediaType == "" && request.Media != nil {
		mediaType = request.Media.MediaType
	}
	if request.ID <= 0 || request.Media == nil || request.Media.TMDBID <= 0 || (mediaType != "movie" && mediaType != "tv") {
		r.logger.Warn("skip malformed pending Seerr request", "request_id", request.ID)
		return seer.ApprovalRequest{}, false
	}
	media, err := r.seer.MediaDetails(ctx, mediaType, request.Media.TMDBID)
	if err != nil {
		r.logger.Error("load pending request media", "request_id", request.ID, "error", err)
		return seer.ApprovalRequest{}, false
	}
	media.MediaType = mediaType
	approval := seer.ApprovalRequest{RequestID: request.ID, Media: media}
	if request.RequestedBy != nil {
		approval.Requester = request.RequestedBy.DisplayLabel()
		settings, err := r.seer.NotificationSettings(ctx, request.RequestedBy.ID)
		if err != nil {
			r.logger.Warn("load pending requester Discord IDs", "request_id", request.ID, "error", err)
		} else {
			for _, discordID := range settings.DiscordIDs {
				discordID = strings.TrimSpace(discordID)
				if config.IsDiscordID(discordID) {
					approval.RequesterID = discordID
					break
				}
			}
		}
	}
	if mediaType == "tv" {
		seen := make(map[int]struct{}, len(request.Seasons))
		for _, season := range request.Seasons {
			if season.SeasonNumber < 0 {
				continue
			}
			if _, duplicate := seen[season.SeasonNumber]; duplicate {
				continue
			}
			seen[season.SeasonNumber] = struct{}{}
			approval.Seasons.Numbers = append(approval.Seasons.Numbers, season.SeasonNumber)
		}
		sort.Ints(approval.Seasons.Numbers)
	}
	return approval, true
}
