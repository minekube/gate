package gate

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"go.minekube.com/gate/pkg/internal/configs"
)

func configCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Output default configuration file",
		Description: `Output the default configuration file to stdout or a file.
You can redirect to a file or use the --write flag:

	gate config > config.yml
	gate config --write              # Writes to config.yml

Available config types:
  - full (default): Full configuration with all options
  - minimal: Empty/minimal configuration (uses all defaults)
  - simple: Minimal configuration example with servers
  - lite: Lite mode configuration example
  - bedrock: Bedrock cross-play configuration example`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "type",
				Aliases: []string{"t"},
				Usage:   "Config type: full, minimal, simple, lite, or bedrock",
				Value:   "full",
			},
			&cli.BoolFlag{
				Name:    "write",
				Aliases: []string{"w"},
				Usage:   "Write config to config.yml instead of stdout",
			},
		},
		Action: func(c *cli.Context) error {
			configType := c.String("type")
			var configBytes []byte

			switch configType {
			case "full":
				configBytes = configs.DefaultConfigBytes
			case "minimal":
				configBytes = configs.MinimalConfigBytes
			case "simple":
				configBytes = configs.SimpleConfigBytes
			case "lite":
				configBytes = configs.LiteConfigBytes
			case "bedrock":
				configBytes = configs.BedrockConfigBytes
			default:
				return cli.Exit(fmt.Sprintf("unknown config type: %s (valid types: full, minimal, simple, lite, bedrock)", configType), 1)
			}

			if c.Bool("write") {
				outputFile := "config.yml"
				err := os.WriteFile(outputFile, configBytes, 0644)
				if err != nil {
					return cli.Exit(fmt.Errorf("error writing config to %q: %w", outputFile, err), 1)
				}
				fmt.Printf("Configuration written to %s\n", outputFile)
				return nil
			}

			_, err := os.Stdout.Write(configBytes)
			if err != nil {
				return cli.Exit(fmt.Errorf("error writing config: %w", err), 1)
			}

			return nil
		},
	}
}
