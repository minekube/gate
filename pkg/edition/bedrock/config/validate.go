package config

import (
	"fmt"
	"os"
	"strings"
)

// Validate validates the Bedrock edition configuration.
func (c *Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }
	w := func(m string, args ...any) { warns = append(warns, fmt.Errorf(m, args...)) }

	// Validate Floodgate key path
	if c.FloodgateKeyPath != "" {
		if _, err := os.Stat(c.FloodgateKeyPath); os.IsNotExist(err) {
			w("Floodgate key file not found at %q - Bedrock support will be disabled", c.FloodgateKeyPath)
		}
	} else {
		w("No Floodgate key path specified - Bedrock support will be disabled")
	}

	// Validate Geyser listen address
	if c.GeyserListenAddr == "" {
		e("Geyser listen address cannot be empty")
	}

	// Validate username format
	if c.UsernameFormat != "" && !strings.Contains(c.UsernameFormat, "%s") {
		e("Username format must contain %%s placeholder")
	}

	return warns, errs
}
