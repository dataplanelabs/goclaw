import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Methods } from "@/api/protocol";
import { Button } from "@/components/ui/button";
import { useWs } from "@/hooks/use-ws";

export interface TeamAnalyticsExportButtonProps {
  channelInstanceId: string;
  channelName: string;
}

interface ExportResponse {
  jsonl: string;
  count: number;
  bytes: number;
}

export function TeamAnalyticsExportButton({
  channelInstanceId,
  channelName,
}: TeamAnalyticsExportButtonProps) {
  const { t } = useTranslation("channels");
  const ws = useWs();
  const [exporting, setExporting] = useState(false);
  const [status, setStatus] = useState<string | null>(null);

  async function onExport() {
    setExporting(true);
    setStatus(null);
    try {
      const res = await ws.call<ExportResponse>(
        Methods.CHANNELS_TEAM_REPLIES_EXPORT_JSONL,
        { channel_instance_id: channelInstanceId },
      );
      if (!res.jsonl) {
        setStatus(t("teamAnalytics.noCaptures"));
        return;
      }
      const blob = new Blob([res.jsonl], { type: "application/jsonl" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      const date = new Date().toISOString().slice(0, 10);
      a.download = `team-replies-${channelName}-${date}.jsonl`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
      setStatus(t("teamAnalytics.exportSuccess", { count: res.count, bytes: res.bytes }));
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setStatus(msg);
    } finally {
      setExporting(false);
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <Button onClick={onExport} disabled={exporting} variant="secondary">
        {exporting ? t("teamAnalytics.exporting") : t("teamAnalytics.exportJsonl")}
      </Button>
      {status && (
        <div className="text-muted-foreground text-xs">{status}</div>
      )}
    </div>
  );
}
