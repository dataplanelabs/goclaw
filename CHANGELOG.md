# Changelog

All notable changes to GoClaw are documented here. For full documentation, see [docs.goclaw.sh](https://docs.goclaw.sh).

## Unreleased

### Fixed

- **Zalo native renderer — drop list styles + fix fragment-bold** — on
  `zalo_personal` with `enable_native_styles=true`, Zalo mobile dumps `lst_1`
  / `lst_2` spans as raw `<list>` / `<number index="N">…</number>` XML in the
  message body. Renderer no longer emits list styles at all; `- item` and
  `1. item` lines pass through as literal text (visible dash / number).
  Companion fix: `**…**` glued to a letter/digit on either side
  (e.g. `Dễ ove**rtrain nếu khôn**g`) drops the markers — no Style emitted —
  so the reader sees the words plainly instead of broken partial-word bold.
  Triple-emphasis `***x***` left intact via the adjacent-`*` guard.
  Vietnamese-diacritic-safe word boundary via `unicode.IsLetter` over runes.
  Prompt addendum (`ZALO_PERSONAL_ADDENDUM.md`) updated to tell the LLM lists
  are literal text on this channel.

- **Orphaned traces can now be Stopped and Retried** — when an agent process
  died mid-run (panic, OOM, rolling update), the trace previously remained stuck
  at `status='running'` forever; Stop returned "run already finished" (in-memory
  `activeRuns` map was empty after restart), Retry rejected because status wasn't
  error/cancelled. The abort handler now falls through to a tenant-scoped DB
  lookup on map-miss and force-marks the trace as `cancelled` so the existing
  Retry path works. New `AbortResult.Orphaned` field + `abortOrphan` FE toast
  distinguish the case. Tenant filter prevents cross-tenant clobber. Single-pod
  assumption documented in code; multi-pod safety needs ownership protocol
  (Option 3 deferred).

- **`create_image` and `read_image` tools respect a configurable timeout** —
  env `CREATE_IMAGE_TIMEOUT_SEC` and `READ_IMAGE_TIMEOUT_SEC` (default 180s)
  bound the tools' HTTP/IO calls so a hung provider fails loud within the
  window instead of producing new zombies. Shared helper at
  `internal/tools/timeouts.go`. Companion to the orphan-trace fix above.

- **Agent answers no longer hard-code UTC for time/date questions** — system
  prompt's `Current date:` line was always emitting `(UTC)` regardless of any
  configured timezone, forcing the model to do error-prone TZ arithmetic on
  the user's side and producing wrong day-of-week answers (e.g. saying "Sunday"
  for a Monday request). Adds channel-instance-level timezone
  (`channel_instances.config.timezone` JSONB key) with fallback chain:
  channel config → workspace `cron.default_timezone` → UTC. Resolved once per
  turn via `RunContext.UserTimezone` so the per-iteration `buildMessages`
  path stays DB-free. Bootstrap USER.md pre-fill (`internal/bootstrap/seed_store.go`)
  shares the same resolution source. Time format also gains `HH:MM` so the
  model no longer needs to guess the time of day. Verified against trace
  `019e5d02-aa7c-748f-8d3a-b0c9c8b35f8f` on the cluster.
  No schema migration — channel_instances.config is already JSONB. Backfill
  per-channel via:
  ```sql
  UPDATE channel_instances
  SET config = config || '{"timezone":"Asia/Ho_Chi_Minh"}'::jsonb
  WHERE name = '<channel-name>';
  ```

### Added

- **Skill ownership attribution + force_imperative overwrite gate** — `POST /v1/skills/upload`
  now accepts `source` (`unknown` | `cli` | `gcplane`) and `force_imperative` form fields.
  Skills stamped `source=gcplane` (uploaded via the declarative pipeline) refuse to be
  overwritten from a non-gcplane source with **HTTP 409 `managed_skill_overwrite`**,
  unless `force_imperative=true` is set — which is audit-logged both via
  `slog.Warn("security.skill_force_imperative_overwrite", …)` AND via the persistent
  msgBus event `skill.force_overwrite` (so SIEM subscribers see it as a distinct
  security event, not a routine upload). `GET /v1/skills/{id}` response now also
  exposes `content_hash` (= existing `file_hash` column) for client-side dedup
  verification. New `skills.source` column (PG migration 000072, SQLite v41→v42).
  Tenant-scoped enforcement; 4 behavioral tests (refuse, allow-with-force,
  invalid-source × 5 cases, tenant-scoped). 3 new i18n keys across en/vi/zh.

- **General JPEG XL (.jxl) inbound support** — Zalo HD photos (and any other
  `.jxl` input) now decode transparently before reaching the LLM, since
  Anthropic / OpenAI / Gemini reject `image/jxl`. Two-layer fix:
  (1) `internal/agent/media_sanitize.go` blank-imports `github.com/gen2brain/jpegxl`
  (wazero-WASM libjxl, CGo-free) — self-registers a decoder for both raw codestream
  (`\xff\x0a`) and ISOBMFF container (`????JXL`) magic bytes; `SanitizeImage`
  decodes JXL → JPEG q=85 transparently.
  (2) `internal/channels/zalo/personal/handlers.go` adds `pickInboundImageURL` +
  `urlIsJXL` + reordered `attachMediaURLFields` to prefer non-`.jxl` CDN URLs
  when alternatives exist (skips a 150-400ms WASM decode). MIME plumbing
  (`mime_detect.go`, `media.go`, `store.go`) maps `.jxl ↔ image/jxl`.
  Tool-layer `read_image` re-encodes JXL via `imaging.Open` + `jpeg.Encode`.
  Defensive: `persistMedia` drops JXL/HEIC on `SanitizeImage` failure rather
  than shipping raw bytes that providers reject. Trigger trace `019e56f5`.

- **Skill agent manage grants** — Adds per-agent skill edit/delete grants with
  backend checks, HTTP/WS support, SQLite and PostgreSQL schema updates, and web
  dashboard controls for granting and revoking manage access.

- **Packages Update Flow (Phase 2a: pip + npm)** — closes #900 (Phase 2a). Extends
  Phase 1 update infrastructure to pip and npm package sources. `/v1/packages/updates`
  now returns mixed-source results with an `availability: {github, pip, npm}` map.
  Multi-source UI with per-source filter pills; unavailable sources (binary not on PATH
  or Lite edition) hidden automatically. apk deferred to Phase 2b.
  See `docs/packages-pip-npm.md` for command matrix, runbook, and min versions.

- **Packages Update Flow (Phase 1: GitHub binaries)** — closes #900. Proactive
  "N updates available" badge + per-row `[Update]` + `[Update All]` on the
  Runtime & Packages page. Backend endpoints under `/v1/packages/updates*`
  (master-scope). ETag-aware polling (304 responses don't burn rate limit),
  stale-while-revalidate cache, atomic two-phase `.bak` swap with rollback.
  Pre-release detection via regex + GitHub API flag; semver ordering via
  `golang.org/x/mod/semver`; non-semver tags use string-inequality fallback
  with downgrade protection. WebSocket events `package.update.*` for owner
  clients. See `docs/packages-github.md` § "Updating Installed Packages".

### Changed

- **ChatGPT Subscription (OAuth)** — default model and backend-owned model catalog
  now prefer `gpt-5.5`, with reasoning metadata and context-window defaults updated
  for provider-first model selection.

### Fixed

- **create_image: codex Responses-API native path now actually sends reference images**
  — supersedes the v3.16.12 fail-fast. `buildNativeImageRequestBody` appends
  `{"type":"input_image","image_url":"data:<mime>;base64,..."}` content parts
  when refs are present, switches the tool's `action` from `"generate"` to
  `"edit"`, and adds `input_fidelity: "high"` for face preservation (omitted
  for `gpt-image-1-mini` which rejects it). `callProvider` passes
  `params["reference_images"]` into `NativeImageRequest` so the bytes reach
  the model. gpt-image-2 finally sees the user's selfie.

- **create_image: gpt-image-2 rejects `input_fidelity` parameter** — v3.17.1 added
  `input_fidelity: "high"` for all gpt-image-1/1.5/2 when refs are present, but
  gpt-image-2 errors `400 "The model 'gpt-image-2' does not support the
  'input_fidelity' parameter"` (v2 auto-handles fidelity internally). Now gated
  via `supportsInputFidelity`: only gpt-image-1 and gpt-image-1.5 receive it.

- **create_image: revert DashScope `enable_interleave`** — v3.17.1 added
  `enable_interleave: true` to fix text-only, but that mode requires SSE
  streaming we don't implement (server returns `400 "stream=False is not
  supported"`). Reverted. DashScope text-only is still broken on wan2.6, but
  codex handles text-only successfully so the chain works in practice. Real
  DashScope refs/text-only fix needs SSE wiring (followup).

- **create_image: filter ref pool to user-uploaded images only** — PR #37 made
  `WithMediaImageRefs` history-aware via `collectRefsByKind`, but that helper
  collected MediaRefs from ALL message roles — including assistant messages
  carrying previously-generated images from `create_image`. When the LLM
  passed a stale ID, the auto-bind fallback picked the 4 most-recent IDs,
  which were the bot's own prior output images (`rainy-run-face-v5`, `v4`, etc),
  not the user's selfie. Result: the bot fed its hallucinated face back into
  the next generation. Added `collectUserUploadedRefs` that filters to
  `Role == "user"` messages so bot outputs can never be re-used as input refs.

- **create_image: image refs now span the whole session** — `WithMediaImageRefs`
  registered only the current turn's uploads (docs/audio/video already used a
  history-aware collector; image was the outlier). After the user sent a selfie
  then said "khong giong" with no new photo, the LLM recalled the prior ID but
  `MediaImageRefsFromCtx` returned empty → resolved to zero refs → text-only
  generation → random face. Now images use the same `collectRefsByKind` path as
  other media; any ID anywhere in the session resolves to its persisted file.

- **create_image: stale `reference_image_ids` silently dropped → random output** —
  when the LLM passed a UUID from a previous turn (common with chatty models),
  `resolveRefImageIDs` returned zero refs and the chain ran text-only, producing
  an image that ignored the user's photo. Two-step fix: (1) if requested IDs
  fail to resolve but the current turn DOES have attached images, auto-bind to
  those (LLM clearly intended a ref, just got the ID wrong); (2) if no current
  refs exist either, return a tool error telling the agent to ask the user to
  resend the image instead of silently fabricating.

- **create_image/create_audio/create_video: duplicate media delivery** — when the
  agent called `create_*` followed by `message(MEDIA:<path>)`, the file shipped
  twice (once via the auto-attached `result.Media`, once via the explicit message
  send). `message`'s self-send dedup guard checks `DeliveredMedia.IsDelivered`,
  but the `create_*` tools never marked their generated files. Now mirror the
  `filesystem_write(deliver=true)` pattern: call `dm.Mark(path)` after building
  the result.

- **Zalo Personal: chunked upload for images/files > 512KB** — the file-service
  rejects any single chunk over 512KB with inner error code 201
  (*"Dung lượng chunk upload không được vượt quá 512K"*). `UploadImage` and
  `UploadFile` were hard-coding `totalChunk: 1, chunkId: 1`, so 2K posters and
  other media > 512KB silently failed delivery to Zalo. Now both paths chunk
  by 512KB (`totalChunk = ceil(size/524288)`), keep `clientId` constant across
  chunks of one upload, re-encrypt `params` per chunk (chunkId changes), and
  take the final `photoId`/`fileId` from whichever chunk response carries it
  (matches zca-js `uploadAttachment.ts` protocol). Applies to DM and group.

### Changed

- **create_image: native 2K resolution on gpt-image-2** — `SizeFromAspectForModel`
  picks model-aware dims so gpt-image-2 generates at its native 2K bucket
  (1536×2048 for 3:4, 2048×1536 for 4:3, 2048×2048 for 1:1, 2304×1296 / 1296×2304
  for 16:9 / 9:16) — ~2× pixel quality vs the previous 1K bucket while staying
  within v2's 655k–8.3M total-pixel cap and 16-divisible rule. gpt-image-1.5
  snaps to its 3 canonical sizes (1024², 1024×1536, 1536×1024); unknown models
  fall through to the conservative 1K SizeFromAspect default.

### Fixed

- **create_image: 3:4 / 4:3 aspect rejected by codex (`divisible by 16`)** —
  `SizeFromAspect` returned `1024x1365` / `1365x1024` for 3:4 / 4:3; 1365 % 16 = 5
  so the codex native-image endpoint 400'd. Bumped to `1024x1360` / `1360x1024`
  (both dimensions 16-divisible, ratio 0.7529 ≈ 0.75).

- **create_image: DashScope (wan2.x) rejected string `messages.content`** —
  the multimodal-generation endpoint requires `content` as a list of parts;
  string form returned `400 InvalidParameter: Input should be a valid list:
  input.messages.0.content`. Now sends `[{"text": prompt}]`.

- **Zalo Personal media-with-caption attachments dropped (regression of #14)** —
  inbound `{title, href}` frames (image/video/audio/file with caption) were caught
  by the quote-reply text probe before `ParseAttachment` ran, so the agent received
  caption-only and `read_image`/media tools saw "No images available". Attachment
  detection now runs before the title probe; caption text is prepended to the
  `<media:*>` tag. Applies to DM and group paths.

- **Upstream critical security remediation** — hardens gateway no-token fallback,
  Feishu/Lark and Pancake webhooks, sandbox path/write handling, tenant-admin
  checks for mutable HTTP surfaces, and Lite hook schema migration verification.

- **SecureCLI runtime npm binaries** — binary discovery and credentialed exec now
  resolve tools installed under the GoClaw runtime directories, including
  `{runtimeDir}/npm-global/bin`, and support single-binary npm package aliases
  such as `openrouter-cli` exposing `orc`.

### Breaking Changes

- **Context pruning now opt-in.** Previously tool-result trimming ran by default
  for all providers; now requires explicit `contextPruning.mode: "cache-ttl"` in
  `config.agents.defaults` to enable. Matches upstream TS design and prevents
  silent prompt-cache invalidation on Anthropic.

  Migration — add to `config.json5`:
  ```json5
  agents: {
    defaults: {
      contextPruning: { mode: "cache-ttl" }
    }
  }
  ```

### New Features

- **Pancake private-reply (comment → DM).** Enables a one-time DM to commenters
  after the public reply. Stateless on GoClaw side — no DB dedup table, no
  in-memory state:
  - Config: `features.private_reply` (bool) + `private_reply_message` (text).
  - **Template variables** `{{commenter_name}}` and `{{post_title}}` with
    literal-replace semantics (pre-sanitizes `{{`/`}}` from var values to
    prevent var-in-var substitution).
  - Empty `private_reply_message` → English fallback constant.
  - **Dedup strategy**: webhook-level comment_id dedup (already in
    `comment_handler.go`) + Facebook's per-comment idempotent `private_replies`
    endpoint handle duplicates platform-side. No GoClaw state required.
  - No DB migration.

### Improvements

- **Context pruning cleanup.** Removed redundant Pass 0 (per-result 30% guard),
  deduplicated double prune call per iteration, added SanitizeHistory to
  PruneStage for broken tool_use/tool_result pair cleanup.
- **Context pruning config backfill (migration).** Agents with existing custom
  `context_pruning` config (e.g., `softTrimRatio`, `keepLastAssistants`) but
  missing a `mode` field get auto-backfilled with `mode: "cache-ttl"` to
  preserve their intent after the opt-in flip. Rows with NULL config stay
  NULL (new opt-in default applies). PG migration 51; SQLite schema v19.
- **Pancake channel metadata routing.** Whitelist in
  `internal/channels/routing_metadata.go` now preserves `post_id` and
  `display_name` across the inbound → outbound hop so the private-reply
  template variables survive the agent pipeline round-trip.

### Fixed

- **Skill grant tenant isolation.** Agent skill grants now validate both the
  skill and agent tenant scope before insert, revoke, grant listing, or
  can-manage checks. Visibility auto-promote/auto-demote updates are scoped to
  the calling tenant or system skills so one tenant cannot mutate another
  tenant's skill.

- **Agent provider switching.** Saving an agent after changing provider/model now
  handles cleared ChatGPT OAuth routing config without writing SQL NULL into
  NOT NULL JSON config columns.

## Project Status

### Implemented & Tested in Production

- **Agent management & configuration** — Create, update, delete agents via API and web dashboard. Agent types (`open` / `predefined`), agent routing, and lazy resolution all tested.
- **Telegram channel** — Full integration tested: message handling, streaming responses, rich formatting (HTML, tables, code blocks), reactions, media, chunked long messages.
- **Seed data & bootstrapping** — Auto-onboard, DB seeding, migration pipeline tested end-to-end.
- **User-scope & content files** — Per-user context files (`user_context_files`), agent-level context files (`agent_context_files`), virtual FS interceptors, per-user seeding (`SeedUserFiles`), and user-agent profile tracking all implemented and tested.
- **Core built-in tools** — File system tools (`read_file`, `write_file`, `edit_file`, `list_files`, `search`, `glob`), shell execution (`exec`), web tools (`web_search`, `web_fetch`), and session management tools tested in real agent loops.
- **Memory system** — Long-term memory with pgvector hybrid search (FTS + vector) implemented and tested with real conversations.
- **Agent loop** — Think-act-observe cycle, tool use, session history, auto-summarization, and subagent spawning tested in production.
- **WebSocket RPC protocol (v3)** — Connect handshake, chat streaming, event push all tested with web dashboard and integration tests.
- **Store layer (PostgreSQL)** — All PG stores (sessions, agents, providers, skills, cron, pairing, tracing, memory, teams) implemented and running.
- **Browser automation** — Rod/CDP integration for headless Chrome, tested in production agent workflows.
- **Lane-based scheduler** — Main/subagent/team/cron lane isolation with concurrent execution tested. Group chats support up to 3 concurrent agent runs per session with adaptive throttle and deferred session writes for history isolation.
- **Security hardening** — Rate limiting, prompt injection detection, CORS, shell deny patterns, SSRF protection, credential scrubbing all implemented and verified.
- **Web dashboard** — Channel management, agent management, pairing approval, traces & spans viewer, skills, MCP, cron, sessions, teams, and config pages all implemented and working.
- **Prompt caching** — Anthropic (explicit `cache_control`), OpenAI/MiniMax/OpenRouter (automatic). Cache metrics tracked in trace spans and displayed in web dashboard.
- **Agent delegation** — Inter-agent task delegation with permission links, sync/async modes, per-user restrictions, concurrency limits, and hybrid agent search. Tested in production.
- **Agent teams** — Team creation with lead/member roles, shared task board (create, claim, complete, search, blocked_by dependencies), team mailbox (send, broadcast, read). Tested in production.
- **Evaluate loop** — Generator-evaluator feedback cycles with configurable max rounds and pass criteria. Tested in production.
- **Delegation history** — Queryable audit trail of inter-agent delegations. Tested in production.
- **Skill system** — BM25 search, ZIP upload, SKILL.md parsing, and embedding hybrid search. Tested in production.
- **MCP integration** — stdio, SSE, and streamable-http transports with per-agent/per-user grants. Tested in production.
- **Cron scheduling** — `at`, `every`, and cron expression scheduling. Tested in production.
- **Docker sandbox** — Isolated code execution in containers. Tested in production.
- **Text-to-Speech** — OpenAI, ElevenLabs, Edge, MiniMax providers. Tested in production.
- **HTTP API** — `/v1/chat/completions`, `/v1/agents`, `/v1/skills`, etc. Tested in production. Interactive Swagger UI at `/docs`.
- **API key management** — Multi-key auth with RBAC scopes, SHA-256 hashed storage, show-once pattern, optional expiry, revocation. HTTP + WebSocket CRUD. Web UI for management.
- **Hooks system** — Event-driven hooks with command evaluators (shell exit code) and agent evaluators (delegate to reviewer). Blocking gates with auto-retry and recursion-safe evaluation.
- **Media tools** — `create_image` (DashScope, MiniMax), `create_audio` (OpenAI, ElevenLabs, MiniMax, Suno), `create_video` (MiniMax, Veo), `read_document` (Gemini File API), `read_image`, `read_audio`, `read_video`. Persistent media storage with lazy-loaded MediaRef.
- **Additional provider modes** — Claude CLI (Anthropic via stdio + MCP bridge), Codex (OpenAI gpt-5.3-codex via OAuth).
- **Google Cloud Vertex AI provider** — Enterprise GCP integration via Vertex OpenAI-compatible endpoint. OAuth2 service account auth (inline JSON or file path) with automatic token refresh, plus Application Default Credentials (ADC) for GKE/Cloud Run/Compute Engine. Regional endpoints for data residency (e.g. `asia-southeast1`, `us-central1`). Addresses [#576](https://github.com/nextlevelbuilder/goclaw/issues/576).
- **Knowledge graph** — LLM-powered entity extraction, graph traversal, force-directed visualization, and `knowledge_graph_search` agent tool.
- **Memory management** — Admin dashboard for memory documents (CRUD, semantic search, chunk/embedding details, bulk re-indexing).
- **Persistent pending messages** — Channel messages persisted to PostgreSQL with auto-compaction (LLM summarization) and monitoring dashboard.
- **Heartbeat system** — Periodic agent check-ins via HEARTBEAT.md checklists with suppress-on-OK, active hours, retry logic, and channel delivery.

### Implemented but Not Fully Tested

- **Slack** — Channel integration implemented, not yet validated with real users.
- **Other messaging channels** — Discord, Zalo OA, Zalo Personal, Feishu/Lark, WhatsApp channel adapters are implemented but have not been tested end-to-end in production. Only Telegram has been validated with real users.
- **OpenTelemetry export** — OTLP gRPC/HTTP exporter implemented (build-tag gated). In-app tracing works; external OTel export not validated in production.
- **Tailscale integration** — tsnet listener implemented (build-tag gated). Not tested in a real deployment.
- **Redis cache** — Optional distributed cache backend (build-tag gated). Not tested in production.
- **Browser pairing** — Pairing code flow implemented with CLI and web UI approval. Basic flow tested but not validated at scale.
