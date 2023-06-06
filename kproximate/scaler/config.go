package scaler

import (
	"context"

	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	PmUrl              string `env:"pmUrl"`
	PmUserID           string `env:"pmUserID"`
	PmToken            string `env:"pmToken"`
	AllowInsecure      bool   `env:"allowInsecure"`
	KpNodeTemplateName string `env:"kpNodeTemplateName"`
	MaxKpNodes         int    `env:"maxKPNodes"`
	SshKey             string `env:"sshKey"`
}

func GetConfig() *Config {
	config := &Config{}

	err := envconfig.Process(context.Background(), config)
	if err != nil {
		panic(err.Error())
	}

	return config
}
