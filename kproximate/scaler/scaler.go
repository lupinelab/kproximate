package scaler

import (
	"fmt"
	"time"

	"github.com/lupinelab/kproximate/kubernetes"
	"github.com/lupinelab/kproximate/proxmox"
	tproxmox "github.com/Telmate/proxmox-api-go/proxmox"
)

type KProximateScaler struct {
	KCluster        kubernetes.Kubernetes
	KpsNodeTemplate tproxmox.VmRef
	MaxKpsNodes     int
	NumKpsNodes     int
	PCluster        proxmox.Proxmox
	ScaleState      int64
}

func NewScaler(config Config) *KProximateScaler {
	kClient := kubernetes.NewKubernetesClient()
	pClient := proxmox.NewProxmoxClient(config.PMUrl, config.AllowInsecure, config.PMUserID, config.PMToken)

	numKpsNodes := len(pClient.GetKpsNodes())
	
	kpsNodeTemplate, err := pClient.Client.GetVmRefByName(config.KpsNodeTemplateName)
	if err != nil {
		panic(err.Error())
	}

	s := &KProximateScaler{
		KCluster:        *kClient,
		KpsNodeTemplate: *kpsNodeTemplate,
		MaxKpsNodes:     config.MaxKpsNodes,
		NumKpsNodes:     numKpsNodes,
		PCluster:        *pClient,
		ScaleState:      0,
	}

	return s
}

func (scaler *KProximateScaler) UpdateState() {
	scaler.NumKpsNodes = len(scaler.PCluster.GetKpsNodes())
}

func (scaler *KProximateScaler) Start() {
	for {
		scaler.UpdateState()
		requiredResources := scaler.KCluster.GetUnschedulableResources()
		fmt.Println(requiredResources.Cpu, requiredResources.Memory)
		if requiredResources.Memory != 0 {
			go scaler.ScaleUp(requiredResources)
		}
		time.Sleep(5 * time.Second)
	}
}

func (scaler *KProximateScaler) ScaleUp(requiredResources *kubernetes.UnschedulableResources) error {
	if scaler.ScaleState == 0 && scaler.NumKpsNodes < scaler.MaxKpsNodes {
		// TODO calculate scale amount
		// TODO select target node
		scaler.ScaleState = 1
		
		err := scaler.PCluster.NewKpNode(
			scaler.KpsNodeTemplate,
			"qtiny-02",
		)
		if err != nil {
			return err
		}
		scaler.ScaleState = 0

	}

	return nil
}
