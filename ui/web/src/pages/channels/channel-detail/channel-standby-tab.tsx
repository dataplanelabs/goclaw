import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { Methods } from "@/api/protocol";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useWs } from "@/hooks/use-ws";

import { parseSchedule, type Schedule } from "./channel-standby-schema";
import {
  ChannelStandbyThreadList,
  type ThreadSchedule,
} from "./channel-standby-thread-list";

export interface ChannelStandbyTabProps {
  channelInstanceId: string;
}

export function ChannelStandbyTab({ channelInstanceId }: ChannelStandbyTabProps) {
  const { t } = useTranslation("channels");
  const ws = useWs();
  const [scheduleText, setScheduleText] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [threads, setThreads] = useState<ThreadSchedule[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await ws.call<{ schedule: Schedule | null }>(
        Methods.CHANNELS_SCHEDULE_GET,
        { channel_instance_id: channelInstanceId },
      );
      setScheduleText(res.schedule ? JSON.stringify(res.schedule, null, 2) : "");
      const tres = await ws.call<{ threads: ThreadSchedule[] }>(
        Methods.CHANNELS_THREAD_SCHEDULE_LIST,
        { channel_instance_id: channelInstanceId },
      );
      setThreads(tres.threads ?? []);
    } catch (err) {
      setStatus(t("standby.loadError", { error: errMsg(err) }));
    } finally {
      setLoading(false);
    }
  }, [ws, channelInstanceId, t]);

  useEffect(() => {
    load();
  }, [load]);

  const parsed = useMemo(() => parseSchedule(scheduleText), [scheduleText]);

  async function save() {
    if (parsed.error) {
      setValidationError(t("standby.validationError", { error: parsed.error }));
      return;
    }
    setValidationError(null);
    try {
      await ws.call(Methods.CHANNELS_SCHEDULE_SET, {
        channel_instance_id: channelInstanceId,
        schedule: parsed.schedule,
      });
      setStatus(t("standby.saved"));
    } catch (err) {
      setStatus(t("standby.saveError", { error: errMsg(err) }));
    }
  }

  async function deleteSchedule() {
    if (!window.confirm(t("standby.deleteConfirm"))) return;
    try {
      await ws.call(Methods.CHANNELS_SCHEDULE_DELETE, {
        channel_instance_id: channelInstanceId,
      });
      setScheduleText("");
      setStatus(t("standby.deleted"));
    } catch (err) {
      setStatus(errMsg(err));
    }
  }

  async function deleteThread(threadKey: string) {
    try {
      await ws.call(Methods.CHANNELS_THREAD_SCHEDULE_DELETE, {
        channel_instance_id: channelInstanceId,
        thread_key: threadKey,
      });
      await load();
    } catch (err) {
      setStatus(errMsg(err));
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">{t("standby.title")}</h2>
        <p className="mt-1 text-sm text-muted-foreground">{t("standby.description")}</p>
      </div>

      <div className="space-y-2">
        <label className="text-sm font-medium" htmlFor="schedule-json">
          {t("standby.rawJsonLabel")}
        </label>
        <Textarea
          id="schedule-json"
          value={scheduleText}
          onChange={(e) => setScheduleText(e.target.value)}
          placeholder='{"default_mode":"active","windows":[{"mode":"standby","weekday":"mon-fri","start":"09:00","end":"17:00","tz":"Asia/Saigon"}]}'
          className="min-h-[220px] font-mono text-base md:text-sm"
        />
        <p className="text-xs text-muted-foreground">{t("standby.rawJsonHint")}</p>
      </div>

      {validationError && <p className="text-sm text-destructive">{validationError}</p>}
      {status && <p className="text-sm text-muted-foreground">{status}</p>}

      <div className="flex flex-wrap gap-2">
        <Button onClick={save} disabled={loading || !!parsed.error}>
          {t("standby.save")}
        </Button>
        <Button variant="outline" onClick={deleteSchedule} disabled={loading}>
          {t("standby.deleteSchedule")}
        </Button>
      </div>

      <ChannelStandbyThreadList
        threads={threads}
        onDelete={deleteThread}
        labels={{
          title: t("standby.threadOverrides"),
          empty: t("standby.threadOverridesEmpty"),
          thread: t("standby.threadKey"),
          expires: t("standby.expiresAt"),
          reason: t("standby.reason"),
          createdBy: t("standby.createdBy"),
          deleteLabel: t("standby.rowDelete"),
        }}
      />
    </div>
  );
}

function errMsg(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
