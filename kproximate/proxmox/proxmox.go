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

type pHostList struct {
	Data []PHostInformation
}

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

type Proxmox struct {
	Client *proxmox.Client
}

func NewProxmoxClient(pm_url string, allowinsecure bool, pm_user string, pm_token string) *Proxmox {
	tlsconf := &tls.Config{InsecureSkipVerify: allowinsecure}
	newClient, err := proxmox.NewClient(pm_url, nil, "", tlsconf, "", 300)
	if err != nil {
		panic(err.Error())
	}
	newClient.SetAPIToken(pm_user, pm_token)

	// *proxmox.Debug = true

	proxmox := &Proxmox{
		Client: newClient,
	}

	return proxmox
}

func (p *Proxmox) GetClusterStats() []PHostInformation {
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

func (p *Proxmox) GetKpNodes() []VmInformation {
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
		if kpNodeName.MatchString(vm.Name) {
			kpNodes = append(kpNodes, vm)
		}
	}

	return kpNodes
}

func (p *Proxmox) GetKpNode(kpNodeName string) VmInformation {
	kpNodes := p.GetKpNodes()

	for _, vm := range kpNodes {
		if strings.Contains(vm.Name, kpNodeName) {
			return vm
		}
	}

	return VmInformation{}
}

func (p *Proxmox) GetKpTemplateConfig(kpNodeTemplateRef *proxmox.VmRef) (VMConfig, error) {
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

func (p *Proxmox) NewKpNode(ctx context.Context, ok chan<- bool, errchan chan<- error, kpNodeTemplate proxmox.VmRef, newKpNodeName string, targetNode string, kpNodeParams map[string]interface{}) {
	nextID, err := p.Client.GetNextID(650)
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
			time.Sleep(1 * time.Second)
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

func (p *Proxmox) DeleteKpNode(kpNodeName string) error {
	kpNode := p.GetKpNode(kpNodeName)

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
