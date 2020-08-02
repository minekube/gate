package permission

type Subject interface {
	HasPermission(permission string)
	PermissionValue(permission string) TriState
}

// TriState can be in three states (True, False, NotSet), used for a setting.
type TriState uint8

const (
	NotSet TriState = iota
	False
	True
)

// Bool returns the bool value of a TriState where
// NotSet is converted to false.
func (t TriState) Bool() bool {
	return t == True
}
