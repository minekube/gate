// Package bufpool - All credits go to https://github.com/valyala/bytebufferpool.
// This package uses []byte instead.
package bufpool

import (
	"bytes"
	"sort"
	"sync"
	"sync/atomic"
)

const (
	minBitSize = 6 // 2**6=64 is a CPU cache line size
	steps      = 20

	minSize = 1 << minBitSize
	//maxSize = 1 << (minBitSize + steps - 1)

	calibrateCallsThreshold = 42000
	maxPercentile           = 0.95
)

// Pool represents byte buffer pool.
//
// Distinct pools may be used for distinct types of byte buffers.
// Properly determined byte buffer types with their own pools may help to reduce
// memory waste.
type Pool struct {
	DisableCalibration bool

	calls       [steps]atomic.Uint64
	calibrating atomic.Uint64

	defaultSize atomic.Uint64
	maxSize     atomic.Uint64

	pool sync.Pool
}

var defaultPool Pool

// Get returns an empty byte buffer from the pool.
//
// Got byte buffer may be returned to the pool via Put call.
// This reduces the number of memory allocations required for byte buffer
// management.
func Get() *bytes.Buffer { return defaultPool.Get() }

// Get returns new byte buffer with zero length.
//
// The byte buffer may be returned to the pool via Put after the use
// in order to minimize GC overhead.
func (p *Pool) Get() *bytes.Buffer {
	v := p.pool.Get()
	if v != nil {
		return v.(*bytes.Buffer)
	}
	return bytes.NewBuffer(make([]byte, 0, p.defaultSize.Load()))
}

// Put returns byte buffer to the pool.
//
// ByteBuffer.B mustn't be touched after returning it to the pool.
// Otherwise, data races will occur.
func Put(b *bytes.Buffer) { defaultPool.Put(b) }

// Put releases byte buffer obtained via Get to the pool.
//
// The buffer mustn't be accessed after returning to the pool.
func (p *Pool) Put(b *bytes.Buffer) {
	idx := index(b.Len())

	if !p.DisableCalibration &&
		p.calls[idx].Add(1) > calibrateCallsThreshold {
		p.calibrate()
	}

	maxSize := int(p.maxSize.Load())
	if maxSize == 0 || b.Cap() <= maxSize {
		b.Reset()
		p.pool.Put(b)
	}
}

func (p *Pool) calibrate() {
	if !p.calibrating.CompareAndSwap(0, 1) {
		return
	}

	a := make(callSizes, 0, steps)
	var callsSum uint64
	for i := uint64(0); i < steps; i++ {
		calls := p.calls[i].Swap(0)
		callsSum += calls
		a = append(a, callSize{
			calls: calls,
			size:  minSize << i,
		})
	}
	sort.Sort(a)

	defaultSize := a[0].size
	maxSize := defaultSize

	maxSum := uint64(float64(callsSum) * maxPercentile)
	callsSum = 0
	for i := 0; i < steps; i++ {
		if callsSum > maxSum {
			break
		}
		callsSum += a[i].calls
		size := a[i].size
		if size > maxSize {
			maxSize = size
		}
	}

	p.defaultSize.Store(defaultSize)
	p.maxSize.Store(maxSize)

	p.calibrating.Store(0)
}

type callSize struct {
	calls uint64
	size  uint64
}

type callSizes []callSize

func (ci callSizes) Len() int {
	return len(ci)
}

func (ci callSizes) Less(i, j int) bool {
	return ci[i].calls > ci[j].calls
}

func (ci callSizes) Swap(i, j int) {
	ci[i], ci[j] = ci[j], ci[i]
}

func index(n int) int {
	n--
	n >>= minBitSize
	idx := 0
	for n > 0 {
		n >>= 1
		idx++
	}
	if idx >= steps {
		idx = steps - 1
	}
	return idx
}
