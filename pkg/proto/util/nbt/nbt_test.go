package nbt

import (
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"testing"
)

func TestReadNbt(t *testing.T) {
	f, err := ioutil.ReadFile("nbt.dat")
	require.NoError(t, err)
	nbt, err := ReadNbt(bytes.NewBuffer(f))
	require.NoError(t, err)
	spew.Dump(nbt)
}
