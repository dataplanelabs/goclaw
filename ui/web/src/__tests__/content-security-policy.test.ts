import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

function readCspContent(): string {
  const html = readFileSync(resolve(process.cwd(), "index.html"), "utf8");
  const match = html.match(/<meta\s+http-equiv="Content-Security-Policy"\s+content="([^"]+)"/);
  expect(match?.[1]).toBeDefined();
  return match?.[1] ?? "";
}

function parseDirectives(csp: string): Map<string, string[]> {
  const directives = new Map<string, string[]>();

  for (const directive of csp.split(";")) {
    const parts = directive.trim().split(/\s+/).filter(Boolean);
    const name = parts.shift();
    if (name) {
      directives.set(name, parts);
    }
  }

  return directives;
}

describe("Content-Security-Policy", () => {
  const directives = parseDirectives(readCspContent());

  it("allows graph layout workers without relaxing script execution", () => {
    expect(directives.get("worker-src")).toEqual(["'self'", "blob:"]);
    expect(directives.get("script-src")).toEqual(["'self'"]);
  });

  it("keeps unsafe script sources out of script-src", () => {
    const scriptSources = directives.get("script-src") ?? [];

    expect(scriptSources).not.toContain("blob:");
    expect(scriptSources).not.toContain("'unsafe-eval'");
    expect(scriptSources).not.toContain("'unsafe-inline'");
  });
});
