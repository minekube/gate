# Project agent memory

This file is the project's committed home for project-intrinsic agent knowledge: build, test, release, architecture, and sharp-edge notes that should travel with the code.

- Add durable project-specific notes here as they are discovered through real work.
- Secure-chat acknowledgement state is maintained in `pkg/edition/java/proxy/chat_queue.go` and driven by the handlers in `handle_cmd.go`/`handle_chat.go`; it is a direct port of Velocity (`ChatQueue`, `SessionCommandHandler`), which is the reference for correct behavior. The `LastSeenMessages.Offset += delayedAckCount` adjustment is intentional and matches Velocity — not dead code — so validate any change against Velocity's source and a proxy-level reproduction. Key invariant: an `UnsignedPlayerCommand` (1.20.5+, commands with no signable args — the common path in offline mode) carries NO last-seen update, so it must not be fed into the chat queue (that flushes and discards the player's held acknowledgements) and must be forwarded command-only; a consumed command's `ChatAcknowledgement` gates on the offset, not the acknowledged bitset. Regression coverage: `handle_cmd_ack_test.go`.

## Maintaining this file

Keep this file for knowledge useful to almost every future agent session in this project.
Do not repeat what the codebase already shows; point to the authoritative file or command instead.
Prefer rewriting or pruning existing entries over appending new ones.
When updating this file, preserve this bar for all agents and keep entries concise.
