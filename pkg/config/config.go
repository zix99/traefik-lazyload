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
	StatusHost string // Host that will serve the status page (empty is disabled)

	StopDelay time.Duration // Amount of time to wait before stopping a container
	PollFreq  time.Duration // How often to check for changes
	Timeout   time.Duration // Default operation timeout (eg. starting/stopping a container)

	Verbose bool // Debug-level logging

	LabelPrefix string
}

var Model *ConfigModel = new(ConfigModel)

func Load() {
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
	return Model.LabelPrefix + "." + name
}
