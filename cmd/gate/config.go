package gate

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"go.minekube.com/gate/pkg/configs"
)

func configCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Output default configuration file",
		Description: `Output the default configuration file to stdout or a file.
You can redirect to a file or use the --write flag:

	gate config > config.yml
	gate config --write              # Writes to config.yml`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "write",
				Aliases: []string{"w"},
				Usage:   "Write config to config.yml instead of stdout",
			},
		},
		Action: func(c *cli.Context) error {
			configBytes := configs.DefaultConfigBytes

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
