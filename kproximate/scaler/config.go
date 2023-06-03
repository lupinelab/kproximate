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
	KpNodeTemplateName string `env:"KPNodeTemplateName"`
	MaxKpNodes         int    `env:"MaxKPNodes"`
}

func GetConfig() *Config {
	config := &Config{}
	err := envconfig.Process(context.Background(), config)
	if err != nil {
		panic(err.Error())
	}
	return config
}