import { describe, expect, it } from "vitest";

import { bucketScores } from "./team-analytics-histogram";

describe("bucketScores", () => {
  it("buckets values into [0,1] correctly", () => {
    const got = bucketScores([0.05, 0.15, 0.95]);
    expect(got).toEqual([1, 1, 0, 0, 0, 0, 0, 0, 0, 1]);
  });

  it("clamps and ignores invalid values", () => {
    const got = bucketScores([-0.5, 1.5, NaN, 0.42]);
    // -0.5 → 0 (bucket 0), 1.5 → 1 (bucket 9), NaN dropped, 0.42 → bucket 4.
    expect(got).toEqual([1, 0, 0, 0, 1, 0, 0, 0, 0, 1]);
  });

  it("handles empty input", () => {
    expect(bucketScores([])).toEqual([0, 0, 0, 0, 0, 0, 0, 0, 0, 0]);
  });
});
