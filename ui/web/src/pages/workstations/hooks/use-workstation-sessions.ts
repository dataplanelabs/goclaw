import { useState, useCallback, useEffect, useRef } from "react";
import { useWs } from "@/hooks/use-ws";
import { Methods } from "@/api/protocol";

export type SessionStatus = "running" | "done" | "failed";

export interface SessionSummary {
  sessionKey: string;
  agentId: string;
  command: string;
  startedAt: string;
  endedAt?: string;
  exitCode?: number;
  status: SessionStatus;
  lineCount: number;
}

export interface SessionLine {
  stream: "stdout" | "stderr";
  seq: number;
  data: string;
}

export interface SessionOutput {
  lines: SessionLine[];
  status: SessionStatus;
  exitCode?: number;
}

export function useWorkstationSessions(workstationId: string) {
  const ws = useWs();
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!workstationId) return;
    setLoading(true);
    setError(null);
    try {
      const res = await ws.call<{ sessions: SessionSummary[] }>(
        Methods.WORKSTATIONS_SESSIONS_LIST,
        { workstationId },
      );
      setSessions(res.sessions ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load sessions");
    } finally {
      setLoading(false);
    }
  }, [ws, workstationId]);

  // Refresh on start/done events for this workstation.
  useEffect(() => {
    const unsubStart = ws.on("workstation.exec.start", (payload: unknown) => {
      const p = payload as { workstation_id?: string };
      if (p.workstation_id === workstationId) {
        refresh();
      }
    });
    const unsubDone = ws.on("workstation.exec.done", (payload: unknown) => {
      const p = payload as { workstation_id?: string };
      if (p.workstation_id === workstationId) {
        refresh();
      }
    });
    return () => {
      unsubStart();
      unsubDone();
    };
  }, [ws, workstationId, refresh]);

  // Auto-refresh every 5s when any session is running.
  const hasRunning = sessions.some((s) => s.status === "running");
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  useEffect(() => {
    if (hasRunning) {
      intervalRef.current = setInterval(refresh, 5000);
    }
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [hasRunning, refresh]);

  return { sessions, loading, error, refresh };
}

export function useSessionOutput(workstationId: string, sessionKey: string | null) {
  const ws = useWs();
  const [output, setOutput] = useState<SessionOutput | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Fetch buffered output for replay.
  useEffect(() => {
    if (!workstationId || !sessionKey) {
      setOutput(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    ws.call<SessionOutput>(Methods.WORKSTATIONS_SESSIONS_OUTPUT, {
      workstationId,
      sessionKey,
    })
      .then((res) => {
        if (!cancelled) setOutput(res);
      })
      .catch((err) => {
        if (!cancelled)
          setError(err instanceof Error ? err.message : "Failed to load output");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [ws, workstationId, sessionKey]);

  // Append live chunks after replay.
  useEffect(() => {
    if (!sessionKey) return;
    const unsub = ws.on("workstation.exec.chunk", (payload: unknown) => {
      const p = payload as {
        workstation_id?: string;
        session_key?: string;
        stream?: string;
        seq?: number;
        data?: string;
      };
      if (p.workstation_id !== workstationId || p.session_key !== sessionKey) return;
      const line: SessionLine = {
        stream: (p.stream === "stderr" ? "stderr" : "stdout") as SessionLine["stream"],
        seq: p.seq ?? 0,
        data: p.data ?? "",
      };
      setOutput((prev) => {
        if (!prev) return prev;
        const lines = [...prev.lines, line];
        return { ...prev, lines: lines.slice(-5000) };
      });
    });
    const unsubDone = ws.on("workstation.exec.done", (payload: unknown) => {
      const p = payload as {
        workstation_id?: string;
        session_key?: string;
        exit_code?: number;
      };
      if (p.workstation_id !== workstationId || p.session_key !== sessionKey) return;
      setOutput((prev) => {
        if (!prev) return prev;
        const exitCode = p.exit_code ?? 0;
        return {
          ...prev,
          status: exitCode === 0 ? "done" : "failed",
          exitCode,
        };
      });
    });
    return () => {
      unsub();
      unsubDone();
    };
  }, [ws, workstationId, sessionKey]);

  return { output, loading, error };
}
