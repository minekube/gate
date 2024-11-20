package api

import (
	"errors"
	"fmt"
	"strings"

	"go.minekube.com/gate/pkg/util/validation"
)

// DefaultConfig is the default configuration for the Config.
var DefaultConfig = Config{
	Bind: "localhost:8080",
}

// Config is the configuration for the Gate API.
type Config struct {
	// Bind is the address to bind the API server to.
	// Using a localhost address is recommended to avoid exposing the API to the public.
	Bind string `json:"bind,omitempty" yaml:"bind,omitempty"`
}

// Validate validates the API configuration.
func (c Config) Validate() (warns []error, errs []error) {
	if strings.TrimSpace(c.Bind) == "" {
		return nil, []error{errors.New("bind address must not be empty")}
	}
	if err := validation.ValidHostPort(c.Bind); err != nil {
		return nil, []error{fmt.Errorf("invalid bind %q: %v", c.Bind, err)}
	}
	return nil, nil
}
