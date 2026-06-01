const INVALID_AGENT_IDS = new Set(["", "undefined", "null"]);

export function normalizeMemoryAgentId(agentId?: string | null): string {
  const value = agentId?.trim() ?? "";
  return INVALID_AGENT_IDS.has(value.toLowerCase()) ? "" : value;
}

export function requireMemoryAgentId(agentId?: string | null): string {
  const value = normalizeMemoryAgentId(agentId);
  if (!value) {
    throw new Error("No agent selected");
  }
  return value;
}
