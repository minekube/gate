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
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.minekube.com/gate/pkg/gate"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gate",
	Short: "Gate is an extensible Minecraft proxy.",
	Long: `A high performant & paralleled Minecraft proxy server with
	scalability, flexibility & excelled server version support.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := gate.Start(cmd.Context().Done()); err != nil {
			cmd.PrintErr(fmt.Sprintf("Error running Gate Proxy: %v", err))
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	v := gate.Viper

	rootCmd.PersistentFlags().String("config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug mode")
	v.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
	v.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	v := gate.Viper

	// Load Environment Variables
	v.SetEnvPrefix("GATE")
	v.AutomaticEnv() // read in environment variables that match
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if cfgFile := v.GetString("config"); cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config.yml")
		v.AddConfigPath(".")
	}

	// If a config file is found, read it in.
	if err := v.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", v.ConfigFileUsed())
	}
}
