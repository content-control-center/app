package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// TenantRepository defines all persistence operations for the Tenant domain.
//
// Tenants are the isolation boundary itself, so this repository is
// intentionally NOT routed through the tenant-scoped query layer (CON-97 §6) —
// it operates on the global tenants table directly.
type TenantRepository interface {
	Create(ctx context.Context, tenant *models.Tenant) error
	GetByID(ctx context.Context, id string) (*models.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*models.Tenant, error)
	Update(ctx context.Context, tenant *models.Tenant) error
	// SoftDeleteTx marks a workspace deleted (CON-147 PR4) on the provided bun.IDB
	// so it can join the delete handler's transaction (which also, later, enqueues
	// the Zernio teardown). Idempotent: a second call while already deleted is a
	// no-op. Passing nil uses the default DB.
	SoftDeleteTx(ctx context.Context, tx bun.IDB, id string, at time.Time) error

	// --- CON-208 operator/admin classification read + write (Harbor gRPC) ---

	// GetByIDWithClassification loads a non-deleted tenant and hydrates its Tier
	// and Groups. Returns sql.ErrNoRows for an unknown or soft-deleted tenant.
	GetByIDWithClassification(ctx context.Context, id string) (*models.Tenant, error)
	// ListWithClassification returns non-deleted tenants (optionally filtered by
	// tier and/or group), each hydrated with Tier + Groups, plus the total match
	// count ignoring the page window.
	ListWithClassification(ctx context.Context, f TenantListFilter) ([]models.Tenant, int, error)
	// SetTier reassigns a non-deleted tenant's required tier. Returns false if no
	// such (live) tenant exists; an unknown tier fails the FK (23503).
	SetTier(ctx context.Context, tenantID, tierID string) (bool, error)
}

// TenantListFilter narrows and pages ListWithClassification. Empty TierID/GroupID
// means "no filter on that dimension"; Limit defaults to 100 and is capped at 500.
type TenantListFilter struct {
	TierID  string
	GroupID string
	Limit   int
	Offset  int
}

type tenantRepository struct {
	db *bun.DB
}

// NewTenantRepository returns a Bun-backed TenantRepository.
func NewTenantRepository(db *bun.DB) TenantRepository {
	return &tenantRepository{db: db}
}

func (r *tenantRepository) Create(ctx context.Context, tenant *models.Tenant) error {
	_, err := r.db.NewInsert().Model(tenant).Exec(ctx)
	return err
}

func (r *tenantRepository) GetByID(ctx context.Context, id string) (*models.Tenant, error) {
	tenant := new(models.Tenant)
	err := r.db.NewSelect().Model(tenant).Where("tn.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return tenant, nil
}

func (r *tenantRepository) GetBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	tenant := new(models.Tenant)
	err := r.db.NewSelect().Model(tenant).Where("tn.slug = ?", slug).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return tenant, nil
}

func (r *tenantRepository) Update(ctx context.Context, tenant *models.Tenant) error {
	_, err := r.db.NewUpdate().Model(tenant).WherePK().Exec(ctx)
	return err
}

func (r *tenantRepository) SoftDeleteTx(ctx context.Context, tx bun.IDB, id string, at time.Time) error {
	db := bun.IDB(r.db)
	if tx != nil {
		db = tx
	}
	_, err := db.NewUpdate().Model((*models.Tenant)(nil)).
		Set("deleted_at = ?", at).
		Set("updated_at = ?", at).
		Where("id = ?", id).
		Where("deleted_at IS NULL").
		Exec(ctx)
	return err
}

func (r *tenantRepository) GetByIDWithClassification(ctx context.Context, id string) (*models.Tenant, error) {
	tenant := new(models.Tenant)
	err := r.db.NewSelect().Model(tenant).
		Where("tn.id = ?", id).
		Where("tn.deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	tenants := []models.Tenant{*tenant}
	if err := r.hydrateClassification(ctx, tenants); err != nil {
		return nil, err
	}
	return &tenants[0], nil
}

func (r *tenantRepository) ListWithClassification(ctx context.Context, f TenantListFilter) ([]models.Tenant, int, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	var tenants []models.Tenant
	q := r.db.NewSelect().Model(&tenants).Where("tn.deleted_at IS NULL")
	if f.TierID != "" {
		q = q.Where("tn.tier_id = ?", f.TierID)
	}
	if f.GroupID != "" {
		q = q.Where(
			"EXISTS (SELECT 1 FROM tenant_group_assignments AS tga WHERE tga.tenant_id = tn.id AND tga.group_id = ?)",
			f.GroupID,
		)
	}
	// Tiebreak on the PK so ties on created_at yield a total, stable order —
	// otherwise limit/offset paging could drop or repeat rows across pages. The
	// id tiebreak is pinned to the C (byte) collation so the order is identical
	// across clusters regardless of their default collation: a locale collation
	// sorts mixed-case Sqids case-insensitively (stable per-cluster but not
	// portable across dev/CI/prod, and diverging from a bytewise sort.Strings).
	total, err := q.OrderExpr(`tn.created_at ASC, tn.id COLLATE "C" ASC`).Limit(limit).Offset(offset).ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if err := r.hydrateClassification(ctx, tenants); err != nil {
		return nil, 0, err
	}
	return tenants, total, nil
}

func (r *tenantRepository) SetTier(ctx context.Context, tenantID, tierID string) (bool, error) {
	res, err := r.db.NewUpdate().Model((*models.Tenant)(nil)).
		Set("tier_id = ?", tierID).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", tenantID).
		Where("deleted_at IS NULL").
		Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// hydrateClassification loads each tenant's Tier (by tier_id) and Groups (via
// the assignment join) in two batched queries, then attaches them in Go —
// mirroring campaignRepository's hydrate helpers. Groups default to an empty
// (non-nil) slice so the response is [] rather than null.
func (r *tenantRepository) hydrateClassification(ctx context.Context, tenants []models.Tenant) error {
	if len(tenants) == 0 {
		return nil
	}

	// Tiers, deduplicated by id.
	tierIDs := make([]string, 0, len(tenants))
	seen := make(map[string]struct{}, len(tenants))
	for _, t := range tenants {
		if _, ok := seen[t.TierID]; !ok {
			seen[t.TierID] = struct{}{}
			tierIDs = append(tierIDs, t.TierID)
		}
	}
	var tiers []models.TenantTier
	if err := r.db.NewSelect().Model(&tiers).Where("tt.id IN (?)", bun.In(tierIDs)).Scan(ctx); err != nil {
		return err
	}
	tierByID := make(map[string]*models.TenantTier, len(tiers))
	for i := range tiers {
		tierByID[tiers[i].ID] = &tiers[i]
	}

	// Groups per tenant, via the assignment join.
	tenantIDs := make([]string, len(tenants))
	for i, t := range tenants {
		tenantIDs[i] = t.ID
	}
	var rows []struct {
		TenantID    string    `bun:"tenant_id"`
		ID          string    `bun:"id"`
		Name        string    `bun:"name"`
		Color       string    `bun:"color"`
		Description string    `bun:"description"`
		CreatedAt   time.Time `bun:"created_at"`
		UpdatedAt   time.Time `bun:"updated_at"`
	}
	if err := r.db.NewSelect().
		TableExpr("tenant_group_assignments AS tga").
		ColumnExpr("tga.tenant_id, tg.id, tg.name, tg.color, tg.description, tg.created_at, tg.updated_at").
		Join("JOIN tenant_groups AS tg ON tg.id = tga.group_id").
		Where("tga.tenant_id IN (?)", bun.In(tenantIDs)).
		OrderExpr("tg.name ASC").
		Scan(ctx, &rows); err != nil {
		return err
	}
	groupsByTenant := make(map[string][]models.TenantGroup, len(tenants))
	for _, row := range rows {
		groupsByTenant[row.TenantID] = append(groupsByTenant[row.TenantID], models.TenantGroup{
			ID:          row.ID,
			Name:        row.Name,
			Color:       row.Color,
			Description: row.Description,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		})
	}

	for i := range tenants {
		tenants[i].Tier = tierByID[tenants[i].TierID]
		if g, ok := groupsByTenant[tenants[i].ID]; ok {
			tenants[i].Groups = g
		} else {
			tenants[i].Groups = []models.TenantGroup{}
		}
	}
	return nil
}
