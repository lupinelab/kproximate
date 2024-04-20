package proxmox

import (
	"context"
	"regexp"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

type Mock struct {
	ClusterStats       []HostInformation
	RunningKpNodes     []VmInformation
	KpNodes            []VmInformation
	KpNode             VmInformation
	KpNodeTemplateRef  proxmox.VmRef
	JoinExecPid        int
	QemuExecJoinStatus QemuExecStatus
}

func (p *Mock) GetClusterStats() ([]HostInformation, error) {
	return p.ClusterStats, nil
}

func (p *Mock) GetRunningKpNodes(kpNodeName regexp.Regexp) ([]VmInformation, error) {
	return p.RunningKpNodes, nil
}

func (p *Mock) GetAllKpNodes(kpNodeName regexp.Regexp) ([]VmInformation, error) {
	return p.KpNodes, nil
}

func (p *Mock) GetKpNode(name string, kpNodeName regexp.Regexp) (VmInformation, error) {
	return p.KpNode, nil
}

func (p *Mock) GetKpNodeTemplateRef(kpNodeTemplateName string, LocalTemplateStorage bool, cloneTargetNode string) (*proxmox.VmRef, error) {
	return &p.KpNodeTemplateRef, nil
}

func (p *Mock) NewKpNode(ctx context.Context, okchan chan<- bool, errchan chan<- error, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}, usingLocalStorage bool, kpNodeTemplateName string, kpJoinCommand string) {
}

func (p *Mock) DeleteKpNode(name string, kpNodeName regexp.Regexp) error {
	return nil
}

func (p *Mock) QemuExecJoin(nodeName string, joinCommand string) (int, error) {
	return p.JoinExecPid, nil
}

func (p *Mock) GetQemuExecJoinStatus(nodeName string, pid int) (QemuExecStatus, error) {
	return p.QemuExecJoinStatus, nil
}

func (p *Mock) CheckNodeReady(ctx context.Context, okchan chan<- bool, errchan chan<- error, nodeName string) {
}
