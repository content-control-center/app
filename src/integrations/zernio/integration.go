package zernio

import "sync"

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

	mu    sync.RWMutex
	state State
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

// Enabled reports whether an API key was configured. False means the
// integration was never going to work this boot — endpoints can short
// circuit with 409 without consulting State().
func (i *Integration) Enabled() bool {
	return i != nil && i.Client != nil
}
