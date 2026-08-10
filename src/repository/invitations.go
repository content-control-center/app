package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/models"
)

// InvitationRepository is the persistence surface for workspace invitations
// (CON-26). The token-minting insert and the accept flow run inside the calling
// handler's transaction (mirroring password reset), so they are not here; this
// interface covers the plain reads + the revoke used by the management endpoints
// and the public preview. Invitations are not TenantScoped — the accept/preview
// paths look them up by token before any tenant is in context — so tenant-scoped
// methods take the tenant id explicitly.
type InvitationRepository interface {
	// Create inserts an invitation. The owner-facing create endpoint inserts
	// inside its own tx (so the email enqueue commits atomically); this is for
	// tests and any non-transactional caller.
	Create(ctx context.Context, inv *models.Invitation) error
	// GetByTokenHash resolves an invitation by the sha256 of its token — the
	// capability the preview/accept endpoints hold. Not tenant-scoped by design.
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.Invitation, error)
	// GetPendingByTenantEmail returns the live (pending) invite for an address in
	// a workspace, if any — the friendly pre-check behind the create endpoint's
	// duplicate 409 (the partial unique index is the hard backstop).
	GetPendingByTenantEmail(ctx context.Context, tenantID, email string) (*models.Invitation, error)
	// ListByTenant returns a workspace's invitations, newest first.
	ListByTenant(ctx context.Context, tenantID string) ([]models.Invitation, error)
	// Revoke marks a pending invite revoked. Returns false if no pending invite
	// with that id exists in the tenant (already accepted/revoked, or not found).
	Revoke(ctx context.Context, id, tenantID string) (bool, error)
}

type invitationRepository struct {
	db *bun.DB
}

// NewInvitationRepository returns a Bun-backed InvitationRepository.
func NewInvitationRepository(db *bun.DB) InvitationRepository {
	return &invitationRepository{db: db}
}

func (r *invitationRepository) Create(ctx context.Context, inv *models.Invitation) error {
	_, err := r.db.NewInsert().Model(inv).Exec(ctx)
	return err
}

func (r *invitationRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*models.Invitation, error) {
	inv := new(models.Invitation)
	err := r.db.NewSelect().Model(inv).Where("token_hash = ?", tokenHash).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return inv, nil
}

func (r *invitationRepository) GetPendingByTenantEmail(ctx context.Context, tenantID, email string) (*models.Invitation, error) {
	inv := new(models.Invitation)
	err := r.db.NewSelect().Model(inv).
		Where("tenant_id = ?", tenantID).
		Where("email = ?", email).
		Where("status = ?", models.InvitationPending).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	return inv, nil
}

func (r *invitationRepository) ListByTenant(ctx context.Context, tenantID string) ([]models.Invitation, error) {
	var invs []models.Invitation
	err := r.db.NewSelect().Model(&invs).
		Where("tenant_id = ?", tenantID).
		OrderExpr("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return invs, nil
}

func (r *invitationRepository) Revoke(ctx context.Context, id, tenantID string) (bool, error) {
	res, err := r.db.NewUpdate().Model((*models.Invitation)(nil)).
		Set("status = ?", models.InvitationRevoked).
		Where("id = ?", id).
		Where("tenant_id = ?", tenantID).
		Where("status = ?", models.InvitationPending).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
