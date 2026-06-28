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
