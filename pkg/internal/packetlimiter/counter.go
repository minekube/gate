package packetlimiter

import "time"

// counter is a sliding-window counter over a fixed interval. It keeps
// (time, count) data points in a growable ring buffer, expiring entries older
// than the interval and maintaining a running sum. Times are nanoseconds.
//
// It is a Go port of the IntervalledCounter approach used by Velocity/Paper.
// Not safe for concurrent use; callers synchronize.
type counter struct {
	interval int64 // window length in nanoseconds
	times    []int64
	counts   []int64
	head     int // inclusive
	tail     int // exclusive
	total    int64
	minTime  int64
}

const initialCounterSize = 8

func newCounter(interval time.Duration) *counter {
	return &counter{
		interval: int64(interval),
		times:    make([]int64, initialCounterSize),
		counts:   make([]int64, initialCounterSize),
	}
}

// updateAndAdd expires entries older than the window relative to now, then
// records count at now.
func (c *counter) updateAndAdd(count, now int64) {
	c.expire(now)
	c.add(now, count)
}

// expire drops entries older than now-interval. Subtraction is used for the
// comparison to stay correct across clock wraparound.
func (c *counter) expire(now int64) {
	minTime := now - c.interval
	arrayLen := len(c.times)
	for c.head != c.tail && c.times[c.head]-minTime < 0 {
		c.total -= c.counts[c.head]
		c.counts[c.head] = 0
		c.head++
		if c.head >= arrayLen {
			c.head = 0
		}
	}
	c.minTime = minTime
}

func (c *counter) add(now, count int64) {
	if now-c.minTime < 0 {
		return // older than the current window, ignore
	}
	nextTail := (c.tail + 1) % len(c.times)
	if nextTail == c.head {
		c.resize()
		nextTail = (c.tail + 1) % len(c.times)
	}
	c.times[c.tail] = now
	c.counts[c.tail] += count
	c.total += count
	c.tail = nextTail
}

func (c *counter) resize() {
	oldTimes, oldCounts := c.times, c.counts
	oldLen := len(oldTimes)
	size := c.tail - c.head
	if size < 0 {
		size += oldLen
	}
	newTimes := make([]int64, oldLen*2)
	newCounts := make([]int64, oldLen*2)
	if c.tail >= c.head {
		copy(newTimes, oldTimes[c.head:c.tail])
		copy(newCounts, oldCounts[c.head:c.tail])
	} else {
		n := copy(newTimes, oldTimes[c.head:])
		copy(newTimes[n:], oldTimes[:c.tail])
		n = copy(newCounts, oldCounts[c.head:])
		copy(newCounts[n:], oldCounts[:c.tail])
	}
	c.times, c.counts = newTimes, newCounts
	c.head = 0
	c.tail = size
}

// sum returns the total count currently within the window.
func (c *counter) sum() int64 { return c.total }

// rate returns the per-second rate over the window.
func (c *counter) rate() float64 {
	return float64(c.total) / (float64(c.interval) * 1e-9)
}
