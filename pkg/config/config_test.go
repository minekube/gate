package config

import (
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestC(t *testing.T) {
	viper.SetConfigFile("config.yml")
	var f File
	require.NoError(t, viper.Unmarshal(&f))
	require.NoError(t, viper.WriteConfig())
}
