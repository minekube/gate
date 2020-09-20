package configutil

// SetDefault is an interface to abstract setting Viper defaults.
// (e.g. Allows adding a key prefix to every call to SetDefault when used with SetDefaultFunc.)
type SetDefault interface {
	SetDefault(key string, value interface{})
}

// SetDefaultFunc implements SetDefault.
type SetDefaultFunc func(key string, value interface{})

// See SetDefault interface.
func (f SetDefaultFunc) SetDefault(key string, value interface{}) {
	if f == nil {
		return
	}
	f(key, value)
}
