import { useState, useCallback, useRef, useEffect } from "react";
import { useWs } from "@/hooks/use-ws";

const MAX_LINES = 5000;

interface ChunkPayload {
  workstation_id?: string;
  stream?: string;
  data?: string;
  seq?: number;
}

interface DonePayload {
  workstation_id?: string;
  exit_code?: number;
}

export interface LogLine {
  stream: "stdout" | "stderr";
  data: string;
  seq: number;
}

export function useWorkstationLiveLog(workstationId: string) {
  const ws = useWs();
  const [lines, setLines] = useState<LogLine[]>([]);
  const [active, setActive] = useState(false);
  const activeRef = useRef(false);

  const clear = useCallback(() => setLines([]), []);

  useEffect(() => {
    const unsubChunk = ws.on("workstation.exec.chunk", (payload: unknown) => {
      const p = payload as ChunkPayload;
      if (p.workstation_id !== workstationId) return;
      setActive(true);
      activeRef.current = true;
      const rawData = p.data ?? "";
      const stream = (p.stream === "stderr" ? "stderr" : "stdout") as LogLine["stream"];
      const seq = p.seq ?? 0;
      setLines((prev) => {
        const next = [...prev, { stream, data: rawData, seq }];
        return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next;
      });
    });

    const unsubDone = ws.on("workstation.exec.done", (payload: unknown) => {
      const p = payload as DonePayload;
      if (p.workstation_id !== workstationId) return;
      setActive(false);
      activeRef.current = false;
    });

    return () => {
      unsubChunk();
      unsubDone();
    };
  }, [ws, workstationId]);

  return { lines, active, clear };
}
