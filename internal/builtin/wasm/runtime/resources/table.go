package resources

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

type ErrorKind string

const (
	ErrorInvalidHandle   ErrorKind = "invalid-handle"
	ErrorForeignHandle   ErrorKind = "foreign-handle"
	ErrorTypeMismatch    ErrorKind = "type-mismatch"
	ErrorStaleHandle     ErrorKind = "stale-handle"
	ErrorExpiredHandle   ErrorKind = "expired-handle"
	ErrorDoubleDrop      ErrorKind = "double-drop"
	ErrorCapacity        ErrorKind = "capacity"
	ErrorClosed          ErrorKind = "closed"
	ErrorInvalidLifetime ErrorKind = "invalid-lifetime"
	ErrorForeignScope    ErrorKind = "foreign-scope"
	ErrorExpiredScope    ErrorKind = "expired-scope"
)

type Error struct {
	Kind     ErrorKind
	Handle   Handle
	Plugin   string
	Expected string
	Actual   string
	Detail   string
}

func (err *Error) Error() string {
	message := "wasm resource " + string(err.Kind)
	if err.Detail != "" {
		message += ": " + err.Detail
	}
	return message
}

func (err *Error) Is(target error) bool {
	var resourceError *Error
	return errors.As(target, &resourceError) && err.Kind == resourceError.Kind
}

var (
	ErrInvalidHandle   = &Error{Kind: ErrorInvalidHandle}
	ErrForeignHandle   = &Error{Kind: ErrorForeignHandle}
	ErrTypeMismatch    = &Error{Kind: ErrorTypeMismatch}
	ErrStaleHandle     = &Error{Kind: ErrorStaleHandle}
	ErrExpiredHandle   = &Error{Kind: ErrorExpiredHandle}
	ErrDoubleDrop      = &Error{Kind: ErrorDoubleDrop}
	ErrCapacity        = &Error{Kind: ErrorCapacity}
	ErrClosed          = &Error{Kind: ErrorClosed}
	ErrInvalidLifetime = &Error{Kind: ErrorInvalidLifetime}
	ErrForeignScope    = &Error{Kind: ErrorForeignScope}
	ErrExpiredScope    = &Error{Kind: ErrorExpiredScope}
)

type Lifetime string

const (
	LifetimePlugin        Lifetime = "plugin"
	LifetimeOwned         Lifetime = "owned"
	LifetimeBorrowedCall  Lifetime = "borrowed-call"
	LifetimeBorrowedEvent Lifetime = "borrowed-event"
	LifetimeGateOwned     Lifetime = "gate-owned"
)

type Metadata struct {
	Plugin   string
	Type     string
	Lifetime Lifetime
}

type Stats struct {
	Live     uint64
	Inserted uint64
	Released uint64
	Dropped  uint64
	Expired  uint64
}

type slotState uint8

const (
	slotUnused slotState = iota
	slotActive
	slotDropped
	slotExpired
)

type slot struct {
	generation uint16
	state      slotState
	value      any
	metadata   Metadata
	release    func()
	scopeID    uint64
	retired    bool
}

type Table struct {
	mu        sync.Mutex
	plugin    string
	owner     uint32
	capacity  uint32
	slots     []slot
	free      []uint32
	scopes    map[uint64]*Scope
	nextScope uint64
	stats     Stats
	closed    bool
}

var (
	nextTableOwner atomic.Uint32
	liveResources  atomic.Int64
)

func NewTable(plugin string, capacity uint32) *Table {
	if capacity == 0 || capacity > maxSlot {
		capacity = maxSlot
	}
	owner := nextTableOwner.Add(1)
	if owner == 0 || owner > maxOwner {
		panic("wasm resource table owner space exhausted")
	}
	return &Table{
		plugin: plugin, owner: owner, capacity: capacity,
		scopes: make(map[uint64]*Scope),
	}
}

func LiveResources() int64 {
	return liveResources.Load()
}

func (table *Table) Insert(
	value any,
	typeIdentity string,
	lifetime Lifetime,
	release func(),
) (Handle, error) {
	switch lifetime {
	case LifetimePlugin, LifetimeOwned, LifetimeGateOwned:
	default:
		return 0, &Error{
			Kind: ErrorInvalidLifetime, Plugin: table.plugin,
			Actual: string(lifetime),
		}
	}
	return table.insert(value, typeIdentity, lifetime, release, 0)
}

func (table *Table) Borrow(
	scope *Scope,
	value any,
	typeIdentity string,
	release func(),
) (Handle, error) {
	if scope == nil || scope.table != table {
		return 0, &Error{Kind: ErrorForeignScope, Plugin: table.plugin}
	}
	table.mu.Lock()
	if table.closed {
		table.mu.Unlock()
		return 0, &Error{Kind: ErrorClosed, Plugin: table.plugin}
	}
	active := table.scopes[scope.id]
	if active != scope || scope.closed {
		table.mu.Unlock()
		return 0, &Error{Kind: ErrorExpiredScope, Plugin: table.plugin}
	}
	lifetime := scope.lifetime
	table.mu.Unlock()
	return table.insert(value, typeIdentity, lifetime, release, scope.id)
}

func (table *Table) insert(
	value any,
	typeIdentity string,
	lifetime Lifetime,
	release func(),
	scopeID uint64,
) (Handle, error) {
	if typeIdentity == "" {
		return 0, &Error{
			Kind: ErrorTypeMismatch, Plugin: table.plugin,
			Detail: "resource type identity is empty",
		}
	}
	table.mu.Lock()
	defer table.mu.Unlock()
	if table.closed {
		return 0, &Error{Kind: ErrorClosed, Plugin: table.plugin}
	}
	if scopeID != 0 {
		scope := table.scopes[scopeID]
		if scope == nil || scope.closed || scope.lifetime != lifetime {
			return 0, &Error{Kind: ErrorExpiredScope, Plugin: table.plugin}
		}
	}
	index, generation, ok := table.allocateSlot()
	if !ok {
		return 0, &Error{
			Kind: ErrorCapacity, Plugin: table.plugin,
			Detail: fmt.Sprintf("limit is %d", table.capacity),
		}
	}
	handle, err := encodeHandle(table.owner, generation, index+1)
	if err != nil {
		return 0, err
	}
	table.slots[index] = slot{
		generation: generation,
		state:      slotActive,
		value:      value,
		metadata: Metadata{
			Plugin: table.plugin, Type: typeIdentity, Lifetime: lifetime,
		},
		release: release,
		scopeID: scopeID,
	}
	table.stats.Live++
	table.stats.Inserted++
	liveResources.Add(1)
	return handle, nil
}

func (table *Table) allocateSlot() (uint32, uint16, bool) {
	for len(table.free) > 0 {
		last := len(table.free) - 1
		index := table.free[last]
		table.free = table.free[:last]
		existing := &table.slots[index]
		if existing.retired || existing.generation == ^uint16(0) {
			existing.retired = true
			continue
		}
		return index, existing.generation + 1, true
	}
	if uint32(len(table.slots)) >= table.capacity {
		return 0, 0, false
	}
	index := uint32(len(table.slots))
	table.slots = append(table.slots, slot{})
	return index, 1, true
}

func (table *Table) Resolve(
	handle Handle,
	typeIdentity string,
) (any, error) {
	table.mu.Lock()
	defer table.mu.Unlock()
	resource, err := table.resolveSlot(handle)
	if err != nil {
		return nil, err
	}
	if resource.metadata.Type != typeIdentity {
		return nil, &Error{
			Kind: ErrorTypeMismatch, Handle: handle, Plugin: table.plugin,
			Expected: typeIdentity, Actual: resource.metadata.Type,
		}
	}
	return resource.value, nil
}

// ResolveAny returns the Go value behind a validated handle without applying a
// second string identity check. Callers must still validate the Go type they
// require. This is used after the component model has already type-checked a
// resource at the WIT boundary.
func (table *Table) ResolveAny(handle Handle) (any, Metadata, error) {
	table.mu.Lock()
	defer table.mu.Unlock()
	resource, err := table.resolveSlot(handle)
	if err != nil {
		return nil, Metadata{}, err
	}
	return resource.value, resource.metadata, nil
}

func ResolveAs[T any](
	table *Table,
	handle Handle,
	typeIdentity string,
) (T, error) {
	var zero T
	value, err := table.Resolve(handle, typeIdentity)
	if err != nil {
		return zero, err
	}
	typed, ok := value.(T)
	if !ok {
		return zero, &Error{
			Kind: ErrorTypeMismatch, Handle: handle, Plugin: table.plugin,
			Expected: typeIdentity,
			Detail:   fmt.Sprintf("stored Go value has type %T", value),
		}
	}
	return typed, nil
}

func (table *Table) Metadata(handle Handle) (Metadata, error) {
	table.mu.Lock()
	defer table.mu.Unlock()
	resource, err := table.resolveSlot(handle)
	if err != nil {
		return Metadata{}, err
	}
	return resource.metadata, nil
}

func (table *Table) resolveSlot(handle Handle) (*slot, error) {
	if table.closed {
		return nil, &Error{Kind: ErrorClosed, Handle: handle, Plugin: table.plugin}
	}
	parts, err := DecodeHandle(handle)
	if err != nil {
		return nil, err
	}
	if parts.Owner != table.owner {
		return nil, &Error{
			Kind: ErrorForeignHandle, Handle: handle, Plugin: table.plugin,
		}
	}
	index := parts.Slot - 1
	if index >= uint32(len(table.slots)) {
		return nil, &Error{
			Kind: ErrorStaleHandle, Handle: handle, Plugin: table.plugin,
		}
	}
	resource := &table.slots[index]
	if resource.generation != parts.Generation {
		return nil, &Error{
			Kind: ErrorStaleHandle, Handle: handle, Plugin: table.plugin,
		}
	}
	switch resource.state {
	case slotActive:
		return resource, nil
	case slotExpired:
		return nil, &Error{
			Kind: ErrorExpiredHandle, Handle: handle, Plugin: table.plugin,
		}
	default:
		return nil, &Error{
			Kind: ErrorStaleHandle, Handle: handle, Plugin: table.plugin,
		}
	}
}

func (table *Table) Drop(handle Handle) error {
	return table.releaseHandle(handle, slotDropped, false)
}

func (table *Table) Invalidate(handle Handle) error {
	return table.releaseHandle(handle, slotExpired, true)
}

func (table *Table) releaseHandle(
	handle Handle,
	state slotState,
	requireGateOwned bool,
) error {
	table.mu.Lock()
	if table.closed {
		table.mu.Unlock()
		return &Error{Kind: ErrorClosed, Handle: handle, Plugin: table.plugin}
	}
	parts, err := DecodeHandle(handle)
	if err != nil {
		table.mu.Unlock()
		return err
	}
	if parts.Owner != table.owner {
		table.mu.Unlock()
		return &Error{
			Kind: ErrorForeignHandle, Handle: handle, Plugin: table.plugin,
		}
	}
	index := parts.Slot - 1
	if index >= uint32(len(table.slots)) {
		table.mu.Unlock()
		return &Error{
			Kind: ErrorStaleHandle, Handle: handle, Plugin: table.plugin,
		}
	}
	resource := &table.slots[index]
	if resource.generation != parts.Generation {
		table.mu.Unlock()
		return &Error{
			Kind: ErrorStaleHandle, Handle: handle, Plugin: table.plugin,
		}
	}
	if resource.state == slotDropped {
		table.mu.Unlock()
		return &Error{
			Kind: ErrorDoubleDrop, Handle: handle, Plugin: table.plugin,
		}
	}
	if resource.state == slotExpired {
		table.mu.Unlock()
		return &Error{
			Kind: ErrorExpiredHandle, Handle: handle, Plugin: table.plugin,
		}
	}
	if resource.state != slotActive {
		table.mu.Unlock()
		return &Error{
			Kind: ErrorStaleHandle, Handle: handle, Plugin: table.plugin,
		}
	}
	if requireGateOwned && resource.metadata.Lifetime != LifetimeGateOwned {
		table.mu.Unlock()
		return &Error{
			Kind: ErrorInvalidLifetime, Handle: handle, Plugin: table.plugin,
			Expected: string(LifetimeGateOwned),
			Actual:   string(resource.metadata.Lifetime),
		}
	}
	if !requireGateOwned &&
		(resource.metadata.Lifetime == LifetimeBorrowedCall ||
			resource.metadata.Lifetime == LifetimeBorrowedEvent) {
		table.mu.Unlock()
		return &Error{
			Kind: ErrorInvalidLifetime, Handle: handle, Plugin: table.plugin,
			Detail: "borrowed resources expire with their scope",
		}
	}
	release := table.markReleased(index, state)
	if state == slotDropped {
		table.stats.Dropped++
	} else {
		table.stats.Expired++
	}
	table.mu.Unlock()
	if release != nil {
		release()
	}
	return nil
}

func (table *Table) markReleased(index uint32, state slotState) func() {
	resource := &table.slots[index]
	release := resource.release
	resource.state = state
	resource.value = nil
	resource.release = nil
	resource.scopeID = 0
	table.free = append(table.free, index)
	table.stats.Live--
	table.stats.Released++
	liveResources.Add(-1)
	return release
}

type Scope struct {
	table    *Table
	id       uint64
	lifetime Lifetime
	closed   bool
}

func (table *Table) BeginScope(lifetime Lifetime) (*Scope, error) {
	if lifetime != LifetimeBorrowedCall &&
		lifetime != LifetimeBorrowedEvent {
		return nil, &Error{
			Kind: ErrorInvalidLifetime, Plugin: table.plugin,
			Actual: string(lifetime),
		}
	}
	table.mu.Lock()
	defer table.mu.Unlock()
	if table.closed {
		return nil, &Error{Kind: ErrorClosed, Plugin: table.plugin}
	}
	table.nextScope++
	scope := &Scope{
		table: table, id: table.nextScope, lifetime: lifetime,
	}
	table.scopes[scope.id] = scope
	return scope, nil
}

func (scope *Scope) Close() error {
	if scope == nil || scope.table == nil {
		return &Error{Kind: ErrorExpiredScope}
	}
	return scope.table.closeScope(scope)
}

func (table *Table) closeScope(scope *Scope) error {
	table.mu.Lock()
	if table.closed {
		table.mu.Unlock()
		return &Error{Kind: ErrorClosed, Plugin: table.plugin}
	}
	if table.scopes[scope.id] != scope || scope.closed {
		table.mu.Unlock()
		return &Error{Kind: ErrorExpiredScope, Plugin: table.plugin}
	}
	scope.closed = true
	delete(table.scopes, scope.id)
	var releases []func()
	for index := range table.slots {
		resource := &table.slots[index]
		if resource.state != slotActive || resource.scopeID != scope.id {
			continue
		}
		if release := table.markReleased(uint32(index), slotExpired); release != nil {
			releases = append(releases, release)
		}
		table.stats.Expired++
	}
	table.mu.Unlock()
	for _, release := range releases {
		release()
	}
	return nil
}

func (table *Table) Stats() Stats {
	table.mu.Lock()
	defer table.mu.Unlock()
	return table.stats
}

func (table *Table) Close() error {
	table.mu.Lock()
	if table.closed {
		table.mu.Unlock()
		return nil
	}
	table.closed = true
	for _, scope := range table.scopes {
		scope.closed = true
	}
	clear(table.scopes)
	var releases []func()
	for index := range table.slots {
		resource := &table.slots[index]
		if resource.state != slotActive {
			continue
		}
		release := resource.release
		resource.state = slotExpired
		resource.value = nil
		resource.release = nil
		resource.scopeID = 0
		table.stats.Live--
		table.stats.Released++
		table.stats.Expired++
		liveResources.Add(-1)
		if release != nil {
			releases = append(releases, release)
		}
	}
	table.mu.Unlock()
	for _, release := range releases {
		release()
	}
	return nil
}
