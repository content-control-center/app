package handlers_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// testCookieName is the session cookie name used across all handler tests.
const testCookieName = "test_session"

// tenantCtx is the default-tenant context for test fixtures that seed or query
// tenant-scoped models directly (outside the request path, where CON-97's
// scoping hooks would otherwise fail closed). Data created through the
// authenticated API already carries the session's tenant, so this is only for
// direct DB/repo access in tests.
func tenantCtx() context.Context {
	return tenantctx.With(context.Background(), models.DefaultTenantID)
}

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Handlers Suite")
}

// seedTenantUser inserts a user in the default tenant directly, bypassing the
// (now auth-gated) POST /api/users route — since CON-97, signup is the only
// HTTP path that creates the first user. Login via POST /api/sessions still
// works because the user row exists. The default tenant is seeded by the
// foundation migration, so it is always present in a migrated test DB.
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
