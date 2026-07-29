package generate

import (
	"fmt"

	"go.minekube.com/gate/internal/builtin/wasm/codegen/model"
)

const (
	WITFile          = "gate.wit"
	ManifestFile     = "manifest.json"
	ContractFile     = "contract.json"
	GoValuesFile     = "values_gen.go"
	GoDispatchFile   = "dispatch_gen.go"
	GoCallbacksFile  = "callbacks_gen.go"
	CHeaderFile      = "gate_wasm_generated.h"
	RustBindingsFile = "bindings.rs"
	RustDispatchFile = "dispatch.rs"
	RustValuesFile   = "values.rs"
)

// Artifacts renders every synchronized contract artifact.
func Artifacts(api *model.API) (map[string][]byte, error) {
	artifacts, err := PublicArtifacts(api)
	if err != nil {
		return nil, err
	}
	manifest, err := RenderManifest(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", ManifestFile, err)
	}
	values, err := RenderGoValues(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", GoValuesFile, err)
	}
	dispatch, err := RenderGoDispatch(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", GoDispatchFile, err)
	}
	callbacks, err := RenderGoCallbacks(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", GoCallbacksFile, err)
	}
	header, err := RenderCHeader(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", CHeaderFile, err)
	}
	artifacts[GoValuesFile] = values
	artifacts[ManifestFile] = manifest
	artifacts[GoDispatchFile] = dispatch
	artifacts[GoCallbacksFile] = callbacks
	artifacts[CHeaderFile] = header
	return artifacts, nil
}

// PublicArtifacts renders the canonical language-neutral authoring contract.
// The verbose Go-to-WIT manifest remains in the internal artifact directory
// and is added to release bundles without duplicating it in the repository.
func PublicArtifacts(api *model.API) (map[string][]byte, error) {
	wit, err := RenderWIT(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", WITFile, err)
	}
	contract, err := RenderContract(api, wit)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", ContractFile, err)
	}
	return map[string][]byte{
		WITFile:      wit,
		ContractFile: contract,
	}, nil
}

// NativeArtifacts renders synchronized Rust host sources.
func NativeArtifacts(api *model.API) (map[string][]byte, error) {
	values, err := RenderRustValues(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", RustValuesFile, err)
	}
	bindings, err := RenderRustBindings(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", RustBindingsFile, err)
	}
	dispatch, err := RenderRustDispatch(api)
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", RustDispatchFile, err)
	}
	return map[string][]byte{
		RustValuesFile:   values,
		RustBindingsFile: bindings,
		RustDispatchFile: dispatch,
	}, nil
}
