package lite

import "go.minekube.com/gate/pkg/edition/java/lite/voicechat"

// Lite encapsulates all lite mode functionality for a Gate proxy instance.
// This provides a clean abstraction for lite mode features and avoids global state.
type Lite struct {
	strategyManager *StrategyManager
	voice           *voicechat.Manager
}

// NewLite creates a new Lite instance for a Gate proxy.
func NewLite() *Lite {
	return &Lite{
		strategyManager: NewStrategyManager(),
		voice:           voicechat.NewManager(),
	}
}

// StrategyManager returns the strategy manager for load balancing.
func (l *Lite) StrategyManager() *StrategyManager {
	return l.strategyManager
}

// Voice returns the Simple Voice Chat relay manager.
func (l *Lite) Voice() *voicechat.Manager { return l.voice }
