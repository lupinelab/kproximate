package proxmox

import (
	"context"
	"crypto/tls"
	"fmt"
	"regexp"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/mitchellh/mapstructure"
)

var exitStatusSuccess = regexp.MustCompile(`^(OK|WARNINGS)`)

type HostInformation struct {
	Id     string  `json:"id"`
	Node   string  `json:"node"`
	Cpu    float64 `json:"cpu"`
	Mem    int64   `json:"mem"`
	Maxmem int64   `json:"maxmem"`
	Status string  `json:"status"`
}

type vmList struct {
	Data []VmInformation
}

type VmInformation struct {
	VmID    int     `json:"vmid"`
	Name    string  `json:"name"`
	Cpu     float64 `json:"cpu"`
	CpuType string  `json:"cputype"`
	Status  string  `json:"status"`
	MaxMem  int64   `json:"maxmem"`
	Mem     int64   `json:"mem"`
	MaxDisk int64   `json:"maxdisk"`
	NetIn   int64   `json:"netin"`
	NetOut  int64   `json:"netout"`
	Node    string  `json:"node"`
	Uptime  int     `json:"uptime"`
}

type QemuExecResponse struct {
	Pid int `json:"pid"`
}

type QemuExecStatus struct {
	Exited   int    `mapstructure:"exited"`
	ExitCode int    `mapstructure:"exitcode"`
	ErrData  string `mapstructure:"err-data"`
	OutData  string `mapstructure:"out-data"`
}

type Proxmox interface {
	GetClusterStats() ([]HostInformation, error)
	GetRunningKpNodes(regexp.Regexp) ([]VmInformation, error)
	GetAllKpNodes(regexp.Regexp) ([]VmInformation, error)
	GetKpNode(name string, kpNodeNameRegex regexp.Regexp) (VmInformation, error)
	GetKpNodeTemplateRef(kpNodeTemplateName string) (*proxmox.VmRef, error)
	NewKpNode(ctx context.Context, ok chan<- bool, errchan chan<- error, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}, kpNodeTemplate proxmox.VmRef, kpJoinCommand string)
	DeleteKpNode(name string, kpnodeName regexp.Regexp) error
	QemuExecJoin(nodeName string, joinCommand string) (int, error)
	GetQemuExecJoinStatus(nodeName string, pid int) (QemuExecStatus, error)
	CheckNodeReady(ctx context.Context, okchan chan<- bool, errchan chan<- error, nodeName string)
}

type ProxmoxClient struct {
	client *proxmox.Client
}

var userRequiresTokenRegex = regexp.MustCompile("[a-z0-9]+@[a-z0-9]+![a-z0-9]+")

func userRequiresAPIToken(pmUser string) bool {
	return userRequiresTokenRegex.MatchString(pmUser)
}

func NewProxmoxClient(pm_url string, allowInsecure bool, pmUser string, pmToken string, pmPassword string, debug bool) (ProxmoxClient, error) {
	tlsconf := &tls.Config{InsecureSkipVerify: allowInsecure}
	newClient, err := proxmox.NewClient(pm_url, nil, "", tlsconf, "", 300)
	if err != nil {
		return ProxmoxClient{}, err
	}

	if userRequiresAPIToken(pmUser) {
		newClient.SetAPIToken(pmUser, pmToken)
	} else {
		err = newClient.Login(pmUser, pmPassword, "")
		if err != nil {
			return ProxmoxClient{}, err
		}
	}

	proxmox.Debug = &debug

	proxmox := ProxmoxClient{
		client: newClient,
	}

	return proxmox, nil
}

func (p *ProxmoxClient) GetClusterStats() ([]HostInformation, error) {
	hostList, err := p.client.GetResourceList("node")
	if err != nil {
		return nil, err
	}

	var pHosts []HostInformation

	err = mapstructure.Decode(hostList, &pHosts)
	if err != nil {
		return nil, err
	}

	return pHosts, nil
}

func (p *ProxmoxClient) GetRunningKpNodes(kpNodeNameRegex regexp.Regexp) ([]VmInformation, error) {
	vmlist, err := p.client.GetVmList()
	if err != nil {
		return []VmInformation{}, err
	}

	var kpnodes vmList

	err = mapstructure.Decode(vmlist, &kpnodes)
	if err != nil {
		return nil, err
	}

	var kpNodes []VmInformation

	for _, vm := range kpnodes.Data {
		if kpNodeNameRegex.MatchString(vm.Name) && vm.Status == "running" {
			kpNodes = append(kpNodes, vm)
		}
	}

	return kpNodes, nil
}

func (p *ProxmoxClient) GetAllKpNodes(kpNodeNameRegex regexp.Regexp) ([]VmInformation, error) {
	vmlist, err := p.client.GetVmList()
	if err != nil {
		return nil, err
	}

	var kpnodes vmList

	err = mapstructure.Decode(vmlist, &kpnodes)
	if err != nil {
		return nil, err
	}

	var kpNodes []VmInformation

	for _, vm := range kpnodes.Data {
		if kpNodeNameRegex.MatchString(vm.Name) {
			kpNodes = append(kpNodes, vm)
		}
	}

	return kpNodes, err
}

func (p *ProxmoxClient) GetKpNode(kpNodeName string, kpNodeNameRegex regexp.Regexp) (VmInformation, error) {
	kpNodes, err := p.GetAllKpNodes(kpNodeNameRegex)
	if err != nil {
		return VmInformation{}, err
	}

	for _, vm := range kpNodes {
		if vm.Name == kpNodeName {
			return vm, err
		}
	}

	return VmInformation{}, err
}

func (p *ProxmoxClient) GetKpNodeTemplateRef(kpNodeTemplateName string) (*proxmox.VmRef, error) {
	vmRef, err := p.client.GetVmRefByName(kpNodeTemplateName)
	if err != nil {
		return nil, err
	}

	return vmRef, err
}

func (p *ProxmoxClient) NewKpNode(
	ctx context.Context,
	okchan chan<- bool,
	errchan chan<- error,
	newKpNodeName string,
	targetNode string,
	kpNodeParams map[string]interface{},
	kpNodeTemplate proxmox.VmRef,
	kpJoinCommand string,
) {
	nextID, err := p.client.GetNextID(kpNodeTemplate.VmId())
	if err != nil {
		errchan <- err
		return
	}

	cloneParams := map[string]interface{}{
		"name":   newKpNodeName,
		"newid":  nextID,
		"target": targetNode,
		"vmid":   kpNodeTemplate.VmId(),
	}

	_, err = p.client.CloneQemuVm(&kpNodeTemplate, cloneParams)
	if err != nil {
		errchan <- err
		return
	}

	for {
		newVmRef, err := p.client.GetVmRefByName(newKpNodeName)
		if err != nil {
			time.Sleep(time.Second * 1)
			continue
		}

		_, err = p.client.SetVmConfig(newVmRef, kpNodeParams)
		if err != nil {
			errchan <- err
			return
		}

		_, err = p.client.StartVm(newVmRef)
		if err != nil {
			errchan <- err
			return
		}
		break
	}

	okchan <- true
}

func (p *ProxmoxClient) CheckNodeReady(ctx context.Context, okchan chan<- bool, errchan chan<- error, nodeName string) {
	vmRef, err := p.client.GetVmRefByName(nodeName)
	if err != nil {
		errchan <- err
	}

	_, pingErr := p.client.QemuAgentPing(vmRef)

	for pingErr != nil {
		_, pingErr = p.client.QemuAgentPing(vmRef)
		time.Sleep(time.Second * 1)
	}

	okchan <- true
}

func (p *ProxmoxClient) QemuExecJoin(nodeName string, joinCommand string) (int, error) {
	vmRef, err := p.client.GetVmRefByName(nodeName)
	if err != nil {
		return 0, err
	}

	params := map[string]interface{}{
		"command": []string{"bash", "-c", joinCommand},
	}

	result, err := p.client.QemuAgentExec(vmRef, params)
	if err != nil {
		return 0, err
	}

	var response QemuExecResponse

	err = mapstructure.Decode(result, &response)
	if err != nil {
		return 0, err
	}

	return response.Pid, err
}

func (p *ProxmoxClient) GetQemuExecJoinStatus(nodeName string, pid int) (QemuExecStatus, error) {
	vmRef, err := p.client.GetVmRefByName(nodeName)
	if err != nil {
		return QemuExecStatus{}, err
	}

	execStatus, err := p.client.GetExecStatus(vmRef, fmt.Sprintf("%d", pid))
	if err != nil {
		return QemuExecStatus{}, err
	}

	var status QemuExecStatus

	err = mapstructure.Decode(execStatus, &status)
	if err != nil {
		return QemuExecStatus{}, err
	}

	return status, err
}

func (p *ProxmoxClient) DeleteKpNode(name string, kpNodeName regexp.Regexp) error {
	kpNode, err := p.GetKpNode(name, kpNodeName)
	if err != nil {
		return err
	}

	vmRef, err := p.client.GetVmRefByName(kpNode.Name)
	if err != nil {
		return err
	}

	exitStatus, err := p.client.StopVm(vmRef)
	if err != nil {
		return err
	}
	if !exitStatusSuccess.MatchString(exitStatus) {
		err = fmt.Errorf(exitStatus)
		return err
	}

	exitStatus, err = p.client.DeleteVm(vmRef)
	if err != nil {
		return err
	}
	if !exitStatusSuccess.MatchString(exitStatus) {
		err = fmt.Errorf(exitStatus)
		return err
	}

	return nil
}
