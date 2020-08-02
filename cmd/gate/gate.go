package main

import (
	"fmt"
	"github.com/spf13/viper"
	"go.minekube.com/gate/pkg/config"
	"go.minekube.com/gate/pkg/proxy"
	"go.uber.org/zap"
)

func main() {
	if err := Main(); err != nil {
		zap.S().Errorf("Error initializing Gate Proxy: %v", err)
	}
}

var DevMode bool = true

func init() {
	viper.AddConfigPath(".")
}

func Main() (err error) {
	if err := InitLogger(); err != nil {
		return fmt.Errorf("error initializing global logger: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}
	server := proxy.NewProxy(cfg)

	//stopChan := make(chan struct{})
	return server.Run()
}

func InitLogger() (err error) {
	var l *zap.Logger
	if DevMode {
		l, err = zap.NewDevelopment()
	} else {
		l, err = zap.NewProduction()
	}

	if err != nil {
		return err
	}
	zap.ReplaceGlobals(l)
	return nil
}
