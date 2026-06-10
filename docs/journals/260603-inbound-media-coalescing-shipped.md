# Inbound Media Coalescing Shipped

Date: 2026-06-03

## What Shipped

- Gateway inbound debounce now treats media as part of the debounce window
  instead of bypassing and splitting text/media into separate turns.
- Zalo Personal addressed turns now use a short grace window before dispatch so
  nearby follow-up media can be included in the same agent run.
- Zalo Personal group flush rebuilds pending-history context and collects media
  from the same snapshot before clearing history.
- Added a reusable `channels.TurnCoalescer` so Zalo OA and Zalo Bot can adopt
  the same turn assembly pattern later without duplicating timer logic.

## Key Decision

The fix belongs at inbound turn assembly, not inside the agent pipeline. The
failed trace had no media in the replay payload, so pipeline-time refetch would
make replay nondeterministic without guaranteeing the channel-local media was
visible. Keeping the coalescing before `HandleMessage` preserves deterministic
trace input.

## Gotchas

- Mention-gated group media may be recorded in pending history and never reach
  the shared gateway debouncer, so Zalo Personal needed channel-boundary logic.
- Timer callbacks use generation guards in the shared coalescer to avoid stale
  callbacks flushing replaced pending turns.
