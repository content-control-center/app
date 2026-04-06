package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// SettingRepository defines all persistence operations for the Setting domain.
type SettingRepository interface {
	List(ctx context.Context) ([]models.Setting, error)
	GetByKey(ctx context.Context, key string) (*models.Setting, error)
	Upsert(ctx context.Context, setting *models.Setting) error
	Delete(ctx context.Context, key string) (bool, error)
}

type settingRepository struct {
	db *bun.DB
}

// NewSettingRepository returns a Bun-backed SettingRepository.
func NewSettingRepository(db *bun.DB) SettingRepository {
	return &settingRepository{db: db}
}

func (r *settingRepository) List(ctx context.Context) ([]models.Setting, error) {
	var settings []models.Setting
	if err := r.db.NewSelect().Model(&settings).OrderExpr("key ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return settings, nil
}

func (r *settingRepository) GetByKey(ctx context.Context, key string) (*models.Setting, error) {
	setting := new(models.Setting)
	err := r.db.NewSelect().Model(setting).Where("st.key = ?", key).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return setting, nil
}

func (r *settingRepository) Upsert(ctx context.Context, setting *models.Setting) error {
	_, err := r.db.NewInsert().Model(setting).
		On("CONFLICT (key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

func (r *settingRepository) Delete(ctx context.Context, key string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Setting)(nil)).Where("key = ?", key).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
