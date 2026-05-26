import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";

export interface Voice {
  voice_id: string;
  name: string;
  preview_url?: string;
  labels?: Record<string, string>;
  category?: string;
}

interface VoicesResponse {
  voices: Voice[];
}

export const voiceKeys = {
  all: ["voices"] as const,
  byProvider: (provider?: string) => ["voices", provider ?? "default"] as const,
};

function buildQuery(provider?: string): string {
  return provider ? `?provider=${encodeURIComponent(provider)}` : "";
}

export function useVoices(provider?: string) {
  const http = useHttp();
  return useQuery({
    queryKey: voiceKeys.byProvider(provider),
    queryFn: async () => {
      const res = await http.get<VoicesResponse>(`/v1/voices${buildQuery(provider)}`);
      return res.voices ?? [];
    },
    staleTime: 5 * 60_000,
    retry: 1,
  });
}

export function useRefreshVoices(provider?: string) {
  const http = useHttp();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => http.post<{ status: string }>(`/v1/voices/refresh${buildQuery(provider)}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: voiceKeys.byProvider(provider) });
    },
  });
}
