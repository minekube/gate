package ping

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.minekube.com/common/minecraft/component"
	"go.minekube.com/gate/pkg/util/uuid"
)

func TestServerPing_JSON(t *testing.T) {
	// Test json marshalling and unmarshalling of ServerPing.
	p := &ServerPing{
		Version: Version{
			Protocol: 1,
			Name:     "1",
		},
		Players: &Players{
			Online: 1,
			Max:    1,
			Sample: []SamplePlayer{
				{
					Name: "1",
					ID:   uuid.New(),
				},
			},
		},
		Description: &component.Text{
			Content: "Hello",
		},
		Favicon: "favicon",
	}

	// Marshal to json.
	b, err := json.Marshal(p)
	require.NoError(t, err)

	// Unmarshal from json.
	var p2 ServerPing
	err = json.Unmarshal(b, &p2)
	require.NoError(t, err)

	// Compare.
	require.Equal(t, p, &p2)
}
