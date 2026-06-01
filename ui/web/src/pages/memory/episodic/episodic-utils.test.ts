import { describe, expect, it } from "vitest";

import { getEpisodicKeyTopics } from "./episodic-utils";

describe("getEpisodicKeyTopics", () => {
  it("treats nullable API topic arrays as empty", () => {
    expect(getEpisodicKeyTopics({ key_topics: null })).toEqual([]);
  });

  it("keeps non-empty string topics", () => {
    expect(getEpisodicKeyTopics({ key_topics: ["planning", "", "memory"] })).toEqual(["planning", "memory"]);
  });
});
