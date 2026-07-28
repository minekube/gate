package generate

import (
	"fmt"

	"go.minekube.com/gate/internal/wasm/model"
)

const (
	WITFile      = "gate.wit"
	ManifestFile = "manifest.json"
	ContractFile = "contract.json"
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
	return map[string][]byte{
		WITFile:      wit,
		ManifestFile: manifest,
		ContractFile: contract,
	}, nil
}
