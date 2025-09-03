package configutil

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

// Test struct for BoolOrStruct tests
type testStruct struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Name    string `json:"name" yaml:"name"`
	Count   int    `json:"count" yaml:"count"`
}

// Verify that BoolOrStruct correctly implements all marshaling interfaces at compile time.
var (
	_ yaml.Marshaler   = (*BoolOrStruct[testStruct])(nil)
	_ yaml.Unmarshaler = (*BoolOrStruct[testStruct])(nil)
	_ json.Marshaler   = (*BoolOrStruct[testStruct])(nil)
	_ json.Unmarshaler = (*BoolOrStruct[testStruct])(nil)
)

func TestBoolOrStruct_BoolValue(t *testing.T) {
	// Test boolean true
	boolTrue := NewBoolOrStructBool[testStruct](true)
	if !boolTrue.IsBool() {
		t.Error("Expected IsBool() to be true")
	}
	if !boolTrue.BoolValue() {
		t.Error("Expected BoolValue() to be true")
	}
	if boolTrue.IsNil() {
		t.Error("Expected IsNil() to be false for bool value")
	}

	// Test boolean false
	boolFalse := NewBoolOrStructBool[testStruct](false)
	if !boolFalse.IsBool() {
		t.Error("Expected IsBool() to be true")
	}
	if boolFalse.BoolValue() {
		t.Error("Expected BoolValue() to be false")
	}
	if boolFalse.IsNil() {
		t.Error("Expected IsNil() to be false for bool value")
	}
}

func TestBoolOrStruct_StructValue(t *testing.T) {
	testData := testStruct{
		Enabled: true,
		Name:    "test",
		Count:   42,
	}

	structVal := NewBoolOrStructStruct(testData)
	if structVal.IsBool() {
		t.Error("Expected IsBool() to be false for struct value")
	}
	if structVal.IsNil() {
		t.Error("Expected IsNil() to be false for struct value")
	}

	result := structVal.StructValue()
	if result.Enabled != testData.Enabled {
		t.Errorf("Expected Enabled %v, got %v", testData.Enabled, result.Enabled)
	}
	if result.Name != testData.Name {
		t.Errorf("Expected Name %v, got %v", testData.Name, result.Name)
	}
	if result.Count != testData.Count {
		t.Errorf("Expected Count %v, got %v", testData.Count, result.Count)
	}
}

func TestBoolOrStruct_IsNil(t *testing.T) {
	// Test zero value
	var zeroValue BoolOrStruct[testStruct]
	if !zeroValue.IsNil() {
		t.Error("Expected zero value to be nil")
	}

	// Test explicit empty construction
	emptyValue := BoolOrStruct[testStruct]{}
	if !emptyValue.IsNil() {
		t.Error("Expected empty value to be nil")
	}
}

func TestBoolOrStruct_YAMLMarshaling(t *testing.T) {
	tests := []struct {
		name         string
		boolOrStruct BoolOrStruct[testStruct]
		expectedYAML string
	}{
		{
			name:         "boolean true",
			boolOrStruct: NewBoolOrStructBool[testStruct](true),
			expectedYAML: "true\n",
		},
		{
			name:         "boolean false",
			boolOrStruct: NewBoolOrStructBool[testStruct](false),
			expectedYAML: "false\n",
		},
		{
			name: "struct value",
			boolOrStruct: NewBoolOrStructStruct(testStruct{
				Enabled: true,
				Name:    "test",
				Count:   42,
			}),
		},
		{
			name:         "nil value",
			boolOrStruct: BoolOrStruct[testStruct]{},
			expectedYAML: "null\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			yamlData, err := yaml.Marshal(tt.boolOrStruct)
			if err != nil {
				t.Errorf("Failed to marshal to YAML: %v", err)
				return
			}

			if tt.expectedYAML != "" && string(yamlData) != tt.expectedYAML {
				t.Errorf("Expected YAML %q, got %q", tt.expectedYAML, string(yamlData))
			}

			// Test round trip
			var unmarshaled BoolOrStruct[testStruct]
			if err := yaml.Unmarshal(yamlData, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal YAML: %v", err)
				return
			}

			// Verify round trip preserves type and value
			if tt.boolOrStruct.IsBool() != unmarshaled.IsBool() {
				t.Errorf("Round trip failed: IsBool mismatch")
			}
			if tt.boolOrStruct.IsNil() != unmarshaled.IsNil() {
				t.Errorf("Round trip failed: IsNil mismatch")
			}

			if tt.boolOrStruct.IsBool() && !tt.boolOrStruct.IsNil() {
				if tt.boolOrStruct.BoolValue() != unmarshaled.BoolValue() {
					t.Errorf("Round trip failed: BoolValue mismatch")
				}
			}
		})
	}
}

func TestBoolOrStruct_JSONMarshaling(t *testing.T) {
	tests := []struct {
		name         string
		boolOrStruct BoolOrStruct[testStruct]
		expectedJSON string
	}{
		{
			name:         "boolean true",
			boolOrStruct: NewBoolOrStructBool[testStruct](true),
			expectedJSON: "true",
		},
		{
			name:         "boolean false",
			boolOrStruct: NewBoolOrStructBool[testStruct](false),
			expectedJSON: "false",
		},
		{
			name: "struct value",
			boolOrStruct: NewBoolOrStructStruct(testStruct{
				Enabled: true,
				Name:    "test",
				Count:   42,
			}),
			expectedJSON: `{"enabled":true,"name":"test","count":42}`,
		},
		{
			name:         "nil value",
			boolOrStruct: BoolOrStruct[testStruct]{},
			expectedJSON: "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			jsonData, err := json.Marshal(tt.boolOrStruct)
			if err != nil {
				t.Errorf("Failed to marshal to JSON: %v", err)
				return
			}

			if tt.expectedJSON != "" && string(jsonData) != tt.expectedJSON {
				t.Errorf("Expected JSON %q, got %q", tt.expectedJSON, string(jsonData))
			}

			// Test round trip
			var unmarshaled BoolOrStruct[testStruct]
			if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
				t.Errorf("Failed to unmarshal JSON: %v", err)
				return
			}

			// Verify round trip preserves type and value
			if tt.boolOrStruct.IsBool() != unmarshaled.IsBool() {
				t.Errorf("Round trip failed: IsBool mismatch")
			}
			if tt.boolOrStruct.IsNil() != unmarshaled.IsNil() {
				t.Errorf("Round trip failed: IsNil mismatch")
			}

			if tt.boolOrStruct.IsBool() && !tt.boolOrStruct.IsNil() {
				if tt.boolOrStruct.BoolValue() != unmarshaled.BoolValue() {
					t.Errorf("Round trip failed: BoolValue mismatch")
				}
			}
		})
	}
}

func TestBoolOrStruct_UnmarshalErrors(t *testing.T) {
	tests := []struct {
		name     string
		yamlData string
		jsonData string
		wantErr  bool
	}{
		{
			name:     "invalid type - string",
			yamlData: `"invalid"`,
			jsonData: `"invalid"`,
			wantErr:  true,
		},
		{
			name:     "invalid type - array",
			yamlData: `[1, 2, 3]`,
			jsonData: `[1, 2, 3]`,
			wantErr:  true,
		},
		{
			name:     "valid bool",
			yamlData: `true`,
			jsonData: `true`,
			wantErr:  false,
		},
		{
			name:     "valid struct",
			yamlData: `enabled: true`,
			jsonData: `{"enabled": true}`,
			wantErr:  false,
		},
		{
			name:     "null value",
			yamlData: `null`,
			jsonData: `null`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run("YAML: "+tt.name, func(t *testing.T) {
			var bs BoolOrStruct[testStruct]
			err := yaml.Unmarshal([]byte(tt.yamlData), &bs)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalYAML() error = %v, wantErr %v", err, tt.wantErr)
			}
		})

		t.Run("JSON: "+tt.name, func(t *testing.T) {
			var bs BoolOrStruct[testStruct]
			err := json.Unmarshal([]byte(tt.jsonData), &bs)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
