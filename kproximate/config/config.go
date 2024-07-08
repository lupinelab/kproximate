package config

import (
	"context"
	"regexp"

	"github.com/sethvargo/go-envconfig"
)

type KproximateConfig struct {
	Debug                   bool   `env:"debug"`
	KpJoinCommand           string `env:"kpJoinCommand"`
	KpNodeCores             int    `env:"kpNodeCores"`
	KpNodeDisableSsh        bool   `env:"kpNodeDisableSsh"`
	KpNodeMemory            int    `env:"kpNodeMemory"`
	KpNodeLabels            string `env:"kpNodeLabels"`
	KpNodeNamePrefix        string `env:"kpNodeNamePrefix"`
	KpNodeNameRegex         regexp.Regexp
	KpNodeParams            map[string]interface{}
	KpNodeTemplateName      string  `env:"kpNodeTemplateName"`
	KpQemuExecJoin          bool    `env:"kpQemuExecJoin"`
	KpLocalTemplateStorage  bool    `env:"kpLocalTemplateStorage"`
	LoadHeadroom            float64 `env:"loadHeadroom"`
	MaxKpNodes              int     `env:"maxKpNodes"`
	PmAllowInsecure         bool    `env:"pmAllowInsecure"`
	PmDebug                 bool    `env:"pmDebug"`
	PmPassword              string  `env:"pmPassword"`
	PmToken                 string  `env:"pmToken"`
	PmUrl                   string  `env:"pmUrl"`
	PmUserID                string  `env:"pmUserID"`
	PollInterval            int     `env:"pollInterval"`
	SshKey                  string  `env:"sshKey"`
	WaitSecondsForJoin      int     `env:"waitSecondsForJoin"`
	WaitSecondsForProvision int     `env:"waitSecondsForProvision"`
}

type RabbitConfig struct {
	Host     string `env:"rabbitMQHost"`
	Password string `env:"rabbitMQPassword"`
	Port     int    `env:"rabbitMQPort"`
	User     string `env:"rabbitMQUser"`
}

func GetKpConfig() (KproximateConfig, error) {
	config := &KproximateConfig{}

	err := envconfig.Process(context.Background(), config)
	if err != nil {
		return *config, err
	}

	*config = validateConfig(config)

	return *config, nil
}

func GetRabbitConfig() (RabbitConfig, error) {
	config := &RabbitConfig{}

	err := envconfig.Process(context.Background(), config)
	if err != nil {
		return *config, err
	}

	return *config, nil
}

func validateConfig(config *KproximateConfig) KproximateConfig {
	if config.LoadHeadroom < 0.2 {
		config.LoadHeadroom = 0.2
	}

	if config.PollInterval < 10 {
		config.PollInterval = 10
	}

	if config.WaitSecondsForJoin < 60 {
		config.WaitSecondsForJoin = 60
	}

	if config.WaitSecondsForProvision < 60 {
		config.WaitSecondsForProvision = 60
	}

	return *config
}
