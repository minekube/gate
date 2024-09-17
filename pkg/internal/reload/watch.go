package reload

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/knadh/koanf/providers/file"
)

var mu sync.Mutex
var debounceTimer *time.Timer

const debounceDuration = 100 * time.Millisecond

func Watch(ctx context.Context, path string, cb func() error) error {
	if ctx.Err() != nil {
		return nil
	}
	log := logr.FromContextOrDiscard(ctx).WithValues("path", path)
	return file.Provider(path).Watch(func(_ any, err error) {
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Info("failed watching config", "error", err)
			return
		}

		mu.Lock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(debounceDuration, func() {
			mu.Lock()
			defer mu.Unlock()

			log.Info("auto-reloading config")
			start := time.Now()
			if err := cb(); err != nil {
				log.Info("failed to reload config", "error", err)
				return
			}
			log.Info("reloaded config successfully", "duration", time.Since(start).Round(time.Millisecond).String())
		})
		mu.Unlock()
	})
}
