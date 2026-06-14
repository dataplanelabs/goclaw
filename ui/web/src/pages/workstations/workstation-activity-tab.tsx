import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { RefreshCw, CheckCircle, XCircle, ShieldOff, FileText, ChevronDown, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDate } from "@/lib/format";
import {
  useWorkstationActivity,
  type WorkstationActivity,
} from "./hooks/use-workstation-activity";

interface WorkstationActivityTabProps {
  workstationId: string;
}

function ActionBadge({ action }: { action: WorkstationActivity["action"] }) {
  const { t } = useTranslation("workstations");
  if (action === "deny") {
    return (
      <Badge variant="destructive" className="gap-1 text-xs">
        <ShieldOff className="h-3 w-3" />
        {t("activity.actions.deny")}
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" className="gap-1 text-xs">
      {t("activity.actions.exec")}
    </Badge>
  );
}

function ExitCodeCell({ exitCode }: { exitCode: number | null }) {
  if (exitCode === null) return <span className="text-muted-foreground">—</span>;
  const ok = exitCode === 0;
  return (
    <span className="flex items-center gap-1">
      {ok ? (
        <CheckCircle className="h-3.5 w-3.5 text-green-500" />
      ) : (
        <XCircle className="h-3.5 w-3.5 text-red-500" />
      )}
      <span className={ok ? "text-green-700 dark:text-green-400" : "text-red-700 dark:text-red-400"}>
        {exitCode}
      </span>
    </span>
  );
}

function formatDuration(ms: number | null): string {
  if (ms === null) return "—";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function ActivityRowDetail({ row }: { row: WorkstationActivity }) {
  const { t } = useTranslation("workstations");
  const hasDetail = !!row.cmdFull || !!row.outputTail;

  if (!hasDetail) return null;

  return (
    <div className="px-3 pb-3 space-y-2">
      {row.cmdFull && (
        <div>
          <p className="text-xs font-medium text-muted-foreground mb-1">{t("activity.detail.fullCommand")}</p>
          <pre className="text-xs bg-muted/50 rounded p-2 overflow-x-auto whitespace-pre-wrap break-all font-mono">
            {row.cmdFull}
          </pre>
        </div>
      )}
      {row.outputTail && (
        <div>
          <p className="text-xs font-medium text-muted-foreground mb-1">{t("activity.detail.outputTail")}</p>
          <div className="rounded bg-black/80 dark:bg-black p-2 h-40 overflow-y-auto overscroll-contain">
            <pre className="text-xs font-mono text-green-400 dark:text-green-300 whitespace-pre-wrap break-all">
              {row.outputTail}
            </pre>
          </div>
        </div>
      )}
    </div>
  );
}

function ActivityRow({ row }: { row: WorkstationActivity }) {
  const [expanded, setExpanded] = useState(false);
  const hasDetail = !!row.cmdFull || !!row.outputTail;

  return (
    <>
      <tr
        className={"hover:bg-muted/30 transition-colors" + (hasDetail ? " cursor-pointer" : "")}
        onClick={() => hasDetail && setExpanded((v) => !v)}
      >
        <td className="px-3 py-2 w-6">
          {hasDetail && (
            expanded
              ? <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
              : <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
          )}
        </td>
        <td className="px-3 py-2">
          <ActionBadge action={row.action} />
        </td>
        <td className="px-3 py-2 font-mono text-xs text-muted-foreground max-w-[240px] truncate">
          {row.cmdPreview || <span className="italic">—</span>}
        </td>
        <td className="px-3 py-2">
          {hasDetail && (
            <span title="Has detail">
              <FileText className="h-3.5 w-3.5 text-muted-foreground" />
            </span>
          )}
        </td>
        <td className="px-3 py-2">
          <ExitCodeCell exitCode={row.exitCode} />
        </td>
        <td className="px-3 py-2 text-muted-foreground whitespace-nowrap text-xs">
          {formatDuration(row.durationMs)}
        </td>
        <td className="px-3 py-2 text-muted-foreground whitespace-nowrap text-xs">
          {formatDate(row.createdAt)}
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={7} className="bg-muted/20 border-b">
            <ActivityRowDetail row={row} />
          </td>
        </tr>
      )}
    </>
  );
}

export function WorkstationActivityTab({ workstationId }: WorkstationActivityTabProps) {
  const { t } = useTranslation("workstations");
  const { rows, loading, error, hasMore, load, loadMore } = useWorkstationActivity();

  useEffect(() => {
    load(workstationId);
  }, [workstationId, load]);

  if (loading && rows.length === 0) {
    return (
      <div className="space-y-2 p-4">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center gap-2 p-8 text-center">
        <p className="text-sm text-destructive">{error}</p>
        <Button variant="outline" size="sm" onClick={() => load(workstationId)}>
          {t("common:retry", "Retry")}
        </Button>
      </div>
    );
  }

  if (rows.length === 0) {
    return (
      <div className="flex flex-col items-center gap-2 p-12 text-center">
        <p className="font-medium text-muted-foreground">{t("activity.emptyTitle")}</p>
        <p className="text-sm text-muted-foreground">{t("activity.emptyDescription")}</p>
      </div>
    );
  }

  return (
    <div className="space-y-3 p-4">
      <div className="flex items-center justify-between">
        <p className="text-sm font-medium">{t("activity.title")}</p>
        <Button
          variant="ghost"
          size="sm"
          className="h-7 gap-1 text-xs"
          onClick={() => load(workstationId)}
          disabled={loading}
        >
          <RefreshCw className={"h-3 w-3" + (loading ? " animate-spin" : "")} />
          {t("common:refresh", "Refresh")}
        </Button>
      </div>

      <div className="overflow-x-auto rounded-md border">
        <table className="min-w-[640px] w-full text-sm">
          <thead className="border-b bg-muted/50">
            <tr>
              <th className="px-3 py-2 w-6" />
              <th className="px-3 py-2 text-left font-medium text-muted-foreground text-xs">
                {t("activity.columns.action")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground text-xs">
                {t("activity.columns.cmdPreview")}
              </th>
              <th className="px-3 py-2 w-8 text-left font-medium text-muted-foreground text-xs">
                {t("activity.columns.output")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground text-xs">
                {t("activity.columns.exitCode")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground text-xs">
                {t("activity.columns.duration")}
              </th>
              <th className="px-3 py-2 text-left font-medium text-muted-foreground text-xs">
                {t("activity.columns.timestamp")}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {rows.map((row) => (
              <ActivityRow key={row.id} row={row} />
            ))}
          </tbody>
        </table>
      </div>

      {hasMore && (
        <div className="flex justify-center pt-2">
          <Button variant="outline" size="sm" onClick={loadMore} disabled={loading}>
            {loading ? (
              <RefreshCw className="h-3.5 w-3.5 animate-spin" />
            ) : (
              t("activity.loadMore")
            )}
          </Button>
        </div>
      )}
    </div>
  );
}
