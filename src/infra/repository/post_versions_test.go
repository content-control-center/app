package repository_test

import (
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/domain/models"
	"github.com/ogen-app/ogen/src/infra/repository"
)

// TestPostVersionGetLatestByPostID pins the two behaviours that CON-184's
// resolveAssessedContent relies on from the *real* Bun-backed repository, which
// the fake in resolve_content_test.go can only assume:
//
//   - a post with no versions yields (nil, nil) — the sql.ErrNoRows→nil mapping
//     that the quality flow's fallback to posts.content depends on. If this
//     regressed to a raw error, versionless posts would stop being assessable.
//   - a post with several versions yields the highest version_number.
func TestPostVersionGetLatestByPostID(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	repo := repository.NewPostVersionRepository(db)

	// Missing record → (nil, nil), not an error.
	got, err := repo.GetLatestByPostID(ctx, "no-such-post")
	if err != nil {
		t.Fatalf("missing post must not error: %v", err)
	}
	if got != nil {
		t.Fatalf("missing post must return a nil version, got %+v", got)
	}

	// Seed a post and two versions out of numeric order to prove the result is
	// chosen by version_number (DESC), not insertion/created order.
	seedPost(t, db, "post-v", "", "", time.Now().UTC())
	seedVersion(t, db, "ver-2", "post-v", 2, "second draft")
	seedVersion(t, db, "ver-1", "post-v", 1, "first draft")

	got, err = repo.GetLatestByPostID(ctx, "post-v")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if got == nil {
		t.Fatal("expected the latest version, got nil")
	}
	if got.VersionNumber != 2 || got.Content != "second draft" {
		t.Fatalf("got v%d %q, want v2 %q", got.VersionNumber, got.Content, "second draft")
	}
}

// seedVersion inserts a post_versions row via the tenant-scoped context so the
// CON-97 hook populates tenant_id, mirroring seedPost.
func seedVersion(t *testing.T, db *bun.DB, id, postID string, num int, content string) {
	t.Helper()
	v := &models.PostVersion{
		ID:            id,
		PostID:        postID,
		VersionNumber: num,
		Content:       content,
		Note:          "",
		Creator:       "user",
		CreatedAt:     time.Now().UTC(),
	}
	if _, err := db.NewInsert().Model(v).Exec(tenantCtx()); err != nil {
		t.Fatalf("seed version %s: %v", id, err)
	}
}
