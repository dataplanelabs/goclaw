import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ExternalLink, Loader2, ShieldCheck, ShieldX } from "lucide-react";
import { useWsEvent } from "@/hooks/use-ws-event";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import type { MCPServerData, MCPOAuthStartResponse, MCPOAuthStatus } from "./hooks/use-mcp";

interface MCPOAuthDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  server: MCPServerData;
  onStartOAuth: (serverId: string, mcpUrl: string, userId?: string) => Promise<MCPOAuthStartResponse>;
  onGetStatus: (serverId: string, userId?: string) => Promise<MCPOAuthStatus>;
  onRevoke: (serverId: string, userId?: string) => Promise<void>;
}

export function MCPOAuthDialog({
  open,
  onOpenChange,
  server,
  onStartOAuth,
  onGetStatus,
  onRevoke,
}: MCPOAuthDialogProps) {
  const { t } = useTranslation("mcp");
  const [status, setStatus] = useState<MCPOAuthStatus | null>(null);
  const [authorizing, setAuthorizing] = useState(false);
  const [revoking, setRevoking] = useState(false);
  const [error, setError] = useState("");
  const popupRef = useRef<Window | null>(null);

  const loadStatus = useCallback(async () => {
    try {
      setStatus(await onGetStatus(server.id));
    } catch {
      setStatus(null);
    }
  }, [onGetStatus, server.id]);

  useEffect(() => {
    if (!open) return;
    setError("");
    void loadStatus();
  }, [loadStatus, open]);

  useEffect(() => () => popupRef.current?.close(), []);

  useEffect(() => {
    if (!authorizing) return;
    const timer = window.setInterval(() => {
      if (popupRef.current?.closed) {
        popupRef.current = null;
        setAuthorizing(false);
        void loadStatus();
      }
    }, 800);
    return () => window.clearInterval(timer);
  }, [authorizing, loadStatus]);

  useEffect(() => {
    if (!open) return;
    const handleComplete = (payload: unknown) => {
      const p = payload as { type?: string; status?: string; error?: string };
      if (p?.type !== "mcp-oauth-complete") return;
      popupRef.current?.close();
      popupRef.current = null;
      setAuthorizing(false);
      if (p.status === "success") {
        void loadStatus();
      } else {
        setError(p.error || t("form.oauth.authFailed"));
      }
    };

    const handleWindowMessage = (event: MessageEvent) => handleComplete(event.data);
    window.addEventListener("message", handleWindowMessage);

    let channel: BroadcastChannel | null = null;
    try {
      channel = new BroadcastChannel("mcp-oauth");
      channel.onmessage = (event) => handleComplete(event.data);
    } catch {
      channel = null;
    }

    return () => {
      window.removeEventListener("message", handleWindowMessage);
      channel?.close();
    };
  }, [loadStatus, open, t]);

  useWsEvent("mcp.oauth_complete", (payload: unknown) => {
    const p = payload as { serverId?: string; userId?: string; status?: string; error?: string };
    if (p.serverId !== server.id || (p.userId ?? "") !== "") return;
    popupRef.current?.close();
    popupRef.current = null;
    setAuthorizing(false);
    if (p.status === "success") {
      void loadStatus();
    } else {
      setError(p.error ?? t("form.oauth.authFailed"));
    }
  });

  const handleAuthorize = async () => {
    if (!server.url) {
      setError(t("form.oauth.noUrl"));
      return;
    }
    setAuthorizing(true);
    setError("");
    try {
      const result = await onStartOAuth(server.id, server.url);
      if (result.completed || !result.auth_url) {
        await loadStatus();
        setAuthorizing(false);
        return;
      }
      const popup = window.open(result.auth_url, "mcp-oauth", "width=620,height=720,menubar=no,toolbar=no");
      if (!popup) {
        setAuthorizing(false);
        setError(t("form.oauth.popupBlocked"));
        return;
      }
      popupRef.current = popup;
    } catch (err) {
      setError(err instanceof Error ? err.message : t("form.oauth.authFailed"));
      setAuthorizing(false);
    }
  };

  const handleRevoke = async () => {
    setRevoking(true);
    setError("");
    try {
      await onRevoke(server.id);
      await loadStatus();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("form.oauth.revokeFailed"));
    } finally {
      setRevoking(false);
    }
  };

  const hasToken = status?.has_token ?? false;
  const expired = status?.expired ?? false;

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next && authorizing) {
          popupRef.current?.close();
          popupRef.current = null;
          setAuthorizing(false);
        }
        if (!revoking) onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheck className="h-4 w-4 text-primary" />
            {t("form.oauth.title")}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4 py-2">
          <div className="text-sm">
            <div className="font-medium">{server.display_name || server.name}</div>
            {server.url && <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{server.url}</div>}
          </div>

          <div className="rounded-md border p-3">
            {status === null ? (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                {t("form.oauth.loading")}
              </div>
            ) : hasToken ? (
              <div className="space-y-1">
                <div className="flex items-center gap-2 text-sm">
                  {expired ? (
                    <ShieldX className="h-3.5 w-3.5 text-amber-500" />
                  ) : (
                    <ShieldCheck className="h-3.5 w-3.5 text-emerald-500" />
                  )}
                  <span className={expired ? "text-amber-600 dark:text-amber-400" : "text-emerald-600 dark:text-emerald-400"}>
                    {expired ? t("form.oauth.expired") : t("form.oauth.authorized")}
                  </span>
                </div>
                {status.client_id && <div className="truncate font-mono text-xs text-muted-foreground">{status.client_id}</div>}
                {status.expires_at && (
                  <div className="text-xs text-muted-foreground">{new Date(status.expires_at).toLocaleString()}</div>
                )}
              </div>
            ) : (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <ShieldX className="h-3.5 w-3.5" />
                {t("form.oauth.notAuthorized")}
              </div>
            )}
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>

        <DialogFooter className="flex-row gap-2 sm:justify-between">
          {hasToken && (
            <Button
              variant="outline"
              size="sm"
              onClick={handleRevoke}
              disabled={authorizing || revoking}
              className="text-destructive hover:text-destructive"
            >
              {revoking && <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />}
              {t("form.oauth.revoke")}
            </Button>
          )}
          <div className="ml-auto flex gap-2">
            <Button variant="outline" size="sm" onClick={() => onOpenChange(false)} disabled={revoking}>
              {t("form.cancel")}
            </Button>
            <Button size="sm" onClick={handleAuthorize} disabled={authorizing || revoking} className="gap-1">
              {authorizing ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  {t("form.oauth.authorizing")}
                </>
              ) : (
                <>
                  <ExternalLink className="h-3.5 w-3.5" />
                  {hasToken ? t("form.oauth.reauthorize") : t("form.oauth.authorize")}
                </>
              )}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
