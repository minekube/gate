package oncetrue

import (
	"sync"
)

type OnceWhenTrue struct {
	condition bool
	onTrue    func()
	called    bool
	mu        sync.Mutex
}

func NewOnceWhenTrue() *OnceWhenTrue {
	return &OnceWhenTrue{}
}

func (o *OnceWhenTrue) DoWhenTrue(onTrue func()) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.onTrue = onTrue

	// If condition is true and onTrue hasn't been called, call it
	if o.condition && !o.called {
		o.onTrue()
		o.called = true
	}
}

func (o *OnceWhenTrue) SetTrue() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.condition = true

	// If onTrue is set and hasn't been called, call it
	if o.onTrue != nil && !o.called {
		o.onTrue()
		o.called = true
	}
}
