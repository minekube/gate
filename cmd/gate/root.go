/*
Copyright © 2020 Minekube Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package gate

import (
	"fmt"
	"github.com/spf13/cobra"
	"go.minekube.com/gate/pkg/gate"
	"os"
	"strings"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gate",
	Short: "Gate is an extensible Minecraft proxy.",
	Long: `A high performant & paralleled Minecraft proxy server with
	scalability, flexibility & excelled server version support.`,
	PreRunE: func(cmd *cobra.Command, args []string) error { return initErr },
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := gate.Start(cmd.Context().Done()); err != nil {
			return fmt.Errorf("error running Gate: %w", err)
		}
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

var initErr error

func init() {
	cobra.OnInitialize(func() { initErr = initConfig() })

	rootCmd.PersistentFlags().StringP("config", "c", "", `config file (default: ./config.yml)
Supports: yaml/yml, json, toml, hcl, ini, prop/properties/props, env/dotenv`)
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug mode")
}

// initConfig binds flags, reads in config file and ENV variables if set.
func initConfig() error {
	v := gate.Viper

	_ = v.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	_ = v.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))

	// Load Environment Variables
	v.SetEnvPrefix("GATE")
	v.AutomaticEnv() // read in environment variables that match
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if cfgFile := v.GetString("config"); cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
	}

	// If a config file is found, read it in.
	// A config file is not required.
	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) && !rootCmd.PersistentFlags().Changed("config") {
			return nil
		}
		return fmt.Errorf("error reading config file %q: %w", v.ConfigFileUsed(), err)
	}
	fmt.Println("Using config file:", v.ConfigFileUsed())
	return nil
}
