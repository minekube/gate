package tablist

import "reflect"

type (
	tryRLocker interface {
		TryRLock() bool
		RUnlock()
	}
	tryLocker interface {
		TryLock() bool
		Unlock()
	}
)

func equalLocked(a, b any) bool {
	return equalWithLocker(a, a, b, b)
}

func equalWithLocker(lockerA, valueA, lockerB, valueB any) bool {
	if x, ok := lockerA.(tryRLocker); ok && x.TryRLock() {
		defer x.RUnlock()
	}
	if y, ok := lockerB.(tryLocker); ok && y.TryLock() {
		defer y.Unlock()
	}
	return reflect.DeepEqual(valueA, valueB)
}
