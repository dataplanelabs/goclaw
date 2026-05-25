import { useMemo, useState } from "react";
import { ChevronDownIcon } from "lucide-react";
import { Accordion } from "radix-ui";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";

import { scoreBgClass, truncateThreadKey, type ThreadGroup } from "./aggregate-threads";
import { TeamAnalyticsDetailDialog } from "./team-analytics-detail-dialog";
import { TeamAnalyticsThreadPanel } from "./team-analytics-thread-panel";
import type { TeamReplyEvaluation } from "./team-reply-types";

export interface TeamAnalyticsThreadListProps {
  threads: ThreadGroup[];
  threadFilter?: string;
}

export function TeamAnalyticsThreadList({ threads, threadFilter }: TeamAnalyticsThreadListProps) {
  const { t } = useTranslation("channels");
  const [open, setOpen] = useState<string[]>([]);
  const [selected, setSelected] = useState<TeamReplyEvaluation | null>(null);

  const filtered = useMemo(() => {
    if (!threadFilter) return threads;
    return threads.filter((g) => g.thread_key === threadFilter);
  }, [threads, threadFilter]);

  if (filtered.length === 0) {
    return (
      <div className="text-muted-foreground text-sm py-8 text-center">
        {threadFilter ? t("teamAnalytics.noMatchingThreads") : t("teamAnalytics.noCaptures")}
      </div>
    );
  }

  const allKeys = filtered.map((g) => g.thread_key);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted-foreground">
          {t("teamAnalytics.threadCount", { count: filtered.length })}
        </span>
        <div className="flex gap-2">
          <Button size="sm" variant="ghost" onClick={() => setOpen(allKeys)}>
            {t("teamAnalytics.expandAll")}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setOpen([])}>
            {t("teamAnalytics.collapseAll")}
          </Button>
        </div>
      </div>
      <Accordion.Root type="multiple" value={open} onValueChange={setOpen} className="flex flex-col">
        {filtered.map((g) => (
          <Accordion.Item key={g.thread_key} value={g.thread_key} className="border-b">
            <Accordion.Header>
              <Accordion.Trigger className="group flex w-full items-center justify-between gap-3 px-2 py-3 text-left hover:bg-muted/50 transition">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
                  {g.customer_name ? (
                    <>
                      <div className="text-sm font-medium truncate">{g.customer_name}</div>
                      <div className="font-mono text-2xs text-muted-foreground/70 truncate hidden sm:block" title={g.thread_key}>
                        {truncateThreadKey(g.thread_key)}
                      </div>
                    </>
                  ) : (
                    <div className="font-mono text-xs text-muted-foreground truncate">
                      {truncateThreadKey(g.thread_key)}
                    </div>
                  )}
                  <div className="text-xs text-muted-foreground whitespace-nowrap">
                    {t("teamAnalytics.threadCaptureCount", { count: g.capture_count })}
                  </div>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span className={`inline-flex items-center rounded px-2 py-0.5 font-mono text-xs ${scoreBgClass(g.avg_diff_score)}`}>
                    {g.avg_diff_score === null ? "—" : g.avg_diff_score.toFixed(2)}
                  </span>
                  <span className="text-xs text-muted-foreground whitespace-nowrap">
                    {relativeTime(g.last_activity)}
                  </span>
                </div>
              </Accordion.Trigger>
            </Accordion.Header>
            <Accordion.Content className="bg-muted/20">
              <TeamAnalyticsThreadPanel captures={g.captures} onSelectCapture={setSelected} />
            </Accordion.Content>
          </Accordion.Item>
        ))}
      </Accordion.Root>
      <TeamAnalyticsDetailDialog capture={selected} onClose={() => setSelected(null)} />
    </div>
  );
}

function relativeTime(iso: string): string {
  if (!iso) return "";
  const ms = Date.now() - new Date(iso).getTime();
  if (ms < 60_000) return `${Math.floor(ms / 1000)}s`;
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)}m`;
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)}h`;
  return `${Math.floor(ms / 86_400_000)}d`;
}
