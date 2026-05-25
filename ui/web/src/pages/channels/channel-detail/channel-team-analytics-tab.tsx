import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { Methods } from "@/api/protocol";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useWs } from "@/hooks/use-ws";

import {
  TeamAnalyticsHistogram,
} from "./team-analytics-histogram";
import {
  TeamAnalyticsTable,
  type TeamReplyEvaluation,
} from "./team-analytics-table";
import { TeamAnalyticsExportButton } from "./team-analytics-export-button";

export interface ChannelTeamAnalyticsTabProps {
  channelInstanceId: string;
  channelName: string;
  initialConfig?: {
    capture_team_replies?: boolean;
    judge_evaluation?: boolean;
    judge_agent_key?: string;
  };
}

interface ListResponse {
  evaluations: TeamReplyEvaluation[];
  total: number;
}

function errMsg(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export function ChannelTeamAnalyticsTab({
  channelInstanceId,
  channelName,
  initialConfig,
}: ChannelTeamAnalyticsTabProps) {
  const { t } = useTranslation("channels");
  const ws = useWs();
  const [rows, setRows] = useState<TeamReplyEvaluation[]>([]);
  const [threadFilter, setThreadFilter] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [status, setStatus] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [capture, setCapture] = useState(initialConfig?.capture_team_replies ?? false);
  const [judge, setJudge] = useState(initialConfig?.judge_evaluation ?? false);
  const [judgeKey, setJudgeKey] = useState(initialConfig?.judge_agent_key ?? "");
  const [restartHint, setRestartHint] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    // Skip re-sync if operator has unsaved edits — avoids clobbering form
    // state when a cross-tab update refreshes initialConfig mid-typing.
    if (dirty) return;
    setCapture(initialConfig?.capture_team_replies ?? false);
    setJudge(initialConfig?.judge_evaluation ?? false);
    setJudgeKey(initialConfig?.judge_agent_key ?? "");
  }, [initialConfig?.capture_team_replies, initialConfig?.judge_evaluation, initialConfig?.judge_agent_key, dirty]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const params: Record<string, unknown> = {
        channel_instance_id: channelInstanceId,
        limit: 50,
      };
      if (threadFilter.trim()) {
        params.thread_key = threadFilter.trim();
      }
      const res = await ws.call<ListResponse>(
        Methods.CHANNELS_TEAM_REPLIES_LIST,
        params,
      );
      setRows(res.evaluations ?? []);
      setLoadError(null);
    } catch (err) {
      setLoadError(errMsg(err));
    } finally {
      setLoading(false);
    }
  }, [ws, channelInstanceId, threadFilter]);

  useEffect(() => {
    load();
    const id = setInterval(() => {
      if (document.visibilityState !== "visible") return;
      if (!ws.isConnected) return;
      load();
    }, 30_000);
    return () => clearInterval(id);
  }, [load, ws]);

  const scores = useMemo(
    () =>
      rows
        .map((r) => r.diff_score)
        .filter((v): v is number => typeof v === "number"),
    [rows],
  );

  const threadOptions = useMemo(() => {
    const set = new Set<string>();
    for (const r of rows) set.add(r.thread_key);
    return Array.from(set).sort();
  }, [rows]);

  async function saveToggle() {
    setStatus(null);
    setRestartHint(null);
    try {
      const resp = await ws.call<{ hint?: string }>(Methods.CHANNELS_TEAM_CAPTURE_TOGGLE, {
        channel_instance_id: channelInstanceId,
        capture_team_replies: capture,
        judge_evaluation: judge,
        judge_agent_key: judgeKey.trim(),
      });
      setStatus(t("teamAnalytics.configSaved"));
      setDirty(false);
      if (resp?.hint?.toLowerCase().includes("restart")) {
        setRestartHint(t("teamAnalytics.restartRequired"));
      }
    } catch (err) {
      setStatus(errMsg(err));
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-base font-semibold mb-1">{t("teamAnalytics.title")}</h3>
        <p className="text-sm text-muted-foreground">{t("teamAnalytics.description")}</p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 rounded-md border p-4">
        <div className="flex items-center gap-3">
          <Switch
            id="capture-toggle"
            checked={capture}
            onCheckedChange={(v) => { setCapture(Boolean(v)); setDirty(true); }}
          />
          <Label htmlFor="capture-toggle">{t("teamAnalytics.captureToggle")}</Label>
        </div>
        <div className="flex items-center gap-3">
          <Switch
            id="judge-toggle"
            checked={judge}
            onCheckedChange={(v) => { setJudge(Boolean(v)); setDirty(true); }}
          />
          <Label htmlFor="judge-toggle">{t("teamAnalytics.judgeToggle")}</Label>
        </div>
        <div className="sm:col-span-2">
          <Label htmlFor="judge-agent">{t("teamAnalytics.judgeAgent")}</Label>
          <Input
            id="judge-agent"
            value={judgeKey}
            onChange={(e) => { setJudgeKey(e.target.value); setDirty(true); }}
            placeholder="team-reply-judge"
            className="text-base md:text-sm mt-1"
          />
        </div>
        <div className="sm:col-span-2 flex justify-end">
          <Button onClick={saveToggle}>{t("teamAnalytics.save")}</Button>
        </div>
      </div>

      <div>
        <h4 className="text-sm font-semibold mb-2">{t("teamAnalytics.diffDistribution")}</h4>
        <TeamAnalyticsHistogram scores={scores} />
      </div>

      <div>
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-2">
          <h4 className="text-sm font-semibold">{t("teamAnalytics.recentCaptures")}</h4>
          <div className="flex items-center gap-2">
            <Label htmlFor="thread-filter" className="text-xs whitespace-nowrap">
              {t("teamAnalytics.filterByThread")}
            </Label>
            <select
              id="thread-filter"
              value={threadFilter}
              onChange={(e) => setThreadFilter(e.target.value)}
              className="border rounded text-base md:text-sm h-8 px-2 bg-background"
            >
              <option value="">{t("teamAnalytics.allThreads")}</option>
              {threadOptions.map((tk) => (
                <option key={tk} value={tk}>
                  {tk}
                </option>
              ))}
            </select>
            <Button size="sm" variant="outline" onClick={load} disabled={loading}>
              ↻
            </Button>
          </div>
        </div>
        <TeamAnalyticsTable rows={rows} />
      </div>

      <div className="flex justify-end">
        <TeamAnalyticsExportButton
          channelInstanceId={channelInstanceId}
          channelName={channelName}
        />
      </div>

      {restartHint && (
        <div className="text-xs text-amber-600 dark:text-amber-400 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2">
          {restartHint}
        </div>
      )}
      {status && (
        <div className="text-xs text-muted-foreground">{status}</div>
      )}
      {loadError && (
        <div className="text-xs text-destructive">{loadError}</div>
      )}
    </div>
  );
}
