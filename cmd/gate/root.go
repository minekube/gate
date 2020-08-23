/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

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
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/viper"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "gate",
	Short: "Gate is an extensible Minecraft proxy.",
	Long: `A high performant & paralleled Minecraft proxy server with
scalability, flexibility & excelled server version support.`,
	Run: func(cmd *cobra.Command, args []string) {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		defer func() { signal.Stop(sig); close(sig) }()

		ctx, cancelFunc := context.WithCancel(cmd.Context())
		go func() {
			s, ok := <-sig
			if !ok {
				return
			}
			zap.S().Infof("Received %s signal", s)
			cancelFunc()
		}()
		if err := Run(ctx); err != nil {
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

	rootCmd.PersistentFlags().StringP("bind", "b", "0.0.0.0:25565", "The address to bind to")
	rootCmd.PersistentFlags().String("health", "0.0.0.0:8080", "The grpc health probe service address")
	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug mode")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	_ = viper.BindPFlag("bind", rootCmd.Flags().Lookup("bind"))
	_ = viper.BindPFlag("debug", rootCmd.Flags().Lookup("debug"))
	if rootCmd.Flags().Changed("health") {
		viper.SetDefault("health.enabled", true)
		viper.SetDefault("health.bind", rootCmd.Flags().Lookup("health").Value)
	}

	viper.SetEnvPrefix("GATE")
	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	viper.SetConfigFile("config.yml")
	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
