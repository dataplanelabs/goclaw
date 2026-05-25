import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";

import { scoreBgClass } from "./aggregate-threads";
import type { TeamReplyEvaluation } from "./team-reply-types";

const PAGE = 20;

export interface TeamAnalyticsThreadPanelProps {
  captures: TeamReplyEvaluation[];
  onSelectCapture: (capture: TeamReplyEvaluation) => void;
}

export function TeamAnalyticsThreadPanel({ captures, onSelectCapture }: TeamAnalyticsThreadPanelProps) {
  const { t } = useTranslation("channels");
  const [shown, setShown] = useState(PAGE);
  const hasOlder = captures.length > shown;
  const visible = captures.slice(Math.max(0, captures.length - shown));
  return (
    <div className="flex flex-col gap-3 px-4 py-3">
      {hasOlder && (
        <div className="flex justify-center">
          <Button size="sm" variant="outline" onClick={() => setShown((n) => n + PAGE)}>
            {t("teamAnalytics.loadOlderCaptures", { count: PAGE })}
          </Button>
        </div>
      )}
      {visible.map((c) => (
        <CaptureBubbles
          key={c.id}
          capture={c}
          onSelect={onSelectCapture}
          viewLabel={t("teamAnalytics.viewDetails")}
          failedLabel={t("teamAnalytics.judgeFailed")}
          pendingLabel={t("teamAnalytics.judgePending")}
        />
      ))}
    </div>
  );
}

function CaptureBubbles({
  capture,
  onSelect,
  viewLabel,
  failedLabel,
  pendingLabel,
}: {
  capture: TeamReplyEvaluation;
  onSelect: (capture: TeamReplyEvaluation) => void;
  viewLabel: string;
  failedLabel: string;
  pendingLabel: string;
}) {
  const customer = capture.customer_message?.trim();
  const team = capture.team_reply?.trim();
  const score = capture.diff_score ?? null;
  const time = new Date(capture.captured_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  return (
    <div className="flex flex-col gap-1">
      {customer && (
        <div className="flex">
          <div className="max-w-[80%] rounded-2xl rounded-bl-sm bg-muted px-3 py-2 text-sm whitespace-pre-wrap">
            {customer}
          </div>
        </div>
      )}
      <button
        type="button"
        onClick={() => onSelect(capture)}
        className="flex justify-end text-left hover:opacity-90 transition"
        title={viewLabel}
      >
        <div className="max-w-[80%] rounded-2xl rounded-br-sm bg-primary/10 px-3 py-2 text-sm whitespace-pre-wrap">
          {team}
          <div className="mt-1 flex items-center gap-2 text-xs">
            {capture.judge_completed_at ? (
              <span className={`inline-flex items-center rounded px-1.5 py-0.5 font-mono text-[10px] ${scoreBgClass(score)}`}>
                {score === null ? "—" : score.toFixed(2)}
              </span>
            ) : capture.judge_error ? (
              <span className="inline-flex items-center rounded bg-destructive/15 px-1.5 py-0.5 font-mono text-[10px] text-destructive">
                {failedLabel}
              </span>
            ) : (
              <span className="inline-flex items-center rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground">
                {pendingLabel}
              </span>
            )}
            <span className="text-muted-foreground">{time}</span>
          </div>
        </div>
      </button>
    </div>
  );
}
