---
name: email-templates
description: Understand and modify Ogen's DB-stored email templates (CON-154) — the embedded-default → seed-on-boot → DB-wins lifecycle, the custom [[ ]] Go-template delimiters that coexist with Maizzle/Tailwind {{ }}, and how to add a new template or wire a new lifecycle email. Use when the user asks to add/edit an email, change email copy, add a welcome/drip/reset template, adjust the drip schedule, or asks how email rendering/seeding works.
tools: Read, Edit, Write, Glob, Grep, Bash
---

# Email Templates Skill

Ogen sends transactional + marketing mail via the CON-154 email subsystem
(Resend). Templates are **stored in the `email_templates` DB table** and are
authoritative at runtime — copy can be edited without a redeploy. The Go binary
carries a **default copy** of every template that is used only to *seed an
absent key* on boot. Read the mechanism below before touching anything.

Canonical files (`src/email/templates/`):

- `templates.go` — key constants, the `Data` struct, `//go:embed defaults/*.tmpl`,
  `defaultSpecs` (key → kind + subject), `Defaults()`, `SeedDefaults()`.
- `render.go` — `Render()`, the `LeftDelim`/`RightDelim` (`[[` / `]]`) constants.
- `defaults/*.tmpl` — the embedded default bodies (`<key>.html.tmpl` + `.txt.tmpl`).

Related: `src/repository/email_templates.go` (`GetByKey`, `InsertIfAbsent`),
`src/server/email.go` (`initEmail` → `SeedDefaults`), `src/jobs/queues/send_email.go`
(loads `GetByKey` + `Render` at send time), migration `20260803000001_email`.

## Mechanism (read first)

Three stages — **author → seed → render** — plus one delimiter trick.

### 1. DB is the source of truth

The `email_templates` table (`key` PK, `subject`, `html`, `text`, `kind`,
`version`, `updated_at`) is authoritative at runtime. This is deliberate
(CON-154 D3): copy is editable at runtime (DB/ops in v1; an admin API/editor is
a deferred follow-up) rather than compiled into the binary.

### 2. Seed on boot — "embedded defaults, DB wins"

`Defaults()` reads each embedded `defaults/<key>.html.tmpl` + `.txt.tmpl` pair
and pairs it with an in-code subject + kind (`defaultSpecs`). On **every** boot,
`initEmail` calls `SeedDefaults`, which loops the defaults through
`repo.InsertIfAbsent`:

```sql
INSERT INTO email_templates (...) VALUES (...) ON CONFLICT (key) DO NOTHING
```

`DO NOTHING` is the whole mechanism:

- **Fresh DB** → key absent → inserted (working copy exists immediately).
- **Existing / edited DB** → key present → conflict → **no-op**.

So "DB wins thereafter" means the embedded default is only ever used to seed an
*absent* key. Once a row exists — seeded once, or edited since — the binary's
default for that key is inert; a newer default shipped in a later build does
**not** overwrite an operator's edit. `SeedDefaults` returns the count actually
inserted (0 on a redeploy, N on first boot). Idempotent, safe every boot —
the same env→DB, DB-wins idiom the secrets store uses (`MigrateFromEnv`).

```
  binary:  defaults/welcome.html.tmpl ─┐
                                        │ boot: InsertIfAbsent (ON CONFLICT DO NOTHING)
                                        ▼
  DB:      email_templates[welcome] ◀── authoritative; edits persist across redeploys
                                        │ send: GetByKey(key) → Render(row, data)
                                        ▼
                                    subject / html / text
```

### 3. Render at send time

The `send_email` job loads the current row with `GetByKey(templateKey)` and
calls `Render(row, data)`. Because it reads the DB **per send** (a background
worker, not a request path — a DB read is cheap), a copy edit takes effect on
the **next** send with no redeploy and no re-enqueue — including the
already-scheduled day-5/day-7 drip jobs. `Render` fills a single `Data` struct
(`Name`, `WorkspaceName`, `AppURL`, `UnsubscribeURL`); `subject`/`text` go
through `text/template`, `html` through `html/template` (contextual
auto-escaping).

### 4. The `[[ ]]` delimiter trick — why it exists

HTML is authored in **Maizzle** (`maizzle.com`), a **build-time** tool entirely
outside the Go app. `maizzle build` compiles Tailwind-based source into inlined,
email-client-safe static HTML; that compiled HTML is what you commit as the seed
file and store in the DB. **The Go app never runs Maizzle.**

The collision: Maizzle/Tailwind expressions use `{{ }}`, and so does Go's
`html/template` by default. So Go templates are parsed with **custom
delimiters** (`render.go`):

```go
const (
	LeftDelim  = "[["
	RightDelim = "]]"
)
// template.New(name).Delims(LeftDelim, RightDelim).Parse(src)
```

Contract for every stored body:

- `[[ .Name ]]` → a **runtime variable**, interpolated by Go at send time.
- `{{ anything }}` → **literal text**, passed through untouched (it's Maizzle's).

`TestRenderCustomDelimsPassThroughBraces` locks this: `Hi [[ .Name ]] {{ keep }}`
with `Name=Zed` → `Hi Zed {{ keep }}`.

---

## Task: edit an email's copy (no code change)

v1 has **no template management API** — edit the DB row directly:

```sql
UPDATE email_templates
SET subject = '...', html = '...', text = '...', version = version + 1, updated_at = now()
WHERE key = 'welcome';
```

The next send renders the new copy (no redeploy, no re-enqueue). Keep the
`[[ .Field ]]` variables intact and only reference fields on the `Data` struct.
Note: re-seeding on the next boot will **not** revert your edit (DO NOTHING).
To also change the shipped default, edit the embedded `.tmpl` file too — but that
only affects databases where the key is still absent.

## Task: add a new template

1. **Author** the HTML in Maizzle (build-time, outside this repo/app). Use
   `[[ .Field ]]` for runtime variables and let Tailwind/Maizzle `{{ }}` be.
   Write a plaintext counterpart too.
2. **Commit the compiled output** as `src/email/templates/defaults/<key>.html.tmpl`
   and `<key>.txt.tmpl`.
3. **Register it** in `templates.go`:
   - Add a key const (e.g. `KeyPasswordReset = "password_reset"`).
   - Add a `defaultSpec{Key, Kind, Subject}` entry (kind = `EmailKindTransactional`
     or `EmailKindMarketing`; subject may itself use `[[ .Field ]]`).
4. **Data fields**: if the template needs a variable not already on `Data`
   (`Name`, `WorkspaceName`, `AppURL`, `UnsubscribeURL`), add it to the `Data`
   struct and populate it in `send_email.go`'s `Process` where the `Data` is
   built. (A template with bespoke needs can justify its own data struct +
   render path — the current `Data` is the shared shape for the v1 set.)
5. **Kind semantics**: `marketing` requires an unsubscribe affordance and is
   suppression-gated (blocked by any suppression); `transactional` is sent even
   to marketing-unsubscribed addresses (blocked only by an `all`-scope
   suppression — hard bounce/complaint). The job builds `UnsubscribeURL` +
   `List-Unsubscribe` headers only for `marketing`.

## Task: trigger a new lifecycle email

Rendering a template is only half — something must **enqueue** a `send_email`
job. See `src/jobs/queues/river.go`:

- `EnqueueWelcomeEmailTx` / `EnqueueDripTx` are the models. Add an
  `Enqueue<X>Tx(ctx, tx, userID, tenantID)` that inserts a `SendEmailTask`
  (`TemplateKey`, `EmailKind`, `IdempotencyKey`) — transactional (`InsertTx`)
  so the mail commits atomically with the triggering DB write.
- Immediate mail: default `ScheduledAt` (now). Delayed/drip: pass
  `river.InsertOpts{ScheduledAt: now.Add(offset)}` (see `dripSchedule`).
- `IdempotencyKey` is `"<key>:<userID>"` — it's the Resend `Idempotency-Key` +
  the `email_logs` partial-unique backstop, so a retried enqueue can't double-send.
- The drip cadence (day 2/5/7) is the in-code `dripSchedule` table in `river.go`
  — one edit reschedules the whole sequence.

## Gotchas

1. **Never overwrite on re-seed.** Seeding is `InsertIfAbsent` (`ON CONFLICT DO
   NOTHING`) on purpose. Don't switch it to an upsert — that would clobber
   operator edits on every deploy.
2. **Use `[[ ]]`, not `{{ }}`, for variables.** `{{ }}` is reserved for
   Maizzle/Tailwind in the compiled HTML and passes through as literal text.
   A `{{ .Name }}` in a stored body will NOT interpolate.
3. **Only reference fields that exist on `Data`.** `html/template` execute errors
   are terminal in the send job (logged `failed`, no retry) — a typo'd field
   silently kills that send. There's a render unit test; add a case for a new
   template.
4. **Subjects are templates too.** They render with `text/template` and the same
   `[[ ]]` delimiters (e.g. `Welcome to Ogen, [[ .Name ]]`).
5. **HTML auto-escapes; plaintext does not.** Bodies go through `html/template`
   (HTML) and `text/template` (subject + text). Don't hand-escape.
6. **Editing a row affects in-flight drips.** Because the row is read per send,
   a template edit changes already-scheduled day-5/day-7 sends too. Intended, but
   know it.

## Checklist (adding a template)

- Maizzle-compiled `defaults/<key>.html.tmpl` + `<key>.txt.tmpl` committed.
- `templates.go`: key const + `defaultSpec` (kind + subject) added.
- New `Data` fields (if any) added + populated in `send_email.go`.
- Enqueue path added to `river.go` (+ consumer interface / call site) if it's a
  new trigger.
- Render unit test case for the new template; `go test ./src/email/... ./src/jobs/...`
  green (a real seed is exercised by any `pgtest`-backed suite).
