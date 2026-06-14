import { useState, useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { Methods } from "@/api/protocol";

export interface WorkstationPermission {
  id: string;
  workstationId: string;
  tenantId: string;
  pattern: string;
  enabled: boolean;
  createdBy: string;
  createdAt: string;
}

interface UseWorkstationPermissionsResult {
  permissions: WorkstationPermission[];
  loading: boolean;
  error: string | null;
  load: (workstationId: string) => Promise<void>;
}

export function useWorkstationPermissions(): UseWorkstationPermissionsResult {
  const ws = useWs();
  const [permissions, setPermissions] = useState<WorkstationPermission[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(
    async (workstationId: string) => {
      setLoading(true);
      setError(null);
      try {
        const res = await ws.call<{ permissions: WorkstationPermission[] }>(
          Methods.WORKSTATIONS_PERMS_LIST,
          { workstationId },
        );
        const rows = res.permissions ?? [];
        rows.sort((a, b) => a.pattern.localeCompare(b.pattern));
        setPermissions(rows);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Failed to load allowlist");
      } finally {
        setLoading(false);
      }
    },
    [ws],
  );

  return { permissions, loading, error, load };
}
