import type { TeamReplyEvaluation } from "./team-analytics-table";

export interface RejudgeTally {
  total: number;
  graded: number;
  failed: number;
  retry_exhausted: number;
  retrying: number;
  still_pending: number;
  unsettled: number;
}

export function tallyRejudgeOutcome(
  rows: TeamReplyEvaluation[],
  ids: string[],
  sinceTs: string,
): RejudgeTally {
  const idSet = new Set(ids);
  const sinceMs = new Date(sinceTs).getTime();
  const t: RejudgeTally = {
    total: ids.length,
    graded: 0,
    failed: 0,
    retry_exhausted: 0,
    retrying: 0,
    still_pending: 0,
    unsettled: 0,
  };
  const seen = new Set<string>();
  for (const r of rows) {
    if (!idSet.has(r.id) || seen.has(r.id)) continue;
    seen.add(r.id);
    if (r.judge_completed_at) {
      t.graded++;
      continue;
    }
    if (r.judge_error === "throttle_max_retries" || r.judge_error === "throttle_overflow") {
      t.retry_exhausted++;
      continue;
    }
    if (r.judge_error) {
      t.failed++;
      continue;
    }
    const updatedMs = r.updated_at ? new Date(r.updated_at).getTime() : 0;
    if (updatedMs > sinceMs) {
      t.retrying++;
    } else {
      t.still_pending++;
    }
  }
  t.unsettled = t.retrying + t.still_pending;
  return t;
}
