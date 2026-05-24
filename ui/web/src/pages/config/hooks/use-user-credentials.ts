import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";

export interface Integration {
  binary_name: string;
  account_email: string;
  scopes: string[];
  connected_at: string;
}

interface MeResponse {
  integrations: Integration[];
}

interface StartResponse {
  auth_url: string;
  state: string;
}

const POLL_INTERVAL_MS = 2000;
const POPUP_TIMEOUT_MS = 6 * 60 * 1000;

export function useUserCredentials() {
  const http = useHttp();
  const queryClient = useQueryClient();
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const messageHandlerRef = useRef<((e: MessageEvent) => void) | null>(null);
  const [connecting, setConnecting] = useState<string | null>(null);

  const query = useQuery({
    queryKey: ["integrations", "me"],
    queryFn: () => http.get<MeResponse>("/v1/integrations/me"),
    staleTime: 30 * 1000,
  });

  const stopPolling = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
    if (messageHandlerRef.current) {
      window.removeEventListener("message", messageHandlerRef.current);
      messageHandlerRef.current = null;
    }
    setConnecting(null);
  }, []);

  useEffect(() => () => stopPolling(), [stopPolling]);

  const connect = useCallback(
    async (binaryName: string) => {
      setConnecting(binaryName);
      try {
        // v0: only "gws" is wired. Future binaries get their own /start route.
        const res = await http.post<StartResponse>(`/v1/integrations/${binaryName}/start`, {});
        const win = window.open(res.auth_url, "_blank", "popup,width=520,height=720");
        if (!win || win.closed || typeof win.closed === "undefined") {
          throw new Error("popup_blocked");
        }
        // postMessage path — callback HTML signals completion.
        const onMessage = (e: MessageEvent) => {
          if (e.data === "oauth-complete") {
            stopPolling();
            queryClient.invalidateQueries({ queryKey: ["integrations", "me"] });
          }
        };
        messageHandlerRef.current = onMessage;
        window.addEventListener("message", onMessage);
        // Polling fallback — covers browsers that strip postMessage in cross-origin popups.
        pollRef.current = setInterval(async () => {
          try {
            const meResp = await http.get<MeResponse>("/v1/integrations/me");
            const found = meResp.integrations.find((i) => i.binary_name === binaryName);
            if (found) {
              stopPolling();
              queryClient.setQueryData(["integrations", "me"], meResp);
            }
          } catch {
            // ignore transient errors during polling
          }
        }, POLL_INTERVAL_MS);
        timeoutRef.current = setTimeout(() => stopPolling(), POPUP_TIMEOUT_MS);
      } catch (err) {
        stopPolling();
        throw err;
      }
    },
    [http, queryClient, stopPolling],
  );

  const disconnect = useCallback(
    async (binaryName: string) => {
      await http.delete(`/v1/integrations/${binaryName}`);
      queryClient.invalidateQueries({ queryKey: ["integrations", "me"] });
    },
    [http, queryClient],
  );

  return {
    integrations: query.data?.integrations ?? [],
    isLoading: query.isLoading,
    connect,
    disconnect,
    connecting,
    refetch: query.refetch,
  };
}

export function findIntegration(integrations: Integration[], binaryName: string): Integration | undefined {
  return integrations.find((i) => i.binary_name === binaryName);
}
