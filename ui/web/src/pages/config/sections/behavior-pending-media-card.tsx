import { Clock, HardDrive, Timer } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export interface PendingMediaValues {
  enabled?: boolean;
  max_age_hours?: number;
  sweep_interval_minutes?: number;
}

interface Props {
  value: PendingMediaValues;
  onChange: (v: PendingMediaValues) => void;
}

export function BehaviorPendingMediaCard({ value, onChange }: Props) {
  const { t } = useTranslation("config");

  const update = (patch: Partial<PendingMediaValues>) =>
    onChange({ ...value, ...patch });

  const enabled = value.enabled !== false;

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">
          {t("behavior.pendingMediaTitle")}
        </CardTitle>
        <CardDescription>
          {t("behavior.pendingMediaDescription")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-0">
        {/* Enabled toggle */}
        <div className="border-b py-4">
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <HardDrive className="h-4 w-4 shrink-0 text-blue-500" />
              <div className="space-y-1">
                <Label className="text-sm font-medium">
                  {t("behavior.pendingMediaEnabled")}
                </Label>
              </div>
            </div>
            <Switch
              checked={enabled}
              onCheckedChange={(v) => update({ enabled: v })}
              className="shrink-0"
            />
          </div>
        </div>

        {/* Max Age */}
        <div className="border-b py-4">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <Clock className="mt-0.5 h-4 w-4 shrink-0 text-orange-500" />
              <div className="space-y-1">
                <Label className="text-sm font-medium">
                  {t("behavior.pendingMediaMaxAge")}
                </Label>
                <p className="text-xs text-muted-foreground">
                  {t("behavior.pendingMediaMaxAgeTip")}
                </p>
              </div>
            </div>
            <Input
              type="number"
              value={value.max_age_hours ?? ""}
              onChange={(e) => update({ max_age_hours: Number(e.target.value) })}
              placeholder="72"
              min={1}
              className="w-24 shrink-0"
            />
          </div>
        </div>

        {/* Sweep Interval */}
        <div className="py-4">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <Timer className="mt-0.5 h-4 w-4 shrink-0 text-blue-500" />
              <div className="space-y-1">
                <Label className="text-sm font-medium">
                  {t("behavior.pendingMediaInterval")}
                </Label>
                <p className="text-xs text-muted-foreground">
                  {t("behavior.pendingMediaIntervalTip")}
                </p>
              </div>
            </div>
            <Input
              type="number"
              value={value.sweep_interval_minutes ?? ""}
              onChange={(e) =>
                update({ sweep_interval_minutes: Number(e.target.value) })
              }
              placeholder="60"
              min={1}
              className="w-24 shrink-0"
            />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
