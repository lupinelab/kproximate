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

type VMConfig struct {
	Name   string `json:"name"`
	Cores  int    `json:"cores"`
	Memory int    `json:"memory"`
}

type Proxmox interface {
	GetClusterStats() ([]HostInformation, error)
	GetRunningKpNodes(regexp.Regexp) ([]VmInformation, error)
	GetAllKpNodes(regexp.Regexp) ([]VmInformation, error)
	GetKpNode(name string, kpNodeName regexp.Regexp) (VmInformation, error)
	GetKpTemplateConfig(kpNodeTemplateRef *proxmox.VmRef) (VMConfig, error)
	NewKpNode(ctx context.Context, ok chan<- bool, errchan chan<- error, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}, kpNodeTemplate proxmox.VmRef)
	DeleteKpNode(name string, kpnodeName regexp.Regexp) error
}

type ProxmoxClient struct {
	Client *proxmox.Client
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
		Client: newClient,
	}

	return proxmox, nil
}

func (p *ProxmoxClient) GetClusterStats() ([]HostInformation, error) {
	hostList, err := p.Client.GetResourceList("node")
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
	vmlist, err := p.Client.GetVmList()
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
	vmlist, err := p.Client.GetVmList()
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

func (p *ProxmoxClient) GetKpTemplateConfig(kpNodeTemplateRef *proxmox.VmRef) (VMConfig, error) {
	config, err := p.Client.GetVmConfig(kpNodeTemplateRef)
	if err != nil {
		return VMConfig{}, err
	}

	var vmConfig VMConfig

	err = mapstructure.Decode(config, &vmConfig)
	if err != nil {
		return VMConfig{}, err
	}

	return vmConfig, err
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
	nextID, err := p.Client.GetNextID(kpNodeTemplate.VmId())
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

	_, err = p.Client.CloneQemuVm(&kpNodeTemplate, cloneParams)
	if err != nil {
		errchan <- err
		return
	}

	for {
		newVmRef, err := p.Client.GetVmRefByName(newKpNodeName)
		if err != nil {
			time.Sleep(time.Second * 1)
			continue
		}

		_, err = p.Client.SetVmConfig(newVmRef, kpNodeParams)
		if err != nil {
			errchan <- err
			return
		}

		_, err = p.Client.StartVm(newVmRef)
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

	vmRef, err := p.Client.GetVmRefByName(kpNode.Name)
	if err != nil {
		return err
	}

	exitStatus, err := p.Client.StopVm(vmRef)
	if err != nil {
		return err
	}
	if !exitStatusSuccess.MatchString(exitStatus) {
		err = fmt.Errorf(exitStatus)
		return err
	}

	exitStatus, err = p.Client.DeleteVm(vmRef)
	if err != nil {
		return err
	}
	if !exitStatusSuccess.MatchString(exitStatus) {
		err = fmt.Errorf(exitStatus)
		return err
	}

	return nil
}
