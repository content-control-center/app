package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// TestCreateAtNextPositionRequiresParentPost guards CON-97: the FOR UPDATE lock
// must confirm a matching parent post exists in the tenant. Attaching to a
// missing or cross-tenant post fails closed (sql.ErrNoRows) rather than
// inserting an orphaned attachment. openMigratedDB bypasses FK enforcement, so
// this is caught by the lock check itself, not the post_id foreign key.
func TestCreateAtNextPositionRequiresParentPost(t *testing.T) {
	db := openMigratedDB(t)
	repo := repository.NewPostAttachmentRepository(db)
	ctx := tenantCtx()

	seedPost(t, db, "post-x", "", "", time.Now().UTC())

	ok := &models.PostAttachment{
		ID: "att-1", PostID: "post-x", MimeType: "image/png",
		SizeBytes: 1, ChecksumSHA256: "a", S3Key: "k1", CreatedBy: "user-1",
	}
	if err := repo.CreateAtNextPosition(ctx, ok); err != nil {
		t.Fatalf("attaching to an existing in-tenant post should succeed: %v", err)
	}

	orphan := &models.PostAttachment{
		ID: "att-2", PostID: "does-not-exist", MimeType: "image/png",
		SizeBytes: 1, ChecksumSHA256: "b", S3Key: "k2", CreatedBy: "user-1",
	}
	if err := repo.CreateAtNextPosition(ctx, orphan); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("attaching to a missing/cross-tenant post must fail closed with ErrNoRows, got %v", err)
	}

	// post-x genuinely exists (it was just attached to above), but only in the
	// default tenant. A second tenant must not be able to attach to it: the
	// FOR UPDATE lock is scoped by tenant_id, so the lookup finds no row and
	// fails closed (CON-97) rather than inserting a cross-tenant attachment.
	// This is the case the orphan probe above can't reach — it proves the
	// tenant_id half of the lock predicate, not just post_id existence.
	otherCtx := tenantctx.With(context.Background(), "tenant-2")
	crossTenant := &models.PostAttachment{
		ID: "att-3", PostID: "post-x", MimeType: "image/png",
		SizeBytes: 1, ChecksumSHA256: "c", S3Key: "k3", CreatedBy: "user-1",
	}
	if err := repo.CreateAtNextPosition(otherCtx, crossTenant); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("attaching from another tenant to an existing post must fail closed with ErrNoRows, got %v", err)
	}
}
