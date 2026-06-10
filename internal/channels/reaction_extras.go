package channels

import "context"

// ReactionExtras carries optional agent-run metrics that channels with
// deterministic reactions can use to gate trivial acks (e.g. skip a heart
// on a one-liner reply with no tool use).
type ReactionExtras struct {
	DurationMs  int
	Iterations  int
	ToolCalls   int
	OutputChars int
}

type reactionExtrasKey struct{}

func WithReactionExtras(ctx context.Context, e ReactionExtras) context.Context {
	return context.WithValue(ctx, reactionExtrasKey{}, e)
}

func ReactionExtrasFromCtx(ctx context.Context) (ReactionExtras, bool) {
	e, ok := ctx.Value(reactionExtrasKey{}).(ReactionExtras)
	return e, ok
}
