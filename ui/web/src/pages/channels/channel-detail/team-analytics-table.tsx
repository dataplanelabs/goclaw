import { useTranslation } from "react-i18next";

export interface TeamReplyEvaluation {
  id: string;
  channel_instance_id: string;
  thread_key: string;
  session_key: string;
  team_msg_id: string;
  captured_at: string;
  updated_at: string;
  customer_message: string;
  team_reply: string;
  hypothesized_bot_reply?: string;
  diff_score?: number;
  diff_reasoning?: string;
  judge_agent_key?: string;
  judge_model?: string;
  judge_provider?: string;
  judge_latency_ms?: number;
  judge_error?: string;
  judge_completed_at?: string;
}

export interface TeamAnalyticsTableProps {
  rows: TeamReplyEvaluation[];
}

function truncate(s: string, max = 80): string {
  if (!s) return "";
  return s.length > max ? s.slice(0, max - 1) + "…" : s;
}

function judgeStatus(row: TeamReplyEvaluation, t: (key: string) => string): string {
  if (row.judge_completed_at) return row.diff_score?.toFixed(2) ?? "—";
  if (row.judge_error) return t("teamAnalytics.judgeFailed");
  return t("teamAnalytics.judgePending");
}

export function TeamAnalyticsTable({ rows }: TeamAnalyticsTableProps) {
  const { t } = useTranslation("channels");
  if (rows.length === 0) {
    return (
      <div className="text-muted-foreground text-sm">
        {t("teamAnalytics.noCaptures")}
      </div>
    );
  }
  return (
    <div className="overflow-x-auto">
      <table className="min-w-[700px] w-full text-sm">
        <thead className="text-muted-foreground text-xs">
          <tr className="border-b">
            <th className="text-left py-2 pr-3">captured_at</th>
            <th className="text-left py-2 pr-3">thread</th>
            <th className="text-left py-2 pr-3">customer</th>
            <th className="text-left py-2 pr-3">team_reply</th>
            <th className="text-left py-2 pr-3">judge</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.id} className="border-b last:border-b-0 align-top">
              <td className="py-2 pr-3 font-mono text-xs whitespace-nowrap">
                {new Date(r.captured_at).toLocaleString()}
              </td>
              <td className="py-2 pr-3 font-mono text-xs">{r.thread_key}</td>
              <td className="py-2 pr-3 max-w-[200px]">
                {truncate(r.customer_message, 80)}
              </td>
              <td className="py-2 pr-3 max-w-[260px]">
                {truncate(r.team_reply, 120)}
              </td>
              <td className="py-2 pr-3 whitespace-nowrap">{judgeStatus(r, t)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
