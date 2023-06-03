package scaler

import (
	"fmt"
	"log"
	"os"
	"time"

	tproxmox "github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/lupinelab/kproximate/kubernetes"
	"github.com/lupinelab/kproximate/proxmox"
)

var (
	infoLog    *log.Logger
	warningLog *log.Logger
	errorLog   *log.Logger
)

func init() {
	infoLog = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime)
	warningLog = log.New(os.Stdout, "WARNING: ", log.Ldate|log.Ltime)
	errorLog = log.New(os.Stdout, "ERROR: ", log.Ldate|log.Ltime)
}

type KProximateScaler struct {
	kCluster       kubernetes.Kubernetes
	kpNodeTemplate tproxmox.VmRef
	maxKpNodes     int
	numKpNodes     int
	pCluster       proxmox.Proxmox
	scaleState     int64
}

func NewScaler(config Config) *KProximateScaler {
	kClient := kubernetes.NewKubernetesClient()
	pClient := proxmox.NewProxmoxClient(config.PMUrl, config.AllowInsecure, config.PMUserID, config.PMToken)

	numKpNodes := len(pClient.GetKpNodes())

	kpNodeTemplate, err := pClient.Client.GetVmRefByName(config.KpNodeTemplateName)
	if err != nil {
		panic(err.Error())
	}

	scaler := &KProximateScaler{
		kCluster:       *kClient,
		kpNodeTemplate: *kpNodeTemplate,
		maxKpNodes:     config.MaxKpNodes,
		numKpNodes:     numKpNodes,
		pCluster:       *pClient,
		scaleState:     0,
	}

	return scaler
}

func (scaler *KProximateScaler) UpdateState() {
	scaler.numKpNodes = len(scaler.pCluster.GetKpNodes())
}

func (scaler *KProximateScaler) Start() {
	for {
		scaler.UpdateState()

		requiredResources := scaler.kCluster.GetUnschedulableResources()
		fmt.Println(requiredResources.Cpu, requiredResources.Memory)

		if requiredResources.Memory != 0 || requiredResources.Cpu != 0 {
			// TODO calculate scale amount
			// TODO select target node(s)
			infoLog.Println("Scale up requested")
			go scaler.ScaleUp(requiredResources)
		}

		time.Sleep(5 * time.Second)
	}
}

func (scaler *KProximateScaler) ScaleUp(requiredResources *kubernetes.UnschedulableResources) {
	if scaler.scaleState == 0 {
		if scaler.numKpNodes < scaler.maxKpNodes {
			scaler.scaleState++
			infoLog.Printf("ScaleState=%v", scaler.scaleState)

			infoLog.Println("Provisioning new kp-node")
			newKpNodeName, err := scaler.pCluster.NewKpNode(
				scaler.kpNodeTemplate,
				"qtiny-02",
			)
			if err != nil {
				errorLog.Printf("Cloud not provision new kp-node %s: %s", newKpNodeName, err.Error())
				return
			}

			infoLog.Printf("Waiting for %s to join kcluster", newKpNodeName)
			retry := 0
			for retry < 60 {
				if scaler.kCluster.CheckKpNodeReady(newKpNodeName) {
					infoLog.Printf("%s joined kcluster", newKpNodeName)
					
					scaler.scaleState--
					infoLog.Printf("ScaleState=%v", scaler.scaleState)
					
					return
				}

				retry++
				if retry >= 60 {
					errorLog.Printf("Timeout waiting for %s to join the cluster: ", newKpNodeName)
					
					scaler.scaleState--
					infoLog.Printf("ScaleState=%v", scaler.scaleState)
					
					infoLog.Printf("Cleaning up failed node from scale attempt: %s", newKpNodeName)
					
					err := scaler.DeleteKpNode(newKpNodeName)
					if err != nil {
						errorLog.Printf("Failed to delete %s: %s", newKpNodeName, err.Error())
					}

					return
				}

				time.Sleep(1 * time.Second)
				continue
			}

		} else {
			warningLog.Printf("Already at max nodes(%v), will not scale", scaler.maxKpNodes)
		}

	} else {
		warningLog.Println("Currently scaling, will not scale")
	}

}

func (scaler *KProximateScaler) DeleteKpNode(kpNodeName string) error {
	err := scaler.kCluster.DeleteKpNode(kpNodeName)
	if err != nil {
		return err
	}

	err = scaler.pCluster.DeleteKpNode(kpNodeName)
	if err != nil {
		return err
	}

	return err
}
