package codec

import (
	"bytes"
	"sync"

	"go.minekube.com/gate/pkg/internal/bufpool"
)

var encodePool, compressPool poolMap

type poolMap struct {
	// using sync.Map since optimized for:
	// when the entry for a given key is only ever written once but read many times
	pools sync.Map // map[poolKey]*bytes.Buffer
}

// using this pool to create bufpools and putting them back if already created for a type since
// sync.Map#LoadOrStore already requires the new pool before knowing if it's already present for that type.
var bufpoolPool = sync.Pool{New: func() any {
	return &bufpool.Pool{}
}}

func (p *poolMap) getBuf(key any) (*bytes.Buffer, func()) {
	valPool := bufpoolPool.Get()
	actual, loaded := p.pools.LoadOrStore(key, valPool)
	if loaded {
		bufpoolPool.Put(valPool)
	}
	pool := actual.(*bufpool.Pool)
	buf := pool.Get()
	return buf, func() { pool.Put(buf) }
}
