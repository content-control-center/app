package repository

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/domain/models"
)

// SecretRepository is the BUN-backed CRUD surface for the secret
// table. Plaintext never crosses this boundary — callers hand in
// already-encrypted Records via models.Secret and read the same
// shape back out.
//
// Upsert keys on the unique Name column so the boot-time env-var
// migration and the REST PUT handler share a single write path.
type SecretRepository interface {
	Get(ctx context.Context, name string) (*models.Secret, error)
	List(ctx context.Context) ([]models.Secret, error)
	Upsert(ctx context.Context, s *models.Secret) error
	Delete(ctx context.Context, name string) (bool, error)
}

type secretRepository struct {
	db *bun.DB
}

func NewSecretRepository(db *bun.DB) SecretRepository {
	return &secretRepository{db: db}
}

func (r *secretRepository) Get(ctx context.Context, name string) (*models.Secret, error) {
	out := &models.Secret{}
	err := r.db.NewSelect().Model(out).
		Where("s.name = ?", name).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *secretRepository) List(ctx context.Context) ([]models.Secret, error) {
	var out []models.Secret
	if err := r.db.NewSelect().Model(&out).
		OrderExpr("name ASC").
		Scan(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

// Upsert inserts a new row or replaces the encrypted blob fields on an
// existing row keyed by name. created_at is preserved on conflict;
// updated_at advances on every successful call so the API surface can
// expose "when was this last rotated" without an extra write.
func (r *secretRepository) Upsert(ctx context.Context, s *models.Secret) error {
	now := time.Now().UTC()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now

	_, err := r.db.NewInsert().Model(s).
		On("CONFLICT (name) DO UPDATE").
		Set("ciphertext = EXCLUDED.ciphertext").
		Set("nonce = EXCLUDED.nonce").
		Set("wrapped_dek = EXCLUDED.wrapped_dek").
		Set("dek_nonce = EXCLUDED.dek_nonce").
		Set("algorithm = EXCLUDED.algorithm").
		Set("kek_version = EXCLUDED.kek_version").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *secretRepository) Delete(ctx context.Context, name string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Secret)(nil)).
		Where("name = ?", name).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
