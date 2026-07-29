package model

// DeclarationKind identifies a public API declaration in the canonical model.
type DeclarationKind string

const (
	DeclarationAlias    DeclarationKind = "alias"
	DeclarationConstant DeclarationKind = "constant"
	DeclarationFunction DeclarationKind = "function"
	DeclarationMethod   DeclarationKind = "method"
	DeclarationType     DeclarationKind = "type"
	DeclarationVariable DeclarationKind = "variable"
)

// TypeKind identifies a language-neutral type shape.
type TypeKind string

const (
	TypeBool     TypeKind = "bool"
	TypeS8       TypeKind = "s8"
	TypeS16      TypeKind = "s16"
	TypeS32      TypeKind = "s32"
	TypeS64      TypeKind = "s64"
	TypeU8       TypeKind = "u8"
	TypeU16      TypeKind = "u16"
	TypeU32      TypeKind = "u32"
	TypeU64      TypeKind = "u64"
	TypeF32      TypeKind = "f32"
	TypeF64      TypeKind = "f64"
	TypeChar     TypeKind = "char"
	TypeString   TypeKind = "string"
	TypeList     TypeKind = "list"
	TypeTuple    TypeKind = "tuple"
	TypeRecord   TypeKind = "record"
	TypeVariant  TypeKind = "variant"
	TypeEnum     TypeKind = "enum"
	TypeFlags    TypeKind = "flags"
	TypeOption   TypeKind = "option"
	TypeResult   TypeKind = "result"
	TypeResource TypeKind = "resource"
	TypeCallback TypeKind = "callback"
	TypeDynamic  TypeKind = "dynamic"
)

// Ownership describes how a value crosses the component boundary.
type Ownership string

const (
	OwnershipCopy   Ownership = "copy"
	OwnershipBorrow Ownership = "borrow"
	OwnershipOwn    Ownership = "own"
)

// Lifetime describes how long a resource handle remains valid.
type Lifetime string

const (
	LifetimeValue         Lifetime = "value"
	LifetimePlugin        Lifetime = "plugin"
	LifetimeOwned         Lifetime = "owned"
	LifetimeBorrowedCall  Lifetime = "borrowed-call"
	LifetimeBorrowedEvent Lifetime = "borrowed-event"
	LifetimeGateOwned     Lifetime = "gate-owned"
)

// CallbackDirection identifies which side calls a callback.
type CallbackDirection string

const (
	CallbackGuestToHost CallbackDirection = "guest-to-host"
	CallbackHostToGuest CallbackDirection = "host-to-guest"
)

// ChannelDirection preserves a Go channel's allowed operations.
type ChannelDirection string

const (
	ChannelBidirectional ChannelDirection = "send-receive"
	ChannelSend          ChannelDirection = "send"
	ChannelReceive       ChannelDirection = "receive"
)

// CoverageState records the fail-closed representation decision.
type CoverageState string

const (
	CoverageRepresented CoverageState = "represented"
	CoverageExcluded    CoverageState = "excluded"
)
