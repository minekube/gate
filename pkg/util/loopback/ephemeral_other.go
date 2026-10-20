//go:build !linux

package loopback

// EphemeralRange returns the Linux default ephemeral range on platforms whose
// own range cannot be read. The value only decides whether the allocator accepts
// its configured range, and the Linux default is the widely-used shape
// (32768-60999), so a range outside it is accepted on every platform.
func EphemeralRange() Range {
	return Range{Start: defaultEphemeralStart, End: defaultEphemeralEnd}
}
