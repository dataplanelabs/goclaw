import { describe, expect, it } from "vitest";

import { normalizeMemoryAgentId, requireMemoryAgentId } from "./memory-agent";

describe("memory agent id helpers", () => {
  it("normalizes browser sentinel values to no selection", () => {
    expect(normalizeMemoryAgentId(undefined)).toBe("");
    expect(normalizeMemoryAgentId("")).toBe("");
    expect(normalizeMemoryAgentId(" undefined ")).toBe("");
    expect(normalizeMemoryAgentId("null")).toBe("");
  });

  it("preserves real agent ids", () => {
    const id = "550e8400-e29b-41d4-a716-446655440000";
    expect(normalizeMemoryAgentId(` ${id} `)).toBe(id);
    expect(requireMemoryAgentId(id)).toBe(id);
  });

  it("throws before callers build /agents/undefined routes", () => {
    expect(() => requireMemoryAgentId("undefined")).toThrow("No agent selected");
  });
});
