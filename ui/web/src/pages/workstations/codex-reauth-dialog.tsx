import { CheckCircle2, Copy, Loader2, RotateCcw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { toast } from "@/stores/use-toast-store";
import { useCodexReauth } from "./hooks/use-codex-reauth";

interface CodexReauthDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CodexReauthDialog({ open, onOpenChange }: CodexReauthDialogProps) {
  const { t } = useTranslation("workstations");
  const { phase, url, code, error, start, reset } = useCodexReauth();

  function copyCode() {
    if (!code) return;
    navigator.clipboard.writeText(code).then(() => {
      toast.success(t("codexReauth.copied"));
    });
  }

  function handleOpenChange(v: boolean) {
    if (!v) reset();
    onOpenChange(v);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("codexReauth.title")}</DialogTitle>
          <DialogDescription>{t("codexReauth.description")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 pt-2">
          {phase === "idle" && (
            <Button onClick={start}>{t("codexReauth.startButton")}</Button>
          )}

          {phase === "starting" && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {t("codexReauth.starting")}
            </div>
          )}

          {(phase === "waiting" || phase === "done") && url && (
            <div className="space-y-2">
              <p className="text-sm">
                {t("codexReauth.openUrl")}{" "}
                <a
                  href={url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="break-all font-medium text-blue-600 underline underline-offset-2 hover:text-blue-700 dark:text-blue-400"
                >
                  {url}
                </a>
              </p>

              {code && (
                <div className="flex items-center gap-2">
                  <span className="text-sm text-muted-foreground">{t("codexReauth.codeLabel")}</span>
                  <code className="rounded bg-muted px-2 py-0.5 font-mono text-sm font-semibold tracking-widest">
                    {code}
                  </code>
                  <button
                    onClick={copyCode}
                    className="inline-flex h-7 w-7 min-w-[44px] items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground pointer-events-auto"
                    title={t("codexReauth.copyCode")}
                  >
                    <Copy className="h-3.5 w-3.5" />
                  </button>
                </div>
              )}
            </div>
          )}

          {phase === "waiting" && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {t("codexReauth.waiting")}
            </div>
          )}

          {phase === "done" && (
            <div className="flex items-center gap-2 text-sm text-emerald-600 dark:text-emerald-400">
              <CheckCircle2 className="h-4 w-4" />
              {t("codexReauth.done")}
            </div>
          )}

          {phase === "error" && (
            <div className="space-y-2">
              <p className="text-sm text-destructive">{error}</p>
              <Button variant="outline" size="sm" onClick={reset}>
                <RotateCcw className="mr-1.5 h-3.5 w-3.5" />
                {t("codexReauth.retry")}
              </Button>
            </div>
          )}

          {(phase === "done" || phase === "waiting") && (
            <Button variant="ghost" size="sm" onClick={reset} className="text-xs text-muted-foreground">
              {t("codexReauth.reset")}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
