# pkg/gate agent notes

HTTP config writes share the transactional reload boundary in `pkg/gate/gate.go`: `GetConfig` returns an opaque version, `ApplyConfig` requires it and may target edits with JSON Merge Patch, and only route changes in an already-enabled Java Lite config are live-safe. Keep `pkg/gate/api_handlers.go` on `Gate.ApplyLiveConfigIfVersion`; broad pointer mutation plus `reload.FireConfigUpdate` can claim startup-bound fields changed when the running listeners and forwarding mode did not.
