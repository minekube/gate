package reload

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/knadh/koanf/providers/file"
)

// Watch watches the given path for changes and calls the given callback.
func Watch(ctx context.Context, path string, cb func() error) error {
	if ctx.Err() != nil {
		return nil
	}
	log := logr.FromContextOrDiscard(ctx).WithValues("path", path)
	return file.Provider(path).Watch(func(_ any, err error) {
		// TODO there is no way to stop watching, watchers could pile up
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Info("failed watching config", "error", err)
			return
		}
		log.Info("auto-reloading config")
		start := time.Now()
		if err := cb(); err != nil {
			log.Info("failed to reload config", "error", err)
			return
		}
		log.Info("reloaded config successfully", "duration", time.Since(start).Round(time.Millisecond).String())
	})
}
