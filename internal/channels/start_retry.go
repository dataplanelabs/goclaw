package channels

import (
	"context"
	"time"
)

// channelStartRetryDelays bounds retries for transient startup probes. These
// are intentionally short: channels already spend time in their own network
// calls, so this only gives DNS/network dependencies a chance to recover.
var channelStartRetryDelays = []time.Duration{
	1 * time.Second,
	3 * time.Second,
	10 * time.Second,
}

func channelStartMaxAttempts() int {
	return len(channelStartRetryDelays) + 1
}

func channelStartRetryDelay(err error, failedAttempt int) (ChannelErrorInfo, time.Duration, bool) {
	info := ClassifyChannelError(err)
	if !info.Retryable {
		return info, 0, false
	}
	if failedAttempt < 1 || failedAttempt > len(channelStartRetryDelays) {
		return info, 0, false
	}
	return info, channelStartRetryDelays[failedAttempt-1], true
}

func waitChannelStartRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func markChannelStarting(channel Channel) {
	if hc, ok := channel.(interface{ MarkStarting(string) }); ok {
		hc.MarkStarting("Starting")
	}
}
