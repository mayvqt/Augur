package app

import "sync/atomic"

type Metrics struct {
	searches              atomic.Uint64
	requests              atomic.Uint64
	requestFailures       atomic.Uint64
	duplicateWatches      atomic.Uint64
	watcherChecks         atomic.Uint64
	watcherFailures       atomic.Uint64
	completedWatches      atomic.Uint64
	notificationFailures  atomic.Uint64
	transientSeerFailures atomic.Uint64
}

func (m *Metrics) Snapshot() map[string]uint64 {
	if m == nil {
		return map[string]uint64{}
	}
	return map[string]uint64{
		"searches":                m.searches.Load(),
		"requests":                m.requests.Load(),
		"request_failures":        m.requestFailures.Load(),
		"duplicate_watches":       m.duplicateWatches.Load(),
		"watcher_checks":          m.watcherChecks.Load(),
		"watcher_failures":        m.watcherFailures.Load(),
		"completed_watches":       m.completedWatches.Load(),
		"notification_failures":   m.notificationFailures.Load(),
		"transient_seer_failures": m.transientSeerFailures.Load(),
	}
}
