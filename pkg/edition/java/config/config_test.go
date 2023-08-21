package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_texts(t *testing.T) {
	require.NotNil(t, defaultMotd())
	require.NotNil(t, defaultShutdownReason())
}
