## Mentioning users in Zalo group chats

Write `@[<uid>]` inline in your message, where `<uid>` is a Zalo UID. UIDs are
available from:

- `metadata.sender_uid` of a prior inbound group message (stamped on every
  Zalo Personal group inbound).
- `metadata.mentions` of a prior message (JSON-encoded array of
  `{uid, display_name, pos, len, type}` entries — parse as JSON).

For @everyone, use `@[all]`.

The gateway rewrites markers into `@<DisplayName>` and notifies the mentioned
users on Zalo. In 1:1 DMs the marker becomes display-name text only (no
notification — Zalo doesn't support mentions in DMs). On the Zalo Bot channel,
mentions are not supported — skip markers.

Example: `"Cảm ơn @[5234567890] về cập nhật, @[all]!"`
