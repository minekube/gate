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
	"fmt"
	"github.com/go-logr/zapr"
	"github.com/spf13/viper"
	"go.minekube.com/gate/pkg/gate"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/manager"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
)

// Viper is a viper instance initialized with defaults
// for the Config in `pkg/gate/config` package.
// It can be used to load in config files.
var Viper = viper.New()

func init() { gate.SetDefaults(Viper) }

var log = logr.Log.WithName("setup")

// Run reads in and validates the config to pass into proxy.New,
// initializes the logger, runs the new proxy.Proxy and
// blocks until stopChan is triggered or an OS signal is sent.
// The proxy is already shutdown on method return.
func Run(stop <-chan struct{}) (err error) {
	// Set logger
	setLogger := func(devMode bool) error {
		zl, err := newZapLogger(devMode)
		if err != nil {
			return fmt.Errorf("error creating zap logger: %w", err)
		}
		logr.SetLogger(zapr.NewLogger(zl))
		return nil
	}
	if err = setLogger(Viper.GetBool("debug")); err != nil {
		return err
	}

	// Load in Gate config
	var cfg gate.Config
	if err := Viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Validate Gate config
	warns, errs := cfg.Validate()
	for _, e := range errs {
		log.Info("Config validation error", "error", e.Error())
	}
	for _, w := range warns {
		log.Info("Config validation warn", "warn", w.Error())
	}
	if len(errs) != 0 {
		// Shouldn't run Gate with validation errors
		return fmt.Errorf("config validation failure (errors: %d, warns: %d), inspect the logs for details",
			len(errs), len(warns))
	}

	// Setup os signal channel to trigger Gate shutdown.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer func() { signal.Stop(sig); close(sig) }()

	// Top-level manager starting all sub-components.
	mgr, err := manager.New(manager.Options{})
	if err != nil {
		return fmt.Errorf("error new runtime manager: %w", err)
	}
	// Setup new Gate with manager and loaded config.
	_, err = gate.New(mgr, gate.Options{
		Config: &cfg,
	})
	if err != nil {
		return fmt.Errorf("error creating Gate instance: %w", err)
	}

	childStop := make(chan struct{})
	go func() {
		defer close(childStop)
		select {
		case <-stop:
		case s, ok := <-sig:
			if !ok {
				// Sig chan was closed
				return
			}
			mgr.Logger().Info("Received a signal", "signal", s)
		}
	}()

	// Start everything
	return mgr.Start(childStop)
}

// newZapLogger returns a new zap logger with a modified production
// or development default config to ensure human readability.
func newZapLogger(dev bool) (l *zap.Logger, err error) {
	var cfg zap.Config
	if dev {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	l, err = cfg.Build()
	if err != nil {
		return nil, err
	}
	return l, nil
}
