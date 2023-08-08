package config

import (
	"context"

	"github.com/Telmate/proxmox-api-go/proxmox"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	KpNodeCores                    int     `env:"kpNodeCores"`
	KpLoadHeadroom                 float64 `env:"kpLoadHeadroom"`
	KpNodeMemory                   int     `env:"kpNodeMemory"`
	KpNodeParams                   map[string]interface{}
	KpNodeTemplateConfig           kproxmox.VMConfig
	KpNodeTemplateName             string `env:"kpNodeTemplateName"`
	KpNodeTemplateRef              proxmox.VmRef
	MaxKpNodes                     int    `env:"maxKpNodes"`
	PmAllowInsecure                bool   `env:"pmAllowInsecure"`
	PmDebug                        bool   `env:"pmDebug"`
	PmToken                        string `env:"pmToken"`
	PmUrl                          string `env:"pmUrl"`
	PmUserID                       string `env:"pmUserID"`
	PollInterval                   int    `env:"pollInterval"`
	RabbitMQHost                   string `env:"rabbitMQHost"`
	RabbitMQPassword               string `env:"rabbitMQPassword"`
	RabbitMQPort                   int    `env:"rabbitMQPort"`
	RabbitMQUser                   string `env:"rabbitMQUser"`
	SshKey                         string `env:"sshKey"`
	WaitSecondsForJoin			   int    `env:"waitSecondsForJoin"`
}

func GetConfig() *Config {
	config := &Config{}

	err := envconfig.Process(context.Background(), config)
	if err != nil {
		panic(err.Error())
	}

	*config = validateConfig(config)

	return config
}

func validateConfig(config *Config) Config {
	if config.KpLoadHeadroom < 0.2 {
		config.KpLoadHeadroom = 0.2
	}

	if config.PollInterval < 10 {
		config.PollInterval = 10
	}

	if config.WaitSecondsForJoin < 60 {
		config.WaitSecondsForJoin = 60
	}
	return *config
}
