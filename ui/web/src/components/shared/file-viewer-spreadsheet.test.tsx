import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { SpreadsheetViewer } from "./file-viewer-spreadsheet";

vi.mock("react-i18next", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-i18next")>();
  return { ...actual, useTranslation: () => ({ t: (k: string) => k }) };
});

vi.mock("read-excel-file/browser", () => ({
  default: vi.fn(async () => [
    { sheet: "Sheet1", data: [["Name", "Qty"], ["Boots", 12], ["Gloves", 3]] },
    { sheet: "Summary", data: [["Total"], [15]] },
  ]),
}));

describe("SpreadsheetViewer", () => {
  afterEach(cleanup);

  it("renders the first sheet as a header+body table with sheet tabs", async () => {
    render(<SpreadsheetViewer path="x.xlsx" fetchBlob={async () => new Blob()} />);
    expect(await screen.findByText("Boots")).toBeTruthy();
    expect(screen.getByText("Name")).toBeTruthy();
    expect(screen.getByText("Qty")).toBeTruthy();
    expect(screen.getByText("Gloves")).toBeTruthy();
    // Multiple sheets render as tab buttons.
    expect(screen.getByText("Sheet1")).toBeTruthy();
    expect(screen.getByText("Summary")).toBeTruthy();
  });
});
