import { useCallback, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { toast } from "@/stores/use-toast-store";
import i18n from "@/i18n";
import { normalizeMemoryAgentId, requireMemoryAgentId } from "../lib/memory-agent";
import type {
  MemoryDocument,
  MemoryDocumentDetail,
  MemoryChunk,
  MemorySearchResult,
} from "@/types/memory";

export interface MemoryDocFilters {
  agentId?: string;
  userId?: string;
}

export function useMemoryDocuments(filters: MemoryDocFilters) {
  const http = useHttp();
  const queryClient = useQueryClient();
  const selectedAgentId = normalizeMemoryAgentId(filters.agentId);
  const selectedUserId = filters.userId || undefined;

  const queryKey = queryKeys.memory.list({
    agentId: selectedAgentId || undefined,
    userId: selectedUserId,
  });

  const { data, isLoading, isFetching } = useQuery({
    queryKey,
    queryFn: async () => {
      // No agent selected → list all memory across all agents
      if (!selectedAgentId) {
        const res = await http.get<MemoryDocument[]>("/v1/memory/documents");
        return res ?? [];
      }
      const params: Record<string, string> = {};
      if (selectedUserId) params.user_id = selectedUserId;
      const res = await http.get<MemoryDocument[]>(
        `/v1/agents/${selectedAgentId}/memory/documents`,
        params,
      );
      return res ?? [];
    },
    placeholderData: (prev) => prev,
    staleTime: 60_000,
  });

  const documents = data ?? [];

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: queryKeys.memory.all }),
    [queryClient],
  );

  const getDocument = useCallback(
    async (path: string, userId?: string) => {
      const agentId = requireMemoryAgentId(selectedAgentId);
      const params: Record<string, string> = {};
      if (userId) params.user_id = userId;
      return http.get<MemoryDocumentDetail>(
        `/v1/agents/${agentId}/memory/documents/${path}`,
        params,
      );
    },
    [http, selectedAgentId],
  );

  const createDocument = useCallback(
    async (path: string, content: string, userId?: string) => {
      try {
        const agentId = requireMemoryAgentId(selectedAgentId);
        await http.put(`/v1/agents/${agentId}/memory/documents/${path}`, {
          content,
          user_id: userId || "",
        });
        await invalidate();
        toast.success(i18n.t("memory:toast.docCreated"), path);
      } catch (err) {
        toast.error(i18n.t("memory:toast.docCreateFailed"), err instanceof Error ? err.message : i18n.t("memory:toast.unknownError"));
        throw err;
      }
    },
    [http, selectedAgentId, invalidate],
  );

  const updateDocument = useCallback(
    async (path: string, content: string, userId?: string) => {
      try {
        const agentId = requireMemoryAgentId(selectedAgentId);
        await http.put(`/v1/agents/${agentId}/memory/documents/${path}`, {
          content,
          user_id: userId || "",
        });
        await invalidate();
        toast.success(i18n.t("memory:toast.docUpdated"), path);
      } catch (err) {
        toast.error(i18n.t("memory:toast.docUpdateFailed"), err instanceof Error ? err.message : i18n.t("memory:toast.unknownError"));
        throw err;
      }
    },
    [http, selectedAgentId, invalidate],
  );

  const deleteDocument = useCallback(
    async (path: string, userId?: string, agentId?: string) => {
      try {
        const aid = requireMemoryAgentId(agentId || selectedAgentId);
        const qs = userId ? `?user_id=${encodeURIComponent(userId)}` : "";
        await http.delete(`/v1/agents/${aid}/memory/documents/${path}${qs}`);
        await invalidate();
        toast.success(i18n.t("memory:toast.docDeleted"), path);
      } catch (err) {
        toast.error(i18n.t("memory:toast.docDeleteFailed"), err instanceof Error ? err.message : i18n.t("memory:toast.unknownError"));
        throw err;
      }
    },
    [http, selectedAgentId, invalidate],
  );

  const getChunks = useCallback(
    async (path: string, userId?: string) => {
      const agentId = requireMemoryAgentId(selectedAgentId);
      const params: Record<string, string> = { path };
      if (userId) params.user_id = userId;
      return http.get<MemoryChunk[]>(
        `/v1/agents/${agentId}/memory/chunks`,
        params,
      );
    },
    [http, selectedAgentId],
  );

  const indexDocument = useCallback(
    async (path: string, userId?: string, agentIdOverride?: string) => {
      try {
        const agentId = requireMemoryAgentId(agentIdOverride || selectedAgentId);
        await http.post(`/v1/agents/${agentId}/memory/index`, {
          path,
          user_id: userId || "",
        });
        toast.success(i18n.t("memory:toast.docIndexed"), path);
      } catch (err) {
        toast.error(i18n.t("memory:toast.docIndexFailed"), err instanceof Error ? err.message : i18n.t("memory:toast.unknownError"));
        throw err;
      }
    },
    [http, selectedAgentId],
  );

  const indexAll = useCallback(
    async (userId?: string) => {
      try {
        const agentId = requireMemoryAgentId(selectedAgentId);
        await http.post(`/v1/agents/${agentId}/memory/index-all`, {
          user_id: userId || "",
        });
        toast.success(i18n.t("memory:toast.allIndexed"));
      } catch (err) {
        toast.error(i18n.t("memory:toast.allIndexFailed"), err instanceof Error ? err.message : i18n.t("memory:toast.unknownError"));
        throw err;
      }
    },
    [http, selectedAgentId],
  );

  return {
    documents,
    loading: isLoading,
    fetching: isFetching,
    refresh: invalidate,
    getDocument,
    createDocument,
    updateDocument,
    deleteDocument,
    getChunks,
    indexDocument,
    indexAll,
  };
}

export function useMemorySearch(agentId: string) {
  const http = useHttp();
  const [results, setResults] = useState<MemorySearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const selectedAgentId = normalizeMemoryAgentId(agentId);

  const search = useCallback(
    async (query: string, userId?: string, maxResults?: number, minScore?: number) => {
      setSearching(true);
      try {
        const aid = requireMemoryAgentId(selectedAgentId);
        const res = await http.post<{ results: MemorySearchResult[]; count: number }>(
          `/v1/agents/${aid}/memory/search`,
          {
            query,
            user_id: userId || "",
            max_results: maxResults || 10,
            min_score: minScore || 0,
          },
        );
        setResults(res.results ?? []);
        return res.results ?? [];
      } catch (err) {
        toast.error(i18n.t("memory:toast.searchFailed"), err instanceof Error ? err.message : i18n.t("memory:toast.unknownError"));
        setResults([]);
        return [];
      } finally {
        setSearching(false);
      }
    },
    [http, selectedAgentId],
  );

  return { results, searching, search };
}
