package scaler

import (
	"context"

	"github.com/Telmate/proxmox-api-go/proxmox"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	AllowInsecure                  bool    `env:"allowInsecure"`
	EmptyGraceSecondsAfterCreation int     `env:"emptyGraceSecondsAfterCreation"`
	KpNodeCores                    int     `env:"kpNodeCores"`
	KpNodeHeadroom                 float64 `env:"kpNodeHeadroom"`
	KpNodeMemory                   int     `env:"kpNodeMemory"`
	kpNodeParams                   map[string]interface{}
	kpNodeTemplateConfig           kproxmox.VMConfig
	KpNodeTemplateName             string `env:"kpNodeTemplateName"`
	kpNodeTemplateRef              proxmox.VmRef
	MaxKpNodes                     int    `env:"maxKPNodes"`
	PmDebug                        bool   `env:"pmDebug"`
	PmToken                        string `env:"pmToken"`
	PmUrl                          string `env:"pmUrl"`
	PmUserID                       string `env:"pmUserID"`
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
