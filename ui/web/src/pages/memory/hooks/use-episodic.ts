import { useCallback, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { toast } from "@/stores/use-toast-store";
import { normalizeMemoryAgentId, requireMemoryAgentId } from "../lib/memory-agent";
import type { EpisodicSummary, EpisodicSearchResult } from "@/types/memory";

const EPISODIC_KEY = "episodic";

/** List episodic summaries for an agent. */
export function useEpisodicSummaries(agentId: string, opts: { userId?: string; limit?: number; offset?: number }) {
  const http = useHttp();
  const selectedAgentId = normalizeMemoryAgentId(agentId);

  const params = useMemo(() => {
    const p: Record<string, string> = {};
    if (opts.userId) p.user_id = opts.userId;
    if (opts.limit !== undefined) p.limit = String(opts.limit);
    if (opts.offset !== undefined) p.offset = String(opts.offset);
    return p;
  }, [opts.userId, opts.limit, opts.offset]);

  const { data, isLoading } = useQuery({
    queryKey: [EPISODIC_KEY, selectedAgentId, params],
    queryFn: () => http.get<EpisodicSummary[]>(`/v1/agents/${selectedAgentId}/episodic`, params),
    staleTime: 60_000,
    enabled: !!selectedAgentId,
  });

  return { summaries: Array.isArray(data) ? data : [], loading: isLoading };
}

/** Search episodic summaries. */
export function useEpisodicSearch(agentId: string) {
  const http = useHttp();
  const selectedAgentId = normalizeMemoryAgentId(agentId);

  const search = useCallback(
    async (query: string, userId?: string) => {
      try {
        const aid = requireMemoryAgentId(selectedAgentId);
        const results = await http.post<EpisodicSearchResult[]>(`/v1/agents/${aid}/episodic/search`, {
          query,
          user_id: userId,
          max_results: 20,
        });
        return Array.isArray(results) ? results : [];
      } catch {
        toast.error("Episodic search failed");
        return [];
      }
    },
    [http, selectedAgentId],
  );

  return { search };
}
