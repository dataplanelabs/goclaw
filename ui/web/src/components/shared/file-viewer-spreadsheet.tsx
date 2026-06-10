import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";

const MAX_ROWS = 200;

type Cell = string | number | boolean | Date | null;
interface SheetData {
  sheet: string;
  data: Cell[][];
}

function cellText(c: Cell): string {
  if (c == null) return "";
  if (c instanceof Date) return c.toLocaleDateString();
  return String(c);
}

// SpreadsheetViewer renders an .xlsx file as a table. The parser
// (read-excel-file) is dynamically imported so it never enters the main bundle.
// readXlsxFile returns every sheet at once, so switching tabs needs no refetch.
export function SpreadsheetViewer({
  path,
  fetchBlob,
}: {
  path: string;
  fetchBlob: (path: string) => Promise<Blob>;
}) {
  const { t } = useTranslation("common");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sheets, setSheets] = useState<SheetData[]>([]);
  const [active, setActive] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    setActive(0);
    (async () => {
      try {
        const { default: readXlsxFile } = await import("read-excel-file/browser");
        const blob = await fetchBlob(path);
        const result = (await readXlsxFile(blob)) as unknown as SheetData[];
        if (!cancelled) setSheets(result);
      } catch (e) {
        if (!cancelled) setError(e instanceof Error ? e.message : "Failed to read spreadsheet");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [path, fetchBlob]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (error) {
    return <p className="px-3 py-4 text-sm text-destructive">{error}</p>;
  }

  const sheet = sheets[Math.min(active, Math.max(0, sheets.length - 1))];
  const data = sheet?.data ?? [];
  const [header, ...body] = data;

  return (
    <div className="flex flex-col gap-2">
      {sheets.length > 1 && (
        <div className="flex flex-wrap gap-1">
          {sheets.map((s, i) => (
            <button
              key={s.sheet}
              onClick={() => setActive(i)}
              className={`rounded px-2 py-0.5 text-xs ${
                i === active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-accent/50"
              }`}
            >
              {s.sheet}
            </button>
          ))}
        </div>
      )}
      <div className="overflow-x-auto rounded-md border">
        <table className="min-w-[600px] text-sm">
          {header && (
            <thead>
              <tr className="border-b bg-muted/40">
                {header.map((c, j) => (
                  <th key={j} className="px-2 py-1 text-left font-medium whitespace-nowrap">{cellText(c)}</th>
                ))}
              </tr>
            </thead>
          )}
          <tbody>
            {body.slice(0, MAX_ROWS).map((row, i) => (
              <tr key={i} className="border-b last:border-0">
                {row.map((c, j) => (
                  <td key={j} className="px-2 py-1 whitespace-nowrap tabular-nums">{cellText(c)}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {body.length > MAX_ROWS && (
        <p className="text-2xs text-muted-foreground">
          {t("showingRowsOf", { shown: MAX_ROWS, total: body.length })}
        </p>
      )}
    </div>
  );
}
