package grpcserver

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/uptrace/bun"

	tenantsv1 "github.com/ogen-app/ogen/gen/tenants/v1"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/pgtest"
	"github.com/ogen-app/ogen/src/repository"
)

// TestTenantAdminRoundTrip exercises the full Harbor path for the CON-208
// TenantAdminService: a live listener + the shared-token interceptor + a
// generated client, backed by the real repositories over a migrated Postgres.
// It walks the whole lifecycle (create → assign → read → reassign → delete) and
// the key error codes, so the wire contract, auth, and status mapping are
// covered together.
func TestTenantAdminRoundTrip(t *testing.T) {
	const token = "tenant-admin-token"

	db := pgtest.MustDB()
	db.DB.SetMaxOpenConns(2)
	db.DB.SetMaxIdleConns(2)
	t.Cleanup(func() { _ = db.Close() })

	tierRepo := repository.NewTenantTierRepository(db)
	groupRepo := repository.NewTenantGroupRepository(db)
	tenantRepo := repository.NewTenantRepository(db)

	// store is nil: this test never calls the secrets RPCs, and New only stores
	// the reference. The tenant service gets real repositories.
	srv, err := New(token, nil, tierRepo, groupRepo, tenantRepo)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	newClient := func(tok string) tenantsv1.TenantAdminServiceClient {
		conn, err := grpc.NewClient(
			lis.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
				ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+tok)
				return invoker(ctx, method, req, reply, cc, opts...)
			}),
		)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return tenantsv1.NewTenantAdminServiceClient(conn)
	}

	ctx := context.Background()
	cli := newClient(token)

	// The seeded 'default' tier is present out of the box.
	tiers, err := cli.ListTiers(ctx, &tenantsv1.ListTiersRequest{})
	if err != nil {
		t.Fatalf("ListTiers: %v", err)
	}
	if !containsTier(tiers.GetTiers(), models.DefaultTierID) {
		t.Fatalf("ListTiers missing default tier: %+v", tiers.GetTiers())
	}

	// Create a tier; a duplicate name → AlreadyExists; a bad color → InvalidArgument.
	gold, err := cli.CreateTier(ctx, &tenantsv1.CreateTierRequest{Name: "Gold", Color: "#ffd700", Description: "Top"})
	if err != nil {
		t.Fatalf("CreateTier: %v", err)
	}
	goldID := gold.GetTier().GetId()
	if goldID == "" || gold.GetTier().GetName() != "Gold" {
		t.Fatalf("CreateTier returned %+v", gold.GetTier())
	}
	if _, err := cli.CreateTier(ctx, &tenantsv1.CreateTierRequest{Name: "Gold"}); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate tier: code = %v, want AlreadyExists", status.Code(err))
	}
	if _, err := cli.CreateTier(ctx, &tenantsv1.CreateTierRequest{Name: "Bad", Color: "red"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad color: code = %v, want InvalidArgument", status.Code(err))
	}

	// Create a group.
	beta, err := cli.CreateGroup(ctx, &tenantsv1.CreateGroupRequest{Name: "Beta", Color: "#5e6ad2"})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	betaID := beta.GetGroup().GetId()

	// Seed a tenant directly (signup is exercised elsewhere); it starts on default.
	tenantID := seedRoundTripTenant(t, db)

	// GetTenant hydrates tier + (empty) groups. Unknown / empty id are mapped.
	got, err := cli.GetTenant(ctx, &tenantsv1.GetTenantRequest{TenantId: tenantID})
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if got.GetTenant().GetTier().GetId() != models.DefaultTierID {
		t.Fatalf("new tenant tier = %q, want default", got.GetTenant().GetTier().GetId())
	}
	if len(got.GetTenant().GetGroups()) != 0 {
		t.Fatalf("new tenant groups = %+v, want empty", got.GetTenant().GetGroups())
	}
	if _, err := cli.GetTenant(ctx, &tenantsv1.GetTenantRequest{TenantId: "ghost"}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetTenant unknown: code = %v, want NotFound", status.Code(err))
	}
	if _, err := cli.GetTenant(ctx, &tenantsv1.GetTenantRequest{TenantId: ""}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("GetTenant empty: code = %v, want InvalidArgument", status.Code(err))
	}

	// Membership is idempotent; an unknown group → NotFound.
	if _, err := cli.AddTenantToGroup(ctx, &tenantsv1.AddTenantToGroupRequest{TenantId: tenantID, GroupId: betaID}); err != nil {
		t.Fatalf("AddTenantToGroup: %v", err)
	}
	if _, err := cli.AddTenantToGroup(ctx, &tenantsv1.AddTenantToGroupRequest{TenantId: tenantID, GroupId: betaID}); err != nil {
		t.Fatalf("AddTenantToGroup (idempotent): %v", err)
	}
	if _, err := cli.AddTenantToGroup(ctx, &tenantsv1.AddTenantToGroupRequest{TenantId: tenantID, GroupId: "ghost"}); status.Code(err) != codes.NotFound {
		t.Fatalf("AddTenantToGroup unknown group: code = %v, want NotFound", status.Code(err))
	}
	got, _ = cli.GetTenant(ctx, &tenantsv1.GetTenantRequest{TenantId: tenantID})
	if len(got.GetTenant().GetGroups()) != 1 || got.GetTenant().GetGroups()[0].GetId() != betaID {
		t.Fatalf("tenant groups after add = %+v, want [Beta]", got.GetTenant().GetGroups())
	}

	// SetTenantTier reassigns; unknown tier → FailedPrecondition; unknown tenant → NotFound.
	set, err := cli.SetTenantTier(ctx, &tenantsv1.SetTenantTierRequest{TenantId: tenantID, TierId: goldID})
	if err != nil {
		t.Fatalf("SetTenantTier: %v", err)
	}
	if set.GetTenant().GetTier().GetId() != goldID {
		t.Fatalf("tenant tier after set = %q, want gold", set.GetTenant().GetTier().GetId())
	}
	if _, err := cli.SetTenantTier(ctx, &tenantsv1.SetTenantTierRequest{TenantId: tenantID, TierId: "ghost"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("SetTenantTier unknown tier: code = %v, want FailedPrecondition", status.Code(err))
	}
	if _, err := cli.SetTenantTier(ctx, &tenantsv1.SetTenantTierRequest{TenantId: "ghost", TierId: goldID}); status.Code(err) != codes.NotFound {
		t.Fatalf("SetTenantTier unknown tenant: code = %v, want NotFound", status.Code(err))
	}

	// ListTenants filtered by the gold tier includes our tenant.
	list, err := cli.ListTenants(ctx, &tenantsv1.ListTenantsRequest{TierId: goldID})
	if err != nil {
		t.Fatalf("ListTenants: %v", err)
	}
	if !containsTenant(list.GetTenants(), tenantID) || list.GetTotal() < 1 {
		t.Fatalf("ListTenants by gold = %+v total=%d, want to include tenant", list.GetTenants(), list.GetTotal())
	}

	// DeleteTier is refused for the default tier and for an in-use tier.
	if _, err := cli.DeleteTier(ctx, &tenantsv1.DeleteTierRequest{Id: models.DefaultTierID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeleteTier default: code = %v, want FailedPrecondition", status.Code(err))
	}
	if _, err := cli.DeleteTier(ctx, &tenantsv1.DeleteTierRequest{Id: goldID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DeleteTier in-use: code = %v, want FailedPrecondition", status.Code(err))
	}
	// Reassign off gold, then it deletes.
	if _, err := cli.SetTenantTier(ctx, &tenantsv1.SetTenantTierRequest{TenantId: tenantID, TierId: models.DefaultTierID}); err != nil {
		t.Fatalf("reassign to default: %v", err)
	}
	if _, err := cli.DeleteTier(ctx, &tenantsv1.DeleteTierRequest{Id: goldID}); err != nil {
		t.Fatalf("DeleteTier free: %v", err)
	}

	// Deleting a group cascades the membership away.
	if _, err := cli.DeleteGroup(ctx, &tenantsv1.DeleteGroupRequest{Id: betaID}); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	got, _ = cli.GetTenant(ctx, &tenantsv1.GetTenantRequest{TenantId: tenantID})
	if len(got.GetTenant().GetGroups()) != 0 {
		t.Fatalf("tenant groups after group delete = %+v, want empty", got.GetTenant().GetGroups())
	}

	// A wrong token is rejected by the shared interceptor.
	if _, err := newClient("nope").ListTiers(ctx, &tenantsv1.ListTiersRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong token: code = %v, want Unauthenticated", status.Code(err))
	}
}

func seedRoundTripTenant(t *testing.T, db *bun.DB) string {
	t.Helper()
	id, err := models.NewID()
	if err != nil {
		t.Fatalf("mint id: %v", err)
	}
	now := time.Now().UTC()
	tn := &models.Tenant{ID: id, Name: "Acme", Slug: id, TierID: models.DefaultTierID, CreatedAt: now, UpdatedAt: now}
	if _, err := db.NewInsert().Model(tn).Exec(context.Background()); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return id
}

func containsTier(tiers []*tenantsv1.Tier, id string) bool {
	for _, tr := range tiers {
		if tr.GetId() == id {
			return true
		}
	}
	return false
}

func containsTenant(tenants []*tenantsv1.Tenant, id string) bool {
	for _, tn := range tenants {
		if tn.GetId() == id {
			return true
		}
	}
	return false
}
