// The permission utility package defines primitives that allow to
// check a Subject for a permission.
//
// E.g. A player's permission Func can be setup on join by subscribing
// to proxy.PermissionsSetupEvent.
//
// Note:
// This is a simple package only allowing limited complexity of permission checking
// and may not suffice everyone's requirements.
// Therefore Gate also makes no assumptions on whether this package is used or not.
// Plugins may use their own authorization system internally without a touch on this package.
package permission

// Func is the permission function to obtain the TriState for a permission.
type Func func(permission string) TriState

// Subject is a permission holder like a player.
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
