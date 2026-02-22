# lite-observer

Minimal example showing how to register a `lite.Plugin` and observe Lite TCP forward
start/end events through the Lite runtime event manager.

This example only uses generic Lite observability hooks:
- `lite.ForwardStartedEvent`
- `lite.ForwardEndedEvent`
- `lite.Runtime.ActiveForwardsByClientIP`

It does not add protocol-specific routing logic.
