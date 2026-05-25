export interface TeamReplyEvaluation {
  id: string;
  channel_instance_id: string;
  thread_key: string;
  session_key: string;
  team_msg_id: string;
  captured_at: string;
  updated_at: string;
  customer_message: string;
  team_reply: string;
  hypothesized_bot_reply?: string;
  diff_score?: number;
  diff_reasoning?: string;
  judge_agent_key?: string;
  judge_model?: string;
  judge_provider?: string;
  judge_latency_ms?: number;
  judge_error?: string;
  judge_completed_at?: string;
}
