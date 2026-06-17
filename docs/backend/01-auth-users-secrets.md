# Auth, Sessions, Users, Settings, Secrets, Health

Covers `src/handlers/{auth,sessions,users,settings,secrets,health,validate}.go`,
`src/secrets/{store,bootstrap}.go`, and `src/crypto/envelope/{envelope,kek}.go`.

All routes are under `/api`. Error responses are JSON `{"error": "<message>"}`. Request
bodies are JSON (`c.BodyParser`), validated with `go-playground/validator/v10`.

---

## Authentication model

### `RequireAuth` middleware (`src/handlers/auth.go`)

`RequireAuth(sessionRepo, cookieName) fiber.Handler` gates every protected route. Per request:

1. Read the session token from the cookie named `cookieName` (`SESSION_COOKIE_NAME`, default `c3_session`).
2. Empty cookie → `401` `"authentication required"`.
3. `sessionRepo.GetByID(token)`. `sql.ErrNoRows` → `401` `"invalid or expired session"`; other error → `500`.
4. `time.Now().UTC().After(session.ExpiresAt)` → `401` `"invalid or expired session"`.
5. On success, store `*models.Session` in `c.Locals("session")`, call `c.Next()`.

Expiry is checked in memory; an expired row is treated as invalid but not deleted here.

### Session cookie

Set by `SessionsHandler.Create` (login):

| Attribute | Value |
|---|---|
| `Name` | configured cookie name (default `c3_session`) |
| `Value` | session token |
| `Path` | `/` |
| `Expires` | `now + 7 days` (`sessionTTL = 7 * 24h`) |
| `HTTPOnly` | `true` |
| `Secure` | `!cfg.Debug` (so `Secure=false` only when `DEBUG=true`) |
| `SameSite` | `Lax` |

The token is a crypto-random 32-byte URL-safe string (`models.NewSessionToken`). The
session's `ID` **is** the token (no separate mapping).

### Logout

`SessionsHandler.Delete` removes the session row and overwrites the cookie with empty
value, `Expires: time.Unix(0,0)`, `MaxAge: -1`, matching the login attributes so the
browser clears it.

---

## Sessions — `/api/sessions`

### `POST /api/sessions` — Login (public)
- **Body** `createSessionRequest`: `email` (`required,email`), `password` (`required`).
- Parse/validate failure → `400`. `userRepo.GetByEmail`: not found → `401` `"invalid credentials"`.
  `models.VerifyPassword` mismatch → `401` `"invalid credentials"` (same message → no user enumeration).
- Creates `Session{ID: token, UserID, ExpiresAt: now+7d}`, sets cookie.
- **`201`** with the created `models.Session`. Codes: `201/400/401/500`.

### `DELETE /api/sessions` — Logout
- Reads token from cookie (not `RequireAuth`). Empty → `401` `"no session"`.
- `sessionRepo.Delete`. Nothing deleted → `401` `"invalid session"`. Clears cookie.
- **`204`**. Codes: `204/401/500`.

---

## Users — `/api/users` + `/api/current_user`

| Method + Path | Middleware | Auth |
|---|---|---|
| `GET /api/current_user` | `auth` | always |
| `POST /api/users` | `conditionalAuth` | **open while `setup_complete != "true"`, protected after** |
| `GET /api/users` | `auth` | always |
| `GET /api/users/:id` | `auth` | always |
| `PUT /api/users/:id` | `auth` + `requireSelf` | self-only |
| `DELETE /api/users/:id` | `auth` + `requireSelf` | self-only |

- **`conditionalAuth`**: if setup complete → `auth`, else open (lets the *first* user be created without a session).
- **`requireSelf`**: reads `c.Locals("session")`; missing → `401`; `session.UserID != :id` → `403` `"forbidden"`.
- **`setupComplete`**: reads `setup_complete` setting; `true` only when value == `"true"`; missing row → `false`.

- `GET /api/current_user` → loads `session.UserID`; not found → `401` `"user not found"`. → `200` `User`.
- `GET /api/users` → `200` `[]User`.
- `POST /api/users` (`createUserRequest`): `name` (`required`), `email` (`required,email`), `password` (`required,min=8`). Hashes password, generates Sqid. → `201` `User`.
- `GET /api/users/:id` → `200` `User`; not found → `404` `"user not found"`.
- `PUT /api/users/:id` (`updateUserRequest`): `name` (`required`), `email` (`required,email`), `password` (`omitempty,min=8`). Self-only. Re-hashes password only if present. → `200`.
- `DELETE /api/users/:id` → self-only. → `204`; not found → `404`.

---

## Settings — `/api/settings`

| Method + Path | Middleware | Auth |
|---|---|---|
| `GET /api/settings` | `auth` | always |
| `GET /api/settings/:key` | `setupGuard` | **open while `setup_complete != "true"`, protected after** |
| `PUT /api/settings/:key` | `auth` | always |
| `DELETE /api/settings/:key` | `auth` | always |

- **`setupGuard`**: if the `setup_complete` read errors OR value != `"true"` → open (`c.Next()`); else `auth`. (A DB error here results in *open* access, not `500`.)
- `GET /api/settings` → `200` `[]Setting`.
- `GET /api/settings/:key` → `200` `Setting`; not found → `404` `"setting not found"`.
- `PUT /api/settings/:key` (`upsertSettingRequest`: `value` `required`) → upsert → `200` `Setting`.
- `DELETE /api/settings/:key` → `204`; not found → `404`.

---

## First-run setup flow

Gated entirely by the `settings` row keyed `setup_complete` (seeded `"false"`):

1. **Probe (open):** `GET /api/settings/setup_complete` — `setupGuard` leaves this open while not `"true"`.
2. **Create first user (open):** `POST /api/users` via `conditionalAuth` — no session required while not complete.
3. **Mark complete:** `PUT /api/settings/setup_complete = "true"` (this PUT is `auth`-protected → requires logging in as the just-created user).
4. **Locked down:** once `"true"`, both `conditionalAuth` and `setupGuard` enforce `RequireAuth`.

There is no backend "is there already a user?" check — the gate is purely the setting value being the string `"true"`.

---

## Secrets — `/api/secrets` (all `auth`)

REST (`src/handlers/secrets.go`) over the encrypted `Store` (`src/secrets/store.go`).

### Secret names (allowlist, in code not config)
`AllowedNames = ["anthropic_api_key", "zernio_api_key"]`. Any other `:name` → `ErrUnknownName` → **`400` `"unknown secret name"`** (deliberately distinct from `404`).

### Plaintext is write-only
Plaintext enters **only** via `PUT` bodies and never leaves. `GET`/`List` return metadata only; the structured `logRequest` line records method/path/name/status/duration but never values.

### `secrets.Metadata` shape
`name`, `created_at`, `updated_at`, `kek_version` (int), `algorithm` (string), `decryptable` (bool — computed by attempting a decrypt; `true` confirms the loaded KEK still matches).

### Endpoints
- `GET /api/secrets` → `200` `[]Metadata` (nil → `[]`).
- `GET /api/secrets/:name` → `200` `Metadata`. Errors via `mapSecretError`: `ErrUnknownName`→`400`, `ErrNotFound`→`404` `"secret not found"`, else `500`.
- `PUT /api/secrets/:name` (`secretPutRequest`: `value`). Bad body → `400` `"invalid request body"` (doesn't echo body). `store.Set` runs `validatePlaintext` (non-empty; ≤4096 bytes; no ASCII control chars), encrypts, upserts, **notifies subscribers asynchronously** (Genkit/Zernio pick up rotated keys without restart). → **`201`** on fresh insert, **`200`** on replace (preserves `created_at`).
- `DELETE /api/secrets/:name` → `204`; non-allowlisted → `400`; absent → `404`. Notifies subscribers; dependent features degrade at next call.

---

## Health — `GET /api/health` (public)

1. `db.PingContext`. Failure → `503` `{"status":"unhealthy","error":"..."}`.
2. For each `secrets.AllowedNames`, `GetMetadata`:
   - `ErrNotFound` → `{present:false, decryptable:false}` (no degrade).
   - other error → `{present:true, decryptable:false}` + overall `"degraded"`.
   - success → `{present:true, decryptable: meta.Decryptable}`; `!Decryptable` → `"degraded"`.
3. Overall `status` is `"ok"` unless degraded. A present-but-undecryptable secret degrades but does **not** fail the endpoint.

Response e.g.: `{"status":"ok"|"degraded","secrets":{"anthropic_api_key":{"present":..,"decryptable":..},"zernio_api_key":{...}}}`. Codes: `200` (healthy/degraded), `503` (DB down).

---

## Shared validation helper (`src/handlers/validate.go`)

Package-level `validate = validator.New()`. `validationError(err)` flattens
`validator.ValidationErrors` into a semicolon-joined string used as the `400` body
(e.g. `"email must be a valid email address"`, `"<field> is required"`). Non-validation
errors are returned unchanged.

---

## Envelope encryption (secrets at rest)

`src/crypto/envelope/{envelope,kek}.go`; bootstrap `src/secrets/bootstrap.go`.

Two-layer **AES-256-GCM** envelope encryption:

- **KEK (Key Encryption Key):** single 32-byte key on a mounted volume, never in the DB. `LoadOrCreateKEK(path)`: missing → generate via `crypto/rand`, write `O_EXCL` mode `0600` (refuses overwrite); present → verify length == 32. Boot log records `"generated"` vs `"loaded"`. `InitCipher` loads `<kekDir>/kek.v1` (`KEKFilename="kek.v1"`; `.v1` is the rotation seam) into a process-lifetime `Cipher`. **Any KEK file error fails boot.**
- **Per-secret DEK:** every `Encrypt` draws a fresh 32-byte DEK + two fresh 96-bit nonces. Plaintext sealed under DEK; DEK wrapped under KEK. No nonce reuse by construction.
- **Persisted row fields:** `ciphertext`, `nonce`, `wrapped_dek`, `dek_nonce`, `algorithm` (`"AES-256-GCM"`), `kek_version` (`1`). All required to decrypt.
- **Decrypt** unwraps DEK with KEK, decrypts payload. GCM auth failures (tampered ciphertext, tampered wrapped DEK, or wrong KEK) collapse into one `ErrAuthFailed` (no layer disambiguation). Structural problems → `ErrMalformed`; unknown algorithm → `ErrAlgorithmUnsupported`.
- **Store guarantees:** allowlist, redaction-safe errors (`redactError` strips plaintext; `validatePlaintext` messages carry only lengths/offsets), and a subscription hook so consumers rebuild on rotation.

### Boot-time env → DB seeding (`MigrateFromEnv`)

For each allowlisted `(name, envValue)`:
- DB present + env set → "DB wins, ignoring env".
- DB present + env empty → no-op.
- DB missing + env set → encrypt + insert ("migrated").
- DB missing + env empty → no-op (feature degrades at call time).

**DB always wins over env**; a missing secret never fails boot. `LogBootSummary` logs counts only (no values/fingerprints).
