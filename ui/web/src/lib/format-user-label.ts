type ContactLike = { display_name?: string; username?: string } | null;
type Resolver = (id: string) => ContactLike;

/**
 * Format a user/sender ID into a human-readable label.
 * Display hierarchy: display_name > @username > formatted ID fallback.
 */
export function formatUserLabel(userId: string, resolve?: Resolver): string {
  if (!userId) return "";

  // Try contact resolver first
  if (resolve) {
    const contact = resolve(userId);
    if (contact?.display_name) return contact.display_name;
    if (contact?.username) return `@${contact.username}`;
  }

  // Special cases
  if (userId === "system") return "System";
  if (userId.startsWith("group:") || userId.startsWith("guild:")) {
    const parts = userId.split(":");
    if (parts.length >= 3) {
      // Group contact rows are keyed by the bare chatID, so resolve(chatID) before the slug fallback.
      const chatId = parts.slice(2).join(":");
      if (resolve) {
        const groupContact = resolve(chatId);
        if (groupContact?.display_name) return groupContact.display_name;
        if (groupContact?.username) return `@${groupContact.username}`;
      }
      const channel = parts[1]!.charAt(0).toUpperCase() + parts[1]!.slice(1);
      return `${channel} ${chatId}`;
    }
  }

  // Fallback: prefix numeric IDs with #
  if (/^-?\d+$/.test(userId)) return `#${userId}`;

  // Truncate long opaque IDs (e.g. oc_295eb80d325c976cbeb4a779e2010518)
  if (userId.length > 20) return `${userId.slice(0, 10)}…${userId.slice(-4)}`;

  return userId;
}
