package proxmox

import (
	"context"
	"regexp"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

type ProxmoxMock struct {
	ClusterStats       []HostInformation
	RunningKpNodes     []VmInformation
	KpNodes            []VmInformation
	KpNode             VmInformation
	KpNodeTemplateRef  proxmox.VmRef
	JoinExecPid        int
	QemuExecJoinStatus QemuExecStatus
}

func (p *ProxmoxMock) GetClusterStats() ([]HostInformation, error) {
	return p.ClusterStats, nil
}

func (p *ProxmoxMock) GetRunningKpNodes(kpNodeName regexp.Regexp) ([]VmInformation, error) {
	return p.RunningKpNodes, nil
}

func (p *ProxmoxMock) GetAllKpNodes(kpNodeName regexp.Regexp) ([]VmInformation, error) {
	return p.KpNodes, nil
}

func (p *ProxmoxMock) GetKpNode(name string, kpNodeName regexp.Regexp) (VmInformation, error) {
	return p.KpNode, nil
}

func (p *ProxmoxMock) GetKpNodeTemplateRef(kpNodeTemplateName string, LocalTemplateStorage bool, cloneTargetNode string) (*proxmox.VmRef, error) {
	return &p.KpNodeTemplateRef, nil
}

func (p *ProxmoxMock) NewKpNode(ctx context.Context, okchan chan<- bool, errchan chan<- error, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}, usingLocalStorage bool, kpNodeTemplateName string, kpJoinCommand string) {
}

func (p *ProxmoxMock) DeleteKpNode(name string, kpNodeName regexp.Regexp) error {
	return nil
}

func (p *ProxmoxMock) QemuExecJoin(nodeName string, joinCommand string) (int, error) {
	return p.JoinExecPid, nil
}

func (p *ProxmoxMock) GetQemuExecJoinStatus(nodeName string, pid int) (QemuExecStatus, error) {
	return p.QemuExecJoinStatus, nil
}

func (p *ProxmoxMock) CheckNodeReady(ctx context.Context, okchan chan<- bool, errchan chan<- error, nodeName string) {
}
