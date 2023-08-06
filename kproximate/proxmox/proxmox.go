package proxmox

import (
	"context"
	"crypto/tls"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/mitchellh/mapstructure"
)

type PHostInformation struct {
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
	GetClusterStats() []PHostInformation
	GetRunningKpNodes() []VmInformation
	GetAllKpNodes() ([]VmInformation, error)
	GetKpNode(kpNodeName string) (VmInformation, error)
	GetKpTemplateConfig(kpNodeTemplateRef *proxmox.VmRef) (VMConfig, error)
	NewKpNode(ctx context.Context, ok chan<- bool, errchan chan<- error, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}, kpNodeTemplate proxmox.VmRef)
	DeleteKpNode(kpNodeName string) error
}

type ProxmoxClient struct {
	Client *proxmox.Client
}

func NewProxmoxClient(pm_url string, allowInsecure bool, pmUser string, pmToken string, debug bool) *ProxmoxClient {
	tlsconf := &tls.Config{InsecureSkipVerify: allowInsecure}
	newClient, err := proxmox.NewClient(pm_url, nil, "", tlsconf, "", 300)
	if err != nil {
		panic(err.Error())
	}
	newClient.SetAPIToken(pmUser, pmToken)

	*proxmox.Debug = debug

	proxmox := &ProxmoxClient{
		Client: newClient,
	}

	return proxmox
}

func (p *ProxmoxClient) GetClusterStats() []PHostInformation {
	hostList, err := p.Client.GetResourceList("node")
	if err != nil {
		panic(err.Error())
	}

	var pHosts []PHostInformation

	err = mapstructure.Decode(hostList, &pHosts)
	if err != nil {
		panic(err.Error())
	}

	return pHosts
}

func (p *ProxmoxClient) GetRunningKpNodes() []VmInformation {
	vmlist, err := p.Client.GetVmList()
	if err != nil {
		panic(err.Error())
	}

	var kpnodes vmList

	err = mapstructure.Decode(vmlist, &kpnodes)
	if err != nil {
		panic(err.Error())
	}

	var kpNodes []VmInformation

	var kpNodeName = regexp.MustCompile(`^kp-node-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`)

	for _, vm := range kpnodes.Data {
		if kpNodeName.MatchString(vm.Name) && vm.Status == "running" {
			kpNodes = append(kpNodes, vm)
		}
	}

	return kpNodes
}

func (p *ProxmoxClient) GetAllKpNodes() ([]VmInformation, error) {
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

	var kpNodeName = regexp.MustCompile(`^kp-node-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`)

	for _, vm := range kpnodes.Data {
		if kpNodeName.MatchString(vm.Name) {
			kpNodes = append(kpNodes, vm)
		}
	}

	return kpNodes, err
}

func (p *ProxmoxClient) GetKpNode(kpNodeName string) (VmInformation, error) {
	kpNodes, err := p.GetAllKpNodes()
	if err != nil {
		return VmInformation{}, err
	}

	for _, vm := range kpNodes {
		if strings.Contains(vm.Name, kpNodeName) {
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

func (p *ProxmoxClient) DeleteKpNode(kpNodeName string) error {
	kpNode, err := p.GetKpNode(kpNodeName)
	if err != nil {
		return err
	}

	vmRef, err := p.Client.GetVmRefByName(kpNode.Name)
	if err != nil {
		return err
	}

	exitStatusSuccess := regexp.MustCompile(`^(OK|WARNINGS)`)

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
