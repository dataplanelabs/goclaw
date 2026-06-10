import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { toast } from "@/stores/use-toast-store";
import { findIntegration, useUserCredentials } from "../hooks/use-user-credentials";

export function UserCredentialsSection() {
  const { t } = useTranslation("integrations");
  const { integrations, isLoading, connect, disconnect, connecting } = useUserCredentials();
  const [confirmDisconnect, setConfirmDisconnect] = useState<{ binary: string; email: string } | null>(null);

  const handleConnect = async (binary: string) => {
    try {
      await connect(binary);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "";
      if (msg === "popup_blocked") {
        toast.error(t("google.errorPopupBlocked"));
      } else {
        toast.error(t("google.errorGeneric"));
      }
    }
  };

  const handleDisconnect = async () => {
    if (!confirmDisconnect) return;
    const { binary } = confirmDisconnect;
    setConfirmDisconnect(null);
    try {
      await disconnect(binary);
    } catch {
      toast.error(t("google.errorGeneric"));
    }
  };

  const gws = findIntegration(integrations, "gws");

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("section.title")}</CardTitle>
        <CardDescription>{t("section.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-col gap-2 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="space-y-1">
            <div className="font-medium">{t("google.name")}</div>
            <div className="text-sm text-muted-foreground">{t("google.description")}</div>
            {gws && (
              <div className="text-sm text-emerald-600 dark:text-emerald-400">
                {t("google.connectedAs", { email: gws.account_email || "Google" })}
              </div>
            )}
          </div>
          <div className="flex shrink-0 gap-2">
            {gws ? (
              <Button
                variant="outline"
                onClick={() => setConfirmDisconnect({ binary: "gws", email: gws.account_email })}
                disabled={isLoading}
              >
                {t("google.disconnectButton")}
              </Button>
            ) : (
              <Button
                onClick={() => handleConnect("gws")}
                disabled={isLoading || connecting === "gws"}
                className="min-h-11"
              >
                {connecting === "gws" ? t("google.connecting") : t("google.connectButton")}
              </Button>
            )}
          </div>
        </div>
      </CardContent>

      <Dialog open={!!confirmDisconnect} onOpenChange={(open) => !open && setConfirmDisconnect(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("google.disconnectConfirmTitle")}</DialogTitle>
            <DialogDescription>
              {t("google.disconnectConfirmDescription", { email: confirmDisconnect?.email ?? "" })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDisconnect(null)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDisconnect}>
              {t("google.disconnectConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
