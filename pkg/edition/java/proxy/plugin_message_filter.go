package proxy

// ClientPluginMessageFilter decides whether a plugin message a player sent may
// be forwarded to the backend server the player is currently connected to.
//
// It is a low-level hook for integrations that must keep a client plugin
// message away from a backend that would answer it destructively. Returning
// false drops the message: nothing is written to the backend and the client is
// not disconnected. Returning true leaves the normal routing (custom payload
// forwarding, the PluginMessageEvent for registered channels) untouched.
//
// A filter must be cheap and side-effect free: it runs on the player's read
// loop for every custom payload the client sends, before any forwarding
// decision is made. Implementations must return true for channels they do not
// own - a filter is not a general plugin-message allowlist.
type ClientPluginMessageFilter interface {
	AllowClientPluginMessage(channel string, player Player, target RegisteredServer) bool
}
