package config

import (
	_ "embed"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
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

var Model *ConfigModel = new(ConfigModel)

func init() {
	viper.AddConfigPath(".")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	viper.SetEnvPrefix("tll")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		logrus.Fatal(err)
	}

	if err := viper.Unmarshal(Model); err != nil {
		logrus.Fatal(err)
	}
}

func SubLabel(name string) string {
	return Model.Labels.Prefix + "." + name
}