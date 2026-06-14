import { useState, useCallback, useRef } from "react";
import { useHttp } from "@/hooks/use-ws";

interface StartResult {
  url: string;
  code: string;
}

interface StatusResult {
  authenticated: boolean;
  auth_at?: string;
}

type Phase = "idle" | "starting" | "waiting" | "done" | "error";

export function useCodexReauth() {
  const http = useHttp();
  const [phase, setPhase] = useState<Phase>("idle");
  const [url, setUrl] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const stopPoll = useCallback(() => {
    if (pollRef.current !== null) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }, []);

  const pollStatus = useCallback(() => {
    http
      .get<StatusResult>("/v1/codex/reauth/status")
      .then((res) => {
        if (res.authenticated) {
          stopPoll();
          setPhase("done");
        }
      })
      .catch(() => {
        // non-fatal: keep polling
      });
  }, [http, stopPoll]);

  const start = useCallback(async () => {
    setPhase("starting");
    setError("");
    setUrl("");
    setCode("");
    stopPoll();

    try {
      const res = await http.post<StartResult>("/v1/codex/reauth/start");
      setUrl(res.url);
      setCode(res.code);
      setPhase("waiting");

      // Poll every 3s until auth.json is fresh
      pollRef.current = setInterval(pollStatus, 3_000);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      setError(msg);
      setPhase("error");
    }
  }, [http, stopPoll, pollStatus]);

  const reset = useCallback(() => {
    stopPoll();
    setPhase("idle");
    setUrl("");
    setCode("");
    setError("");
  }, [stopPoll]);

  return { phase, url, code, error, start, reset };
}
