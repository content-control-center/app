package secrets

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/ogen-app/ogen/src/crypto/envelope"
)

// KEKFilename is the on-disk name of the active KEK file inside the
// configured KEK directory. The "v1" suffix is the KEK rotation seam:
// a future v2 KEK lives next to v1 in the same directory and old rows
// continue to decrypt against v1 until re-wrapped.
const KEKFilename = "kek.v1"

// InitCipher loads or creates the KEK at <kekDir>/kek.v1 and returns a
// ready-to-use Cipher together with the source descriptor for the boot
// log. Returns an error (caller should fail boot) on any failure to
// access the KEK file.
func InitCipher(kekDir string) (*envelope.Cipher, envelope.KEKSource, error) {
	if kekDir == "" {
		return nil, "", errors.New("secrets: KEK path is empty")
	}
	path := filepath.Join(kekDir, KEKFilename)
	kek, src, err := envelope.LoadOrCreateKEK(path)
	if err != nil {
		return nil, "", err
	}
	cipher, err := envelope.NewCipher(kek)
	if err != nil {
		return nil, src, err
	}
	return cipher, src, nil
}

// BootstrapResult is the boot-log payload — counts only, no values.
type BootstrapResult struct {
	SecretsInDB      int
	MigratedFromEnv  int
	IgnoredEnvVars   int
}

// EnvSource is the input shape for MigrateFromEnv: a list of (name, env-value)
// pairs the caller has already resolved from config. Empty values mean
// "env var not set" and are skipped.
type EnvSource struct {
	Name     string
	EnvValue string
}

// MigrateFromEnv applies the env-var → DB seeding rule on first boot.
//
// For each (name, envValue) pair:
//
//   - DB row present  + env set     → log "DB wins, ignoring env" (warning)
//   - DB row present  + env empty   → no-op
//   - DB row missing  + env set     → encrypt and insert; log "migrated"
//   - DB row missing  + env empty   → no-op (feature degrades at call time)
//
// Returns counts for the boot summary log.
func MigrateFromEnv(ctx context.Context, store Store, sources []EnvSource) (BootstrapResult, error) {
	out := BootstrapResult{}

	listed, err := store.List(ctx)
	if err != nil {
		return out, fmt.Errorf("secrets: list on bootstrap: %w", err)
	}
	have := make(map[string]bool, len(listed))
	for _, m := range listed {
		have[m.Name] = true
	}
	out.SecretsInDB = len(listed)

	for _, src := range sources {
		if !IsAllowed(src.Name) {
			// The bootstrap wiring is a closed list; an unknown name
			// here means the caller miswired the seed. Log and skip.
			log.Printf("secrets: bootstrap skipped unknown name %q", src.Name)
			continue
		}
		switch {
		case have[src.Name] && src.EnvValue != "":
			log.Printf("secrets: %s present in DB; ignoring env var (DB wins)", src.Name)
			out.IgnoredEnvVars++
		case !have[src.Name] && src.EnvValue != "":
			if _, _, err := store.Set(ctx, src.Name, src.EnvValue); err != nil {
				return out, fmt.Errorf("secrets: migrate %s from env: %w", src.Name, err)
			}
			log.Printf("secrets: %s migrated from env var", src.Name)
			out.MigratedFromEnv++
			out.SecretsInDB++
		}
	}
	return out, nil
}

// LogBootSummary emits the single boot-time summary line. Counts only,
// no fingerprints, no values.
func LogBootSummary(kekSource envelope.KEKSource, kekPath string, r BootstrapResult) {
	log.Printf("secrets: boot kek_source=%s kek_path=%s secrets_in_db=%d migrated_from_env=%d ignored_env_vars=%d",
		kekSource, kekPath, r.SecretsInDB, r.MigratedFromEnv, r.IgnoredEnvVars)
}
