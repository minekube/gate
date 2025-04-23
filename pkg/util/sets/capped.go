package sets

type CappedSet[T comparable] struct {
	items map[T]struct{}
	cap   int
}

func NewCappedSet[T comparable](cap int) *CappedSet[T] {
	return &CappedSet[T]{
		items: make(map[T]struct{}),
		cap:   cap,
	}
}

func (s *CappedSet[T]) Len() int {
	return len(s.items)
}

func (s *CappedSet[T]) Add(items ...T) {
	for _, item := range items {
		s.items[item] = struct{}{}
	}
}

func (s *CappedSet[T]) Remove(items ...T) {
	for _, item := range items {
		delete(s.items, item)
	}
}

func (s *CappedSet[T]) UnsortedList() []T {
	list := make([]T, 0, len(s.items))
	for item := range s.items {
		list = append(list, item)
	}
	return list
}
