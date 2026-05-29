import { describe, it, expect, vi, beforeAll, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { SkillDetailDialog } from "./skill-detail-dialog";
import type { SkillFile, SkillVersions } from "@/types/skill";

// i18n: keep the real module (i18n init needs initReactI18next) but make
// useTranslation deterministic — return keys verbatim so assertions don't
// depend on loaded translations.
vi.mock("react-i18next", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-i18next")>();
  return { ...actual, useTranslation: () => ({ t: (k: string) => k }) };
});

// jsdom shims for Radix (Dialog/Tabs) + FileBrowser.
beforeAll(() => {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
  if (!window.matchMedia) {
    window.matchMedia = ((q: string) => ({
      matches: false,
      media: q,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent() {
        return false;
      },
    })) as unknown as typeof window.matchMedia;
  }
  Element.prototype.scrollIntoView = () => {};
  // jsdom lacks pointer-capture APIs that Radix probes.
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
});

const CURRENT_VERSION = 9;

function renderDialog(overrides: Partial<Parameters<typeof SkillDetailDialog>[0]> = {}) {
  const files: SkillFile[] = [
    { path: "SKILL.md", name: "SKILL.md", isDir: false, size: 120 },
    { path: "assets/logo.png", name: "logo.png", isDir: false, size: 340 },
  ];
  const getSkillVersions = vi.fn(
    async (): Promise<SkillVersions> => ({ versions: [CURRENT_VERSION], current: CURRENT_VERSION }),
  );
  const getSkillFiles = vi.fn(async (_id: string, _version?: number): Promise<SkillFile[]> => files);

  const props = {
    skill: {
      id: "skill-1",
      name: "Thiết kế SHTP Runners",
      slug: "design-shtp",
      version: CURRENT_VERSION,
      source: "managed",
      content: "# Body",
    } as Parameters<typeof SkillDetailDialog>[0]["skill"],
    detailTab: "files",
    selectedVersionParam: null,
    selectedFilePath: null,
    onStateChange: vi.fn(),
    onClose: vi.fn(),
    getSkillVersions,
    getSkillFiles,
    getSkillFileContent: vi.fn(async () => ({ content: "x", path: "SKILL.md", size: 1 })),
    getSkillFileBlob: vi.fn(async () => new Blob()),
    ...overrides,
  };
  return { ...render(<SkillDetailDialog {...props} />), getSkillVersions, getSkillFiles };
}

describe("SkillDetailDialog Files tab", () => {
  beforeEach(() => vi.clearAllMocks());

  it("renders the file list on first view (no stale empty)", async () => {
    const { getSkillFiles } = renderDialog();

    // File appears...
    expect(await screen.findByText("SKILL.md")).toBeTruthy();
    expect(screen.getByText("logo.png")).toBeTruthy();
    // ...and the empty-state key is never shown.
    expect(screen.queryByText("noFiles")).toBeNull();
    // and at least one load happened.
    expect(getSkillFiles).toHaveBeenCalled();
  });

  it("loads files for the authoritative current version, not a stale one", async () => {
    // skill.version intentionally lags behind the real current version to prove
    // we resolve from getSkillVersions().current, not the stale list value (#218/#219 class).
    const { getSkillVersions, getSkillFiles } = renderDialog({
      skill: {
        id: "skill-1",
        name: "Thiết kế SHTP Runners",
        slug: "design-shtp",
        version: 5, // stale
        source: "managed",
        content: "# Body",
      } as Parameters<typeof SkillDetailDialog>[0]["skill"],
    });

    await screen.findByText("SKILL.md");
    await waitFor(() => expect(getSkillVersions).toHaveBeenCalled());
    // Every files load must target the authoritative current version (9), never the stale 5.
    for (const call of getSkillFiles.mock.calls) {
      expect(call[1]).not.toBe(5);
    }
    expect(getSkillFiles.mock.calls.some((c) => c[1] === CURRENT_VERSION)).toBe(true);
  });
});
