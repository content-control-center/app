package zernio

import (
	"sync"
	"time"
)

// State reflects whether the Zernio integration is usable.
//
//   - StateDisabled — no API key configured, or auth failed at boot
//     (401). Endpoints respond 409 integration_disabled. Worker is not
//     started. Only restart or admin /profile/repair can transition out.
//   - StateDegraded — API key valid but profile bootstrap has not yet
//     succeeded (transient network/5xx). Endpoints respond 503
//     integration_degraded. Worker keeps retrying.
//   - StateOK       — fully operational; all endpoints serve normally.
type State string

const (
	StateDisabled State = "disabled"
	StateDegraded State = "degraded"
	StateOK       State = "ok"
)

// Integration is the shared controller passed to handlers and the
// background sync worker. Exactly one *Integration exists per Ogen
// process; its mutex serialises state transitions across goroutines.
//
// A non-nil Integration with Client == nil represents an instance that
// was constructed without an API key — Enabled() returns false and the
// state is permanently StateDisabled.
type Integration struct {
	Client *Client

	mu        sync.RWMutex
	state     State
	fastUntil time.Time // worker reads via FastUntil() to switch cadence
}

// NewIntegration wires a fresh controller around c. The initial state is
// always StateDisabled; bootstrap (Phase 2) transitions to degraded or
// ok depending on the outcome of the boot-time validation.
func NewIntegration(c *Client) *Integration {
	return &Integration{Client: c, state: StateDisabled}
}

// State returns the current integration state.
func (i *Integration) State() State {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.state
}

// SetState transitions the integration to s. Callers should log the
// transition rather than relying on this method to do it — log site
// context (which subsystem caused the transition) is more useful than
// a single shared log line here.
func (i *Integration) SetState(s State) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.state = s
}

// Enabled reports whether the integration is currently usable: a client is
// wired AND it is not in the disabled state (no API key configured, or auth
// rejected). It is evaluated live off the current state, so setting, rotating,
// or clearing the zernio_api_key via the secrets API flips it without a reboot
// (the secrets subscription in initZernio re-validates and updates State).
// Endpoints short-circuit with 409 integration_disabled when this is false.
func (i *Integration) Enabled() bool {
	return i != nil && i.Client != nil && i.State() != StateDisabled
}

// BumpFastUntil extends the fast-polling deadline so the background
// sync worker (Phase 5) tightens its cadence after a connect link is
// issued. Bumping past the existing deadline wins; bumps to an earlier
// time are ignored. Safe to call from any goroutine.
func (i *Integration) BumpFastUntil(deadline time.Time) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if deadline.After(i.fastUntil) {
		i.fastUntil = deadline
	}
}

// FastUntil returns the current fast-polling deadline. The zero time
// means "no fast-polling window active".
func (i *Integration) FastUntil() time.Time {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.fastUntil
}
