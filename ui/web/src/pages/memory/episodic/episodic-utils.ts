import type { EpisodicSummary } from "@/types/memory";

export function getEpisodicKeyTopics(summary: Pick<EpisodicSummary, "key_topics">): string[] {
  if (!Array.isArray(summary.key_topics)) {
    return [];
  }
  return summary.key_topics.filter((topic): topic is string => typeof topic === "string" && topic.trim() !== "");
}
