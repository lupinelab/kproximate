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
	Status  string  `json:"status"`  // stopped | running
	MaxMem  int64   `json:"maxmem"`  // in bytes
	Mem     int64   `json:"mem"`     // In bytes
	MaxDisk int64   `json:"maxdisk"` // In bytes
	NetIn   int64   `json:"netin"`
	NetOut  int64   `json:"netout"`
	Node    string  `json:"node"`
	Uptime  int     `json:"uptime"` // in seconds
}

type Proxmox interface {
	GetClusterStats() ([]HostInformation, error)
	GetRunningKpNodes(regexp.Regexp) ([]VmInformation, error)
	GetAllKpNodes(regexp.Regexp) ([]VmInformation, error)
	GetKpNode(name string, kpNodeName regexp.Regexp) (VmInformation, error)
	GetKpNodeTemplateRef(kpnodeTemplateName string) (*proxmox.VmRef, error)
	NewKpNode(ctx context.Context, ok chan<- bool, errchan chan<- error, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}, kpNodeTemplate proxmox.VmRef)
	DeleteKpNode(name string, kpnodeName regexp.Regexp) error
}

type ProxmoxClient struct {
	client *proxmox.Client
}

func NewProxmoxClient(pm_url string, allowInsecure bool, pmUser string, pmToken string, debug bool) (ProxmoxClient, error) {
	tlsconf := &tls.Config{InsecureSkipVerify: allowInsecure}
	newClient, err := proxmox.NewClient(pm_url, nil, "", tlsconf, "", 300)
	if err != nil {
		return ProxmoxClient{}, err
	}
	newClient.SetAPIToken(pmUser, pmToken)

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
		panic(err.Error())
	}

	return pHosts, nil
}

func (p *ProxmoxClient) GetRunningKpNodes(kpNodeName regexp.Regexp) ([]VmInformation, error) {
	vmlist, err := p.client.GetVmList()
	if err != nil {
		return []VmInformation{}, err
	}

	var kpnodes vmList

	err = mapstructure.Decode(vmlist, &kpnodes)
	if err != nil {
		panic(err.Error())
	}

	var kpNodes []VmInformation

	for _, vm := range kpnodes.Data {
		if kpNodeName.MatchString(vm.Name) && vm.Status == "running" {
			kpNodes = append(kpNodes, vm)
		}
	}

	return kpNodes, nil
}

func (p *ProxmoxClient) GetAllKpNodes(kpNodeName regexp.Regexp) ([]VmInformation, error) {
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
		if kpNodeName.MatchString(vm.Name) {
			kpNodes = append(kpNodes, vm)
		}
	}

	return kpNodes, err
}

func (p *ProxmoxClient) GetKpNode(name string, kpNodeName regexp.Regexp) (VmInformation, error) {
	kpNodes, err := p.GetAllKpNodes(kpNodeName)
	if err != nil {
		return VmInformation{}, err
	}

	nodeName := regexp.MustCompile(name)

	for _, vm := range kpNodes {
		if nodeName.MatchString(vm.Name) {
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
	ok chan<- bool,
	errchan chan<- error,
	newKpNodeName string,
	targetNode string,
	kpNodeParams map[string]interface{},
	kpNodeTemplate proxmox.VmRef,
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

	ok <- true
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
