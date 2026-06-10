import { useTranslation } from "react-i18next";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import { scoreBgClass } from "./aggregate-threads";
import { CaptureContent } from "./capture-content";
import type { TeamReplyEvaluation } from "./team-reply-types";

export interface TeamAnalyticsDetailDialogProps {
  capture: TeamReplyEvaluation | null;
  onClose: () => void;
}

export function TeamAnalyticsDetailDialog({ capture, onClose }: TeamAnalyticsDetailDialogProps) {
  const { t } = useTranslation("channels");
  const open = capture !== null;
  const score = capture?.diff_score ?? null;
  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("teamAnalytics.detailTitle")}</DialogTitle>
          <DialogDescription>
            {capture?.captured_at ? new Date(capture.captured_at).toLocaleString() : ""}
          </DialogDescription>
        </DialogHeader>
        {capture && (
          <div className="flex flex-col gap-4 text-sm">
            <Section label={t("teamAnalytics.detailCustomerMessage")} body={capture.customer_message} rich role="user" />
            <Section label={t("teamAnalytics.detailTeamReply")} body={capture.team_reply} rich role="assistant" />
            {capture.hypothesized_bot_reply && (
              <Section label={t("teamAnalytics.detailBotWouldSay")} body={capture.hypothesized_bot_reply} rich role="assistant" />
            )}
            {capture.diff_reasoning && (
              <Section label={t("teamAnalytics.detailDiffReasoning")} body={capture.diff_reasoning} />
            )}
            <div className="flex items-center justify-between gap-3 pt-2 border-t text-xs text-muted-foreground">
              <span className={`inline-flex items-center rounded px-2 py-0.5 font-mono ${scoreBgClass(score)}`}>
                {score === null ? "—" : score.toFixed(2)}
              </span>
              <span className="font-mono truncate">
                {capture.judge_agent_key ?? ""}
                {capture.judge_model ? ` · ${capture.judge_model}` : ""}
                {typeof capture.judge_latency_ms === "number" ? ` · ${capture.judge_latency_ms}ms` : ""}
              </span>
            </div>
            {capture.judge_error && (
              <div className="rounded border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                {capture.judge_error}
              </div>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}

function Section({
  label,
  body,
  rich,
  role,
}: {
  label: string;
  body: string | undefined;
  rich?: boolean;
  role?: "user" | "assistant";
}) {
  if (!body) return null;
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted-foreground mb-1">{label}</div>
      {rich ? (
        <CaptureContent content={body} role={role ?? "user"} />
      ) : (
        <div className="whitespace-pre-wrap break-words">{body}</div>
      )}
    </div>
  );
}
