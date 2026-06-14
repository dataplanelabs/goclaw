import { useState } from "react";
import { Loader2, Settings2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { KeyValueEditor } from "@/components/shared/key-value-editor";
import { toast } from "@/stores/use-toast-store";
import type { Workstation } from "./hooks/use-workstations";

const SENSITIVE_ENV_RE = /(key|secret|token|password|credential)/i;
const isSensitiveEnv = (key: string) => SENSITIVE_ENV_RE.test(key.trim());

interface WorkstationDefaultEnvDialogProps {
  open: boolean;
  workstation: Workstation;
  onOpenChange: (open: boolean) => void;
  onSave: (env: Record<string, string>) => Promise<void>;
}

export function WorkstationDefaultEnvDialog({
  open,
  workstation,
  onOpenChange,
  onSave,
}: WorkstationDefaultEnvDialogProps) {
  const { t } = useTranslation("workstations");
  const [env, setEnv] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(env);
      toast.success(t("defaultEnvDialog.saved"));
      onOpenChange(false);
    } catch (err) {
      toast.error(t("defaultEnvDialog.saveFailed"), err instanceof Error ? err.message : "");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-sm:inset-0 max-sm:rounded-none">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Settings2 className="h-4 w-4" />
            {t("defaultEnvDialog.title")}
          </DialogTitle>
          <DialogDescription>
            {workstation.name} — {t("defaultEnvDialog.description")}
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-[60vh] overflow-y-auto pr-1">
          <KeyValueEditor
            value={env}
            onChange={setEnv}
            keyPlaceholder={t("defaultEnvDialog.keyPlaceholder")}
            valuePlaceholder={t("defaultEnvDialog.valuePlaceholder")}
            addLabel={t("defaultEnvDialog.addRow")}
            maskValue={isSensitiveEnv}
          />
        </div>

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {t("defaultEnvDialog.cancel")}
          </Button>
          <Button onClick={handleSave} disabled={saving}>
            {saving && <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />}
            {t("defaultEnvDialog.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
