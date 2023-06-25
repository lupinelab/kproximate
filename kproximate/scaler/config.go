package scaler

import (
	"context"

	"github.com/Telmate/proxmox-api-go/proxmox"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	EmptyGraceSecondsAfterCreation int     `env:"emptyGraceSecondsAfterCreation"`
	KpNodeCores                    int     `env:"kpNodeCores"`
	KpNodeHeadroom                 float64 `env:"kpNodeHeadroom"`
	KpNodeMemory                   int     `env:"kpNodeMemory"`
	KpNodeParams                   map[string]interface{}
	KpNodeTemplateConfig           kproxmox.VMConfig
	KpNodeTemplateName             string `env:"kpNodeTemplateName"`
	KpNodeTemplateRef              proxmox.VmRef
	MaxKpNodes                     int    `env:"maxKPNodes"`
	PmAllowInsecure                bool   `env:"pmAllowInsecure"`
	PmDebug                        bool   `env:"pmDebug"`
	PmToken                        string `env:"pmToken"`
	PmUrl                          string `env:"pmUrl"`
	PmUserID                       string `env:"pmUserID"`
	PollInterval                   int    `env:"pollInterval"`
	SshKey                         string `env:"sshKey"`
}

func GetConfig() *Config {
	config := &Config{}

	err := envconfig.Process(context.Background(), config)
	if err != nil {
		panic(err.Error())
	}

	return config
}
