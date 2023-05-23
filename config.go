package main

import (
	_ "embed"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config model and loader

type ConfigModel struct {
	Listen     string // http listen
	StopAtBoot bool   // Stop existing containers at start of app
	Splash     string // Which splash page to serve

	StopDelay time.Duration // Amount of time to wait before stopping a container

	Labels struct {
		Prefix string `mapstructure:"prefix"`
	} `mapstructure:"labels"`
}

var Config *ConfigModel = new(ConfigModel)

func init() {
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("tll")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}

	if err := viper.Unmarshal(Config); err != nil {
		panic(err)
	}
}

func subLabel(name string) string {
	return Config.Labels.Prefix + "." + name
}
