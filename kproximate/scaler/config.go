package scaler

import (
	"context"

	"github.com/Telmate/proxmox-api-go/proxmox"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
	"github.com/sethvargo/go-envconfig"
)

type Config struct {
	PmUrl                          string `env:"pmUrl"`
	PmUserID                       string `env:"pmUserID"`
	PmToken                        string `env:"pmToken"`
	AllowInsecure                  bool   `env:"allowInsecure"`
	EmptyGraceSecondsAfterCreation int    `env:"emptyGraceSecondsAfterCreation"`
	KpNodeTemplateName             string `env:"kpNodeTemplateName"`
	kpNodeParams                   map[string]interface{}
	kpNodeTemplateConfig           kproxmox.VMConfig
	kpNodeTemplateRef              proxmox.VmRef
	KPNodeCores                    string `env:"kpNodeCores"`
	KPNodeMemory                   string `env:"kpNodeMemory"`
	MaxKpNodes                     int    `env:"maxKPNodes"`
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
