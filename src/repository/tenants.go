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
