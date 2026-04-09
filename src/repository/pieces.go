package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

// PieceRepository defines all persistence operations for the Piece domain.
type PieceRepository interface {
	List(ctx context.Context) ([]models.Piece, error)
	Create(ctx context.Context, piece *models.Piece) error
	GetByID(ctx context.Context, id string) (*models.Piece, error)
	Update(ctx context.Context, piece *models.Piece) error
	Delete(ctx context.Context, id string) (bool, error)
}

type pieceRepository struct {
	db      *bun.DB
	tagRepo TagRepository
}

// NewPieceRepository returns a Bun-backed PieceRepository.
func NewPieceRepository(db *bun.DB, tagRepo TagRepository) PieceRepository {
	return &pieceRepository{db: db, tagRepo: tagRepo}
}

func (r *pieceRepository) List(ctx context.Context) ([]models.Piece, error) {
	var pieces []models.Piece
	if err := r.db.NewSelect().Model(&pieces).OrderExpr("created_at ASC").Scan(ctx); err != nil {
		return nil, err
	}
	if err := r.hydrateTags(ctx, pieces); err != nil {
		return nil, err
	}
	return pieces, nil
}

func (r *pieceRepository) Create(ctx context.Context, piece *models.Piece) error {
	_, err := r.db.NewInsert().Model(piece).Exec(ctx)
	return err
}

func (r *pieceRepository) GetByID(ctx context.Context, id string) (*models.Piece, error) {
	piece := new(models.Piece)
	err := r.db.NewSelect().Model(piece).Where("p.id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	pieces := []models.Piece{*piece}
	if err := r.hydrateTags(ctx, pieces); err != nil {
		return nil, err
	}
	return &pieces[0], nil
}

func (r *pieceRepository) Update(ctx context.Context, piece *models.Piece) error {
	_, err := r.db.NewUpdate().Model(piece).WherePK().Exec(ctx)
	return err
}

func (r *pieceRepository) Delete(ctx context.Context, id string) (bool, error) {
	res, err := r.db.NewDelete().Model((*models.Piece)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *pieceRepository) hydrateTags(ctx context.Context, pieces []models.Piece) error {
	return hydrateTags(ctx, pieces, r.tagRepo,
		func(p models.Piece) []string { return p.TagIDs },
		func(p *models.Piece, tags []models.Tag) { p.Tags = tags },
	)
}
