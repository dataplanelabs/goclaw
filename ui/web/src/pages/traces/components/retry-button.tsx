import { useState } from "react";
import { useTranslation } from "react-i18next";
import { RefreshCw, AlertTriangle } from "lucide-react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { toast } from "@/stores/use-toast-store";
import { ApiError } from "@/api/errors";
import type { TraceData } from "@/types/trace";

interface RetryButtonProps {
  trace: TraceData;
  retry: (args: { traceId: string; confirmDoubleSend?: boolean }) => Promise<{ original_trace_id: string; provider: string; message: string }>;
  onRetried?: (originalTraceID: string) => void;
}

export function RetryButton({ trace, retry, onRetried }: RetryButtonProps) {
  const { t } = useTranslation("traces");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  if (trace.status !== "error") return null;

  const doRetry = async (confirmDoubleSend: boolean) => {
    setBusy(true);
    try {
      const res = await retry({ traceId: trace.id, confirmDoubleSend });
      toast.success(t("retry.success", { shortId: res.original_trace_id.slice(0, 8) }));
      setConfirmOpen(false);
      onRetried?.(res.original_trace_id);
    } catch (err) {
      const apiErr = err as ApiError;
      const code = apiErr?.code ?? "HTTP_ERROR";
      switch (code) {
        case "locked":
          toast.warning(t("retry.error.locked"));
          break;
        case "confirm_required":
          setConfirmOpen(true);
          break;
        case "agent_gone":
          toast.error(t("retry.error.agent_gone"));
          break;
        case "provider_gone":
          toast.error(t("retry.error.provider_gone"));
          break;
        case "payload_missing":
          toast.error(t("retry.error.payload_missing"));
          break;
        case "payload_oversize":
          toast.error(t("retry.error.payload_oversize"));
          break;
        default:
          toast.error(t("retry.error.generic", { message: apiErr?.message ?? code }));
      }
    } finally {
      setBusy(false);
    }
  };

  const onClick = () => {
    if (trace.outbound_emitted) {
      setConfirmOpen(true);
      return;
    }
    void doRetry(false);
  };

  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={busy}
        onClick={onClick}
        className="flex items-center gap-1"
      >
        <RefreshCw className={busy ? "h-3.5 w-3.5 animate-spin" : "h-3.5 w-3.5"} />
        {t("retry.button")}
      </Button>

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-amber-500" />
              {t("retry.confirm.title")}
            </DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">{t("retry.confirm.body")}</p>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setConfirmOpen(false)} disabled={busy}>
              {t("retry.confirm.cancel")}
            </Button>
            <Button type="button" variant="destructive" onClick={() => void doRetry(true)} disabled={busy}>
              {busy && <RefreshCw className="mr-1 h-3.5 w-3.5 animate-spin" />}
              {t("retry.confirm.action")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
