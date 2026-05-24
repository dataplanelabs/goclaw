# Skill conventions for CLI-wrapping skills

> Audience: anyone authoring goclaw skills that wrap external CLI tools (gh,
> kubectl, codex, gcplane, …). Pairs with the
> [secure_cli infra](../internal/tools/credentialed_exec.go) and the
> [`/v1/system/cli-versions`](../internal/http/secure_cli.go) endpoint.

## When to use which exec path

```
Skill needs to invoke an external CLI?
├── Yes — auth credential is scope-restricted at the credential layer
│   (e.g. a read-only GH_TOKEN, a service-account JSON limited to one
│   project)
│        → Option A is acceptable. Inject the credential via SOPS-encrypted
│          env var. Document the auth limit in SKILL.md. gh-read is the
│          reference (`_system/skills/gh-read/SKILL.md`).
└── Yes — credential is NOT scope-restricted (writes, cluster ops, codex,
    anything where the LLM could turn the credential into damage)
         → Use Option B: `secure_cli_run` agent tool with `secure_cli_binaries`
           registry + per-agent grants. Enforcement is at the GoClaw side
           (shell-operator detection, deny_args regex, env scrub) — not the
           SKILL.md honor system.
```

## Option B — secure_cli_run usage

Once your binary is registered and granted, the skill's SKILL.md tells the
LLM to invoke the `secure_cli_run` agent tool:

```
To look up a PR, invoke `secure_cli_run` with:
  binary: "gh"
  args:   ["pr", "view", "<N>", "--repo", "owner/repo", "--json", "title,state,body"]
```

The tool returns `{stdout, stderr, exit_code}` through the same `Result`
envelope as the `exec` tool. Credentials are injected per-grant; nothing
ever leaks into the LLM context.

## Registering a binary

A binary lives in the `secure_cli_binaries` table (tenant-scoped):

| field            | meaning                                                                |
|------------------|------------------------------------------------------------------------|
| `binary_name`    | lookup key (lowercased), e.g. `gh`                                     |
| `binary_path`    | optional pinned absolute path; otherwise resolved via `$PATH`          |
| `encrypted_env`  | AES-256-GCM blob of `{KEY: value}` injected at exec time               |
| `deny_args`      | JSON array of regex patterns — args matching any are rejected pre-exec |
| `deny_verbose`   | JSON array — per-arg start-anchored match (blocks `-v` not `--version`)|
| `timeout_seconds`| default 30; overridable per grant                                      |
| `tips`           | string injected into TOOLS.md context for the LLM                      |
| `is_global`      | true → open to all agents in the tenant (no per-grant required)        |
| `enabled`        | toggle without delete                                                  |
| `version`        | free-form semver-shaped string; cross-checked by `gcplane validate`    |

Seed via `gcplane apply` with a `kind: SecureCLI` resource, or via the
HTTP CRUD at `POST /v1/cli-credentials`.

## Per-agent grants

`secure_cli_binaries.is_global = false` binaries require a per-agent grant in
`secure_cli_agent_grants`:

- `deny_args`/`deny_verbose`: per-agent overrides (defense-in-depth on top of
  binary defaults)
- `timeout_seconds`: per-agent override
- `encrypted_env`: per-agent env override (fully replaces binary env when
  non-empty — useful when different agents need different scopes of the same
  credential)
- `enabled`: per-grant toggle

## `requires.cli` frontmatter

Skills that need a minimum CLI version declare it in SKILL.md frontmatter:

```yaml
---
name: my-skill
requires:
  cli:
    gh: ">=2.50"
    kubectl: ">=1.30"
---
```

ONLY `">=X.Y"` syntax is supported in the current release. The following are
explicitly REJECTED at `gcplane validate` time with the error
`unsupported constraint shape %q; only >=X.Y supported in this release`:

- npm-style: `^X.Y`, `~X.Y`
- ranges: `">=X.Y, <Z"`
- exact-pinning: `"X.Y"` (no operator)

`gcplane apply` fetches `/v1/system/cli-versions` (tenant-scoped) and
compares each constrained binary against the installed version. Mismatch =
apply fails fast before upload. Network failures degrade to a warning.

## Cluster ops scope guard (B5 preview)

Per-grant `encrypted_env` enables per-cluster scoping. Recommended pattern
for kubectl skills:

- `kubectl-everest-read`: grant with `KUBECONFIG=<everest-readonly>`,
  `deny_args` blocks `apply|delete|patch|edit|drain`
- `kubectl-everest-admin-dryrun`: grant with full kubeconfig, `deny_args`
  requires `--dry-run=server|client`
- Skill picks the right binding by name

DO NOT register a single `kubectl` binary with one admin kubeconfig and
expect deny_args to be enough — separate grants give defense-in-depth.

## `requires_human_approval` (planned, not implemented)

A future grant flag will gate certain calls behind a human-ack flow (Slack /
goclaw web). Currently NOT implemented — track in
`plans/260524-0949-system-skills/brainstorm-b2-cli-wrapping-conventions.md`.

## Per-tenant OAuth credentials (gws, Pancake / Misa / Zalo OA later)

Skills that wrap APIs with PER-OPERATOR credentials (each user has their own
Google account, gmail, etc.) follow the OAuth pattern shipped in B3-01.

### How the flow works

1. **Operator provisions credentials via web UI** — Settings → Integrations
   → "Connect Google Account" → OAuth popup → backend writes encrypted
   refresh_token to `secure_cli_user_credentials(tenant_id, binary_id,
   user_id)` keyed by the OPERATOR's user_id.
2. **Backend refresh worker** (cron, 24h default) keeps access tokens fresh
   via `internal/oauth/refresh_worker.go`. On token revocation, clears the
   row + logs `goclaw.alert.oauth_revoked` event for the UI badge.
3. **Skills invoke `secure_cli_run`** as usual; the `(*ExecTool).executeCredentialed`
   path's `mergeCredentialedEnv` automatically overlays the user's env vars
   (refresh_token) on top of the binary's shared env (client_id + secret).
4. **Skill author UX:** declare `requires.cli: { <binary>: ">=X.Y" }` in
   SKILL.md frontmatter. Instruct the LLM to ask the user to "Connect <X>
   in Settings → Integrations" if `secure_cli_run` returns "no grant".

### `secure_cli_binaries` shape for OAuth binaries

```yaml
binary_name: gws                    # the binary the skill invokes
is_global: false                    # REQUIRED — forces per-grant lookup so
                                    # LookupByBinary resolves user-specific creds
encrypted_env:
  GWS_CLIENT_ID: "${GOOGLE_CLIENT_ID}"      # shared across all tenants
  GWS_CLIENT_SECRET: "${GOOGLE_CLIENT_SECRET}"
deny_args:
  - '^(gmail|calendar|drive)\s+(send|create|update|delete|patch)'  # write subcommands
version: "0.1.0"                    # gcplane validate cross-checks via /v1/system/cli-versions
```

The per-user **refresh_token** is NOT in the binary's `encrypted_env` — it
lives in `secure_cli_user_credentials.encrypted_env` per-user (written by
the OAuth callback). The two are merged at exec time per the env hierarchy:
`binary env → grant overrides → user env overrides`.

### HTTP surface (B3-01 Phase 2)

| Endpoint | Auth | Purpose |
|---|---|---|
| `POST /v1/integrations/google/start` | RoleViewer | Returns `{auth_url, state}` for the popup |
| `GET /v1/auth/google/callback` | NONE (state-token auth) | Exchanges code, writes credential, redirects |
| `GET /v1/integrations/me` | RoleViewer | Lists caller's connected integrations |
| `DELETE /v1/integrations/{binary_name}` | RoleViewer | Deletes caller's own credential row |
| `GET /v1/healthz/oauth-refresh-worker` | NONE | Liveness probe for the refresh worker |

### Env vars (operator-set)

| Variable | Purpose |
|---|---|
| `GOCLAW_GOOGLE_CLIENT_ID` | OAuth client ID from Google Cloud Console |
| `GOCLAW_GOOGLE_CLIENT_SECRET` | OAuth client secret |
| `GOCLAW_GOOGLE_REDIRECT_URL` | Callback URL (e.g. `https://goclaw.example/v1/auth/google/callback`) |
| `GOCLAW_UI_BASE_URL` | Web UI base — popup redirect target after callback success |
| `GOCLAW_OAUTH_REFRESH_TICK_SECONDS` | Refresh worker tick (default 86400 = 24h) |
| `GOCLAW_OAUTH_REFRESH_THRESHOLD_SECONDS` | Refresh window (default 604800 = 7d) |

Endpoints return `MsgOAuthNotConfigured` (503) when `CLIENT_ID`/`SECRET` are
unset, so safe to deploy unconfigured.

### B-conv-2 retrofit candidate

`gh-read` can adopt this pattern in the future when we want per-operator
GitHub PATs. Currently uses a global read-only PAT (Option A carveout per
B2 brainstorm). The pattern generalizes to Pancake (per-tenant API keys —
different shape from OAuth) and Zalo OA (webhooks + keys — also different)
but the **storage** (secure_cli_user_credentials) and **invocation**
(secure_cli_run) layers are reusable.

## CI lint

`goclaw-config/.github/workflows/skill-lint.yaml` rejects PRs that introduce
bare `gh|kubectl|codex|gcplane` invocations at line start in
`*/skills/*/scripts/*`. The lint is scoped to script files — SKILL.md docs
(which legitimately mention bare command examples) are excluded.

To suppress: don't. Use `secure_cli_run` instead. If you have a genuine
exception (e.g. a setup script that must shell out), keep it OUT of the
`scripts/` directory.

## gh-read special case

`_system/skills/gh-read/` uses Option A (bare exec via `GH_TOKEN` env var)
because the PAT is read-only at the credential layer — there is no
write-level damage the LLM can do regardless of arguments. This is the only
acceptable exception today. The principle:

> If the auth credential itself is scope-restricted (read-only, single
> resource), Option A is acceptable. Otherwise, Option B.

## References

- `internal/tools/credentialed_exec.go` — the 3-gate enforcement (shell ops,
  deny_args, env scrub)
- `internal/tools/secure_cli_run.go` — the agent-tool wrapper
- `internal/http/secure_cli.go` — `/v1/system/cli-versions` endpoint
- `gcplane internal/manifest/validate.go` — `CrossCheckRequiresCli`
- `plans/260524-0949-system-skills/brainstorm-b2-cli-wrapping-conventions.md`
  — the design brief that produced Option B
