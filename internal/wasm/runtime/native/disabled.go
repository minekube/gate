//go:build !wasm_native || !cgo

package native

func New(_ []byte, _ Host, _ Limits) (*Runtime, error) {
	return nil, ErrUnavailable
}
