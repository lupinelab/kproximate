package proxmox

import (
	"context"
	"errors"
	"regexp"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

type ProxmoxMockClient struct {
}

func (p *ProxmoxMockClient) GetClusterStats() ([]HostInformation, error) {
	pHosts := []HostInformation{
		{
			Id:     "node/host-01",
			Node:   "host-01",
			Cpu:    0.209377325725626,
			Mem:    20394792448,
			Maxmem: 16647962624,
			Status: "online",
		},
		{
			Id:     "node/host-02",
			Node:   "host-02",
			Cpu:    0.209377325725626,
			Mem:    20394792448,
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
	return pHosts, nil
}

func (p *ProxmoxMockClient) GetRunningKpNodes(kpNodeName regexp.Regexp) ([]VmInformation, error) {
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

	return kpNodes, nil
}

func (p *ProxmoxMockClient) GetAllKpNodes(kpNodeName regexp.Regexp) ([]VmInformation, error) {
	err := errors.New("")
	return []VmInformation{}, err
}

func (p *ProxmoxMockClient) GetKpNode(name string, kpNodeName regexp.Regexp) (VmInformation, error) {
	err := errors.New("")
	return VmInformation{}, err
}

func (p *ProxmoxMockClient) GetKpNodeTemplateRef(kpNodeTemplateName string, LocalTemplateStorage bool, cloneTargetNode string) (*proxmox.VmRef, error) {
	err := errors.New("")
	return &proxmox.VmRef{}, err
}

func (p *ProxmoxMockClient) NewKpNode(ctx context.Context, okchan chan<- bool, errchan chan<- error, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}, usingLocalStorage bool, kpNodeTemplateName string, kpJoinCommand string) {

}

func (p *ProxmoxMockClient) DeleteKpNode(name string, kpNodeName regexp.Regexp) error {
	err := errors.New("")
	return err
}

func (p *ProxmoxMockClient) QemuExecJoin(nodeName string, joinCommand string) (int, error) {
	err := errors.New("")
	return 0, err
}

func (p *ProxmoxMockClient) GetQemuExecJoinStatus(nodeName string, pid int) (QemuExecStatus, error) {
	err := errors.New("")
	return QemuExecStatus{}, err
}

func (p *ProxmoxMockClient) CheckNodeReady(ctx context.Context, okchan chan<- bool, errchan chan<- error, nodeName string) {

}
