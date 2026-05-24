import { describe, expect, it } from "vitest";

import { parseSchedule, validateSchedule } from "./channel-standby-schema";

describe("parseSchedule", () => {
  it("empty input returns null schedule and null error", () => {
    expect(parseSchedule("")).toEqual({ schedule: null, error: null });
    expect(parseSchedule("   ")).toEqual({ schedule: null, error: null });
  });

  it("invalid JSON yields a parse error", () => {
    const got = parseSchedule("{");
    expect(got.schedule).toBeNull();
    expect(got.error).toMatch(/JSON|Unexpected/);
  });

  it("valid recurring schedule round-trips", () => {
    const json = `{"default_mode":"active","windows":[{"mode":"standby","weekday":"mon-fri","start":"09:00","end":"17:00","tz":"Asia/Saigon"}]}`;
    const got = parseSchedule(json);
    expect(got.error).toBeNull();
    expect(got.schedule?.default_mode).toBe("active");
    expect(got.schedule?.windows).toHaveLength(1);
  });

  it("rejects invalid default_mode", () => {
    expect(parseSchedule(`{"default_mode":"snoozing"}`).error).toMatch(/default_mode/);
  });
});

describe("validateSchedule", () => {
  it("rejects window mixing recurring and one-shot fields", () => {
    expect(
      validateSchedule({
        windows: [
          {
            mode: "standby",
            weekday: "mon",
            start: "09:00",
            end: "17:00",
            from: "2026-06-01T00:00:00Z",
            until: "2026-06-02T00:00:00Z",
          },
        ],
      }),
    ).toMatch(/mix/);
  });

  it("rejects empty window", () => {
    expect(validateSchedule({ windows: [{ mode: "standby" }] })).toMatch(/recurring or one-shot/);
  });

  it("accepts valid one-shot window", () => {
    expect(
      validateSchedule({
        windows: [
          {
            mode: "standby",
            from: "2026-06-01T00:00:00Z",
            until: "2026-06-08T00:00:00Z",
          },
        ],
      }),
    ).toBeNull();
  });

  it("rejects invalid window.mode", () => {
    expect(
      validateSchedule({
        windows: [
          {
            mode: "snoozing" as unknown as "standby",
            weekday: "mon",
            start: "09:00",
            end: "17:00",
          },
        ],
      }),
    ).toMatch(/window\.mode/);
  });
});
