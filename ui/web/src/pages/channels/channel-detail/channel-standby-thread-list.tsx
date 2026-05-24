import { Button } from "@/components/ui/button";

import type { Schedule } from "./channel-standby-schema";

export interface ThreadSchedule {
  channel_instance_id: string;
  thread_key: string;
  schedule: Schedule;
  expires_at?: string;
  reason?: string;
  created_by?: string;
  updated_at?: string;
}

export interface ThreadListLabels {
  title: string;
  empty: string;
  thread: string;
  expires: string;
  reason: string;
  createdBy: string;
  deleteLabel: string;
}

export function ChannelStandbyThreadList({
  threads,
  onDelete,
  labels,
}: {
  threads: ThreadSchedule[];
  onDelete: (threadKey: string) => void;
  labels: ThreadListLabels;
}) {
  return (
    <div className="space-y-2">
      <h3 className="text-sm font-semibold">{labels.title}</h3>
      {threads.length === 0 ? (
        <p className="text-sm text-muted-foreground">{labels.empty}</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-[600px] w-full text-sm">
            <thead>
              <tr className="border-b text-left text-muted-foreground">
                <th className="py-2 pr-3">{labels.thread}</th>
                <th className="py-2 pr-3">{labels.expires}</th>
                <th className="py-2 pr-3">{labels.reason}</th>
                <th className="py-2 pr-3">{labels.createdBy}</th>
                <th className="py-2 pr-3"></th>
              </tr>
            </thead>
            <tbody>
              {threads.map((t) => (
                <tr key={t.thread_key} className="border-b">
                  <td className="py-2 pr-3 font-mono">{t.thread_key}</td>
                  <td className="py-2 pr-3">{t.expires_at ?? "—"}</td>
                  <td className="py-2 pr-3">{t.reason || "—"}</td>
                  <td className="py-2 pr-3">{t.created_by || "—"}</td>
                  <td className="py-2 pr-3 text-right">
                    <Button size="sm" variant="outline" onClick={() => onDelete(t.thread_key)}>
                      {labels.deleteLabel}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
