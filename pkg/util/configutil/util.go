package configutil

// SetDefault is an interface to abstract setting Viper defaults.
// (e.g. Allows adding a key prefix to every call to SetDefault when used with SetDefaultFunc.)
type SetDefault interface {
	SetDefault(key string, value any)
}

// SetDefaultFunc implements SetDefault.
type SetDefaultFunc func(key string, value any)

// See SetDefault interface.
func (f SetDefaultFunc) SetDefault(key string, value any) {
	if f == nil {
		return
	}
	f(key, value)
}
