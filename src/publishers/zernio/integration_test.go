package zernio

import "testing"

// TestIntegrationEnabledIsStateAware locks the contract that makes runtime
// key changes work with no reboot: Enabled() is derived from the live State,
// so warmupZernio (re-run by the zernio_api_key secrets subscription) flips
// the integration between disabled and usable without rebuilding anything.
func TestIntegrationEnabledIsStateAware(t *testing.T) {
	// A nil integration is never enabled (and must not panic).
	var nilIntegration *Integration
	if nilIntegration.Enabled() {
		t.Fatal("nil integration must not be enabled")
	}

	// No client wired → never enabled.
	if NewIntegration(nil).Enabled() {
		t.Fatal("integration without a client must not be enabled")
	}

	// Client wired but disabled (boot default / no key yet) → not enabled, so
	// endpoints return a clean 409 integration_disabled.
	integ := NewIntegration(&Client{})
	if got := integ.State(); got != StateDisabled {
		t.Fatalf("new integration should start disabled, got %s", got)
	}
	if integ.Enabled() {
		t.Fatal("a wired-but-disabled integration must report not enabled")
	}

	// Key validated at runtime (degraded/ok) → Enabled() flips true, no reboot.
	integ.SetState(StateDegraded)
	if !integ.Enabled() {
		t.Fatal("degraded integration should be enabled")
	}
	integ.SetState(StateOK)
	if !integ.Enabled() {
		t.Fatal("ok integration should be enabled")
	}

	// Key cleared/rejected at runtime → Enabled() flips back to false.
	integ.SetState(StateDisabled)
	if integ.Enabled() {
		t.Fatal("disabled integration must report not enabled")
	}
}

// TestPromoteOKOnlyFromDegraded locks the self-heal contract: a successful
// background sync clears a stale StateDegraded, but PromoteOK must never
// resurrect a disabled integration (no key / 401-rejected) nor disturb an
// already-ok one — otherwise the worker could race its own 401 demotion back to
// ok. Without this recovery path a transient boot-time Ping failure pins the
// instance to degraded for the life of the process.
func TestPromoteOKOnlyFromDegraded(t *testing.T) {
	integ := NewIntegration(&Client{})

	// Disabled (boot default / no key) → no promotion, stays disabled.
	if integ.PromoteOK() {
		t.Fatal("PromoteOK must not promote a disabled integration")
	}
	if got := integ.State(); got != StateDisabled {
		t.Fatalf("state should stay disabled, got %s", got)
	}

	// Degraded → promoted to ok, reports the change.
	integ.SetState(StateDegraded)
	if !integ.PromoteOK() {
		t.Fatal("PromoteOK should promote a degraded integration")
	}
	if got := integ.State(); got != StateOK {
		t.Fatalf("state should be ok after promote, got %s", got)
	}

	// Already ok → no-op (no spurious "changed" signal).
	if integ.PromoteOK() {
		t.Fatal("PromoteOK should be a no-op when already ok")
	}
}
