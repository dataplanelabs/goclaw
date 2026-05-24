import { useMemo } from "react";

export interface TeamAnalyticsHistogramProps {
  scores: number[];
}

// bucketScores groups values in [0,1] into 10 equal-width buckets.
// Returns counts[10] where index N is the bucket [N/10, (N+1)/10).
// Score == 1.0 falls into the last bucket (defensive).
export function bucketScores(scores: number[]): number[] {
  const counts = new Array(10).fill(0);
  for (const raw of scores) {
    if (raw == null || Number.isNaN(raw)) continue;
    let s = raw;
    if (s < 0) s = 0;
    if (s > 1) s = 1;
    let idx = Math.floor(s * 10);
    if (idx >= 10) idx = 9;
    counts[idx]++;
  }
  return counts;
}

export function TeamAnalyticsHistogram({ scores }: TeamAnalyticsHistogramProps) {
  const counts = useMemo(() => bucketScores(scores), [scores]);
  const max = Math.max(1, ...counts);
  const barWidth = 22;
  const gap = 4;
  const chartHeight = 80;
  const chartWidth = (barWidth + gap) * counts.length;

  return (
    <div className="overflow-x-auto">
      <svg
        width={chartWidth + 32}
        height={chartHeight + 28}
        role="img"
        aria-label="diff score distribution"
      >
        {counts.map((count, i) => {
          const barHeight = (count / max) * chartHeight;
          const x = i * (barWidth + gap) + 16;
          const y = chartHeight - barHeight + 8;
          return (
            <g key={i}>
              <rect
                x={x}
                y={y}
                width={barWidth}
                height={barHeight}
                fill="currentColor"
                className="text-primary opacity-80"
              >
                <title>{`${(i / 10).toFixed(1)}-${((i + 1) / 10).toFixed(1)}: ${count}`}</title>
              </rect>
              <text
                x={x + barWidth / 2}
                y={chartHeight + 22}
                textAnchor="middle"
                className="fill-muted-foreground text-[10px]"
              >
                {(i / 10).toFixed(1)}
              </text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}
