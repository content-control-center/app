package zernio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// SettingsStore is the narrow contract Bootstrapper (and the Phase 4
// metadata cache) need from the host's settings repository. The host
// implements this with a small adapter so this package never imports
// repository directly.
//
// found=false on Get means "no row"; an error means "I/O failure". A
// Set on a missing key must upsert.
type SettingsStore interface {
	Get(ctx context.Context, key string) (value string, found bool, err error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

// defaultBackoff is the retry schedule from the ticket: 1s, 2s, 4s, 8s
// (max 4 attempts, ~15s total). Exhaustion leaves the integration in
// StateDegraded so the next boot or admin /profile/repair retries.
var defaultBackoff = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
}

// Bootstrapper ensures exactly one Zernio profile named ManagedProfileName
// exists for this Ogen instance and caches its metadata in settings.
//
// Idempotency: Run is safe to call repeatedly. The matrix is:
//
//   - settings has profile_id, Zernio 200 → adopt + refresh meta.
//   - settings has profile_id, Zernio 404 → clear ID, fall through.
//   - settings has profile_id, Zernio 5xx → abort tick, retry.
//   - no profile_id, list contains "Ogen integration" → adopt oldest.
//   - no profile_id, list does not contain it → create.
//   - create fails → exponential backoff, then give up (degraded).
//
// Concurrent-boot safety: the in-process mutex serialises Run calls
// from the same Ogen process. Across processes, two boots may both
// reach CreateProfile after the list-by-name check returns empty. We
// mitigate by re-listing post-create and adopting the oldest matching
// profile — settings converge on a single ID. The duplicate profile
// remains on Zernio and must be cleaned up manually; we log a WARN to
// surface it.
type Bootstrapper struct {
	integ   *Integration
	store   SettingsStore
	backoff []time.Duration

	mu sync.Mutex
}

// NewBootstrapper wires a Bootstrapper around the integration and a
// SettingsStore implementation provided by the host.
func NewBootstrapper(integ *Integration, store SettingsStore) *Bootstrapper {
	return &Bootstrapper{integ: integ, store: store, backoff: defaultBackoff}
}

// Run performs the create-or-fetch sequence.
//
//   - On success: integration → StateOK; documented settings keys are
//     written; nil returned.
//   - On 401: integration → StateDisabled; non-nil error returned.
//   - On any other failure (transient or 4xx): integration → StateDegraded;
//     non-nil error returned. The caller should log; the next sync tick
//     (Phase 5) or admin /profile/repair will retry.
func (b *Bootstrapper) Run(ctx context.Context) error {
	if !b.integ.Enabled() {
		return errors.New("zernio: bootstrap skipped — integration disabled")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	profile, err := b.runWithBackoff(ctx)
	if err != nil {
		if IsStatus(err, http.StatusUnauthorized) {
			b.integ.SetState(StateDisabled)
			return fmt.Errorf("zernio: bootstrap auth failed: %w", err)
		}
		b.integ.SetState(StateDegraded)
		return fmt.Errorf("zernio: bootstrap degraded: %w", err)
	}

	if err := b.persistProfile(ctx, profile); err != nil {
		b.integ.SetState(StateDegraded)
		return fmt.Errorf("zernio: persist profile: %w", err)
	}

	b.integ.SetState(StateOK)
	log.Printf("zernio: bootstrap ok — profile=%s name=%q", profile.ID, profile.Name)
	return nil
}

// runWithBackoff retries tick on transient failures. Authoritative
// 4xx responses (401, 403) short-circuit immediately — no amount of
// retrying will recover from a bad API key.
func (b *Bootstrapper) runWithBackoff(ctx context.Context) (*Profile, error) {
	delays := append([]time.Duration{0}, b.backoff...)
	var lastErr error
	for attempt, delay := range delays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		profile, err := b.tick(ctx)
		if err == nil {
			return profile, nil
		}
		lastErr = err

		var apiErr *APIError
		if errors.As(err, &apiErr) {
			// 4xx (other than 404, which is a recoverable signal that
			// the stored profile_id is stale) won't be fixed by retries.
			if apiErr.Status >= 400 && apiErr.Status < 500 && apiErr.Status != http.StatusNotFound {
				return nil, err
			}
		}
		log.Printf("zernio: bootstrap attempt %d/%d failed: %v", attempt+1, len(delays), err)
	}
	return nil, lastErr
}

// tick is one create-or-fetch pass. Returns the adopted Profile or an
// error describing why this attempt failed.
func (b *Bootstrapper) tick(ctx context.Context) (*Profile, error) {
	storedID, hasStored, err := b.store.Get(ctx, SettingProfileID)
	if err != nil {
		return nil, fmt.Errorf("read setting: %w", err)
	}
	if hasStored && storedID != "" {
		profile, err := b.integ.Client.GetProfile(ctx, storedID)
		if err == nil {
			return profile, nil
		}
		if !IsStatus(err, http.StatusNotFound) {
			return nil, err
		}
		if delErr := b.store.Delete(ctx, SettingProfileID); delErr != nil {
			return nil, fmt.Errorf("clear stale profile_id: %w", delErr)
		}
		log.Printf("zernio: stored profile_id=%s no longer exists; will recreate", storedID)
	}

	profiles, err := b.integ.Client.ListProfiles(ctx, 100)
	if err != nil {
		return nil, err
	}
	if existing := pickOldestByName(profiles, ManagedProfileName); existing != nil {
		return existing, nil
	}

	created, err := b.integ.Client.CreateProfile(ctx, ManagedProfileName, ManagedProfileDescription)
	if err != nil {
		return nil, err
	}

	// Concurrent-boot mitigation: re-list and adopt the oldest matching
	// profile. If another boot raced us, both processes converge on the
	// same ID. List failures fall back to the just-created profile.
	if profiles, listErr := b.integ.Client.ListProfiles(ctx, 100); listErr == nil {
		matches := matchesByName(profiles, ManagedProfileName)
		if len(matches) > 1 {
			log.Printf("zernio: WARN %d profiles named %q exist on Zernio; adopting oldest. Manually clean up duplicates.",
				len(matches), ManagedProfileName)
		}
		if oldest := pickOldestByName(profiles, ManagedProfileName); oldest != nil {
			return oldest, nil
		}
	}
	return created, nil
}

// matchesByName returns all profiles with the given name.
func matchesByName(profiles []Profile, name string) []Profile {
	out := make([]Profile, 0, 1)
	for _, p := range profiles {
		if p.Name == name {
			out = append(out, p)
		}
	}
	return out
}

// pickOldestByName returns the profile with the earliest CreatedAt
// among those with the given name, or nil when none match.
func pickOldestByName(profiles []Profile, name string) *Profile {
	var best *Profile
	for i := range profiles {
		if profiles[i].Name != name {
			continue
		}
		if best == nil || profiles[i].CreatedAt.Before(best.CreatedAt) {
			best = &profiles[i]
		}
	}
	return best
}

// persistProfile writes the documented settings keys. Profile.Raw is
// preferred over a re-marshal so unknown fields survive the round trip.
func (b *Bootstrapper) persistProfile(ctx context.Context, p *Profile) error {
	raw := p.Raw
	if len(raw) == 0 {
		var err error
		if raw, err = json.Marshal(p); err != nil {
			return err
		}
	}
	writes := []struct{ k, v string }{
		{SettingProfileID, p.ID},
		{SettingProfileName, p.Name},
		{SettingProfileCreatedAt, p.CreatedAt.UTC().Format(time.RFC3339)},
		{SettingProfileMeta, string(raw)},
	}
	for _, w := range writes {
		if err := b.store.Set(ctx, w.k, w.v); err != nil {
			return fmt.Errorf("set %s: %w", w.k, err)
		}
	}
	return nil
}
