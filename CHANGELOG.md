# Changelog

All notable changes to GoClaw are documented here. For full documentation, see [docs.goclaw.sh](https://docs.goclaw.sh).

## Unreleased

### Added

- **Per-message local timestamps** — the agent now sees each inbound message's
  arrival date and time (`YYYY-MM-DD HH:MM`) in its resolved timezone. Group-buffer
  timestamps are localized to the agent's preferred TZ (previously rendered in raw
  UTC), and the current inbound message is now stamped with its local date-time.
  This gives the agent a consistent date + time-of-day dimension across the whole
  thread instead of a 7h jump between buffered context and the message it's replying
  to — and an absolute anchor even for prompt modes that omit the system-prompt date.

- **`GOCLAW_BROWSER_ACTION_TIMEOUT_MS`** — env overlay for the browser tool's
  per-action timeout (default 30000ms). Raise it (e.g. 180000 = 3min) so the
  agent can navigate + snapshot heavy authenticated pages (e.g. a logged-in
  Strava dashboard) that exceed the 30s default.

- **Browser tool: persistent-profile mode** (`GOCLAW_BROWSER_PERSISTENT_PROFILE`) — makes
  every tenant share the one default-context remote Chrome (the only context backed by
  `--user-data-dir`), so a human's one-time login on a remote headful Chrome sidecar
  persists and the agent inherits the authenticated session over CDP. Without it,
  non-master tenants route to ephemeral incognito and never see the logged-in profile.
  **Single-identity only** — all tenants share one cookie jar; never enable on a
  multi-identity deployment.

- **Storage browser previews `.xlsx` spreadsheets** — selecting an Excel file
  renders its sheets as a table (sheet tabs, header row, 200-row cap) instead of
  an unsupported-file placeholder. The parser is dynamically imported so it
  stays out of the main bundle.

- **Storage tree shows last-modified time + a spreadsheet icon** — file rows now
  display `<size> · <relative time>` (e.g. `16.3 KB · 2d ago`), and
  `.xlsx`/`.xls`/`.ods` get the green spreadsheet icon. Folders are sorted by
  their resolved display name, with unresolved bare-id folders sorted last.

- **Richer inbound upload filenames** — media received from channels is now saved
  as `{timestamp}_{sender}_{original-or-kind}_{contenthash}.ext` (e.g.
  `20260529-184710_nguyen-nhat-duy_promo_a1b2c3d4.jpg`) instead of an opaque
  `goclaw-zca-…` name — chronological, attributable, and identical re-uploads
  dedup onto one file (full-hash verified). Existing files are unaffected.

- **Vault tree shows human-readable names for top-level id folders** — opaque
  channel/group/user id folders in the Knowledge Vault tree now display the
  contact/group display name (resolved from `channel_contacts`, tenant-scoped)
  with the raw id dimmed beside it and a `·direct`/`·group` pill; scope folders
  (`teams`, `agents`) and bare names fall back unchanged. Display-only — stored
  `vault_documents.path` is untouched.

- **Storage tree shows human-readable names for channel id / group folders** — opaque
  id folders (a user id, or `group_<channel>_<id>`) now display the contact/group
  display name (resolved from `channel_contacts`, tenant-scoped, latest-wins) with
  the raw id dimmed beside it and a `·direct`/`·group` pill; bare names fall back
  unchanged. Each row also gets a hover **copy-server-path** button (copies the full
  absolute workspace path for server-side debugging) that reports success only when
  the clipboard write actually lands.

### Fixed

- **Zalo Personal poll results now expose returned voter identities** —
  `zalo_personal_get_poll` and `zalo_personal_list_polls` preserve Zalo's
  per-option voter IDs and resolve display names when the group member cache can
  identify them, so agents can answer "who voted" instead of only vote counts.
  Poll result name resolution now also uses the current group context and
  tenant-scoped contact store fallback when Zalo board-list payloads omit
  `group_id`.

- **Zalo Personal poll listing can filter open or closed polls** —
  `zalo_personal_list_polls` accepts `status=all|open|closed` in addition to
  existing `page`/`count` pagination, filtering the polls returned by the
  requested board page.

- **Zalo Personal poll expiry now sends Zalo's absolute expiration timestamp** —
  `expired_time_seconds` remains an agent-friendly duration, but the channel now
  converts it to the future Unix millisecond `expired_time` value expected by
  Zalo instead of sending duration milliseconds.

- **Zalo Personal poll create errors now point agents at the likely invalid
  parameter** — Zalo code 114 no longer tells the agent to check
  `expired_time_seconds` first when the duration already passed local validation.
  The tool now preserves the sent shape, prioritizes retrying without
  `allow_add_new_option` when that flag was optional, and warns not to invent
  placeholder options.

- **WhatsApp reactions now preserve the reacted message context** — inbound
  reaction events are stored as feedback with the reactor, emoji, platform
  timestamp, target message id, and short message/file preview. The agent now
  sees total reactions and reactor names instead of inferring from a vague
  recent-reaction signal.

- **Zalo Personal no longer misses media sent just after an addressed message** —
  inbound media now participates in gateway debounce instead of bypassing it,
  and Zalo Personal addressed turns wait briefly for follow-up media before
  dispatch. In groups, pending-history text and media are collected from the
  same snapshot before clearing, so an image sent after a mention can still
  reach the same agent turn.

- **`create_image` resolves current media IDs even when upload paths are reused** —
  same-path aliases from history/current-turn media refs are preserved for
  lookup, while provider payloads still dedupe by file path. This prevents
  prompt-facing `<media:image id="...">` values from failing when `.uploads/`
  already contains the same persisted file under an older ref. Media analysis
  fallbacks now also use the chain-resolved provider wrapper and normalize
  Codex-only chat models to the provider default for ChatGPT OAuth calls.

- **Zalo OA refresh-token rotation now persists under the channel tenant** —
  background refresh paths such as team analytics could call the token source
  with a context that had no tenant scope, causing the rotated refresh token to
  stay only in memory. After a pod restart, the channel reloaded the stale
  already-burned refresh token and required another OA re-auth. The token source
  now carries the channel row's tenant and uses it for credential persistence.

- **Vault no longer indexes the same file under two tree roots** — the vault
  interceptor stored `vault_documents.path` relative to the global workspace
  while write tools resolve under the per-tenant workspace, so non-master
  agent-written docs got a `tenants/<slug>/` prefix that rescan/upload never
  produced — surfacing the same file under both `tenants/<slug>/…` and the bare
  path. Paths are now stored tenant-root-relative; reads strip any legacy
  prefix and resolve against the tenant root (no cross-tenant escape); a dual-DB
  dedupe-then-strip migration normalizes existing rows (PG `000078`, SQLite
  v48). Master/desktop (single tenant) had no prefix and is a near-noop.

- **Skill detail "Files" tab no longer shows a spurious "No files found" on first
  open for multi-version skills** — file loading is now driven by the resolved
  current version (resolved eagerly on open) instead of a versionless load that
  returned the wrong/empty set for multi-version skills; it never requests files
  with an undefined version. Adds a regression test.

- **Storage tree no longer double-nests an expanded folder's contents** — opening
  a deep folder (e.g. `…/generated/2026-05-29`) rendered its files under a
  duplicated path chain (`…/2026-05-29/zalo-shtp/group_…/generated/2026-05-29/…`).
  `mergeSubtree` grafted the rebuilt full ancestor chain (the lazy-loaded children
  carry full paths) under the target node; it now grafts only that folder's direct
  descendants. Disk layout was always correct — this was display-only.

- **`create_image` no longer emits a false "provider accepts at most 4
  references" note for custom-named Codex/OpenAI providers** — the over-cap note
  computed the cap from the provider *instance name* (e.g. `codex-cnb`), which
  missed the canonical-name switch and fell back to the default of 4, so a 5-ref
  request reported "1 not sent" even though the native image path actually sent
  all of them (cap 16). The note now resolves the cap the same way generation
  does — native image providers (Codex/ChatGPT-OAuth) report the native cap
  regardless of instance name, others map by media type — so it only fires when
  references are genuinely dropped.

- **`create_image` now resolves reference images from any in-workspace path** —
  a `reference_image_id` naming an on-disk image (a user upload in `.uploads/`, a
  portrait the assistant organized into `portraits/`, or an absolute workspace
  path) previously "did not resolve" because only the in-conversation media set
  was consulted, so posters were generated with the wrong face/logo while the
  tool still reported success. Resolution now mirrors `read_image` (workspace
  boundary + skill dirs, secrets denied), enumerates the session `.uploads/`
  folder so uploads that aged out of the context window still resolve, and
  returns an actionable error **before** generation when a ref genuinely can't be
  used — missing → fix the id or ask the user to resend; too large or unsupported
  → recompress/convert. Per-image cap aligned to `read_image` (10MB).

- **Group file writers granted "All Groups" are no longer wrongly refused** — a
  user granted `file_writer` (or `*`) at the `group:*` (All Groups) scope was
  told by the bot it had no write permission, because the system-prompt gate used
  an exact-scope lookup while the runtime tool gate was wildcard-aware. The prompt
  gate now decides via the same wildcard-aware `CheckPermission` (fail-open on
  store error — the runtime gate still fails closed), and the displayed writer
  roster reflects wildcard grants via a new `ListEffectiveFileWriters`.
  Channel `/addwriter` management gates keep exact-scope semantics.

- **Skill detail "Files" tab no longer shows a spurious "No files found"** —
  the tab could render empty on first open (then populate after toggling
  Content↔Files) because file loads raced across effects, swallowed errors
  silently, and seeded the version from the possibly-stale skill-list value.
  Loads now use a stale-response guard (latest wins), keep the last good list on
  a transient error, and resolve the authoritative current version. Adds React
  Testing Library + a component test for the tab.

- **`create_image` reference-image caps are now provider-aware** (#219) — a
  global 4-ref cap truncated references *before* provider routing, so OpenAI
  (`/images/edits`, cap 16) and Codex never saw refs 5+, and a valid fifth
  reference was reported with a misleading "did not resolve". References now
  resolve up to the largest provider cap (16, subject to MIME/byte safety caps),
  each provider still truncates to its own cap at call time (Gemini/OpenRouter
  4, MiniMax 1, Codex 16), and a ref dropped for being over a provider's cap is
  reported distinctly from one that was genuinely missing.

- **`use_skill` no longer loads stale managed skill bundles** (#218) — when a
  duplicate tenant skill-store root existed on disk (e.g. a legacy
  `tenants/<uuid>/…` root alongside the current `tenants/<slug>/…`), the loader's
  first-match filesystem scan could resolve an old version's `SKILL.md` + assets
  even though the DB/UI showed a newer current version. `use_skill` now builds
  the managed activation payload from the authoritative DB path
  (`SkillAccessStore.ListAccessible` → `SkillInfo.BaseDir`, which honors the DB
  `file_path`), falling back to the filesystem loader only for non-managed
  skills. Grant semantics unchanged.

- **TTS fallback no longer starves on a slow primary** — when the primary
  provider (e.g. CPU VieNeu) ran long, it consumed the entire TTS deadline and
  the fast Edge fallback inherited an already-expired context, so it failed
  instantly without ever running — long texts produced no audio at all. Each
  provider attempt now gets its own slice of the remaining deadline
  (`remaining / providers-left`), so the fallback always runs with a fresh,
  fair budget.

- **Zalo voice messages now play on mobile, not just desktop** — TTS voice
  bubbles were silent on Zalo Android/iOS because we sent M4A (MP4 container) at
  16 kHz. Zalo's own voice messages are raw **ADTS AAC** (AAC-LC, mono,
  44.1 kHz) — a streamable format with no moov atom — which is what plays on
  both mobile and desktop. The Zalo voice path now normalizes to ADTS AAC at
  44.1 kHz to match. (Supersedes the earlier M4A `+faststart` attempt, which
  still failed on mobile because the MP4 container itself was the problem.)

- **Skill image assets now work as `create_image` references** — activated
  skill image assets returned by `use_skill` are exposed as resolvable
  `reference_image_ids`, so brand logos can be passed directly from a skill
  without requiring a separate chat upload. `create_image` now asks the
  assistant to retry with the available skill asset refs when a logo/reference
  is requested but omitted. `read_image` also recovers the common generated-file
  timestamp typo where the model swaps `-` and `_` in the output path.

- **Zalo Personal file upload no longer fails on IPv6 dial** — trace
  `019e601a-…` showed a TTS audio that never reached the chat: the Session
  HTTP client used Go's default transport, happy-eyeballs raced v6 and v4
  for `tt-files-wpa.chat.zalo.me`, picked the AAAA address, and the K3S pod
  (no IPv6 egress) instant-failed with `connect: network is unreachable`.
  `Session.Client` now uses an explicit `*http.Transport` with a dialer
  that promotes `tcp` / `tcp6` to `tcp4`, sidestepping the dual-stack
  resolution entirely. Covers every Zalo Personal outbound (file upload,
  message send, group ops, reactions) since they all share the session
  client. Regression test verifies the network-string rewrite.

- **Chunked upload retries transient network errors** — defense-in-depth on
  top of the IPv4 force. `postChunkWithRetry` retries each chunk POST up to
  3 times on `ENETUNREACH` / `EHOSTUNREACH` / `ECONNREFUSED` / `ECONNRESET` /
  `io.EOF` / timeouts with `200ms × attempt²` backoff. Body rebuilt per
  attempt (multipart `bytes.Buffer` is one-shot). Context cancellation and
  deadline are NOT retried. Classifier covered by `TestIsRetryableNetErr`.

- **Edge TTS voice name resolution** — trace `019e601e-…` configured
  `voice: HoaiMy` in the UI but got a different voice in output because the
  edge-tts CLI requires the full VoiceID (`vi-VN-HoaiMyNeural`).
  `resolveVoiceID` now maps display Name → VoiceID before invoking the CLI.
  Unknown values pass through unchanged so custom voice IDs still work.

### Changed (BREAKING for LLM behavior — see below)

- **Media generation tools no longer auto-deliver** —
  `create_image` / `create_audio` / `create_video` / `tts` previously
  populated `Result.Media` and marked `DeliveredMedia`, so the consumer
  dispatched the file automatically at end-of-run. That created (a) THREE
  parallel delivery paths (auto-attach, `send_file`, `message(MEDIA:)`)
  with conflicting guards, (b) zero trace observability of the actual
  upload, and (c) confusing error loops when the LLM tried to forward an
  already-queued file (trace `019e5fc3-…`). Now these tools return only
  the path in `ForLLM`; the LLM MUST call `send_file(path)` to deliver,
  same as for any other file. Single delivery path. Every send is a
  tracked tool span. Operator can retry just the send span. Prompt
  addendum (`ZALO_PERSONAL_ADDENDUM.md`) updated with explicit "MUST call
  send_file" rule; `ForLLM` text now says "call send_file(path=…) — file
  is NOT auto-delivered" with the path quoted twice (escape against
  date-format hallucination). Affected test
  `TestCreateImageTool_RoutesNativePath_WithPrompt` updated to assert the
  new empty-Media + send_file-instruction contract.

### Changed

- **Trace list row cleanup + responsive chat title** — the row used to show a
  redundant `Zalo-shtp 845485536…` chip on the agent line (channel-prefixed
  decoded `user_id`) AND a truncated `SHTP _ SUPPORT, HẬU CẦN, PA…` group title
  underneath. The first chip is now hidden for `group:` user_ids since the
  channel badge + group title already convey the routing. The group title
  itself gets responsive `max-w` (`200px / lg:400 / xl:600 / 2xl:800`), and
  the name column matches (`300px / lg:500 / xl:800 / 2xl:none`), so big
  screens display the full name. Trace Detail dialog gains `Chat` + `User ID`
  rows so the chat context isn't lost; 3 new i18n keys (en/vi/zh).

### Added

- **Zalo Personal — TTS audio now arrives as native voice bubble** — agents
  calling `tts` (or any of the audio-producing tools) then `send_file(path=*.mp3)`
  targeting a `zalo_personal` chat now produce a native voice-bubble in the
  recipient's Zalo app (playable inline with waveform/duration), not a file
  attachment. Matches the Telegram `sendVoice` UX. Implementation: (1) new
  `internal/media/audio_normalize.go` re-encodes the source to M4A (mono, 16 kHz,
  AAC-LC) via `ffmpeg` — added to the Docker image under `ENABLE_FULL_SKILLS=true`.
  (2) New `internal/channels/zalo/personal/protocol/send_voice.go` reuses the
  existing chunked `UploadFile` path (returns a Zalo CDN URL via WS callback),
  then POSTs to `/api/message/forward` (or `/api/group/forward`) with `msgType=3`
  + `msgInfo={voiceUrl, m4aUrl, fileSize}` — matches zca-js's `sendVoice` wire
  shape exactly. (3) Channel-level `sendMediaBestEffort` now detects audio
  extensions (`.mp3` / `.m4a` / `.ogg` / `.opus` / `.wav` / `.aac`) and routes
  through voice-send BEFORE the image / file branches; on any failure (ffmpeg
  missing, upload error, voice-endpoint reject) the channel falls back to
  `share.file` so the message is never dropped. (4) New `disable_voice_send`
  per-channel kill switch (default off) routes audio straight to `share.file`
  when set. `ZALO_PERSONAL_ADDENDUM.md` updated so the LLM knows audio is
  auto-routed to voice bubble. ZCA-JS protocol verified live during the spike
  phase against `RFS-ADRENO/zca-js` source.

- **Configurable trace replay retention** — new `trace.replay_retention_days`
  system_config (default 7 via `config.DefaultReplayRetentionDays`). Replaces
  the prior hard-coded "sweep on every successful run" behavior that wiped
  `trace_replay_payloads` rows whose `created_at < currentRunStart`, leaving
  only the latest run per session retryable. Trace `019e5f22-…` was unretry-
  able 42 minutes after creation because intermediate successful runs had
  swept its capture. Sweep now uses `now - retention` cutoff when retention
  > 0, so any completed/failed trace within the window can still be retried.
  Set days to 0 / negative to keep the legacy runStart behavior. Plumbed
  through `GatewayConfig` → `agent.Deps` → `LoopConfig` → `Loop.replayRetention`
  → the sweep call in `loop_run.go`. Seeded into `system_configs` at startup
  so operators see + edit the value in the UI.

- **Team Analytics — customer name on thread rows + customer-bubble
  timestamps** — operators reviewing captures previously saw only the
  opaque `direct:77448…562035` thread key; they had to infer the customer
  identity from message content. The `display_name` column on
  `channel_contacts` was already populated by every Zalo OA inbound (from
  the webhook payload's `sender.display_name`), but the analytics surface
  didn't expose it. PG + SQLite `team_reply_evaluations.List` now LEFT JOIN
  `channel_contacts` via `(tenant_id, channel_type, sender_id)` (sender_id
  parsed from `direct:` prefix) and return `customer_name` on each row.
  Thread accordion headers show the resolved name as the primary label
  with the truncated thread_key as monospace secondary; rows without a
  resolved name keep the existing thread_key display (graceful fallback).
  Customer bubbles in the accordion content also gained a `captured_at`
  timestamp (parity with team-reply bubbles). `buildFilterClause` refactored
  to accept a table alias so the joined `e.*` columns disambiguate cleanly
  in WHERE. New tests: 2 for `aggregateThreads.customer_name` resolution.

### Added

- **Team Analytics — judge-error visibility + visible refresh status** —
  P0 UX fixes from live operator feedback on `zalo-oa-annhien`. (1) Failed
  judge evaluations now show the actual error message: hover tooltip on
  the "Failed" badge + inline truncated error (120 char cap) below the
  team-reply bubble. Operators stop guessing why "3 evaluations failed"
  and see e.g. `judge_error="embedding API error 429"`. (2) Refresh UX —
  the cryptic `↻` icon next to the thread filter is now a labelled
  `↻ Refresh` button with a spinner when fetch is in flight, plus a new
  `Refreshed Xs ago` indicator alongside it. Auto-refresh (30s interval)
  was already in place but invisible to operators; this surfaces the
  cadence so they know fresh data is arriving without having to click.
  Tick state updates every 10s for stable display. 5 new i18n keys
  (en/vi/zh): lastRefreshed, refreshNeverYet, refreshNow, refreshButton,
  refreshJustNow.

### Fixed

- **Team Analytics — customer multi-message turns + file attachments now
  render properly** — two co-located bugs from the rich-content rollout:
  (1) customer turns are joined by the poll worker with single `\n`, but
  markdown's soft-break rule collapsed them to spaces, so 4 customer
  messages ("chao shop / hi shop / hello / hi shop") rendered as a single
  line. Fixed by `preserveLineBreaks()` in `CaptureContent` — promotes
  every `\n` to a markdown hard break (two-space + `\n`) before handing
  to `RichContent`, so each captured turn shows on its own line.
  (2) Customer-sent files (e.g. `HÀ TRUNG NĐ - SỔ HUẤN.xlsx` Excel) embed
  as `<media:document name="…">` tags from the backend (see
  `internal/channels/zalo/oa/media_tags.go`), but the parser only captured
  `url=` — the filename was dropped. Parser now also extracts `name="..."`
  on every `<media:*>` tag; `MediaBadge` renders the filename instead of
  the generic "document" label when available. 6 new vitest cases pin
  `preserveLineBreaks` + `name=` extraction.

- **Team Analytics — Zalo CDN thumbnails were invisible (blocked by hot-link
  protection)** — v3.23.29 wired `<img src=zalo-cdn-url>` inline but Zalo's
  `photo-stal-*.zdn.vn` CDN 403s requests with our origin as Referer, and the
  old `onError` handler hid the `<img>` via `display: none` — operators saw
  empty space with only the URL on browser status-bar hover. Two fixes in
  `rich-content.tsx`: (1) added `referrerPolicy="no-referrer"` to inline
  `<img>` so the browser strips the Referer header (matches how Telegram /
  WhatsApp Web bypass the same protection); (2) replaced silent `display: none`
  with stateful `MediaFallbackLink` — when the image / video / audio load
  fails, render a visible clickable badge (`[icon] Image ↗` opens in new tab).
  Now operators either see the actual thumbnail OR a visible link they can
  click — never an invisible empty anchor. Same treatment for `<video>` /
  `<audio>` with `crossOrigin="anonymous"`.

- **Team Analytics — raw `<media:image url="…">` XML now renders as
  inline thumbnails** — operators reviewing captures on `zalo-oa-annhien`
  saw the literal XML string of customer-attached images / stickers
  instead of the image itself. Two-layer fix: (1) shared chat parser
  (`rich-content-parser.ts`) gained attribute support — `MEDIA_TAG_RE`
  extended from `/<media:(...)>/g` to `/<media:(...)([^>]*)>/g` so the
  `url="..."` attribute is captured; `extractMediaUrl` validates `http(s)`
  scheme only (rejects `javascript:` / `data:` / `file:` for XSS safety).
  (2) `RichContent` gained an opt-in `inlineMediaUrls` prop: when set,
  `MediaBadge` renders `<img>` / `<video>` / `<audio>` inline (click → open
  in new tab); when unset, the existing "[Image attached]" text badge is
  preserved. New `CaptureContent` wrapper (Team Analytics captures + detail
  dialog) opts in; the WS chat-thread does NOT — it continues to use
  `MediaGallery` + `mediaItems` separately, so no double-render. Wrapped in
  the shared `ErrorBoundary` — malformed media tags can't crash the tab.
  Three new i18n keys (en/vi/zh): `teamAnalytics.captureEmpty`,
  `teamAnalytics.captureRenderFailed`. 15 new vitest cases cover the
  parser scheme-rejection + `categorizeCapture` branches.

- **Traces — Retry button now actually delivers to the channel** — the
  retry handler (`cmd/gateway_trace_retry.go`) called `ag.Run` but never
  invoked the consumer's outbound dispatch, so the new RunResult evaporated
  silently. Trace status flipped to `completed` but the chat received
  nothing — operators saw "iterations=0, completed" with no `outbound.cached`
  log line. PR #124's claim "re-invokes through the regular agent pipeline"
  only covered the LLM half. New `dispatchRetryOutbound` mirrors the
  consumer's outMsg shape (Channel, ChatID, Content, Media, TenantID from
  ctx, AgentID, AgentOtherConfig, TraceID) and publishes via `msgBus`.
  Silent / NO_REPLY content + missing Channel are still skipped to match
  the consumer's behavior. Group inbounds get `group_id` metadata stamped
  so `zalo_personal` channel.Send routes via the group API (parity with the
  inbound-path fallback in `send.go`). 5 regression tests in
  `gateway_trace_retry_test.go` pin dispatch + skip semantics.

- **Agent — duplicate-media filter now runs in the v3 pipeline finalize too** —
  v3.23.28 added a `PublishedMedia` ctx tracker and a filter in the legacy
  `loop_finalize.go`, but runs actually flow through `runViaPipeline` →
  `pipeline.FinalizeStage`, which had its own `processMedia` that only
  deduped by string match — no PublishedMedia awareness. Result: mp3 still
  sent twice on zalo-shtp (trace 019e5f1b-…). FinalizeStage.processMedia
  now consults `tools.PublishedMediaFromCtx(ctx)` and drops marked paths
  before the consumer dispatches RunResult.Media. Regression test
  `TestFinalizeStage_DropsPublishedMedia` pins the behavior.

- **Agent — files sent twice when LLM forwarded via `message(MEDIA:…)`** —
  Tools that produced a file (e.g. `tts`) added it to `Result.Media`, which
  the agent loop tracked in `rs.mediaResults` for consumer dispatch. When
  the LLM then called `message(channel=…, target=…, MEDIA:<same path>)`,
  the message tool published the file directly to the bus AND the consumer
  still re-dispatched `RunResult.Media`. Channels saw two outbound messages
  for the same file. New `PublishedMedia` ctx tracker marks paths a tool
  has put on the bus directly; `loop_finalize` filters `rs.mediaResults`
  by it so the consumer never sends what the tool already sent. Distinct
  from `DeliveredMedia` (cross-tool dedup guard that lets `send_file`-
  style tools whose Result.Media still needs consumer delivery to flow
  through). Two regression tests cover MEDIA-prefix and embedded-MEDIA
  publish paths.

- **Zalo personal — `create_poll` (and other group-only tools) failed
  on a freshly-restarted pod until the bot had replied once** —
  `IsGroupApproved` was only populated by the pairing branch (when
  `GroupPolicy=pairing`) or by the outbound send fallback. For
  `GroupPolicy=open` (the default), the cache stayed empty until the bot
  replied, so the FIRST tool call in a group (e.g. `zalo_personal_create_poll`)
  failed with "polls only work in group chats". Now `handleGroupMessage`
  marks the group approved right after the policy check passes — symmetric
  with the existing `send.go` fallback. Regression test added.

- **Zalo native renderer — bold header detection covers `**X** emoji`** —
  Section headers with a trailing emoji (e.g. `**Điểm mạnh** 💪`,
  `**Cảnh báo** 🚨`) were rejected by `isBoldOnlyLine`, so the blank line
  between header and content stayed and the bullets weren't indented. Now
  recognized as headers as long as the trailing chars after the closing
  `**` contain no letters/digits (emoji / punctuation / whitespace only).
  Rename `isBoldOnlyLine` → `isBoldHeaderLine` to match the new semantic.

- **Zalo native renderer — filename/identifier underscores no longer
  italicized** — `_X_` patterns inside identifier-shaped text (e.g.
  `BaoCao_DonHang_20260520.xlsx`) were matched by the italic-underscore
  regex, producing broken `*DonHang*`-style emphasis on filenames. Glued
  underscores (letter/digit on BOTH sides) are now swapped for a 1-UTF-16
  sentinel before scan, then restored 1:1 after — so the italic regex
  can't see them as markup but emitted style positions stay valid.
  Legitimate `_italic_` (space/punct on at least one outer side) still
  works, including `_user_id_` where the inner `_` is part of the word.

- **Zalo native renderer — tables + bullet dot + hierarchical indent** —
  three follow-ups after v3.23.20 dropped lst_* styles. (1) `RenderStyles`
  now calls `renderMarkdownTables` like `StripMarkdown` already does, so
  tables in `enable_native_styles=true` channels become bulleted labeled
  blocks instead of raw pipes. (2) Line-prefix `- ` / `* ` / `+ ` rewritten
  to `• ` (Unicode bullet, U+2022) — UTF-16-safe, emitted style positions
  unchanged. (3) Sections under a bold-only header (e.g. `**Đánh giá:**`)
  get hierarchical indent: 2 spaces for top-level items, 4 for bullets that
  immediately follow a numbered item (sub-bullet inference). Companion
  collapse: blank line between a bold-only header and its content is
  dropped (LLMs inconsistently insert it); blank between two adjacent
  headers is preserved as a section separator. Section ends on the next
  bold-only header OR on a blank line followed by non-list non-header
  prose (treated as closing remarks).

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
