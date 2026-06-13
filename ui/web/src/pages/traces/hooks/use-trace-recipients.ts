import { useQuery } from "@tanstack/react-query";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";

export interface TraceRecipient {
  user_id: string;
  label: string;
}

export function useTraceRecipients() {
  const http = useHttp();

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.traces.recipients,
    queryFn: async () => {
      const res = await http.get<{ recipients: TraceRecipient[] }>("/v1/traces/recipients");
      return res.recipients ?? [];
    },
    staleTime: 5 * 60 * 1000,
  });

  return { recipients: data ?? [], loading: isLoading };
}
