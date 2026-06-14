import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Eraser, ArrowDownToLine, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  useWorkstationSessions,
  useSessionOutput,
  type SessionSummary,
  type SessionStatus,
} from "./hooks/use-workstation-sessions";
import { formatDate } from "@/lib/format";

interface WorkstationLiveLogTabProps {
  workstationId: string;
}

function StatusBadge({ status }: { status: SessionStatus }) {
  const { t } = useTranslation("workstations");
  if (status === "running") {
    return (
      <Badge variant="default" className="gap-1 text-xs animate-pulse bg-amber-500">
        {t("sessions.statusRunning")}
      </Badge>
    );
  }
  if (status === "failed") {
    return (
      <Badge variant="destructive" className="gap-1 text-xs">
        {t("sessions.statusFailed")}
      </Badge>
    );
  }
  return (
    <Badge variant="secondary" className="gap-1 text-xs text-green-700 dark:text-green-400">
      {t("sessions.statusDone")}
    </Badge>
  );
}

function sessionLabel(s: SessionSummary): string {
  const cmd = s.command.length > 40 ? s.command.slice(0, 40) + "…" : s.command;
  return `[${s.status.toUpperCase()}] ${cmd} — ${formatDate(s.startedAt)}`;
}

export function WorkstationLiveLogTab({ workstationId }: WorkstationLiveLogTabProps) {
  const { t } = useTranslation("workstations");
  const { sessions, loading: sessionsLoading, refresh } = useWorkstationSessions(workstationId);
  const [selectedKey, setSelectedKey] = useState<string>("");
  const [displayLines, setDisplayLines] = useState<Array<{ stream: string; data: string; seq: number }>>([]);
  const [followTail, setFollowTail] = useState(true);
  const bottomRef = useRef<HTMLDivElement>(null);

  const selected = sessions.find((s) => s.sessionKey === selectedKey) ?? null;

  const { output, loading: outputLoading } = useSessionOutput(
    workstationId,
    selectedKey || null,
  );

  // Auto-select newest session on first load or when sessions refresh.
  useEffect(() => {
    const first = sessions[0];
    if (first && !selectedKey) {
      setSelectedKey(first.sessionKey);
    }
  }, [sessions, selectedKey]);

  // Sync display lines from output replay.
  useEffect(() => {
    if (output) {
      setDisplayLines(output.lines);
    } else {
      setDisplayLines([]);
    }
  }, [output]);

  // Follow tail.
  useEffect(() => {
    if (followTail && displayLines.length > 0) {
      bottomRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [displayLines, followTail]);

  const clear = () => setDisplayLines([]);
  const isRunning = selected?.status === "running";

  return (
    <div className="flex flex-col gap-2 p-4">
      {/* Header */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-sm font-medium">{t("liveLog.title")}</span>
          {selected && <StatusBadge status={selected.status} />}
          {selected?.exitCode !== undefined && (
            <span className="text-xs text-muted-foreground">exit {selected.exitCode}</span>
          )}
        </div>
        <div className="flex items-center gap-1 flex-wrap">
          {/* Session selector */}
          <div className="flex items-center gap-1">
            <select
              value={selectedKey}
              onChange={(e) => {
                setSelectedKey(e.target.value);
                setDisplayLines([]);
                setFollowTail(true);
              }}
              className="h-7 rounded-md border border-input bg-background px-2 text-xs font-mono text-foreground max-w-[200px] sm:max-w-[260px] truncate"
              aria-label={t("sessions.selectSession")}
            >
              {sessions.length === 0 && (
                <option value="" disabled>
                  {t("sessions.empty")}
                </option>
              )}
              {sessions.map((s) => (
                <option key={s.sessionKey} value={s.sessionKey}>
                  {sessionLabel(s)}
                </option>
              ))}
            </select>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 w-7 p-0"
              onClick={refresh}
              disabled={sessionsLoading}
              title={t("sessions.refreshSessions")}
            >
              <RefreshCw className={"h-3 w-3" + (sessionsLoading ? " animate-spin" : "")} />
            </Button>
          </div>
          <Button
            variant={followTail ? "secondary" : "ghost"}
            size="sm"
            className="h-7 gap-1 text-xs"
            onClick={() => setFollowTail((v) => !v)}
            title={t("liveLog.followTail")}
          >
            <ArrowDownToLine className="h-3 w-3" />
            {t("liveLog.followTail")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 gap-1 text-xs"
            onClick={clear}
            title={t("liveLog.clear")}
          >
            <Eraser className="h-3 w-3" />
            {t("liveLog.clear")}
          </Button>
        </div>
      </div>

      {/* Command preview */}
      {selected && (
        <div className="rounded bg-muted/50 px-2 py-1 font-mono text-xs text-muted-foreground truncate">
          {selected.command}
        </div>
      )}

      {/* Output pane */}
      <div className="relative rounded-md border bg-black/90 dark:bg-black overflow-hidden">
        {outputLoading && displayLines.length === 0 ? (
          <div className="flex items-center justify-center h-32 text-xs text-muted-foreground">
            {t("sessions.loadingOutput")}
          </div>
        ) : (
          <div
            className="h-80 overflow-y-auto p-3 font-mono text-xs text-green-400 dark:text-green-300 overscroll-contain"
            onScroll={(e) => {
              const el = e.currentTarget;
              const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 32;
              setFollowTail(atBottom);
            }}
          >
            {displayLines.length === 0 ? (
              <span className="text-muted-foreground italic">
                {selected
                  ? isRunning
                    ? t("sessions.waitingForOutput")
                    : t("sessions.noOutput")
                  : t("liveLog.empty")}
              </span>
            ) : (
              displayLines.map((line, i) => (
                <span
                  key={`${line.seq}-${i}`}
                  className={line.stream === "stderr" ? "text-red-400 dark:text-red-300" : undefined}
                >
                  {line.data}
                </span>
              ))
            )}
            <div ref={bottomRef} />
          </div>
        )}
      </div>

      {!selectedKey && sessions.length === 0 && !sessionsLoading && (
        <p className="text-xs text-muted-foreground text-center">{t("sessions.goclawOnly")}</p>
      )}
    </div>
  );
}
