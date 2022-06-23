package event

// Nop is an event Manager that does nothing.
var Nop Manager = &nopMgr{}

type nopMgr struct{}

func (n *nopMgr) Subscribe(any, int, HandlerFunc) (unsubscribe func()) {
	return func() {}
}
func (n *nopMgr) Fire(Event)                         {}
func (n *nopMgr) FireParallel(Event, ...HandlerFunc) {}
func (n *nopMgr) Wait()                              {}
