package proxmox

import (
	"crypto/tls"
	"fmt"
	"regexp"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/mitchellh/mapstructure"
	"k8s.io/apimachinery/pkg/util/uuid"
)

type NodeList struct {
	Data []NodeInformation
}

type NodeInformation struct {
	Id     string  `json:"id"`
	Node   string  `json:"node"`
	Cpu    float64 `json:"cpu"`
	Mem    int64   `json:"mem"`
	Maxmem int64   `json:"maxmem"`
	Status string  `json:"status"`
}

type VMInfo struct {
	Data []VMInformation
}

type VMInformation struct {
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

	*proxmox.Debug = true

	proxmox := &Proxmox{
		Client: newClient,
	}

	return proxmox
}

func (p *Proxmox) GetClusterStats() {
	nodelist, err := p.Client.GetResourceList("node")
	if err != nil {
		panic(err.Error())
	}

	var pnodes NodeList

	err = mapstructure.Decode(nodelist, &pnodes)
	if err != nil {
		panic(err.Error())
	}

	for _, node := range pnodes.Data {
		fmt.Println(node.Node, "\n========")
		fmt.Println("CPU: ", node.Cpu)
		fmt.Printf("Free Memory(MiB): %v\n", (node.Maxmem-node.Mem)>>20)
		fmt.Println("Status: ", node.Status)
		fmt.Println("")
	}
	// var list NodeData
}

func (p *Proxmox) GetKpsNodes() []VMInformation {
	vmlist, err := p.Client.GetVmList()
	if err != nil {
		panic(err.Error())
	}

	var kpsnodes VMInfo

	err = mapstructure.Decode(vmlist, &kpsnodes)
	if err != nil {
		panic(err.Error())
	}

	var kpsNodes []VMInformation

	var kpsNodeName = regexp.MustCompile(`^kps-node-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`)

	for _, vm := range kpsnodes.Data {
		if kpsNodeName.MatchString(vm.Name) {
			kpsNodes = append(kpsNodes, vm)
		}
	}

	return kpsNodes
}

func (p *Proxmox) GetKpsTemplateConfig(kpNodeTemplateName string) VMConfig {
	vmRef, err := p.Client.GetVmRefByName(kpNodeTemplateName)
	if err != nil {
		panic(err.Error())
	}

	config, err := p.Client.GetVmConfig(vmRef)
	if err != nil {
		panic(err.Error())
	}

	var vmConfig VMConfig

	err = mapstructure.Decode(config, &vmConfig)
	if err != nil {
		panic(err.Error())
	}

	return vmConfig
}

func (p *Proxmox) NewKpNode(kpNodeTemplate proxmox.VmRef, targetNode string) error {
	nextID, err := p.Client.GetNextID(650)
	if err != nil {
		return err
	}

	newName := fmt.Sprintf("kps-node-%s", uuid.NewUUID())

	cloneParams := map[string]interface{}{
		"name":   newName,
		"newid":  nextID,
		"target": targetNode,
		"vmid":   kpNodeTemplate.VmId(),
	}

	_, err = p.Client.CloneQemuVm(&kpNodeTemplate, cloneParams)
	if err != nil {
		return err
	}

	fmt.Println(newName, "=====")

	time.Sleep(1 * time.Second)

	newVmRef, err := p.Client.GetVmRefByName(newName)
	if err != nil {
		return err
	}

	fmt.Println(newVmRef.VmId())

	_, err = p.Client.StartVm(newVmRef)
	if err != nil {
		return err
	}

	return err
}
