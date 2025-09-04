package lite

// Lite encapsulates all lite mode functionality for a Gate proxy instance.
// This provides a clean abstraction for lite mode features and avoids global state.
type Lite struct {
	strategyManager *StrategyManager
}

// NewLite creates a new Lite instance for a Gate proxy.
func NewLite() *Lite {
	return &Lite{
		strategyManager: NewStrategyManager(),
	}
}

// StrategyManager returns the strategy manager for load balancing.
func (l *Lite) StrategyManager() *StrategyManager {
	return l.strategyManager
}
