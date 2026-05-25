import { describe, expect, it } from "vitest";
import { tallyRejudgeOutcome } from "./tally-rejudge-outcome";
import type { TeamReplyEvaluation } from "./team-reply-types";

const baseRow = (over: Partial<TeamReplyEvaluation> = {}): TeamReplyEvaluation => ({
  id: "r",
  channel_instance_id: "ci",
  thread_key: "t",
  session_key: "s",
  team_msg_id: "m",
  captured_at: "2026-05-25T00:00:00Z",
  updated_at: "2026-05-25T00:00:00Z",
  customer_message: "",
  team_reply: "",
  ...over,
});

describe("tallyRejudgeOutcome", () => {
  const since = "2026-05-25T01:00:00.000Z";

  it("counts graded rows", () => {
    const rows = [
      baseRow({ id: "a", judge_completed_at: "2026-05-25T02:00:00Z", diff_score: 0.7 }),
      baseRow({ id: "b", judge_completed_at: "2026-05-25T02:00:00Z" }),
    ];
    const t = tallyRejudgeOutcome(rows, ["a", "b"], since);
    expect(t).toMatchObject({ total: 2, graded: 2, unsettled: 0 });
  });

  it("counts retry_exhausted separately", () => {
    const rows = [
      baseRow({ id: "a", judge_error: "throttle_max_retries" }),
      baseRow({ id: "b", judge_error: "throttle_overflow" }),
      baseRow({ id: "c", judge_error: "no_judge_agent_configured" }),
    ];
    const t = tallyRejudgeOutcome(rows, ["a", "b", "c"], since);
    expect(t.retry_exhausted).toBe(2);
    expect(t.failed).toBe(1);
  });

  it("retrying when updated_at after since and no terminal state", () => {
    const rows = [
      baseRow({ id: "a", updated_at: "2026-05-25T01:30:00Z" }),
      baseRow({ id: "b", updated_at: "2026-05-25T00:30:00Z" }),
    ];
    const t = tallyRejudgeOutcome(rows, ["a", "b"], since);
    expect(t.retrying).toBe(1);
    expect(t.still_pending).toBe(1);
    expect(t.unsettled).toBe(2);
  });

  it("ignores rows not in the rejudge id set", () => {
    const rows = [
      baseRow({ id: "a", judge_completed_at: "2026-05-25T02:00:00Z" }),
      baseRow({ id: "x", judge_completed_at: "2026-05-25T02:00:00Z" }),
    ];
    const t = tallyRejudgeOutcome(rows, ["a"], since);
    expect(t.total).toBe(1);
    expect(t.graded).toBe(1);
  });

  it("dedups duplicate rows by id", () => {
    const rows = [
      baseRow({ id: "a", judge_completed_at: "2026-05-25T02:00:00Z" }),
      baseRow({ id: "a", judge_completed_at: "2026-05-25T02:00:00Z" }),
    ];
    const t = tallyRejudgeOutcome(rows, ["a"], since);
    expect(t.graded).toBe(1);
  });
});
