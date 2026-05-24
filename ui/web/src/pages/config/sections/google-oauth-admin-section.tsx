import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { toast } from "@/stores/use-toast-store";
import { useGoogleOAuthConfig } from "../hooks/use-user-credentials";

export function GoogleOAuthAdminSection() {
  const { t } = useTranslation("integrations");
  const { config, isLoading, save, clear } = useGoogleOAuthConfig();
  const [clientID, setClientID] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [redirectURL, setRedirectURL] = useState("");
  const [saving, setSaving] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);

  useEffect(() => {
    if (config) {
      setClientID(config.client_id || "");
      setRedirectURL(config.redirect_url || "");
      // never prefill secret — server doesn't return it
      setClientSecret("");
    }
  }, [config]);

  const handleSave = async () => {
    if (!clientID.trim() || !redirectURL.trim()) return;
    setSaving(true);
    try {
      await save({
        client_id: clientID.trim(),
        client_secret: clientSecret.trim() || undefined,
        redirect_url: redirectURL.trim(),
      });
      toast.success(t("admin.saveSuccess"));
      setClientSecret("");
    } catch (err) {
      toast.error(t("admin.saveError"), err instanceof Error ? err.message : "");
    } finally {
      setSaving(false);
    }
  };

  const handleClear = async () => {
    setConfirmClear(false);
    try {
      await clear();
      toast.success(t("admin.clearSuccess"));
      setClientID("");
      setClientSecret("");
      setRedirectURL("");
    } catch (err) {
      toast.error(t("admin.saveError"), err instanceof Error ? err.message : "");
    }
  };

  const statusBadge = () => {
    if (isLoading) return null;
    if (config?.is_configured && !config.inherits_from_env) {
      return <Badge variant="default">{t("admin.statusConfigured")}</Badge>;
    }
    if (config?.is_configured && config.inherits_from_env) {
      return <Badge variant="secondary">{t("admin.statusInheritsFromEnv")}</Badge>;
    }
    return <Badge variant="destructive">{t("admin.statusNotConfigured")}</Badge>;
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center gap-2">
          <CardTitle>{t("admin.googleConfigTitle")}</CardTitle>
          {statusBadge()}
        </div>
        <CardDescription>{t("admin.googleConfigDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="oauth-client-id">{t("admin.clientIdLabel")}</Label>
          <Input
            id="oauth-client-id"
            value={clientID}
            onChange={(e) => setClientID(e.target.value)}
            placeholder="123456-abc.apps.googleusercontent.com"
            className="text-base md:text-sm"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="oauth-client-secret">{t("admin.clientSecretLabel")}</Label>
          <Input
            id="oauth-client-secret"
            type="password"
            value={clientSecret}
            onChange={(e) => setClientSecret(e.target.value)}
            placeholder={
              config?.has_client_secret
                ? t("admin.clientSecretPlaceholderExisting")
                : t("admin.clientSecretPlaceholderNew")
            }
            className="text-base md:text-sm"
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="oauth-redirect-url">{t("admin.redirectUrlLabel")}</Label>
          <Input
            id="oauth-redirect-url"
            value={redirectURL}
            onChange={(e) => setRedirectURL(e.target.value)}
            placeholder="https://goclaw.example/v1/auth/google/callback"
            className="text-base md:text-sm"
          />
          <p className="text-sm text-muted-foreground">{t("admin.redirectUrlHint")}</p>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button onClick={handleSave} disabled={saving || !clientID.trim() || !redirectURL.trim()}>
            {t("admin.saveButton")}
          </Button>
          {config?.has_client_secret && (
            <Button variant="outline" onClick={() => setConfirmClear(true)} disabled={saving}>
              {t("admin.clearButton")}
            </Button>
          )}
        </div>
      </CardContent>

      <Dialog open={confirmClear} onOpenChange={(open) => !open && setConfirmClear(false)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("admin.clearConfirmTitle")}</DialogTitle>
            <DialogDescription>{t("admin.clearConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmClear(false)}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleClear}>
              {t("admin.clearButton")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
