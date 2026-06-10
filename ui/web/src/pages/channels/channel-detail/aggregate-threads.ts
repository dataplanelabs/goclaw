import type { TeamReplyEvaluation } from "./team-reply-types";

// Terminal judge errors that retry can never resolve without the underlying
// capture changing. Add new permanent classes here as backend introduces them.
const PERMANENT_JUDGE_ERROR_CLASSES = new Set<string>([
  "empty_team_reply",
]);

export function isPermanentJudgeError(err: string | null | undefined): boolean {
  if (!err) return false;
  return PERMANENT_JUDGE_ERROR_CLASSES.has(err);
}

export interface ThreadGroup {
  thread_key: string;
  customer_name: string; // empty when contact has no display_name; UI falls back to truncated thread_key
  capture_count: number;
  avg_diff_score: number | null;
  last_activity: string;
  captures: TeamReplyEvaluation[];
}

function isGradable(c: TeamReplyEvaluation): boolean {
  if (isPermanentJudgeError(c.judge_error)) return false;
  if (!c.team_reply || c.team_reply.trim() === "") return false;
  return true;
}

export function aggregateThreads(rows: TeamReplyEvaluation[]): ThreadGroup[] {
  const buckets = new Map<string, TeamReplyEvaluation[]>();
  for (const r of rows) {
    if (!isGradable(r)) continue;
    const existing = buckets.get(r.thread_key);
    if (existing) existing.push(r);
    else buckets.set(r.thread_key, [r]);
  }
  const groups: ThreadGroup[] = [];
  for (const [thread_key, caps] of buckets) {
    caps.sort((a, b) => a.captured_at.localeCompare(b.captured_at));
    let sum = 0;
    let n = 0;
    for (const c of caps) {
      if (typeof c.diff_score === "number") {
        sum += c.diff_score;
        n++;
      }
    }
    const name = caps.find((c) => c.customer_name && c.customer_name.trim() !== "")?.customer_name ?? "";
    groups.push({
      thread_key,
      customer_name: name,
      capture_count: caps.length,
      avg_diff_score: n > 0 ? sum / n : null,
      last_activity: caps[caps.length - 1]?.captured_at ?? "",
      captures: caps,
    });
  }
  groups.sort((a, b) => b.last_activity.localeCompare(a.last_activity));
  return groups;
}

export function scoreColorClass(score: number | null): string {
  if (score === null) return "text-muted-foreground";
  if (score < 0.3) return "text-red-500 dark:text-red-400";
  if (score < 0.7) return "text-amber-500 dark:text-amber-400";
  return "text-emerald-500 dark:text-emerald-400";
}

export function scoreBgClass(score: number | null): string {
  if (score === null) return "bg-muted";
  if (score < 0.3) return "bg-red-500/15 text-red-600 dark:bg-red-500/20 dark:text-red-300";
  if (score < 0.7) return "bg-amber-500/15 text-amber-600 dark:bg-amber-500/20 dark:text-amber-300";
  return "bg-emerald-500/15 text-emerald-600 dark:bg-emerald-500/20 dark:text-emerald-300";
}

export function truncateThreadKey(key: string): string {
  if (key.length <= 35) return key;
  return key.slice(0, 18) + "…" + key.slice(-12);
}
