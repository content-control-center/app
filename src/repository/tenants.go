package repository

import (
	"context"
	"database/sql"
	"errors"

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
