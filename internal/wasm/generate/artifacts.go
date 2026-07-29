package generate

import (
	"fmt"

	"go.minekube.com/gate/internal/wasm/model"
)

const (
	WITFile        = "gate.wit"
	ManifestFile   = "manifest.json"
	ContractFile   = "contract.json"
	GoValuesFile   = "values_gen.go"
	GoDispatchFile = "dispatch_gen.go"
	RustValuesFile = "values.rs"
)

// Artifacts renders every synchronized contract artifact.
func Artifacts(api *model.API) (map[string][]byte, error) {
	wit, err := RenderWIT(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", WITFile, err)
	}
	manifest, err := RenderManifest(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", ManifestFile, err)
	}
	contract, err := RenderContract(api, wit)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", ContractFile, err)
	}
	values, err := RenderGoValues(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", GoValuesFile, err)
	}
	dispatch, err := RenderGoDispatch(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", GoDispatchFile, err)
	}
	return map[string][]byte{
		WITFile:        wit,
		ManifestFile:   manifest,
		ContractFile:   contract,
		GoValuesFile:   values,
		GoDispatchFile: dispatch,
	}, nil
}

// NativeArtifacts renders synchronized Rust host sources.
func NativeArtifacts(api *model.API) (map[string][]byte, error) {
	values, err := RenderRustValues(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", RustValuesFile, err)
	}
	return map[string][]byte{RustValuesFile: values}, nil
}
