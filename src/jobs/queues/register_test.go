package queues

import (
	"testing"

	"github.com/riverqueue/river"
)

// TestAllWorkersSelfRegister asserts the B2 self-registration: every job file's
// init() appended exactly one registrar, and RegisterAll wires them all to a
// River registry without panicking (river.AddWorker panics on duplicate kinds,
// so this also proves the kinds are distinct).
func TestAllWorkersSelfRegister(t *testing.T) {
	const wantWorkers = 12 // submit, poll, cancel, cleanup, reconcile, analytics, followers, bootstrap-profile, process-pdf, send-email, cleanup-email-logs, cleanup-connect-sessions
	if len(registrars) != wantWorkers {
		t.Fatalf("self-registered workers: got %d, want %d", len(registrars), wantWorkers)
	}
	RegisterAll(river.NewWorkers(), Deps{})
}
