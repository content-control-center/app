package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// seedUser inserts a user with an explicit tenant_id. Users are not TenantScoped,
// so there is no hook to stamp the tenant — the model field is used verbatim.
// openMigratedDB bypasses FK enforcement, so a synthetic tenant_id needs no
// tenants row.
func seedUser(t *testing.T, db *bun.DB, id, tenantID, email string) {
	t.Helper()
	u := &models.User{
		ID: id, TenantID: tenantID, Name: "User " + id, Email: email,
		PasswordHash: "x", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := db.NewInsert().Model(u).Exec(context.Background()); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

// TestUserRepositoryTenantIsolation guards CON-97: the User model is deliberately
// not TenantScoped, so the repository must apply the tenant filter by hand.
// Without it, GET /api/users (List) and GET /api/users/:id (GetByID) leak users
// across tenants.
func TestUserRepositoryTenantIsolation(t *testing.T) {
	db := openMigratedDB(t)
	repo := repository.NewUserRepository(db)

	ctxA := tenantctx.With(context.Background(), "ta")

	seedUser(t, db, "u-a", "ta", "a@example.com")
	seedUser(t, db, "u-b", "tb", "b@example.com")

	// List returns only the caller tenant's users — never tenant tb's.
	usersA, err := repo.List(ctxA)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(usersA) != 1 || usersA[0].ID != "u-a" {
		t.Fatalf("tenant ta must see only its own user, got %+v", usersA)
	}

	// GetByID cannot reach another tenant's user even with a valid id.
	if _, err := repo.GetByID(ctxA, "u-b"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant GetByID must fail closed with ErrNoRows, got %v", err)
	}
	if got, err := repo.GetByID(ctxA, "u-a"); err != nil || got.ID != "u-a" {
		t.Fatalf("in-tenant GetByID should return the user, got %+v err=%v", got, err)
	}

	// A non-system context with no tenant must fail closed rather than return
	// every tenant's users.
	if _, err := repo.List(context.Background()); !errors.Is(err, tenantctx.ErrNoTenant) {
		t.Fatalf("List without a tenant must fail closed, got %v", err)
	}
}
