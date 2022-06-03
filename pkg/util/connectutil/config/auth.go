package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"go.minekube.com/gate/pkg/internal/randstr"
)

const tokenFilename = "connect.json"

// load auth token from file or generates it
func loadToken(filename string) (string, error) {
	// If the file doesn't exist, create it, or append to the file
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return "", fmt.Errorf("could not open %s: %w", filename, err)
	}
	defer f.Close()
	t := new(tokenFile)
	if err = json.NewDecoder(f).Decode(t); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("could not read %s: %w", filename, err)
	}
	if t.Token == "" {
		t.Token = "T-" + randstr.String(20)
		_ = f.Truncate(0)
		_, _ = f.Seek(0, 0)
		if err = json.NewEncoder(f).Encode(t); err != nil {
			return "", fmt.Errorf("could not write %s: %w", filename, err)
		}
	}
	return t.Token, nil
}

type tokenFile struct {
	Token string `json:"token"`
}
