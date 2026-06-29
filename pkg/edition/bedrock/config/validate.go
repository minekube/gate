package config

import (
	"fmt"
	"os"
	"strings"

	"go.minekube.com/gate/pkg/util/validation"
)

// Validate validates the Bedrock edition configuration.
func (c *Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }
	w := func(m string, args ...any) { warns = append(warns, fmt.Errorf(m, args...)) }

	// Validate Floodgate key path (but skip if managed mode will generate it)
	managed := c.GetManaged()
	if c.FloodgateKeyPath != "" {
		if _, err := os.Stat(c.FloodgateKeyPath); os.IsNotExist(err) {
			if managed.Enabled {
				// Managed mode will generate the key, so just log it
				w("Floodgate key will be auto-generated in managed mode at %q", c.FloodgateKeyPath)
			} else {
				w("Floodgate key file not found at %q - Bedrock support will be disabled", c.FloodgateKeyPath)
			}
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

	if c.BackendFloodgate.Enabled {
		if len(c.BackendFloodgate.AllowedServers) == 0 {
			e("backendFloodgate.allowedServers must not be empty when backendFloodgate is enabled")
		}
		seen := make(map[string]struct{}, len(c.BackendFloodgate.AllowedServers))
		for _, name := range c.BackendFloodgate.AllowedServers {
			if !validation.ValidServerName(name) {
				e("Invalid backendFloodgate allowed server name %q: %s and length be 1-%d", name,
					validation.QualifiedNameErrMsg, validation.QualifiedNameMaxLength)
				continue
			}
			normalized := strings.ToLower(name)
			if _, ok := seen[normalized]; ok {
				e("Duplicate backendFloodgate allowed server %q", name)
				continue
			}
			seen[normalized] = struct{}{}
		}
	}

	// Validate managed mode options
	if managed.Enabled {
		switch managed.Engine {
		case "", ManagedEngineGeyserlite, ManagedEngineJava:
		default:
			e("Invalid managed engine %q, use %q or %q", managed.Engine, ManagedEngineGeyserlite, ManagedEngineJava)
		}
		if managed.Mode != "" && managed.Mode != "embedded" && managed.Mode != "subprocess" {
			e("Invalid managed mode %q, use \"embedded\" or \"subprocess\"", managed.Mode)
		}
		if managed.JarURL == "" {
			w("managed mode enabled but jarUrl is empty; using latest default")
		}
		if managed.JavaPath == "" {
			w("managed mode enabled but javaPath is empty; using 'java' on PATH")
		}
		if managed.DataDir == "" {
			w("managed mode enabled but dataDir is empty; using working directory")
		}
	}

	return warns, errs
}
