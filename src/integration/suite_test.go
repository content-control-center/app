//go:build integration

package integration_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/pgtest"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// tenantCtx is the default-tenant context for integration fixtures that seed or
// query tenant-scoped models directly (CON-97 scoping fails closed without a
// tenant). Requests through the authenticated API already carry the session's
// tenant, so this is only for direct DB/repo access in test setup.
func tenantCtx() context.Context {
	return tenantctx.With(context.Background(), models.DefaultTenantID)
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

// seedTenantUser inserts a user in the default tenant directly, bypassing the
// (now auth-gated) POST /api/users route — since CON-97, signup is the only
// HTTP path that creates the first user. Login via POST /api/sessions still
// works because the user row exists.
func seedTenantUser(db *bun.DB, name, email, password string) *models.User {
	GinkgoHelper()
	hash, err := models.HashPassword(password)
	Expect(err).NotTo(HaveOccurred())
	id, err := models.NewID()
	Expect(err).NotTo(HaveOccurred())
	u := &models.User{
		ID:           id,
		TenantID:     models.DefaultTenantID,
		Name:         name,
		Email:        email,
		PasswordHash: hash,
	}
	_, err = db.NewInsert().Model(u).Exec(context.Background())
	Expect(err).NotTo(HaveOccurred())
	return u
}

// mustOpenIntegrationDB returns a fresh, fully-migrated Postgres database
// (CON-87 WS5). Each Describe's BeforeAll gets its own database, so suites
// stay isolated. The pool is sized up from pgtest's default so the
// concurrent-upload integration test can exercise parallel handler requests.
func mustOpenIntegrationDB() *bun.DB {
	db := pgtest.MustDB()
	db.DB.SetMaxOpenConns(10)
	db.DB.SetMaxIdleConns(5)
	return db
}
