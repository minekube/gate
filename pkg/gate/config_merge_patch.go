package gate

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.minekube.com/gate/pkg/gate/config"
)

func mergeConfigPatch(current *config.Config, patch string) (*config.Config, error) {
	if current == nil {
		return nil, errors.New("current config is unavailable")
	}

	currentJSON, err := json.Marshal(current)
	if err != nil {
		return nil, fmt.Errorf("encode current config: %w", err)
	}
	var target any
	if err := json.Unmarshal(currentJSON, &target); err != nil {
		return nil, fmt.Errorf("decode current config: %w", err)
	}
	var patchValue any
	if err := json.Unmarshal([]byte(patch), &patchValue); err != nil {
		return nil, fmt.Errorf("invalid JSON Merge Patch: %w", err)
	}

	mergedJSON, err := json.Marshal(applyMergePatch(target, patchValue))
	if err != nil {
		return nil, fmt.Errorf("encode patched config: %w", err)
	}
	var candidate config.Config
	if err := decodeConfigStrict(mergedJSON, ".json", &candidate); err != nil {
		return nil, fmt.Errorf("invalid patched config: %w", err)
	}
	return &candidate, nil
}

func applyMergePatch(target, patch any) any {
	patchObject, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	targetObject, ok := target.(map[string]any)
	if !ok {
		targetObject = make(map[string]any)
	}
	for key, value := range patchObject {
		if value == nil {
			delete(targetObject, key)
			continue
		}
		targetObject[key] = applyMergePatch(targetObject[key], value)
	}
	return targetObject
}
