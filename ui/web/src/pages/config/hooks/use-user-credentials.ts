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

// --- B3-01.1: per-tenant Google OAuth config admin hook ---

export interface GoogleOAuthConfig {
  client_id: string;
  redirect_url: string;
  has_client_secret: boolean;
  is_configured: boolean;
  inherits_from_env: boolean;
}

export function useGoogleOAuthConfig() {
  const http = useHttp();
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: ["admin", "oauth", "google"],
    queryFn: () => http.get<GoogleOAuthConfig>("/v1/admin/oauth/google"),
    staleTime: 30 * 1000,
  });

  const save = useCallback(
    async (cfg: { client_id: string; client_secret?: string; redirect_url: string }) => {
      await http.put<GoogleOAuthConfig>("/v1/admin/oauth/google", cfg);
      queryClient.invalidateQueries({ queryKey: ["admin", "oauth", "google"] });
    },
    [http, queryClient],
  );

  const clear = useCallback(async () => {
    await http.delete("/v1/admin/oauth/google");
    queryClient.invalidateQueries({ queryKey: ["admin", "oauth", "google"] });
  }, [http, queryClient]);

  return { config: query.data, isLoading: query.isLoading, save, clear, refetch: query.refetch };
}
