package gate

import (
	"encoding/json"
	"errors"
	"fmt"

	"go.minekube.com/gate/pkg/gate/config"
	"gopkg.in/yaml.v3"
)

func mergeConfigPatch(current *config.Config, patch string) (*config.Config, error) {
	if current == nil {
		return nil, errors.New("current config is unavailable")
	}

	currentJSON, err := canonicalConfigJSON(current)
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
	if err := decodeConfigStrict(mergedJSON, ".yaml", &candidate); err != nil {
		return nil, fmt.Errorf("invalid patched config: %w", err)
	}
	return &candidate, nil
}

func canonicalConfigJSON(current *config.Config) ([]byte, error) {
	currentYAML, err := yaml.Marshal(current)
	if err != nil {
		return nil, err
	}
	var value any
	if err := yaml.Unmarshal(currentYAML, &value); err != nil {
		return nil, err
	}
	value, err = normalizeYAMLValue(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func normalizeYAMLValue(value any) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(value))
		for key, nested := range value {
			canonical, err := normalizeYAMLValue(nested)
			if err != nil {
				return nil, err
			}
			normalized[key] = canonical
		}
		return normalized, nil
	case map[any]any:
		normalized := make(map[string]any, len(value))
		for key, nested := range value {
			stringKey, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("configuration key %v is not a string", key)
			}
			canonical, err := normalizeYAMLValue(nested)
			if err != nil {
				return nil, err
			}
			normalized[stringKey] = canonical
		}
		return normalized, nil
	case []any:
		for i, nested := range value {
			canonical, err := normalizeYAMLValue(nested)
			if err != nil {
				return nil, err
			}
			value[i] = canonical
		}
	}
	return value, nil
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
