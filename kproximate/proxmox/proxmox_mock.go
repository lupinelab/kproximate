package proxmox

import (
	"context"
	"errors"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

type ProxmoxMockClient struct {
}

func (p *ProxmoxMockClient) GetClusterStats() []PHostInformation {
	pHosts := []PHostInformation{
		{
			Id:     "node/host-01",
			Node:   "host-01",
			Cpu:    0.209377325725626,
			Mem:    13394792448,
			Maxmem: 16647962624,
			Status: "online",
		},
		{
			Id:     "node/host-02",
			Node:   "host-02",
			Cpu:    0.209377325725626,
			Mem:    13394792448,
			Maxmem: 16647962624,
			Status: "online",
		},
		{
			Id:     "node/host-03",
			Node:   "host-03",
			Cpu:    0.209377325725626,
			Mem:    11394792448,
			Maxmem: 16647962624,
			Status: "online",
		},
	}
	return pHosts
}

func (p *ProxmoxMockClient) GetRunningKpNodes() []VmInformation {
	kpNodes := []VmInformation{
		{
			Cpu:     0.114889359119222,
			MaxDisk: 10737418240,
			MaxMem:  2147483648,
			Mem:     1074127542,
			Name:    "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			NetIn:   35838253204,
			NetOut:  56111331754,
			Node:    "host-03",
			Status:  "running",
			Uptime:  740227,
			VmID:    603,
		},
	}

	return kpNodes
}

func (p *ProxmoxMockClient) GetAllKpNodes() ([]VmInformation, error) {
	err := errors.New("")
	return []VmInformation{}, err
}

func (p *ProxmoxMockClient) GetKpNode(kpNodeName string) (VmInformation, error) {
	err := errors.New("")
	return VmInformation{}, err
}

func (p *ProxmoxMockClient) GetKpTemplateConfig(kpNodeTemplateRef *proxmox.VmRef) (VMConfig, error) {
	err := errors.New("")
	return VMConfig{}, err
}

func (p *ProxmoxMockClient) NewKpNode(ctx context.Context, ok chan<- bool, errchan chan<- error, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}, kpNodeTemplate proxmox.VmRef) {

}

func (p *ProxmoxMockClient) DeleteKpNode(kpNodeName string) error {
	err := errors.New("")
	return err
}
