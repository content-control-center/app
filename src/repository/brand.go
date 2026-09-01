package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// BrandRepository is the persistence for the Brand module (CON-228): one
// aggregate read plus per-section whole-resource writes. Everything is
// tenant-scoped automatically by the TenantScoped bun hooks on the models —
// these methods never add a tenant predicate by hand, and inserts never carry a
// client-supplied tenant.
type BrandRepository interface {
	// GetAll returns every slot for the caller's tenant. Lists are non-nil
	// (empty, not null); singletons are nil when unset.
	GetAll(ctx context.Context) (*models.BrandData, error)

	CreateVoice(ctx context.Context, v *models.BrandVoice) error
	UpdateVoice(ctx context.Context, v *models.BrandVoice) error // sql.ErrNoRows when unknown
	DeleteVoice(ctx context.Context, id string) (bool, error)

	CreateAudience(ctx context.Context, a *models.BrandAudience) error
	UpdateAudience(ctx context.Context, a *models.BrandAudience) error // sql.ErrNoRows when unknown
	DeleteAudience(ctx context.Context, id string) (bool, error)

	GetGuardrails(ctx context.Context) (*models.BrandGuardrails, error) // nil when unset
	UpsertGuardrails(ctx context.Context, g *models.BrandGuardrails) error
	DeleteGuardrails(ctx context.Context) (bool, error)

	GetLook(ctx context.Context) (*models.BrandLook, error) // nil when unset
	UpsertLook(ctx context.Context, l *models.BrandLook) error
	DeleteLook(ctx context.Context) (bool, error)

	CreateTemplate(ctx context.Context, t *models.BrandTemplate) error
	UpdateTemplate(ctx context.Context, t *models.BrandTemplate) error // sql.ErrNoRows when unknown
	DeleteTemplate(ctx context.Context, id string) (bool, error)
}

type brandRepository struct {
	db *bun.DB
}

// NewBrandRepository returns a Bun-backed BrandRepository.
func NewBrandRepository(db *bun.DB) BrandRepository {
	return &brandRepository{db: db}
}

// ── Aggregate ───────────────────────────────────────────────────────────────

func (r *brandRepository) GetAll(ctx context.Context) (*models.BrandData, error) {
	voices := []models.BrandVoice{}
	if err := r.db.NewSelect().Model(&voices).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	audiences := []models.BrandAudience{}
	if err := r.db.NewSelect().Model(&audiences).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	templates := []models.BrandTemplate{}
	if err := r.db.NewSelect().Model(&templates).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	guardrails, err := r.GetGuardrails(ctx)
	if err != nil {
		return nil, err
	}
	look, err := r.GetLook(ctx)
	if err != nil {
		return nil, err
	}
	// bun leaves the slices nil when zero rows come back on some paths; the
	// aggregate contract is `[]`, never null.
	if voices == nil {
		voices = []models.BrandVoice{}
	}
	if audiences == nil {
		audiences = []models.BrandAudience{}
	}
	if templates == nil {
		templates = []models.BrandTemplate{}
	}
	return &models.BrandData{
		Voices:     voices,
		Audiences:  audiences,
		Guardrails: guardrails,
		Look:       look,
		Templates:  templates,
	}, nil
}

// ── Voices ──────────────────────────────────────────────────────────────────

func (r *brandRepository) CreateVoice(ctx context.Context, v *models.BrandVoice) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if v.IsDefault {
			if err := demoteDefault(ctx, tx, (*models.BrandVoice)(nil), ""); err != nil {
				return err
			}
		}
		_, err := tx.NewInsert().Model(v).Exec(ctx)
		return err
	})
}

func (r *brandRepository) UpdateVoice(ctx context.Context, v *models.BrandVoice) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		existing := new(models.BrandVoice)
		if err := tx.NewSelect().Model(existing).Where("bv.id = ?", v.ID).For("UPDATE").Scan(ctx); err != nil {
			return err // sql.ErrNoRows → handler maps to 404
		}
		// Server-owned / write-once fields survive a replace.
		v.TenantID = existing.TenantID
		v.CreatedAt = existing.CreatedAt
		v.Origin = existing.Origin // FR6: origin is provenance, write-once.
		if v.IsDefault {
			if err := demoteDefault(ctx, tx, (*models.BrandVoice)(nil), v.ID); err != nil {
				return err
			}
		}
		_, err := tx.NewUpdate().Model(v).WherePK().Exec(ctx)
		return err
	})
}

func (r *brandRepository) DeleteVoice(ctx context.Context, id string) (bool, error) {
	var deleted bool
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		target := new(models.BrandVoice)
		err := tx.NewSelect().Model(target).Where("bv.id = ?", id).For("UPDATE").Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // not found → deleted stays false
		}
		if err != nil {
			return err
		}
		if _, err := tx.NewDelete().Model((*models.BrandVoice)(nil)).Where("id = ?", id).Exec(ctx); err != nil {
			return err
		}
		deleted = true
		if target.IsDefault {
			return promoteEarliestVoice(ctx, tx)
		}
		return nil
	})
	return deleted, err
}

// promoteEarliestVoice hands the default flag to the earliest remaining voice.
// FR4: a workspace with voices and no default is a state the UI cannot repair.
func promoteEarliestVoice(ctx context.Context, tx bun.Tx) error {
	survivor := new(models.BrandVoice)
	err := tx.NewSelect().Model(survivor).OrderExpr("created_at ASC").Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // last voice deleted; nothing to promote
	}
	if err != nil {
		return err
	}
	survivor.IsDefault = true
	survivor.UpdatedAt = time.Now().UTC()
	_, err = tx.NewUpdate().Model(survivor).Column("is_default", "updated_at").WherePK().Exec(ctx)
	return err
}

// ── Audiences ───────────────────────────────────────────────────────────────

func (r *brandRepository) CreateAudience(ctx context.Context, a *models.BrandAudience) error {
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *brandRepository) UpdateAudience(ctx context.Context, a *models.BrandAudience) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		existing := new(models.BrandAudience)
		if err := tx.NewSelect().Model(existing).Where("ba.id = ?", a.ID).Scan(ctx); err != nil {
			return err // sql.ErrNoRows → 404
		}
		a.TenantID = existing.TenantID
		a.CreatedAt = existing.CreatedAt
		a.Origin = existing.Origin // FR6
		_, err := tx.NewUpdate().Model(a).WherePK().Exec(ctx)
		return err
	})
}

func (r *brandRepository) DeleteAudience(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.BrandAudience)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ── Guardrails (singleton) ──────────────────────────────────────────────────

func (r *brandRepository) GetGuardrails(ctx context.Context) (*models.BrandGuardrails, error) {
	g := new(models.BrandGuardrails)
	err := r.db.NewSelect().Model(g).Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

func (r *brandRepository) UpsertGuardrails(ctx context.Context, g *models.BrandGuardrails) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		existing := new(models.BrandGuardrails)
		err := tx.NewSelect().Model(existing).For("UPDATE").Limit(1).Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			_, ierr := tx.NewInsert().Model(g).Exec(ctx)
			return ierr
		}
		if err != nil {
			return err
		}
		g.ID = existing.ID
		g.TenantID = existing.TenantID
		g.CreatedAt = existing.CreatedAt
		_, uerr := tx.NewUpdate().Model(g).WherePK().Exec(ctx)
		return uerr
	})
}

func (r *brandRepository) DeleteGuardrails(ctx context.Context) (bool, error) {
	return deleteSingleton(ctx, r.db, (*models.BrandGuardrails)(nil))
}

// ── Look (singleton) ────────────────────────────────────────────────────────

func (r *brandRepository) GetLook(ctx context.Context) (*models.BrandLook, error) {
	l := new(models.BrandLook)
	err := r.db.NewSelect().Model(l).Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (r *brandRepository) UpsertLook(ctx context.Context, l *models.BrandLook) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		existing := new(models.BrandLook)
		err := tx.NewSelect().Model(existing).For("UPDATE").Limit(1).Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			_, ierr := tx.NewInsert().Model(l).Exec(ctx)
			return ierr
		}
		if err != nil {
			return err
		}
		l.ID = existing.ID
		l.TenantID = existing.TenantID
		l.CreatedAt = existing.CreatedAt
		_, uerr := tx.NewUpdate().Model(l).WherePK().Exec(ctx)
		return uerr
	})
}

func (r *brandRepository) DeleteLook(ctx context.Context) (bool, error) {
	return deleteSingleton(ctx, r.db, (*models.BrandLook)(nil))
}

// ── Templates ───────────────────────────────────────────────────────────────

func (r *brandRepository) CreateTemplate(ctx context.Context, t *models.BrandTemplate) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if t.IsDefault {
			if err := demoteDefault(ctx, tx, (*models.BrandTemplate)(nil), ""); err != nil {
				return err
			}
		}
		_, err := tx.NewInsert().Model(t).Exec(ctx)
		return err
	})
}

func (r *brandRepository) UpdateTemplate(ctx context.Context, t *models.BrandTemplate) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		existing := new(models.BrandTemplate)
		if err := tx.NewSelect().Model(existing).Where("bt.id = ?", t.ID).For("UPDATE").Scan(ctx); err != nil {
			return err // sql.ErrNoRows → 404
		}
		t.TenantID = existing.TenantID
		t.CreatedAt = existing.CreatedAt
		t.Origin = existing.Origin // FR6
		if t.IsDefault {
			if err := demoteDefault(ctx, tx, (*models.BrandTemplate)(nil), t.ID); err != nil {
				return err
			}
		}
		_, err := tx.NewUpdate().Model(t).WherePK().Exec(ctx)
		return err
	})
}

func (r *brandRepository) DeleteTemplate(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.BrandTemplate)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ── shared helpers ──────────────────────────────────────────────────────────

// demoteDefault clears is_default on every row of the model except exceptID
// (empty on create). Run inside the same tx as the insert/update, and before it,
// so the partial-unique-default index never sees two defaults.
//
// The TenantScoped BeforeUpdate hook already scopes this to the caller's tenant,
// but this is a cross-row write with no bound instance, so the tenant predicate
// is also added explicitly (and fails closed with no tenant in context) —
// belt-and-braces on the one operation that touches rows other than the target.
func demoteDefault(ctx context.Context, tx bun.Tx, model any, exceptID string) error {
	tid, ok := tenantctx.From(ctx)
	if !ok {
		return tenantctx.ErrNoTenant
	}
	q := tx.NewUpdate().Model(model).
		Set("is_default = ?", false).
		Where("tenant_id = ?", tid).
		Where("is_default = ?", true)
	if exceptID != "" {
		q = q.Where("id <> ?", exceptID)
	}
	_, err := q.Exec(ctx)
	return err
}

// deleteSingleton removes the caller's one row for a per-tenant singleton. The
// explicit tenant predicate is belt-and-braces alongside the model hook and
// fails closed when no tenant is in context.
func deleteSingleton(ctx context.Context, db *bun.DB, model any) (bool, error) {
	tid, ok := tenantctx.From(ctx)
	if !ok {
		return false, tenantctx.ErrNoTenant
	}
	res, err := db.NewDelete().Model(model).Where("tenant_id = ?", tid).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
