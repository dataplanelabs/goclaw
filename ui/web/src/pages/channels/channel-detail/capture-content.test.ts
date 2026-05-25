import { describe, expect, it } from "vitest";

import { categorizeCapture, preserveLineBreaks } from "./capture-content";

describe("categorizeCapture", () => {
  it("empty string → empty", () => {
    expect(categorizeCapture("")).toBe("empty");
  });
  it("null → empty", () => {
    expect(categorizeCapture(null)).toBe("empty");
  });
  it("undefined → empty", () => {
    expect(categorizeCapture(undefined)).toBe("empty");
  });
  it("whitespace-only → empty", () => {
    expect(categorizeCapture("   \n  \t")).toBe("empty");
  });
  it("plain text → rich", () => {
    expect(categorizeCapture("hello")).toBe("rich");
  });
  it("media tag without attrs → rich", () => {
    expect(categorizeCapture("<media:image>")).toBe("rich");
  });
  it("media tag with url attribute → rich (the screenshot bug case)", () => {
    expect(categorizeCapture('<media:image url="https://example.com/foo.jpg">')).toBe("rich");
  });
  it("file block → rich", () => {
    expect(categorizeCapture('<file name="a.txt" mime="text/plain">body</file>')).toBe("rich");
  });
});

describe("preserveLineBreaks", () => {
  it("single \\n → two-space + \\n (markdown hard break)", () => {
    expect(preserveLineBreaks("a\nb")).toBe("a  \nb");
  });
  it("multi-line chat → all newlines become hard breaks", () => {
    expect(preserveLineBreaks("chao shop\nhi shop\nhello\nhi shop")).toBe(
      "chao shop  \nhi shop  \nhello  \nhi shop"
    );
  });
  it("no newlines → unchanged", () => {
    expect(preserveLineBreaks("single line")).toBe("single line");
  });
  it("empty → empty", () => {
    expect(preserveLineBreaks("")).toBe("");
  });
});
