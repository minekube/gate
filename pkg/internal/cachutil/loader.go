package cachutil

import (
	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/sync/singleflight"
)

// SuppressedLoader wraps another Loader and suppresses duplicate
// calls to its Load method.
type SuppressedLoader[V any] struct {
	ttlcache.Loader[string, V]

	group singleflight.Group
}

// Load executes a custom item retrieval logic and returns the item that
// is associated with the key.
// It returns nil if the item is not found/valid.
// It also ensures that only one execution of the wrapped Loader's Load
// method is in-flight for a given key at a time.
func (l *SuppressedLoader[V]) Load(c *ttlcache.Cache[string, V], key string) *ttlcache.Item[string, V] {
	// the error can be discarded since the singleflight.Group
	// itself does not return any of its errors, it returns
	// the error that we return ourselves in the func below, which
	// is also nil
	res, _, _ := l.group.Do(key, func() (interface{}, error) {
		item := l.Loader.Load(c, key)
		if item == nil {
			return nil, nil
		}

		return item, nil
	})
	if res == nil {
		return nil
	}

	return res.(*ttlcache.Item[string, V])
}
