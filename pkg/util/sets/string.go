package sets

type Empty struct{}

// sets.String is a set of strings, implemented via map[string]struct{} for minimal memory consumption.
type String map[string]Empty

// NewString creates a String from a list of values.
func NewString(items ...string) String {
	return String{}.Insert(items...)
}

// Insert adds items to the set.
func (s String) Insert(items ...string) String {
	for _, insert := range items {
		s[insert] = Empty{}
	}
	return s
}

// Delete removes all items from the set.
func (s String) Delete(items ...string) String {
	for _, rem := range items {
		delete(s, rem)
	}
	return s
}

// Has returns true if and only if item is contained in the set.
func (s String) Has(item string) bool {
	_, ok := s[item]
	return ok
}

// HasAll returns true if and only if all items are contained in the set.
func (s String) HasAll(items ...string) bool {
	for _, item := range items {
		if !s.Has(item) {
			return false
		}
	}
	return true
}

// UnsortedList returns the slice with contents in random order.
func (s String) UnsortedList() []string {
	res := make([]string, 0, len(s))
	for key := range s {
		res = append(res, key)
	}
	return res
}

// InsertSet inserts all items from sets into s.
func (s String) InsertSet(sets ...String) String {
	for _, other := range sets {
		for key := range other {
			s.Insert(key)
		}
	}
	return s
}

// Len returns the size of the set.
func (s String) Len() int {
	return len(s)
}
