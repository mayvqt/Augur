package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func scanSubscriptions(rows *sql.Rows) ([]Subscription, error) {
	subscriptions := make([]Subscription, 0)
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, subscription)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}
	return subscriptions, nil
}

type subscriptionScanner interface {
	Scan(dest ...any) error
}

func scanSubscription(scanner subscriptionScanner) (Subscription, error) {
	var subscription Subscription
	var createdAt string
	var completedAt sql.NullString
	if err := scanner.Scan(&subscription.RequestID, &subscription.DiscordID, &subscription.Title, &subscription.MediaType, &createdAt, &completedAt); err != nil {
		return Subscription{}, err
	}
	parsedCreatedAt, err := parseTime(createdAt)
	if err != nil {
		return Subscription{}, fmt.Errorf("parse created_at for subscription %d: %w", subscription.RequestID, err)
	}
	subscription.CreatedAt = parsedCreatedAt
	if completedAt.Valid && strings.TrimSpace(completedAt.String) != "" {
		parsedCompletedAt, err := parseTime(completedAt.String)
		if err != nil {
			return Subscription{}, fmt.Errorf("parse completed_at for subscription %d: %w", subscription.RequestID, err)
		}
		subscription.CompletedAt = parsedCompletedAt
	}
	return subscription, nil
}

func scanOptionalSubscription(scanner subscriptionScanner) (Subscription, bool, error) {
	subscription, err := scanSubscription(scanner)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, false, nil
	}
	if err != nil {
		return Subscription{}, false, err
	}
	return subscription, true, nil
}
