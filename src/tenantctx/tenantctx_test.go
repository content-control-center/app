package tenantctx_test

import (
	"context"
	"testing"

	"github.com/ogen-app/ogen/src/tenantctx"
)

func TestWithFromRoundTrip(t *testing.T) {
	ctx := tenantctx.With(context.Background(), "tn-1")
	id, ok := tenantctx.From(ctx)
	if !ok || id != "tn-1" {
		t.Fatalf("expected tn-1/true, got %q/%v", id, ok)
	}
}

func TestFromAbsent(t *testing.T) {
	if id, ok := tenantctx.From(context.Background()); ok || id != "" {
		t.Fatalf("expected empty/false, got %q/%v", id, ok)
	}
}

// An empty tenant id must read back as fail-closed (not present), so the
// scoped query layer never runs an unscoped query (CON-97 §6).
func TestFromEmptyStringIsAbsent(t *testing.T) {
	ctx := tenantctx.With(context.Background(), "")
	if id, ok := tenantctx.From(ctx); ok || id != "" {
		t.Fatalf("expected empty/false for empty tenant, got %q/%v", id, ok)
	}
}
