import { z } from "zod";

export const cronAdvancedSchema = z.object({
  timezone: z.string(),
  deliver: z.boolean(),
  channel: z.string(),
  to: z.string(),
  wakeHeartbeat: z.boolean(),
  injectTargetHistory: z.boolean(),
  // Empty/cleared input yields NaN/0; round-trip to the default 50 instead of
  // failing .min(5) and silently blocking submit with no field error.
  injectTargetHistoryLimit: z
    .number()
    .transform((v) => (Number.isFinite(v) && v >= 5 ? Math.min(Math.round(v), 200) : 50)),
  deleteAfterRun: z.boolean(),
  stateless: z.boolean(),
});

export type CronAdvancedFormData = z.infer<typeof cronAdvancedSchema>;
