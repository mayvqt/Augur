package app

import "sync/atomic"

type Metrics struct {
	searches               atomic.Uint64
	requests               atomic.Uint64
	requestFailures        atomic.Uint64
	duplicateSubscriptions atomic.Uint64
	monitorChecks          atomic.Uint64
	monitorFailures        atomic.Uint64
	completedSubscriptions atomic.Uint64
	notificationFailures   atomic.Uint64
	transientSeerFailures  atomic.Uint64
}

func (m *Metrics) Snapshot() map[string]uint64 {
	if m == nil {
		return map[string]uint64{}
	}
	return map[string]uint64{
		"searches":                m.searches.Load(),
		"requests":                m.requests.Load(),
		"request_failures":        m.requestFailures.Load(),
		"duplicate_subscriptions": m.duplicateSubscriptions.Load(),
		"monitor_checks":          m.monitorChecks.Load(),
		"monitor_failures":        m.monitorFailures.Load(),
		"completed_subscriptions": m.completedSubscriptions.Load(),
		"notification_failures":   m.notificationFailures.Load(),
		"transient_seer_failures": m.transientSeerFailures.Load(),
	}
}
