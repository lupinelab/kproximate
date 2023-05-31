package scaler

import (
	"context"
	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	PMUrl			string	`env:"PMUrl"`
	PMUserID           string `env:"PMUserID"`
	PMToken            string `env:"PMToken"`
	AllowInsecure           bool   `env:"Insecure"`
	KpsNodeTemplateName string `env:"KPNodeTemplateName"`
	MaxKpsNodes         int    `env:"MaxKPNodes"`
}

func GetConfig() *Config {
	config := &Config{}
	err := envconfig.Process(context.Background(), config)
	if err != nil {
		panic(err.Error())
	}
	return config
}