package model

// API is the deterministic canonical model consumed by every generator.
type API struct {
	FormatVersion uint32        `json:"formatVersion"`
	ModulePath    string        `json:"modulePath"`
	Packages      []Package     `json:"packages"`
	Declarations  []Declaration `json:"declarations"`
}

// Package identifies one in-scope Go package and its WIT interface.
type Package struct {
	Path          string   `json:"path"`
	Name          string   `json:"name"`
	WITName       string   `json:"witName"`
	Documentation string   `json:"documentation,omitempty"`
	Declarations  []string `json:"declarations"`
}

// Declaration is one public Go declaration and its lowered contract shape.
type Declaration struct {
	Identity      string          `json:"identity"`
	PackagePath   string          `json:"packagePath"`
	GoName        string          `json:"goName"`
	WITName       string          `json:"witName"`
	Kind          DeclarationKind `json:"kind"`
	Documentation string          `json:"documentation,omitempty"`
	Source        Source          `json:"source"`
	Receiver      *Receiver       `json:"receiver,omitempty"`
	Type          *Type           `json:"type,omitempty"`
	Callable      *Callable       `json:"callable,omitempty"`
	Constant      *Constant       `json:"constant,omitempty"`
	Variable      *Variable       `json:"variable,omitempty"`
	Coverage      Coverage        `json:"coverage"`
	Dependencies  []string        `json:"dependencies,omitempty"`
}

// Source is the stable source location used in diagnostics.
type Source struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Receiver identifies a method receiver after method-set resolution.
type Receiver struct {
	TypeIdentity string `json:"typeIdentity"`
	Pointer      bool   `json:"pointer"`
	Promoted     bool   `json:"promoted"`
}

// Type is a recursively lowered language-neutral type.
type Type struct {
	Identity     string    `json:"identity,omitempty"`
	WITName      string    `json:"witName,omitempty"`
	GoType       string    `json:"goType"`
	Kind         TypeKind  `json:"kind"`
	Ownership    Ownership `json:"ownership"`
	Lifetime     Lifetime  `json:"lifetime"`
	Nullable     bool      `json:"nullable"`
	Element      *Type     `json:"element,omitempty"`
	Key          *Type     `json:"key,omitempty"`
	Fields       []Field   `json:"fields,omitempty"`
	Cases        []Case    `json:"cases,omitempty"`
	Tuple        []Type    `json:"tuple,omitempty"`
	Callback     *Callback `json:"callback,omitempty"`
	ResourceType string    `json:"resourceType,omitempty"`
}

// Field is a copied record field.
type Field struct {
	GoName        string `json:"goName"`
	WITName       string `json:"witName"`
	Documentation string `json:"documentation,omitempty"`
	Type          Type   `json:"type"`
}

// Case is an enum or variant case.
type Case struct {
	GoName  string `json:"goName"`
	WITName string `json:"witName"`
	Type    *Type  `json:"type,omitempty"`
}

// Callable models a function, method, accessor, or generated operation.
type Callable struct {
	Parameters []Parameter    `json:"parameters,omitempty"`
	Results    []Parameter    `json:"results,omitempty"`
	Error      *ErrorBehavior `json:"error,omitempty"`
	Variadic   bool           `json:"variadic"`
}

// Parameter is an ordered callable input or output.
type Parameter struct {
	GoName  string `json:"goName"`
	WITName string `json:"witName"`
	Type    Type   `json:"type"`
}

// ErrorBehavior records result lowering for Go errors.
type ErrorBehavior struct {
	TypedErrorIdentity string `json:"typedErrorIdentity,omitempty"`
	Fallback           bool   `json:"fallback"`
}

// Callback is a guest or host callable resource signature.
type Callback struct {
	Identity  string            `json:"identity"`
	Direction CallbackDirection `json:"direction"`
	Callable  Callable          `json:"callable"`
	Retained  bool              `json:"retained"`
	Reentrant bool              `json:"reentrant"`
}

// Constant preserves an exported constant's exact Go value.
type Constant struct {
	ExactValue string `json:"exactValue"`
}

// Variable records generated read/write access.
type Variable struct {
	Readable bool `json:"readable"`
	Writable bool `json:"writable"`
}

// Coverage is the explicit representation decision for a declaration.
type Coverage struct {
	State  CoverageState `json:"state"`
	Reason string        `json:"reason,omitempty"`
}
