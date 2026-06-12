import { describe, it, expect } from "vitest";
import { formatUserLabel } from "./format-user-label";

type Contact = { display_name?: string; username?: string } | null;

const resolverFrom = (map: Record<string, Contact>) => (id: string): Contact => map[id] ?? null;

describe("formatUserLabel", () => {
  it("returns empty string for empty id", () => {
    expect(formatUserLabel("")).toBe("");
  });

  it("resolves a direct id via display_name", () => {
    const resolve = resolverFrom({ u123: { display_name: "Alice" } });
    expect(formatUserLabel("u123", resolve)).toBe("Alice");
  });

  it("resolves a direct id via @username when no display_name", () => {
    const resolve = resolverFrom({ u123: { username: "alice" } });
    expect(formatUserLabel("u123", resolve)).toBe("@alice");
  });

  it("maps 'system' to System", () => {
    expect(formatUserLabel("system")).toBe("System");
  });

  it("prefixes numeric ids with #", () => {
    expect(formatUserLabel("12345")).toBe("#12345");
    expect(formatUserLabel("-42")).toBe("#-42");
  });

  it("truncates long opaque ids", () => {
    expect(formatUserLabel("oc_295eb80d325c976cbeb4a779e2010518")).toBe("oc_295eb80…0518");
  });

  it("resolves a group: prefixed id via the bare chatID contact row", () => {
    const resolve = resolverFrom({ "44163918360303312": { display_name: "SHTP _ SUPPORT" } });
    expect(formatUserLabel("group:zalo-shtp:44163918360303312", resolve)).toBe("SHTP _ SUPPORT");
  });

  it("resolves a group: prefixed id via the bare chatID @username", () => {
    const resolve = resolverFrom({ "999": { username: "ops" } });
    expect(formatUserLabel("group:zalo-shtp:999", resolve)).toBe("@ops");
  });

  it("resolves a guild: prefixed id via the bare chatID", () => {
    const resolve = resolverFrom({ "555": { display_name: "Dev Guild" } });
    expect(formatUserLabel("guild:discord:555", resolve)).toBe("Dev Guild");
  });

  it("falls back to the channel slug when the bare chatID is unresolvable", () => {
    expect(formatUserLabel("group:zalo-shtp:44163918360303312")).toBe("Zalo-shtp 44163918360303312");
    const resolve = resolverFrom({ other: { display_name: "x" } });
    expect(formatUserLabel("group:zalo-shtp:44163918360303312", resolve)).toBe("Zalo-shtp 44163918360303312");
  });
});
