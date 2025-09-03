package managed

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestApplyConfigOverrides(t *testing.T) {
	r := &Runner{}

	baseConfig := `bedrock:
  port: 19132
  motd1: "Original MOTD"
  compression-level: 6
remote:
  address: localhost
  port: 25567
debug-mode: false
max-players: 100`

	overrides := map[string]any{
		"bedrock": map[string]any{
			"motd1":             "Custom MOTD",
			"compression-level": 8,
		},
		"debug-mode":  true,
		"max-players": 500,
		"new-setting": "added",
	}

	result, err := r.applyConfigOverrides(baseConfig, overrides)
	if err != nil {
		t.Fatalf("applyConfigOverrides failed: %v", err)
	}

	// Parse result to verify overrides were applied
	var resultMap map[string]any
	if err := yaml.Unmarshal([]byte(result), &resultMap); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	// Check that nested override worked
	bedrock := resultMap["bedrock"].(map[string]any)
	if bedrock["motd1"] != "Custom MOTD" {
		t.Errorf("expected bedrock.motd1 = 'Custom MOTD', got %v", bedrock["motd1"])
	}
	if bedrock["compression-level"] != 8 {
		t.Errorf("expected bedrock.compression-level = 8, got %v", bedrock["compression-level"])
	}
	// Check that non-overridden nested values remain
	if bedrock["port"] != 19132 {
		t.Errorf("expected bedrock.port = 19132, got %v", bedrock["port"])
	}

	// Check top-level overrides
	if resultMap["debug-mode"] != true {
		t.Errorf("expected debug-mode = true, got %v", resultMap["debug-mode"])
	}
	if resultMap["max-players"] != 500 {
		t.Errorf("expected max-players = 500, got %v", resultMap["max-players"])
	}

	// Check new setting was added
	if resultMap["new-setting"] != "added" {
		t.Errorf("expected new-setting = 'added', got %v", resultMap["new-setting"])
	}

	// Check that non-overridden values remain
	remote := resultMap["remote"].(map[string]any)
	if remote["address"] != "localhost" {
		t.Errorf("expected remote.address = 'localhost', got %v", remote["address"])
	}
}

func TestMergeConfigMaps(t *testing.T) {
	base := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"keep":     "original",
				"override": "old",
			},
			"keep": "original",
		},
		"keep": "original",
	}

	override := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"override": "new",
				"add":      "added",
			},
		},
		"add": "added",
	}

	mergeConfigMaps(base, override)

	// Check deep merge worked
	level1 := base["level1"].(map[string]any)
	level2 := level1["level2"].(map[string]any)

	if level2["keep"] != "original" {
		t.Errorf("expected level2.keep = 'original', got %v", level2["keep"])
	}
	if level2["override"] != "new" {
		t.Errorf("expected level2.override = 'new', got %v", level2["override"])
	}
	if level2["add"] != "added" {
		t.Errorf("expected level2.add = 'added', got %v", level2["add"])
	}

	// Check that level1.keep was preserved
	if level1["keep"] != "original" {
		t.Errorf("expected level1.keep = 'original', got %v", level1["keep"])
	}

	// Check top-level additions
	if base["add"] != "added" {
		t.Errorf("expected add = 'added', got %v", base["add"])
	}
	if base["keep"] != "original" {
		t.Errorf("expected keep = 'original', got %v", base["keep"])
	}
}
