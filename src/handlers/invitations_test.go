package handlers_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// fakeInviteEnqueuer records the invitation emails a create would enqueue, so a
// test can assert the enqueue fired without standing up River (CON-26). It is
// invoked inside the create tx; recording + returning nil lets the tx commit.
type fakeInviteEnqueuer struct {
	toEmails []string
}

func (f *fakeInviteEnqueuer) EnqueueInvitationEmailTx(ctx context.Context, tx *sql.Tx, tenantID, invitationID, toEmail, inviteURL, inviterName, workspaceName, role string) error {
	f.toEmails = append(f.toEmails, toEmail)
	return nil
}

var _ = Describe("InvitationsHandler", Ordered, func() {
	var (
		app      *fiber.App
		db       *bun.DB
		enqueuer *fakeInviteEnqueuer
	)

	ctx := context.Background()

	BeforeAll(func() {
		db = mustOpenTestDBWithMigrations()
	})

	BeforeEach(func() {
		app = fiber.New(fiber.Config{
			ErrorHandler: func(c *fiber.Ctx, err error) error {
				code := fiber.StatusInternalServerError
				if e, ok := err.(*fiber.Error); ok {
					code = e.Code
				}
				return c.Status(code).JSON(fiber.Map{"error": err.Error()})
			},
		})
		userRepo := repository.NewUserRepository(db)
		tenantRepo := repository.NewTenantRepository(db)
		sessionRepo := repository.NewSessionRepository(db)
		settingRepo := repository.NewSettingRepository(db)
		inviteRepo := repository.NewInvitationRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)

		enqueuer = &fakeInviteEnqueuer{}
		ih := handlers.NewInvitationsHandler(db, userRepo, tenantRepo, inviteRepo, "https://app.example.com", testCookieName, false, auth)
		ih.SetEmailEnqueuer(enqueuer)
		ih.Register(app)
		// UsersHandler for the role-change / removal (last-owner) tests; Sessions for login.
		handlers.NewUsersHandler(db, userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
	})

	AfterEach(func() {
		for _, tbl := range []string{"users_invitations", "sessions", "users"} {
			_, err := db.NewDelete().TableExpr(tbl).Where("1 = 1").Exec(ctx)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	// ── helpers ────────────────────────────────────────────────────────────────

	login := func(email, password string) *http.Cookie {
		GinkgoHelper()
		body, _ := json.Marshal(fiber.Map{"email": email, "password": password})
		req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		return resp.Cookies()[0]
	}

	// ownerCookie seeds an owner and returns an authenticated cookie for them.
	ownerCookie := func() (*models.User, *http.Cookie) {
		GinkgoHelper()
		u := seedTenantUserWithRole(db, "Olivia Owner", "owner@example.com", "owner-password", models.RoleOwner)
		return u, login("owner@example.com", "owner-password")
	}

	memberCookie := func() (*models.User, *http.Cookie) {
		GinkgoHelper()
		u := seedTenantUserWithRole(db, "Manny Member", "member@example.com", "member-password", models.RoleMember)
		return u, login("member@example.com", "member-password")
	}

	createInvite := func(cookie *http.Cookie, payload fiber.Map) *http.Response {
		GinkgoHelper()
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/invitations", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	preview := func(token string) *http.Response {
		GinkgoHelper()
		req := httptest.NewRequest("GET", "/api/invitations/accept/"+token, nil)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	accept := func(token string, payload fiber.Map) *http.Response {
		GinkgoHelper()
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/invitations/accept/"+token, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	// seedInvite inserts an invitation directly and returns the plaintext token.
	seedInvite := func(invitedBy, email, role, status string, expiresAt time.Time) string {
		GinkgoHelper()
		token, hash, err := models.NewInvitationToken()
		Expect(err).NotTo(HaveOccurred())
		id, err := models.NewID()
		Expect(err).NotTo(HaveOccurred())
		inv := &models.Invitation{
			ID: id, TenantID: models.DefaultTenantID, Email: email, Role: role,
			TokenHash: hash, InvitedBy: invitedBy, Status: status,
			ExpiresAt: expiresAt, CreatedAt: time.Now().UTC(),
		}
		_, err = db.NewInsert().Model(inv).Exec(ctx)
		Expect(err).NotTo(HaveOccurred())
		return token
	}

	countUsersByEmail := func(email string) int {
		GinkgoHelper()
		n, err := db.NewSelect().Model((*models.User)(nil)).Where("email = ?", email).Count(ctx)
		Expect(err).NotTo(HaveOccurred())
		return n
	}

	getInviteByEmail := func(email string) *models.Invitation {
		GinkgoHelper()
		inv := new(models.Invitation)
		Expect(db.NewSelect().Model(inv).Where("email = ?", email).Scan(ctx)).To(Succeed())
		return inv
	}

	// ── POST /api/invitations ────────────────────────────────────────────────

	Describe("POST /api/invitations", func() {
		It("lets an owner invite a teammate and enqueues the email (201)", func() {
			_, cookie := ownerCookie()
			resp := createInvite(cookie, fiber.Map{"email": "New@Example.com"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

			var inv models.Invitation
			Expect(json.NewDecoder(resp.Body).Decode(&inv)).To(Succeed())
			Expect(inv.Email).To(Equal("new@example.com")) // normalized
			Expect(inv.Role).To(Equal(models.RoleMember))  // default
			Expect(inv.Status).To(Equal(models.InvitationPending))
			Expect(enqueuer.toEmails).To(ContainElement("new@example.com"))
		})

		It("honours an explicit owner role", func() {
			_, cookie := ownerCookie()
			resp := createInvite(cookie, fiber.Map{"email": "co@example.com", "role": "owner"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
			Expect(getInviteByEmail("co@example.com").Role).To(Equal(models.RoleOwner))
		})

		It("rejects a non-owner with 403", func() {
			memberCookie()
			cookie := login("member@example.com", "member-password")
			resp := createInvite(cookie, fiber.Map{"email": "x@example.com"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusForbidden))
		})

		It("requires authentication (401)", func() {
			resp := createInvite(nil, fiber.Map{"email": "x@example.com"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusUnauthorized))
		})

		It("rejects a malformed email (400)", func() {
			_, cookie := ownerCookie()
			resp := createInvite(cookie, fiber.Map{"email": "not-an-email"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusBadRequest))
		})

		It("rejects an email that already has an account (409)", func() {
			_, cookie := ownerCookie()
			seedTenantUser(db, "Existing", "taken@example.com", "some-password")
			resp := createInvite(cookie, fiber.Map{"email": "taken@example.com"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusConflict))
		})

		It("rejects a duplicate pending invite (409)", func() {
			_, cookie := ownerCookie()
			Expect(createInvite(cookie, fiber.Map{"email": "dup@example.com"}).StatusCode).To(Equal(fiber.StatusCreated))
			resp := createInvite(cookie, fiber.Map{"email": "dup@example.com"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusConflict))
		})

		It("replaces an expired pending invite so the address can be re-invited (201)", func() {
			owner, cookie := ownerCookie()
			// An expired-but-still-pending row occupies the partial-unique (pending)
			// slot; a fresh invite must clear it rather than 409 forever.
			seedInvite(owner.ID, "stale@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(-time.Minute))

			resp := createInvite(cookie, fiber.Map{"email": "stale@example.com"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

			// Exactly one row remains for the address — the stale one is gone and a
			// single live pending invite replaces it.
			total, err := db.NewSelect().Model((*models.Invitation)(nil)).Where("email = ?", "stale@example.com").Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(total).To(Equal(1))
			Expect(getInviteByEmail("stale@example.com").Status).To(Equal(models.InvitationPending))
			Expect(time.Now().Before(getInviteByEmail("stale@example.com").ExpiresAt)).To(BeTrue())
		})
	})

	// ── GET / DELETE /api/invitations ─────────────────────────────────────────

	Describe("GET /api/invitations", func() {
		It("lists the workspace's invitations for an owner", func() {
			owner, cookie := ownerCookie()
			seedInvite(owner.ID, "a@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(time.Hour))
			seedInvite(owner.ID, "b@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(time.Hour))

			req := httptest.NewRequest("GET", "/api/invitations", nil)
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(fiber.StatusOK))
			var invs []models.Invitation
			Expect(json.NewDecoder(resp.Body).Decode(&invs)).To(Succeed())
			Expect(invs).To(HaveLen(2))
		})

		It("rejects a non-owner with 403", func() {
			memberCookie()
			cookie := login("member@example.com", "member-password")
			req := httptest.NewRequest("GET", "/api/invitations", nil)
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(fiber.StatusForbidden))
		})
	})

	Describe("DELETE /api/invitations/:id", func() {
		It("revokes a pending invite (204) and kills its link", func() {
			owner, cookie := ownerCookie()
			token := seedInvite(owner.ID, "rev@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(time.Hour))
			id := getInviteByEmail("rev@example.com").ID

			req := httptest.NewRequest("DELETE", "/api/invitations/"+id, nil)
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(fiber.StatusNoContent))
			Expect(getInviteByEmail("rev@example.com").Status).To(Equal(models.InvitationRevoked))

			// The revoked token no longer previews.
			Expect(preview(token).StatusCode).To(Equal(fiber.StatusGone))
		})

		It("returns 404 for an unknown id", func() {
			_, cookie := ownerCookie()
			req := httptest.NewRequest("DELETE", "/api/invitations/nope", nil)
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(fiber.StatusNotFound))
		})
	})

	// ── GET /api/invitations/accept/:token (preview) ──────────────────────────

	Describe("GET /api/invitations/accept/:token", func() {
		It("previews a valid pending invite", func() {
			owner, _ := ownerCookie()
			token := seedInvite(owner.ID, "prev@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(time.Hour))
			resp := preview(token)
			Expect(resp.StatusCode).To(Equal(fiber.StatusOK))
			var out map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&out)).To(Succeed())
			Expect(out["email"]).To(Equal("prev@example.com"))
			Expect(out["role"]).To(Equal(models.RoleMember))
			Expect(out["inviter_name"]).To(Equal("Olivia Owner"))
			Expect(out["workspace_name"]).NotTo(BeEmpty())
		})

		It("returns a generic 404 for an unknown token", func() {
			Expect(preview("does-not-exist").StatusCode).To(Equal(fiber.StatusNotFound))
		})

		It("returns 410 for an expired invite", func() {
			owner, _ := ownerCookie()
			token := seedInvite(owner.ID, "exp@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(-time.Minute))
			Expect(preview(token).StatusCode).To(Equal(fiber.StatusGone))
		})
	})

	// ── POST /api/invitations/accept/:token ───────────────────────────────────

	Describe("POST /api/invitations/accept/:token", func() {
		It("creates the invited user with the invited role, opens a session, and marks the invite accepted", func() {
			owner, _ := ownerCookie()
			token := seedInvite(owner.ID, "join@example.com", models.RoleOwner, models.InvitationPending, time.Now().Add(time.Hour))

			resp := accept(token, fiber.Map{"name": "Joiner", "password": "joiner-password"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

			// Auto-login cookie set.
			var sessionCookie *http.Cookie
			for _, ck := range resp.Cookies() {
				if ck.Name == testCookieName {
					sessionCookie = ck
				}
			}
			Expect(sessionCookie).NotTo(BeNil())
			Expect(sessionCookie.Value).NotTo(BeEmpty())

			// User created with the invited role, in the invite's tenant.
			created := new(models.User)
			Expect(db.NewSelect().Model(created).Where("email = ?", "join@example.com").Scan(ctx)).To(Succeed())
			Expect(created.Role).To(Equal(models.RoleOwner))
			Expect(created.TenantID).To(Equal(models.DefaultTenantID))

			// Invite consumed.
			Expect(getInviteByEmail("join@example.com").Status).To(Equal(models.InvitationAccepted))
		})

		It("validates the password before consuming the token (400, invite still pending)", func() {
			owner, _ := ownerCookie()
			token := seedInvite(owner.ID, "weak@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(time.Hour))
			resp := accept(token, fiber.Map{"name": "Weak", "password": "short"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusBadRequest))
			Expect(getInviteByEmail("weak@example.com").Status).To(Equal(models.InvitationPending))
			Expect(countUsersByEmail("weak@example.com")).To(Equal(0))
		})

		It("returns a generic 410 for an unknown token", func() {
			resp := accept("nonexistent", fiber.Map{"name": "X", "password": "some-password"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusGone))
		})

		It("returns 410 for an expired invite", func() {
			owner, _ := ownerCookie()
			token := seedInvite(owner.ID, "old@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(-time.Minute))
			resp := accept(token, fiber.Map{"name": "Old", "password": "some-password"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusGone))
			Expect(countUsersByEmail("old@example.com")).To(Equal(0))
		})

		It("consumes the invite exactly once under a double accept", func() {
			owner, _ := ownerCookie()
			token := seedInvite(owner.ID, "twice@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(time.Hour))

			first := accept(token, fiber.Map{"name": "First", "password": "first-password"})
			Expect(first.StatusCode).To(Equal(fiber.StatusCreated))
			second := accept(token, fiber.Map{"name": "Second", "password": "second-password"})
			Expect(second.StatusCode).To(Equal(fiber.StatusGone))
			Expect(countUsersByEmail("twice@example.com")).To(Equal(1))
		})

		It("returns 409 when the email gained an account since the invite was sent", func() {
			owner, _ := ownerCookie()
			token := seedInvite(owner.ID, "raced@example.com", models.RoleMember, models.InvitationPending, time.Now().Add(time.Hour))
			seedTenantUser(db, "Raced", "raced@example.com", "some-password")
			resp := accept(token, fiber.Map{"name": "Raced", "password": "another-password"})
			Expect(resp.StatusCode).To(Equal(fiber.StatusConflict))
		})
	})

	// ── PATCH /api/users/:id/role + last-owner invariant ──────────────────────

	Describe("PATCH /api/users/:id/role", func() {
		setRole := func(cookie *http.Cookie, id, role string) *http.Response {
			GinkgoHelper()
			body, _ := json.Marshal(fiber.Map{"role": role})
			req := httptest.NewRequest("PATCH", fmt.Sprintf("/api/users/%s/role", id), bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if cookie != nil {
				req.AddCookie(cookie)
			}
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			return resp
		}

		It("lets an owner promote a member to owner", func() {
			_, cookie := ownerCookie()
			member := seedTenantUserWithRole(db, "Bob", "bob@example.com", "bob-password", models.RoleMember)
			resp := setRole(cookie, member.ID, models.RoleOwner)
			Expect(resp.StatusCode).To(Equal(fiber.StatusOK))

			updated := new(models.User)
			Expect(db.NewSelect().Model(updated).Where("id = ?", member.ID).Scan(ctx)).To(Succeed())
			Expect(updated.Role).To(Equal(models.RoleOwner))
		})

		It("lets an owner demote a co-owner when another owner remains", func() {
			_, cookie := ownerCookie()
			co := seedTenantUserWithRole(db, "Co", "co@example.com", "co-password", models.RoleOwner)
			resp := setRole(cookie, co.ID, models.RoleMember)
			Expect(resp.StatusCode).To(Equal(fiber.StatusOK))
		})

		It("refuses to demote the last owner (409)", func() {
			owner, cookie := ownerCookie()
			resp := setRole(cookie, owner.ID, models.RoleMember)
			Expect(resp.StatusCode).To(Equal(fiber.StatusConflict))
		})

		It("rejects a non-owner (403)", func() {
			member, _ := memberCookie()
			cookie := login("member@example.com", "member-password")
			resp := setRole(cookie, member.ID, models.RoleOwner)
			Expect(resp.StatusCode).To(Equal(fiber.StatusForbidden))
		})
	})

	Describe("DELETE /api/users/:id (member removal)", func() {
		del := func(cookie *http.Cookie, id string) *http.Response {
			GinkgoHelper()
			req := httptest.NewRequest("DELETE", "/api/users/"+id, nil)
			if cookie != nil {
				req.AddCookie(cookie)
			}
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			return resp
		}

		It("lets an owner remove a member", func() {
			_, cookie := ownerCookie()
			member := seedTenantUserWithRole(db, "Bye", "bye@example.com", "bye-password", models.RoleMember)
			Expect(del(cookie, member.ID).StatusCode).To(Equal(fiber.StatusNoContent))
			Expect(countUsersByEmail("bye@example.com")).To(Equal(0))
		})

		It("refuses to remove the last owner, even self (409)", func() {
			owner, cookie := ownerCookie()
			Expect(del(cookie, owner.ID).StatusCode).To(Equal(fiber.StatusConflict))
			Expect(countUsersByEmail("owner@example.com")).To(Equal(1))
		})

		It("lets an owner remove a co-owner when another owner remains", func() {
			_, cookie := ownerCookie()
			co := seedTenantUserWithRole(db, "Co", "co@example.com", "co-password", models.RoleOwner)
			Expect(del(cookie, co.ID).StatusCode).To(Equal(fiber.StatusNoContent))
		})
	})
})
