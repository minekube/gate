package configutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDurationMarshalsAsStringInsideStruct(t *testing.T) {
	data, err := yaml.Marshal(struct {
		Timeout Duration `yaml:"timeout"`
	}{Timeout: Duration(5 * time.Second)})
	require.NoError(t, err)
	require.Equal(t, "timeout: 5s\n", string(data))
}
