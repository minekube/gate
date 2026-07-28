package generate

import (
	"fmt"
	"reflect"
	"strings"

	"go.minekube.com/gate/internal/wasm/model"
)

// CheckCompatibility verifies that host satisfies every required declaration.
func CheckCompatibility(required, host *model.API) error {
	requiredAPI, err := normalizedAPI(required)
	if err != nil {
		return fmt.Errorf("normalize required contract: %w", err)
	}
	hostAPI, err := normalizedAPI(host)
	if err != nil {
		return fmt.Errorf("normalize host contract: %w", err)
	}
	hostDeclarations := make(map[string]model.Declaration, len(hostAPI.Declarations))
	for _, declaration := range hostAPI.Declarations {
		hostDeclarations[declaration.Identity] = declaration
	}
	for _, requiredDeclaration := range requiredAPI.Declarations {
		if requiredDeclaration.Coverage.State != model.CoverageRepresented {
			continue
		}
		hostDeclaration, exists := hostDeclarations[requiredDeclaration.Identity]
		if !exists || hostDeclaration.Coverage.State != model.CoverageRepresented {
			return fmt.Errorf("%s: missing", requiredDeclaration.Identity)
		}
		requiredShape := structuralDeclarationOf(requiredDeclaration)
		hostShape := structuralDeclarationOf(hostDeclaration)
		if path, different := firstDifference(
			reflect.ValueOf(requiredShape),
			reflect.ValueOf(hostShape),
			requiredDeclaration.Identity,
		); different {
			return fmt.Errorf("%s: incompatible", path)
		}
	}
	return nil
}

type structuralDeclaration struct {
	WITName  string                `json:"witName"`
	Kind     model.DeclarationKind `json:"kind"`
	Receiver *model.Receiver       `json:"receiver,omitempty"`
	Type     *model.Type           `json:"type,omitempty"`
	Callable *model.Callable       `json:"callable,omitempty"`
	Constant *model.Constant       `json:"constant,omitempty"`
	Variable *model.Variable       `json:"variable,omitempty"`
	Event    bool                  `json:"event"`
}

func structuralDeclarationOf(declaration model.Declaration) structuralDeclaration {
	return structuralDeclaration{
		WITName:  declaration.WITName,
		Kind:     declaration.Kind,
		Receiver: declaration.Receiver,
		Type:     declaration.Type,
		Callable: declaration.Callable,
		Constant: declaration.Constant,
		Variable: declaration.Variable,
		Event:    declaration.Event,
	}
}

func firstDifference(required, host reflect.Value, path string) (string, bool) {
	if required.IsValid() != host.IsValid() {
		return path, true
	}
	if !required.IsValid() {
		return "", false
	}
	if required.Type() != host.Type() {
		return path, true
	}
	switch required.Kind() {
	case reflect.Pointer, reflect.Interface:
		if required.IsNil() != host.IsNil() {
			return path, true
		}
		if required.IsNil() {
			return "", false
		}
		return firstDifference(required.Elem(), host.Elem(), path)
	case reflect.Struct:
		typ := required.Type()
		for index := range required.NumField() {
			field := typ.Field(index)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				name = field.Name
			}
			if difference, found := firstDifference(
				required.Field(index),
				host.Field(index),
				path+"."+name,
			); found {
				return difference, true
			}
		}
	case reflect.Slice, reflect.Array:
		if required.Len() != host.Len() {
			return path, true
		}
		for index := range required.Len() {
			if difference, found := firstDifference(
				required.Index(index),
				host.Index(index),
				fmt.Sprintf("%s[%d]", path, index),
			); found {
				return difference, true
			}
		}
	default:
		if !reflect.DeepEqual(required.Interface(), host.Interface()) {
			return path, true
		}
	}
	return "", false
}
