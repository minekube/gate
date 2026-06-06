package proxy

import (
	"reflect"

	"go.minekube.com/common/minecraft/component"
)

func normalizeDisconnectReason(reason component.Component) component.Component {
	if reason == nil {
		return &component.Text{Content: ""}
	}
	value := reflect.ValueOf(reason)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return &component.Text{Content: ""}
	}
	return reason
}
