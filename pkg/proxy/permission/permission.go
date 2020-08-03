package permission

// Func is the permission function to obtain the TriState for permission.
type Func func(permission string) TriState

type Subject interface {
	HasPermission(permission string) bool // Equal to PermissionValue(...).Bool()
	PermissionValue(permission string) TriState
}

// TriState can be in three states (True, False, Undefined), used for a setting.
type TriState uint8

const (
	Undefined TriState = iota // A permission is undefined.
	True                      // A permission is allowed.
	False                     // A permission is explicitly denied.
)

// Bool returns the bool value of a TriState where
// Undefined is converted to false.
func (t TriState) Bool() bool {
	return t == True
}
