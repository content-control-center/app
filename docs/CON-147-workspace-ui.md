# CON-147 — Workspaces UI (front-end prototype)

Reference notes on the `feature/workspace` branch in the **ui** repo
(`git@github.com:ogen-app/ui.git`). Captured 2026-08-11 from branch head
`6b711ab` (2026-07-31), merge-base `8b4e247` on `main`. ~4,000 lines, 5 commits.

> Access note: SSH to github.com is currently rejected for this machine
> (`Permission denied (publickey)`); fetch the branch over HTTPS with the `gh`
> token instead, e.g.
> `git fetch https://github.com/ogen-app/ui.git feature/workspace:<local>`.

## TL;DR

A **front-end-only prototype**. Every workspace endpoint is served in dev by
**Mock Service Worker (MSW)** stubs — nothing touches the Go API yet. The real
deliverable to the backend is `docs/workspace-api.md` **+** the MSW handlers,
written as an executable request/response spec. Aligned to **CON-147**, with
deliberate divergences documented (see §7 of that doc).

The invitation **accept flow is intentionally not built** — it's the half that
touches unauthenticated routes and mail; the prototype stops at that boundary.

## Core model

- Membership becomes **many-to-many**; the **session identifies the account**,
  and the active workspace is resolved **per request from an `X-Workspace-Id`
  header**, falling back to the session's default workspace when the header is
  absent. The tenant boundary itself is unchanged and still fail-closed — only
  *which* tenant a request scopes to is now per-request.
- This is what lets two browser tabs sit in two workspaces at once (one cookie,
  different headers). `POST /api/workspaces/:id/switch` repoints only the
  session's **default** workspace — where a fresh tab or the next login seeds —
  and does **not** rescope open tabs. (The prototype originally bound the active
  workspace to the session with a switch + full reload; the shipped API
  overrides that with the header model, which the UI must adopt.)
- Roles: `owner | member` (shipped, inherited from CON-26). The prototype also
  modeled `admin` and `viewer`, but the backend supports **owner and member
  only** — so both `admin` and `viewer` are **prototype-only**. Multiple owners
  allowed; invariant is **≥1 owner** (server answers `409`; UI counts owners to
  grey controls out first).
- RBAC collapses to **two rank rules** in `src/types/workspace.ts`
  (`canActOnMember`, `canGrantRole`, `grantableRoles`): act on someone *below*
  your rank; grant a role *at or below* your own. Owners act on each other as
  peers (the one exception, so an owner can step down).
- No `timezone` field — everything is UTC until CON-94; settings shows `UTC` as
  read-only text.

## Key files (ui repo)

| Area | Path |
|---|---|
| Proposed API + rationale | `docs/workspace-api.md` |
| Types + RBAC helpers | `src/types/workspace.ts` |
| API client | `src/services/api/workspaces.ts` |
| React Query hooks | `src/hooks/useWorkspaces.ts` |
| Switcher page (full screen, no chrome) | `src/routes/workspaces/page.tsx` + `index.tsx` |
| Switch overlay | `src/components/workspace-settings/WorkspaceSwitchOverlay.tsx` |
| Settings page assembly | `src/routes/_authenticated/workspace-settings/index.tsx` |
| Workspace settings card | `src/components/workspace-settings/WorkspaceSection.tsx` |
| People + invitations | `src/components/workspace-settings/PeopleSection.tsx` |
| Create / Delete | `.../CreateWorkspaceDialog.tsx`, `.../DeleteWorkspaceCard.tsx` |
| Sidebar integration | `src/components/layout/AppSidebar.tsx` |
| Identity marks (shared) | `src/lib/identity.ts`, `src/components/layout/WorkspaceMark.tsx` |
| MSW stub backend | `src/mocks/` (`browser.ts`, `handlers.ts`, `db.ts`) |
| Boot wiring | `src/main.tsx` |

## UI surfaces built

1. **Workspace switcher** — full page at `/workspaces`, placed **outside
   `_authenticated`** so it renders with no sidebar (the sidebar belongs to the
   workspace being left). Cards show "Role · N members" and mark the current
   one.
2. **`useSwitchWorkspace`** — blunt on purpose: `queryClient.clear()` + full
   `window.location.assign('/')`, because every cached entry belongs to the old
   workspace (one missed key = another client's content on screen). 1500ms
   floor; the mutation promise never resolves so the overlay doesn't flicker off
   before the reload.
3. **Sidebar** (biggest diff) — footer shows the **workspace's identity mark**
   (not the user avatar) and workspace name (not email). Account dropdown gains
   Profile / Help / current-workspace-row (name + role) / "Create or switch" /
   Log out.
4. **Workspace Settings** assembles `WorkspaceSection` (name, read-only slug,
   read-only UTC) + `PeopleSection` + platforms + `DeleteWorkspaceCard`.
5. **PeopleSection** — members and pending invitations as one list; role selects
   grey out per rank rules; last owner locked; **resend = re-invite** (POST is
   idempotent per email, so no separate resend endpoint).
6. **Create / Delete** — create offers "Create only" vs "Create and switch";
   delete is owner-only, type-name-to-confirm, copy states soft-delete is not a
   self-serve undo.
7. **Identity marks** — generalizes the old `campaignColor.ts` into a shared
   FNV-1a hash → 7-hue palette + abbreviation, now used for both campaigns and
   workspaces (square mark for workspaces vs round avatar for people).

## Stub fidelity

`src/mocks/handlers.ts` calls the **same** `canActOnMember` / `canGrantRole`
functions the UI uses, returns real `403/409/404` with messages, enforces the
last-owner invariant and idempotent-invite semantics. Seeded with two
workspaces (one owned, one where you're admin).

- Toggle off: `VITE_STUB_WORKSPACES=false` in `.env.local` (workspace calls then
  404 against the real API).
- Reset seed: `window.__resetWorkspaceStubs()`.
- `main.tsx` boots the worker before the first request, guarded by
  `import.meta.env.DEV` so MSW is tree-shaken from production.
- Removal when real endpoints land: delete `src/mocks/` and the `startStubs()`
  call in `src/main.tsx`.

## What the backend still owes (from `docs/workspace-api.md` §6)

1. Session-vs-header — is one-workspace-per-session acceptable for v1?
2. Accept flow — token format, expiry, signup-with-token creating account +
   membership in one transaction.
3. Zernio profile per workspace — confirm CON-102 bootstrap reuse on create; what
   happens to the profile on delete.
4. Can a user create workspaces freely (does it become the billing unit)?
5. Cross-workspace user identity — one `users` row per email (assumed) vs
   per-workspace.
6. Timezone rollout (CON-94) — deliberately out of scope for now.

## Divergences from CON-147 §10 (all toward fewer routes/states)

- **+`viewer`** role (read-only agency client).
- **No resend endpoint** — `POST /invitations` idempotent per email.
- List shape **+`member_count`, `is_active`** so the switcher avoids an N+1.
- Delete = **soft-delete, no self-serve restore** (copy says so).
- `switch` (not `activate`), `PATCH` (not `PUT`) — adopted.
