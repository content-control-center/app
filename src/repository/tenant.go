package repository

import (
	"context"

	"github.com/ogen-app/ogen/src/tenantctx"
)

// writeTenantID resolves the tenant_id to stamp on a RAW write. It mirrors the
// models.TenantScoped.BeforeAppendModel hook for the few repositories that build
// INSERTs with raw SQL (which bypasses bun model hooks). On a system context the
// caller-set value is trusted; otherwise the request tenant is required and a
// missing one fails closed (CON-97 §6).
func writeTenantID(ctx context.Context, current string) (string, error) {
	if tenantctx.IsSystem(ctx) {
		return current, nil
	}
	tid, ok := tenantctx.From(ctx)
	if !ok {
		return "", tenantctx.ErrNoTenant
	}
	return tid, nil
}
