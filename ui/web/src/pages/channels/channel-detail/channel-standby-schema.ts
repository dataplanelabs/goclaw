export interface ScheduleWindow {
  id?: string;
  mode?: "active" | "standby";
  tz?: string;
  weekday?: string;
  start?: string;
  end?: string;
  from?: string;
  until?: string;
}

export interface Schedule {
  default_mode?: "active" | "standby";
  windows?: ScheduleWindow[];
}

export function parseSchedule(
  text: string,
): { schedule: Schedule | null; error: string | null } {
  const trimmed = text.trim();
  if (!trimmed) return { schedule: null, error: null };
  try {
    const parsed = JSON.parse(trimmed) as Schedule;
    const err = validateSchedule(parsed);
    return { schedule: err ? null : parsed, error: err };
  } catch (e) {
    return { schedule: null, error: e instanceof Error ? e.message : "invalid JSON" };
  }
}

export function validateSchedule(s: Schedule): string | null {
  if (s.default_mode && s.default_mode !== "active" && s.default_mode !== "standby") {
    return `default_mode must be 'active' or 'standby'`;
  }
  for (const w of s.windows ?? []) {
    const isOneShot = !!w.from && !!w.until;
    const isRecurring = !!w.weekday && !!w.start && !!w.end;
    if (isOneShot && isRecurring) return "window cannot mix one-shot and recurring fields";
    if (!isOneShot && !isRecurring) return "window must have either recurring or one-shot fields";
    if (w.mode && w.mode !== "active" && w.mode !== "standby") {
      return `window.mode must be 'active' or 'standby'`;
    }
  }
  return null;
}
