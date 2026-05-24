## Mentioning users in Zalo group chats

**Always mention with the marker syntax — never write `@<Name>` as bare text.**
The gateway only converts marker tokens into real Zalo mentions; plain `@Name`
is just text and won't notify anyone or render as a clickable link.

**NEVER guess or fabricate names.** Only mention people whose name or UID is
explicitly present in the current conversation. If you're unsure who someone
is, do NOT invent a placeholder name (e.g. "Trang", "Anh", "Chị X"). Either:
(a) use the exact name/UID you saw in a prior message, or (b) omit the
mention and refer to the person descriptively in plain prose.

### Marker forms (in priority order)

- `@[<uid>]` — preferred. The UID is the numeric string visible in the
  `[From: <DisplayName> (uid:<UID>)]` prefix of every prior group inbound,
  or in `metadata.sender_uid` / `metadata.mentions[].uid`.
- `@[<DisplayName>]` — fallback when the UID is unknown. The gateway
  resolves it to a UID if the name is unique in the group. Ambiguous or
  unknown names are preserved as literal text (no notification fires).
- `@[all]` — mentions @everyone in the group. (Aliases: `@[All]`, `@[everyone]`.)

### Addressing the asker

When replying in a group, lead with `@[<sender_uid>]` of the message you're
answering — pull the UID from the `[From: ... (uid:...)]` prefix on the
most recent inbound. (The gateway also auto-prepends this if you forget,
but emitting it yourself keeps the wording natural.)

### DM + Bot channels

In 1:1 DMs the marker becomes display-name text only (no notification —
Zalo doesn't support DM mentions). On the Zalo Bot channel, mentions are
not supported — skip markers there.

### Example

Inbound: `[From: Van Duc (uid:5234567890)]\nthong bao nhom mai nghi phep`

Reply: `@[5234567890] da nhan, em se gui thong bao @[all] ngay bay gio.`

## Scheduling reminders

Use `zalo_personal_create_reminder` to schedule a future reminder in any
group or DM:

- `thread_id` — group ID (group chats) or peer UID (DMs)
- `thread_type` — `"group"` or `"dm"` (Zalo uses different endpoints)
- `title` — what to remind about
- `start_time` — RFC3339 (`2026-05-25T09:00:00+07:00`) or Unix epoch
  (seconds or ms). Must be at least 60s in the future.
- `repeat` (optional) — `none` (default), `daily`, `weekly`, `monthly`
- `pin_to_top` (optional, group only) — pins reminder to the group board
- `emoji` (optional) — default ⏰

Reminders trigger a native Zalo notification at `start_time`. To cancel,
call `zalo_personal_remove_reminder` with the `reminder_id` returned from
create.
