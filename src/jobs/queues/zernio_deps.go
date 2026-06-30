// Package queues holds the typed River queues that drive Ogen's
// auto-publish pipeline (CON-69 §3, §6, §7, §9).
package queues

import (
	"context"

	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/usage"
)

// ProfileIDResolver returns the Zernio profile id this single-tenant
// Ogen install operates under. Resolved lazily so the queue can boot
// before the bootstrapper has completed; an error indicates "no
// integration bootstrap yet" and the queue handler treats it as
// terminal.
type ProfileIDResolver func(ctx context.Context) (string, error)

// ZernioDeps bundles the dependency set every Zernio-related queue
// shares. Held by each processor so the processor's Process method
// can stay narrow and easy to test in isolation. Constructed once at
// boot in server.go.
type ZernioDeps struct {
	PostRepo           repository.PostRepository
	PostLogRepo        repository.PostLogRepository
	PostAttachmentRepo repository.PostAttachmentRepository
	SocialAccountRepo  repository.SocialAccountRepository
	// SettingRepo backs the workspace timezone lookup used to stamp the
	// Zernio submit's Timezone field (CON-78). nil falls back to UTC.
	SettingRepo repository.SettingRepository
	// AnalyticsRepo persists the analytics snapshots the refresh queue
	// upserts (CON-93 §6 FR2). Only the refresh queue uses it; nil on the
	// submit/poll/cancel processors that share this bundle.
	AnalyticsRepo repository.PostAnalyticsRepository
	Client        *zernio.Client
	ProfileID     ProfileIDResolver
	// Recorder meters publish/schedule usage events (CON-86). nil = no-op.
	Recorder *usage.Recorder
}
