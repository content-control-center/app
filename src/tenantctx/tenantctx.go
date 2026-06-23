// Package tenantctx carries the active tenant id through a request- or
// job-scoped context.Context. It is the single mechanism the tenant-scoped
// query layer (CON-97 §6) uses to discover which tenant a query belongs to.
//
// On the request path the Fiber auth middleware stores the tenant via
// c.Locals(tenantctx.Key, id); fasthttp exposes user values through
// (*RequestCtx).Value, so From() reads it back via ctx.Value without having to
// thread c.UserContext() through all ~150 handler→repo call sites. On the
// background-job path there is no request context, so workers rebuild the
// context with With() from the tenant_id carried in the job args.
package tenantctx

import "context"

// ctxKey is an unexported type so the context value cannot collide with keys
// set by other packages. It is shared with Fiber's c.Locals on the request
// path (see package doc).
type ctxKey struct{}

// Key is the context/locals key under which the tenant id is stored. It is
// exported (as a value of an unexported type) so the Fiber auth middleware can
// call c.Locals(tenantctx.Key, id) and have From() read it back, while callers
// still cannot forge the key type.
var Key = ctxKey{}

// With returns a copy of ctx carrying the given tenant id.
func With(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, Key, tenantID)
}

// From returns the tenant id carried by ctx and whether a non-empty one was
// present. Callers that enforce isolation must treat (_, false) as fail-closed
// (CON-97 §6) — never run an unscoped query.
func From(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(Key).(string)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}
