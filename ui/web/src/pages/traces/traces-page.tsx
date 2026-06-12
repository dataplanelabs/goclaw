import { useState, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Activity, GitFork, RefreshCw, Square, Bot, User, Users, Clock, Network, Globe, CheckCircle2, XCircle, Loader2, CircleDot, CircleDashed } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { Pagination } from "@/components/shared/pagination";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { formatDate, formatDuration, formatTokens, computeDurationMs } from "@/lib/format";
import { formatUserLabel } from "@/lib/format-user-label";
import { useContactResolver } from "@/hooks/use-contact-resolver";
import { useTraces, type TraceData } from "./hooks/use-traces";
import { TraceDetailDialog } from "./trace-detail-dialog";
import { useMinLoading } from "@/hooks/use-min-loading";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";
import { useUiStore } from "@/stores/use-ui-store";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import { useChannelInstances } from "@/pages/channels/hooks/use-channel-instances";
import { useQueryClient } from "@tanstack/react-query";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import { Methods, Events } from "@/api/protocol";
import { queryKeys } from "@/lib/query-keys";
import { toast } from "@/stores/use-toast-store";

/**
 * Clean preview text shown on the row line 2: strip the leading chat_title
 * prefix (group name or DM sender name often prepended on inbound), drop the
 * `[From: <Name> (uid:...)]` annotation (sender/UID live in the detail modal),
 * and replace `<media:*>` tags with a `[media]` placeholder.
 */
function cleanPreview(text: string, chatTitle?: string): string {
  if (!text) return text;
  let out = text;
  if (chatTitle && out.startsWith(chatTitle)) {
    out = out.slice(chatTitle.length).trimStart();
  }
  out = out.replace(/^\[From:[^\]]*\]\s*/, "");
  out = out.replace(/<media:\w+>/g, "[media]");
  return out.trim();
}

/** Parse session_key to extract source type: Direct, Group, Cron, Team, WS */
function parseSourceType(sessionKey: string): { type: string; topic?: string } {
  if (!sessionKey) return { type: "unknown" };
  if (sessionKey.includes(":cron:")) return { type: "cron" };
  if (sessionKey.includes(":team:")) return { type: "team" };
  const topicMatch = sessionKey.match(/:topic:(\d+)/);
  if (topicMatch) return { type: "group", topic: topicMatch[1] };
  if (sessionKey.includes(":group:")) return { type: "group" };
  if (sessionKey.includes(":ws:")) return { type: "ws" };
  if (sessionKey.includes(":direct:")) return { type: "direct" };
  return { type: "unknown" };
}

const SOURCE_ICONS: Record<string, typeof Bot> = {
  cron: Clock,
  team: Network,
  group: Users,
  direct: User,
  ws: Globe,
};

/** Bare chat/group id for a "group:<channel>:<chatID>" / "guild:..." key, used to resolve the group contact row keyed by bare id. */
function bareGroupChatId(userId: string): string | undefined {
  if (!userId.startsWith("group:") && !userId.startsWith("guild:")) return undefined;
  const parts = userId.split(":");
  if (parts.length < 3) return undefined;
  return parts.slice(2).join(":");
}

export function TracesPage() {
  const { t } = useTranslation("traces");
  const { t: tc } = useTranslation("common");
  const tz = useUiStore((s) => s.timezone);
  const globalPageSize = useUiStore((s) => s.pageSize);
  const setGlobalPageSize = useUiStore((s) => s.setPageSize);
  const [agentFilter, setAgentFilter] = useState<string>();
  const [channelFilter, setChannelFilter] = useState<string>();
  const [userFilter, setUserFilter] = useState<string>();
  const [sourceTypeFilter, setSourceTypeFilter] = useState<string>();
  const [selectedTraceId, setSelectedTraceId] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSizeRaw] = useState(globalPageSize);
  const setPageSize = (size: number) => { setPageSizeRaw(size); setPage(1); setGlobalPageSize(size); };

  const ws = useWs();
  const queryClient = useQueryClient();

  // Invalidate traces list on immediate status events (no need to wait for 5s flush).
  useWsEvent(Events.TRACE_STATUS, useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.traces.all }),
    [queryClient],
  ));

  const { agents } = useAgents();
  const { instances: channels } = useChannelInstances();

  const agentMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const a of agents) map.set(a.id, a.display_name || a.agent_key || a.id);
    return map;
  }, [agents]);

  const [abortingRunId, setAbortingRunId] = useState<string | null>(null);

  const { traces, total, loading, fetching, refresh, getTrace, retryTrace } = useTraces({
    agentId: agentFilter,
    channel: channelFilter,
    userId: userFilter,
    sourceType: sourceTypeFilter,
    limit: pageSize,
    offset: (page - 1) * pageSize,
  });

  // Resolve the prefixed ids AND the bare group/guild chatIDs (group contact rows are keyed by bare id).
  const traceUserIds = useMemo(() => {
    const ids = new Set<string>();
    for (const tr of traces) {
      if (!tr.user_id) continue;
      ids.add(tr.user_id);
      const bare = bareGroupChatId(tr.user_id);
      if (bare) ids.add(bare);
    }
    return [...ids];
  }, [traces]);
  const { resolve } = useContactResolver(traceUserIds);

  // Distinct recipients on the current page: user_id → human label. Prefer a real
  // chat_title (set on group traces) over a slug; a group also appears in cron traces
  // with empty chat_title, so chat_title must win when any trace provides it.
  const recipientOptions = useMemo(() => {
    const labels = new Map<string, string>();
    for (const tr of traces) {
      if (!tr.user_id) continue;
      const existing = labels.get(tr.user_id);
      if (tr.chat_title) {
        labels.set(tr.user_id, tr.chat_title);
      } else if (existing === undefined) {
        labels.set(tr.user_id, formatUserLabel(tr.user_id, resolve));
      }
    }
    return [...labels.entries()].sort((a, b) => a[1].localeCompare(b[1]));
  }, [traces, resolve]);

  const spinning = useMinLoading(fetching);
  const showSkeleton = useDeferredLoading(loading && traces.length === 0);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  const handleAbortRun = useCallback(
    async (trace: TraceData, e: React.MouseEvent) => {
      e.stopPropagation();
      if (!ws.isConnected || abortingRunId) return;
      setAbortingRunId(trace.run_id);
      try {
        const res = await ws.call(Methods.CHAT_ABORT, {
          sessionKey: trace.session_key,
          runId: trace.run_id,
        }) as {
          aborted?: boolean;
          stopped?: boolean;
          forced?: boolean;
          orphaned?: boolean;
          alreadyAborting?: boolean;
          notFound?: boolean;
          unauthorized?: boolean;
        };
        if (res?.stopped) {
          toast.success(t("toast.abortStopped"));
        } else if (res?.forced) {
          toast.warning(t("toast.abortForced"));
        } else if (res?.orphaned) {
          toast.info(t("toast.abortOrphan"));
        } else if (res?.alreadyAborting) {
          toast.info(t("toast.abortAlreadyAborting"));
        } else if (res?.unauthorized) {
          toast.error(t("toast.abortUnauthorized"));
        } else if (res?.notFound) {
          toast.info(t("toast.abortNotFound"));
        } else {
          toast.error(t("toast.abortFailed"));
        }
        refresh();
      } catch {
        toast.error(t("toast.abortFailed"));
      } finally {
        // Auto re-enable within 5s max (3s grace + 2s buffer) in case WS event is delayed.
        setTimeout(() => setAbortingRunId(null), 5000);
        setAbortingRunId(null);
      }
    },
    [ws, t, refresh, abortingRunId],
  );

  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <Button variant="outline" size="sm" onClick={refresh} disabled={spinning} className="gap-1">
            <RefreshCw className={"h-3.5 w-3.5" + (spinning ? " animate-spin" : "")} /> {tc("refresh")}
          </Button>
        }
      />

      <div className="mt-4 flex flex-wrap items-center gap-2">
        {/* Agent filter */}
        <Select
          value={agentFilter ?? "__all__"}
          onValueChange={(v) => { setAgentFilter(v === "__all__" ? undefined : v); setPage(1); }}
        >
          <SelectTrigger className="h-8 w-44 text-xs">
            <SelectValue placeholder={t("allAgents")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t("allAgents")}</SelectItem>
            {agents.map((a) => (
              <SelectItem key={a.id} value={a.id}>{a.display_name || a.agent_key || a.id}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Channel filter */}
        <Select
          value={channelFilter ?? "__all__"}
          onValueChange={(v) => { setChannelFilter(v === "__all__" ? undefined : v); setPage(1); }}
        >
          <SelectTrigger className="h-8 w-44 text-xs">
            <SelectValue placeholder={t("allChannels")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t("allChannels")}</SelectItem>
            {channels.map((ch) => (
              <SelectItem key={ch.id} value={ch.name}>{ch.display_name || ch.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Delivered-to (user/group) filter */}
        <Select
          value={userFilter ?? "__all__"}
          onValueChange={(v) => { setUserFilter(v === "__all__" ? undefined : v); setPage(1); }}
        >
          <SelectTrigger className="h-8 w-44 text-xs">
            <SelectValue placeholder={t("allRecipients")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t("allRecipients")}</SelectItem>
            {recipientOptions.map(([id, label]) => (
              <SelectItem key={id} value={id}>{label}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Invocation type filter */}
        <Select
          value={sourceTypeFilter ?? "__all__"}
          onValueChange={(v) => { setSourceTypeFilter(v === "__all__" ? undefined : v); setPage(1); }}
        >
          <SelectTrigger className="h-8 w-44 text-xs">
            <SelectValue placeholder={t("allInvocationTypes")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">{t("allInvocationTypes")}</SelectItem>
            <SelectItem value="cron">{t("source.cron")}</SelectItem>
            <SelectItem value="group">{t("source.group")}</SelectItem>
            <SelectItem value="direct">{t("source.direct")}</SelectItem>
            <SelectItem value="team">{t("source.team")}</SelectItem>
            <SelectItem value="ws">{t("source.ws")}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="mt-4">
        {showSkeleton ? (
          <TableSkeleton rows={8} />
        ) : traces.length === 0 ? (
          <EmptyState
            icon={Activity}
            title={t("emptyTitle")}
            description={t("emptyDescription")}
          />
        ) : (
          <div className="rounded-md border overflow-x-auto">
            <table className="w-full min-w-[600px] text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium max-w-[40%]">{t("columns.name")}</th>
                  <th className="px-3 py-3 text-center font-medium w-10"></th>
                  <th className="px-4 py-3 text-left font-medium whitespace-nowrap">{t("columns.tokens")}</th>
                  <th className="px-4 py-3 text-center font-medium whitespace-nowrap">{t("columns.spans")}</th>
                  <th className="px-4 py-3 text-right font-medium whitespace-nowrap">{t("columns.time")}</th>
                </tr>
              </thead>
              <tbody>
                {traces.map((trace: TraceData) => {
                  const source = parseSourceType(trace.session_key);
                  const rawUserLabel = formatUserLabel(trace.user_id, resolve);
                  // Suppress the redundant `Channel chatId` chip when the user_id is
                  // a group key — channel badge + chat_title below already carry it,
                  // and the full chat_id lives in the trace detail modal. Cron rows
                  // are the exception: they carry a group user_id but no channel/chat
                  // context, so show the resolved delivered-to name there.
                  const userLabel =
                    source.type === "cron"
                      ? rawUserLabel
                      : trace.user_id?.startsWith("group:")
                        ? ""
                        : rawUserLabel;
                  const agentName = trace.agent_id ? agentMap.get(trace.agent_id) : undefined;
                  const SourceIcon = SOURCE_ICONS[source.type] || Bot;

                  return (
                    <tr
                      key={trace.id}
                      className="cursor-pointer border-b last:border-0 hover:bg-muted/30"
                      onClick={() => setSelectedTraceId(trace.id)}
                    >
                      <td className="px-4 py-2.5 max-w-[300px] lg:max-w-[500px] xl:max-w-[700px] 2xl:max-w-[900px]">
                        <div className="flex items-center gap-1.5 text-sm font-medium min-w-0">
                          {trace.parent_trace_id && (
                            <GitFork className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                          )}
                          <span className="truncate">{agentName || trace.name || t("unnamed")}</span>
                          {(trace.chat_title || userLabel) && (
                            <>
                              <span className="shrink-0 text-muted-foreground">·</span>
                              <span
                                className="truncate text-muted-foreground max-w-[180px] lg:max-w-[360px] xl:max-w-[560px] 2xl:max-w-[760px]"
                                title={trace.chat_title || userLabel}
                              >
                                {trace.chat_title || userLabel}
                              </span>
                            </>
                          )}
                        </div>
                        <div className="mt-0.5 flex items-center gap-1">
                          <Badge variant="outline" className="shrink-0 gap-0.5 text-2xs px-1.5 py-0">
                            <SourceIcon className="h-2.5 w-2.5" />
                            {t(`source.${source.type}`)}
                            {source.topic && ` #${source.topic}`}
                          </Badge>
                          {trace.channel && (
                            <Badge variant="secondary" className="shrink-0 text-2xs px-1.5 py-0">
                              {trace.channel}
                            </Badge>
                          )}
                          {trace.input_preview && (
                            <span className="truncate text-xs text-muted-foreground">
                              {cleanPreview(trace.input_preview, trace.chat_title)}
                            </span>
                          )}
                        </div>
                      </td>
                      <td className="px-3 py-2.5 text-center">
                        <div className="flex items-center justify-center gap-1">
                          <StatusIcon status={trace.status} />
                          {(trace.status === "running") && (
                            <Button
                              variant="destructive"
                              size="icon-xs"
                              onClick={(e) => handleAbortRun(trace, e)}
                              disabled={abortingRunId === trace.run_id}
                              title={t("stopRun")}
                            >
                              <Square className="h-3 w-3" />
                            </Button>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-2.5 text-muted-foreground whitespace-nowrap">
                        <div>{formatTokens(trace.total_input_tokens)} / {formatTokens(trace.total_output_tokens)}</div>
                        {(trace.metadata?.total_cache_read_tokens ?? 0) > 0 && (
                          <div className="text-xs text-green-400">
                            {formatTokens(trace.metadata!.total_cache_read_tokens!)} {t("cached")}
                          </div>
                        )}
                      </td>
                      <td className="px-4 py-2.5 text-center text-muted-foreground">
                        {trace.span_count}
                      </td>
                      <td className="px-4 py-2.5 text-right text-muted-foreground whitespace-nowrap">
                        <div>{formatDate(trace.start_time, tz)}</div>
                        <div className="text-xs">{formatDuration(trace.duration_ms || computeDurationMs(trace.start_time, trace.end_time))}</div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            <Pagination
              page={page}
              pageSize={pageSize}
              total={total}
              totalPages={totalPages}
              onPageChange={setPage}
              onPageSizeChange={(size) => { setPageSize(size); setPage(1); }}
            />
          </div>
        )}
      </div>

      {selectedTraceId && (
        <TraceDetailDialog
          traceId={selectedTraceId}
          onClose={() => setSelectedTraceId(null)}
          getTrace={getTrace}
          retryTrace={retryTrace}
          onRetried={() => refresh()}
          onNavigateTrace={setSelectedTraceId}
          onAbortRun={handleAbortRun}
        />
      )}
    </div>
  );
}

function StatusIcon({ status }: { status: string }) {
  if (status === "ok" || status === "success" || status === "completed") {
    return <CheckCircle2 className="h-4 w-4 text-green-500" />;
  }
  if (status === "error" || status === "failed") {
    return <XCircle className="h-4 w-4 text-destructive" />;
  }
  if (status === "running") {
    return <Loader2 className="h-4 w-4 text-blue-500 animate-spin" />;
  }
  if (status === "pending") {
    return <CircleDashed className="h-4 w-4 text-muted-foreground" />;
  }
  return <CircleDot className="h-4 w-4 text-muted-foreground" />;
}
