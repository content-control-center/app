package repository_test

import (
	"context"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// tenantCtx is the default-tenant context for repository tests that exercise
// tenant-scoped models (CON-97 scoping hooks fail closed without a tenant).
func tenantCtx() context.Context {
	return tenantctx.With(context.Background(), models.DefaultTenantID)
}
