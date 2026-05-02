//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/content-control-center/app/src/crypto/envelope"
	"github.com/content-control-center/app/src/handlers"
	"github.com/content-control-center/app/src/repository"
	"github.com/content-control-center/app/src/secrets"
)

// secretsTestRig wires a freshly migrated DB, a SecretStore on a
// random KEK, the SecretsHandler, and a logged-in session cookie.
// Each test gets its own rig so DB + KEK are pristine.
type secretsTestRig struct {
	app    *fiber.App
	cookie *http.Cookie
	store  secrets.Store
}

func newSecretsRig() *secretsTestRig {
	db := mustOpenIntegrationDB()

	kek := make([]byte, envelope.KeySize)
	_, err := rand.Read(kek)
	Expect(err).NotTo(HaveOccurred())
	cipher, err := envelope.NewCipher(kek)
	Expect(err).NotTo(HaveOccurred())
	store := secrets.NewStore(repository.NewSecretRepository(db), cipher)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	const cookieName = "c3_session_secrets"
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	auth := handlers.RequireAuth(sessionRepo, cookieName)

	handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
	handlers.NewSessionsHandler(userRepo, sessionRepo, cookieName, false).Register(app)
	handlers.NewSecretsHandler(store, auth).Register(app)
	handlers.NewHealthHandler(db, store).Register(app)

	// Register an admin user and capture the session cookie.
	body, _ := json.Marshal(fiber.Map{
		"name": "Secrets Tester", "email": "secrets@test.local", "password": "test-password",
	})
	req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

	body, _ = json.Marshal(fiber.Map{"email": "secrets@test.local", "password": "test-password"})
	req = httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == cookieName {
			cookie = c
			break
		}
	}
	Expect(cookie).NotTo(BeNil())

	return &secretsTestRig{app: app, cookie: cookie, store: store}
}

func (r *secretsTestRig) do(method, path string, body any) *http.Response {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(r.cookie)
	resp, err := r.app.Test(req, -1)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

var _ = Describe("Secrets API lifecycle", func() {
	It("supports the full PUT/GET/DELETE flow without leaking plaintext", func() {
		rig := newSecretsRig()
		const plaintext = "sk-ant-api03-LIFECYCLE-VALUE"

		// Empty list to start.
		resp := rig.do("GET", "/api/secrets/", nil)
		Expect(resp.StatusCode).To(Equal(200))
		body, _ := io.ReadAll(resp.Body)
		Expect(body).NotTo(ContainSubstring(plaintext))
		var list []secrets.Metadata
		Expect(json.Unmarshal(body, &list)).To(Succeed())
		Expect(list).To(BeEmpty())

		// PUT (create) → 201.
		resp = rig.do("PUT", "/api/secrets/anthropic_api_key", fiber.Map{"value": plaintext})
		Expect(resp.StatusCode).To(Equal(201))
		body, _ = io.ReadAll(resp.Body)
		Expect(body).NotTo(ContainSubstring(plaintext))
		var meta secrets.Metadata
		Expect(json.Unmarshal(body, &meta)).To(Succeed())
		Expect(meta.Name).To(Equal("anthropic_api_key"))
		Expect(meta.Decryptable).To(BeTrue())
		firstUpdate := meta.UpdatedAt

		// GET single → 200 with metadata, no plaintext.
		resp = rig.do("GET", "/api/secrets/anthropic_api_key", nil)
		Expect(resp.StatusCode).To(Equal(200))
		body, _ = io.ReadAll(resp.Body)
		Expect(body).NotTo(ContainSubstring(plaintext))

		// PUT renew → 200, updated_at advances.
		time.Sleep(10 * time.Millisecond) // ensure clock tick
		resp = rig.do("PUT", "/api/secrets/anthropic_api_key", fiber.Map{"value": plaintext + "-RENEWED"})
		Expect(resp.StatusCode).To(Equal(200))
		body, _ = io.ReadAll(resp.Body)
		var renewed secrets.Metadata
		Expect(json.Unmarshal(body, &renewed)).To(Succeed())
		Expect(renewed.UpdatedAt.After(firstUpdate)).To(BeTrue(),
			"updated_at should advance: was %s, now %s", firstUpdate, renewed.UpdatedAt)

		// Ensure the stored value is the rotated one.
		current, err := rig.store.Get(context.Background(), secrets.NameAnthropicAPIKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(current).To(Equal(plaintext + "-RENEWED"))

		// DELETE → 204.
		resp = rig.do("DELETE", "/api/secrets/anthropic_api_key", nil)
		Expect(resp.StatusCode).To(Equal(204))

		// GET → 404.
		resp = rig.do("GET", "/api/secrets/anthropic_api_key", nil)
		Expect(resp.StatusCode).To(Equal(404))
	})

	It("rejects unknown names with 400 and never echoes the value", func() {
		rig := newSecretsRig()
		const sentinel = "SENTINEL_VALUE_SHOULD_NEVER_LEAK"

		resp := rig.do("PUT", "/api/secrets/openai_api_key", fiber.Map{"value": sentinel})
		Expect(resp.StatusCode).To(Equal(400))
		body, _ := io.ReadAll(resp.Body)
		Expect(string(body)).NotTo(ContainSubstring(sentinel))
	})

	It("rejects invalid values with 400 and never echoes the value", func() {
		rig := newSecretsRig()
		const sentinel = "SENTINEL_NEWLINE_ATTEMPT"

		resp := rig.do("PUT", "/api/secrets/anthropic_api_key", fiber.Map{"value": sentinel + "\n"})
		Expect(resp.StatusCode).To(Equal(400))
		body, _ := io.ReadAll(resp.Body)
		Expect(string(body)).NotTo(ContainSubstring(sentinel))

		resp = rig.do("PUT", "/api/secrets/anthropic_api_key", fiber.Map{"value": ""})
		Expect(resp.StatusCode).To(Equal(400))

		resp = rig.do("PUT", "/api/secrets/anthropic_api_key", fiber.Map{"value": strings.Repeat("a", secrets.MaxValueLen+1)})
		Expect(resp.StatusCode).To(Equal(400))
	})

	It("requires authentication", func() {
		rig := newSecretsRig()
		// No cookie attached → 401.
		req := httptest.NewRequest("GET", "/api/secrets/", nil)
		resp, err := rig.app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(401))
	})
})

var _ = Describe("Secrets env-var migration on boot", func() {
	It("migrates env values into the DB on first boot and DB wins thereafter", func() {
		ctx := context.Background()
		db := mustOpenIntegrationDB()

		kek := make([]byte, envelope.KeySize)
		_, err := rand.Read(kek)
		Expect(err).NotTo(HaveOccurred())
		cipher, err := envelope.NewCipher(kek)
		Expect(err).NotTo(HaveOccurred())
		store := secrets.NewStore(repository.NewSecretRepository(db), cipher)

		// First boot: env vars set, DB empty → migrated.
		result, err := secrets.MigrateFromEnv(ctx, store, []secrets.EnvSource{
			{Name: secrets.NameAnthropicAPIKey, EnvValue: "env-anthropic"},
			{Name: secrets.NameZernioAPIKey, EnvValue: "env-zernio"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.MigratedFromEnv).To(Equal(2))
		Expect(result.IgnoredEnvVars).To(Equal(0))
		Expect(result.SecretsInDB).To(Equal(2))

		got, err := store.Get(ctx, secrets.NameAnthropicAPIKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("env-anthropic"))

		// Second boot: env vars still set, DB also populated. DB wins,
		// envs ignored and counted.
		result, err = secrets.MigrateFromEnv(ctx, store, []secrets.EnvSource{
			{Name: secrets.NameAnthropicAPIKey, EnvValue: "env-anthropic-NEW-IGNORED"},
			{Name: secrets.NameZernioAPIKey, EnvValue: "env-zernio-NEW-IGNORED"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.MigratedFromEnv).To(Equal(0))
		Expect(result.IgnoredEnvVars).To(Equal(2))
		Expect(result.SecretsInDB).To(Equal(2))

		// DB value is still the original — env did not overwrite it.
		got, err = store.Get(ctx, secrets.NameAnthropicAPIKey)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("env-anthropic"))
	})

	It("leaves the app in a degraded state when neither DB nor env has a key", func() {
		ctx := context.Background()
		db := mustOpenIntegrationDB()
		kek := make([]byte, envelope.KeySize)
		_, _ = rand.Read(kek)
		cipher, _ := envelope.NewCipher(kek)
		store := secrets.NewStore(repository.NewSecretRepository(db), cipher)

		result, err := secrets.MigrateFromEnv(ctx, store, []secrets.EnvSource{
			{Name: secrets.NameAnthropicAPIKey, EnvValue: ""},
			{Name: secrets.NameZernioAPIKey, EnvValue: ""},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.MigratedFromEnv).To(Equal(0))
		Expect(result.SecretsInDB).To(Equal(0))

		_, err = store.Get(ctx, secrets.NameAnthropicAPIKey)
		Expect(err).To(MatchError(secrets.ErrNotFound))
	})
})

var _ = Describe("Secrets hot-reload subscriber", func() {
	It("notifies subscribers after PUT and DELETE", func() {
		rig := newSecretsRig()
		var fires int
		rig.store.Subscribe(secrets.NameZernioAPIKey, func() { fires++ })

		resp := rig.do("PUT", "/api/secrets/zernio_api_key", fiber.Map{"value": "key-1"})
		Expect(resp.StatusCode).To(Equal(201))
		Expect(fires).To(Equal(1))

		resp = rig.do("PUT", "/api/secrets/zernio_api_key", fiber.Map{"value": "key-2"})
		Expect(resp.StatusCode).To(Equal(200))
		Expect(fires).To(Equal(2))

		resp = rig.do("DELETE", "/api/secrets/zernio_api_key", nil)
		Expect(resp.StatusCode).To(Equal(204))
		Expect(fires).To(Equal(3))
	})

	// Read-after-write semantics: a Zernio-style resolver wired to the
	// store sees the rotated value on its very next call, no restart.
	It("resolves the new value through a per-call resolver after PUT", func() {
		rig := newSecretsRig()
		ctx := context.Background()
		resolver := func(ctx context.Context) (string, error) {
			return rig.store.Get(ctx, secrets.NameZernioAPIKey)
		}

		// PUT key v1.
		resp := rig.do("PUT", "/api/secrets/zernio_api_key", fiber.Map{"value": "v1"})
		Expect(resp.StatusCode).To(Equal(201))
		got, err := resolver(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("v1"))

		// Rotate to v2.
		resp = rig.do("PUT", "/api/secrets/zernio_api_key", fiber.Map{"value": "v2"})
		Expect(resp.StatusCode).To(Equal(200))
		got, err = resolver(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal("v2"))
	})
})

var _ = Describe("Health endpoint reports secret resolvability", func() {
	It("reports presence + decryptable per allowlisted name", func() {
		rig := newSecretsRig()

		// Boot: no secrets present.
		resp := rig.do("GET", "/api/health", nil)
		Expect(resp.StatusCode).To(Equal(200))
		body, _ := io.ReadAll(resp.Body)
		var health struct {
			Status  string                          `json:"status"`
			Secrets map[string]map[string]bool      `json:"secrets"`
		}
		Expect(json.Unmarshal(body, &health)).To(Succeed())
		Expect(health.Status).To(Equal("ok"))
		Expect(health.Secrets["anthropic_api_key"]["present"]).To(BeFalse())
		Expect(health.Secrets["zernio_api_key"]["present"]).To(BeFalse())

		// Add an Anthropic key.
		resp = rig.do("PUT", "/api/secrets/anthropic_api_key", fiber.Map{"value": "k"})
		Expect(resp.StatusCode).To(Equal(201))

		resp = rig.do("GET", "/api/health", nil)
		body, _ = io.ReadAll(resp.Body)
		Expect(json.Unmarshal(body, &health)).To(Succeed())
		Expect(health.Status).To(Equal("ok"))
		Expect(health.Secrets["anthropic_api_key"]["present"]).To(BeTrue())
		Expect(health.Secrets["anthropic_api_key"]["decryptable"]).To(BeTrue())
	})
})
