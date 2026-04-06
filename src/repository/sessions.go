package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// SessionRepository defines all persistence operations for the Session domain.
type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) error
	GetByID(ctx context.Context, id string) (*models.Session, error)
	Delete(ctx context.Context, id string) (bool, error)
}

type sessionRepository struct {
	db *bun.DB
}

// NewSessionRepository returns a Bun-backed SessionRepository.
func NewSessionRepository(db *bun.DB) SessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) Create(ctx context.Context, session *models.Session) error {
	_, err := r.db.NewInsert().Model(session).Exec(ctx)
	return err
}

func (r *sessionRepository) GetByID(ctx context.Context, id string) (*models.Session, error) {
	session := new(models.Session)
	err := r.db.NewSelect().Model(session).Where("s.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return session, nil
}

func (r *sessionRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Session)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
