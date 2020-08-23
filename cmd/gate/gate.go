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
	"github.com/spf13/viper"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/proxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func Run(ctx context.Context) (err error) {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	if err := initLogger(cfg.Debug); err != nil {
		return fmt.Errorf("error initializing global logger: %w", err)
	}

	if err = config.Validate(&cfg); err != nil {
		return fmt.Errorf("error validating config: %w", err)
	}
	p := proxy.New(cfg)
	return p.Run(ctx)
}

func initLogger(debug bool) (err error) {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}

	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	l, err := cfg.Build()
	if err != nil {
		return err
	}
	zap.ReplaceGlobals(l)
	return nil
}
