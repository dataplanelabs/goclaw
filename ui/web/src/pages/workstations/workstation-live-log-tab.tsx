import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Eraser, ArrowDownToLine } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useWorkstationLiveLog } from "./hooks/use-workstation-live-log";

interface WorkstationLiveLogTabProps {
  workstationId: string;
}

export function WorkstationLiveLogTab({ workstationId }: WorkstationLiveLogTabProps) {
  const { t } = useTranslation("workstations");
  const { lines, active, clear } = useWorkstationLiveLog(workstationId);
  const [followTail, setFollowTail] = useState(true);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (followTail && lines.length > 0) {
      bottomRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [lines, followTail]);

  return (
    <div className="flex flex-col gap-2 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium">{t("liveLog.title")}</span>
          {active ? (
            <Badge variant="default" className="gap-1 text-xs animate-pulse">
              {t("liveLog.live")}
            </Badge>
          ) : (
            <Badge variant="secondary" className="text-xs">
              {t("liveLog.idle")}
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-1">
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

      <div className="relative rounded-md border bg-black/90 dark:bg-black overflow-hidden">
        <div
          className="h-80 overflow-y-auto p-3 font-mono text-xs text-green-400 dark:text-green-300 overscroll-contain"
          onScroll={(e) => {
            const el = e.currentTarget;
            const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 32;
            setFollowTail(atBottom);
          }}
        >
          {lines.length === 0 ? (
            <span className="text-muted-foreground italic">{t("liveLog.empty")}</span>
          ) : (
            lines.map((line, i) => (
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
      </div>
    </div>
  );
}
