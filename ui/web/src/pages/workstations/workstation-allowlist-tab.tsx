import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { RefreshCw, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import type { Workstation } from "./hooks/use-workstations";
import { useWorkstationPermissions } from "./hooks/use-workstation-permissions";

interface WorkstationAllowlistTabProps {
  workstation: Workstation;
}

function configString(meta: Record<string, unknown> | undefined, key: string): string | undefined {
  const v = meta?.[key];
  return typeof v === "string" && v ? v : typeof v === "number" ? String(v) : undefined;
}

export function WorkstationAllowlistTab({ workstation }: WorkstationAllowlistTabProps) {
  const { t } = useTranslation("workstations");
  const { permissions, loading, error, load } = useWorkstationPermissions();

  useEffect(() => {
    load(workstation.id);
  }, [workstation.id, load]);

  const host = configString(workstation.metadataSummary, "host");
  const user = configString(workstation.metadataSummary, "user");
  const port = configString(workstation.metadataSummary, "port");

  const configRows: Array<{ label: string; value: string }> = [
    { label: t("allowlist.config.backend"), value: t(`backend.${workstation.backendType}`) },
    ...(host ? [{ label: t("allowlist.config.host"), value: port ? `${host}:${port}` : host }] : []),
    ...(user ? [{ label: t("allowlist.config.user"), value: user }] : []),
    ...(workstation.defaultCwd ? [{ label: t("allowlist.config.defaultCwd"), value: workstation.defaultCwd }] : []),
  ];

  return (
    <div className="space-y-4 p-4">
      {/* Configuration summary */}
      <div className="grid grid-cols-1 gap-x-6 gap-y-1.5 sm:grid-cols-2">
        {configRows.map((row) => (
          <div key={row.label} className="flex items-baseline justify-between gap-3 border-b py-1.5 last:border-0">
            <span className="text-xs text-muted-foreground">{row.label}</span>
            <span className="font-mono text-xs">{row.value}</span>
          </div>
        ))}
      </div>

      {/* Allowed commands */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1.5">
            <ShieldCheck className="h-3.5 w-3.5 text-muted-foreground" />
            <p className="text-sm font-medium">{t("allowlist.title")}</p>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 gap-1 text-xs"
            onClick={() => load(workstation.id)}
            disabled={loading}
          >
            <RefreshCw className={"h-3 w-3" + (loading ? " animate-spin" : "")} />
            {t("common:refresh", "Refresh")}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">{t("allowlist.description")}</p>

        {loading && permissions.length === 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-16" />
            ))}
          </div>
        ) : error ? (
          <div className="flex flex-col items-start gap-2">
            <p className="text-sm text-destructive">{error}</p>
            <Button variant="outline" size="sm" onClick={() => load(workstation.id)}>
              {t("common:retry", "Retry")}
            </Button>
          </div>
        ) : permissions.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("allowlist.empty")}</p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {permissions.map((p) => (
              <Badge
                key={p.id}
                variant={p.enabled ? "secondary" : "outline"}
                className={
                  "font-mono text-xs" + (p.enabled ? "" : " text-muted-foreground line-through opacity-60")
                }
                title={p.enabled ? undefined : t("allowlist.disabledHint")}
              >
                {p.pattern}
              </Badge>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
