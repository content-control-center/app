// Package usage is the vendor-neutral metering layer for CON-86: an async
// Recorder that snapshots cost and writes vendor_usage_events without adding latency
// to the calling flow, and a Checker that gates synchronous flows against a
// tenant's spend caps. It depends only on narrow consumer interfaces (Writer,
// LimitsReader, SpendReader) that the repositories satisfy, plus the vendor
// registry for identity + pricing — never on the repository package itself.
package usage

import "expvar"

// Metrics holds the CON-86 usage counters (§11). NewMetrics registers them
// under ogen_usage_* on the global expvar registry for /debug/vars; call it
// once at boot. Tests construct a Metrics with unregistered counters instead.
type Metrics struct {
	EventsRecorded *expvar.Int
	EventsDropped  *expvar.Int
	WriteErrors    *expvar.Int

	UnknownModel  *expvar.Int
	UnknownVendor *expvar.Int

	LimitBlocks         *expvar.Int
	LimitWarnings       *expvar.Int
	EnforcementDegraded *expvar.Int
}

// NewMetrics registers and returns the usage counters. Call once at boot.
func NewMetrics() *Metrics {
	return &Metrics{
		EventsRecorded:      expvar.NewInt("ogen_usage_events_recorded"),
		EventsDropped:       expvar.NewInt("ogen_usage_events_dropped"),
		WriteErrors:         expvar.NewInt("ogen_usage_write_errors"),
		UnknownModel:        expvar.NewInt("ogen_usage_unknown_model"),
		UnknownVendor:       expvar.NewInt("ogen_usage_unknown_vendor"),
		LimitBlocks:         expvar.NewInt("ogen_usage_limit_blocks"),
		LimitWarnings:       expvar.NewInt("ogen_usage_limit_warnings"),
		EnforcementDegraded: expvar.NewInt("ogen_usage_enforcement_degraded"),
	}
}
