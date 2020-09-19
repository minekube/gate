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
	"go.minekube.com/gate/pkg/edition/java/config"
	"go.minekube.com/gate/pkg/gate"
	"go.minekube.com/gate/pkg/runtime/logr"
	"go.minekube.com/gate/pkg/runtime/manager"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"os/signal"
	"syscall"
)

// Run reads in and validates the config to pass into proxy.New,
// initializes the logger, runs the new proxy.Proxy and
// blocks until stopChan is triggered or an OS signal is sent.
// The proxy is already shutdown on method return.
func Run(stop <-chan struct{}) (err error) {
	setLogger := func(dev bool) error {
		zl, err := newZapLogger(dev)
		if err != nil {
			return fmt.Errorf("error creating zap logger: %w", err)
		}
		logr.SetLogger(zapr.NewLogger(zl))
		return nil
	}
	if err = setLogger(false); err != nil {
		return err
	}

	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	if cfg.Debug {
		if err = setLogger(cfg.Debug); err != nil {
			return err
		}
	}

	// Validate after we initialized the logger.
	if err = config.Validate(&cfg); err != nil {
		return fmt.Errorf("error validating config: %w", err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer func() { signal.Stop(sig); close(sig) }()

	mgr, err := manager.New(manager.Options{})
	if err != nil {
		return fmt.Errorf("error new runtime manager: %w", err)
	}
	_, err = gate.New(mgr, gate.Config{})
	if err != nil {
		return fmt.Errorf("error creating Gate: %w", err)
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

	return mgr.Start(childStop)
}

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
