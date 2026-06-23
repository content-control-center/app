package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// UserRepository defines all persistence operations for the User domain.
type UserRepository interface {
	List(ctx context.Context) ([]models.User, error)
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id string) (*models.User, error)
	// GetByIDWithTenant is GetByID with the Tenant relation eager-loaded, for
	// GET /api/current_user (CON-97).
	GetByIDWithTenant(ctx context.Context, id string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	Delete(ctx context.Context, id string) (bool, error)
}

type userRepository struct {
	db *bun.DB
}

// NewUserRepository returns a Bun-backed UserRepository.
func NewUserRepository(db *bun.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) List(ctx context.Context) ([]models.User, error) {
	var users []models.User
	if err := r.db.NewSelect().Model(&users).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	return users, nil
}

func (r *userRepository) Create(ctx context.Context, user *models.User) error {
	_, err := r.db.NewInsert().Model(user).Exec(ctx)
	return err
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*models.User, error) {
	user := new(models.User)
	err := r.db.NewSelect().Model(user).Where("u.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return user, nil
}

func (r *userRepository) GetByIDWithTenant(ctx context.Context, id string) (*models.User, error) {
	user := new(models.User)
	err := r.db.NewSelect().Model(user).Relation("Tenant").Where("u.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	user := new(models.User)
	err := r.db.NewSelect().Model(user).Where("u.email = ?", email).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return user, nil
}

func (r *userRepository) Update(ctx context.Context, user *models.User) error {
	_, err := r.db.NewUpdate().Model(user).WherePK().Exec(ctx)
	return err
}

func (r *userRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.User)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
