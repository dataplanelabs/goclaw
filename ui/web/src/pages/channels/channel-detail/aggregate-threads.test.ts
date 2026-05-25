import { describe, expect, it } from "vitest";

import { aggregateThreads, scoreColorClass, truncateThreadKey } from "./aggregate-threads";
import type { TeamReplyEvaluation } from "./team-reply-types";

const row = (over: Partial<TeamReplyEvaluation>): TeamReplyEvaluation => ({
  id: "r",
  channel_instance_id: "ci",
  thread_key: "t",
  session_key: "s",
  team_msg_id: "m",
  captured_at: "2026-05-25T00:00:00Z",
  updated_at: "2026-05-25T00:00:00Z",
  customer_message: "",
  team_reply: "non-empty",
  ...over,
});

describe("aggregateThreads", () => {
  it("groups by thread_key", () => {
    const rows = [
      row({ id: "a", thread_key: "t1" }),
      row({ id: "b", thread_key: "t1" }),
      row({ id: "c", thread_key: "t2" }),
    ];
    const groups = aggregateThreads(rows);
    expect(groups).toHaveLength(2);
    expect(groups.find((g) => g.thread_key === "t1")!.capture_count).toBe(2);
    expect(groups.find((g) => g.thread_key === "t2")!.capture_count).toBe(1);
  });

  it("excludes empty_team_reply judge_error rows from groups", () => {
    const rows = [
      row({ id: "a", thread_key: "t1", judge_error: "empty_team_reply" }),
      row({ id: "b", thread_key: "t1", team_reply: "real" }),
    ];
    const groups = aggregateThreads(rows);
    expect(groups[0]?.capture_count).toBe(1);
    expect(groups[0]?.captures[0]?.id).toBe("b");
  });

  it("excludes trimmed-empty team_reply", () => {
    const rows = [
      row({ id: "a", thread_key: "t1", team_reply: "   " }),
      row({ id: "b", thread_key: "t1", team_reply: "real" }),
    ];
    expect(aggregateThreads(rows)[0]?.capture_count).toBe(1);
  });

  it("avg_diff_score ignores missing scores", () => {
    const rows = [
      row({ id: "a", thread_key: "t1", diff_score: 0.4 }),
      row({ id: "b", thread_key: "t1", diff_score: 0.6 }),
      row({ id: "c", thread_key: "t1" }),
    ];
    expect(aggregateThreads(rows)[0]?.avg_diff_score).toBeCloseTo(0.5);
  });

  it("avg_diff_score is null when no row has score", () => {
    const rows = [row({ id: "a", thread_key: "t1" })];
    expect(aggregateThreads(rows)[0]?.avg_diff_score).toBeNull();
  });

  it("sorts threads by last_activity DESC", () => {
    const rows = [
      row({ id: "a", thread_key: "old", captured_at: "2026-05-20T00:00:00Z" }),
      row({ id: "b", thread_key: "new", captured_at: "2026-05-25T00:00:00Z" }),
    ];
    expect(aggregateThreads(rows).map((g) => g.thread_key)).toEqual(["new", "old"]);
  });

  it("captures within thread sorted by captured_at ASC", () => {
    const rows = [
      row({ id: "later", thread_key: "t", captured_at: "2026-05-25T10:00:00Z" }),
      row({ id: "earlier", thread_key: "t", captured_at: "2026-05-25T08:00:00Z" }),
    ];
    const groups = aggregateThreads(rows);
    expect(groups[0]?.captures.map((c) => c.id)).toEqual(["earlier", "later"]);
  });

  it("empty input → empty groups", () => {
    expect(aggregateThreads([])).toEqual([]);
  });

  it("customer_name picks first non-empty across captures of a thread", () => {
    const rows = [
      row({ id: "a", thread_key: "t1", customer_name: "" }),
      row({ id: "b", thread_key: "t1", customer_name: "Bình" }),
      row({ id: "c", thread_key: "t1", customer_name: "Bình stale" }),
    ];
    expect(aggregateThreads(rows)[0]?.customer_name).toBe("Bình");
  });

  it("customer_name is empty when no capture has one", () => {
    const rows = [row({ id: "a", thread_key: "t1" })];
    expect(aggregateThreads(rows)[0]?.customer_name).toBe("");
  });
});

describe("scoreColorClass", () => {
  it("null → neutral", () => {
    expect(scoreColorClass(null)).toContain("muted");
  });
  it("low score → red", () => {
    expect(scoreColorClass(0.2)).toContain("red");
  });
  it("mid score → amber", () => {
    expect(scoreColorClass(0.5)).toContain("amber");
  });
  it("high score → emerald", () => {
    expect(scoreColorClass(0.8)).toContain("emerald");
  });
  it("0.3 boundary → amber (inclusive)", () => {
    expect(scoreColorClass(0.3)).toContain("amber");
  });
  it("0.7 boundary → emerald (inclusive)", () => {
    expect(scoreColorClass(0.7)).toContain("emerald");
  });
});

describe("truncateThreadKey", () => {
  it("short keys pass through", () => {
    expect(truncateThreadKey("direct:123")).toBe("direct:123");
  });
  it("long keys ellipsize in the middle", () => {
    const long = "direct:5886292414955702584";
    const out = truncateThreadKey(long);
    expect(out).toContain("…");
    expect(out.length).toBeLessThan(long.length);
  });
});
