package scaler

import (
	"log"
	"os"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/lupinelab/kproximate/kubernetes"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
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
	kCluster             kubernetes.Kubernetes
	kpNodeTemplateConfig kproxmox.VMConfig
	kpNodeTemplateRef    proxmox.VmRef
	maxKpNodes           int
	numKpNodes           int
	pCluster             kproxmox.Proxmox
	scaleState           int64
}

func NewScaler(config Config) *KProximateScaler {
	kClient := kubernetes.NewKubernetesClient()
	pClient := kproxmox.NewProxmoxClient(config.PMUrl, config.AllowInsecure, config.PMUserID, config.PMToken)

	numKpNodes := len(pClient.GetKpNodes())

	kpNodeTemplateRef, err := pClient.Client.GetVmRefByName(config.KpNodeTemplateName)
	if err != nil {
		panic(err.Error())
	}

	
	if err != nil {
		panic(err.Error())
	}

	scaler := &KProximateScaler{
		kCluster:          *kClient,
		kpNodeTemplateRef: *kpNodeTemplateRef,
		maxKpNodes:        config.MaxKpNodes,
		numKpNodes:        numKpNodes,
		pCluster:          *pClient,
		scaleState:        0,
	}

	kpNodeTemplateConfig, err := scaler.pCluster.GetKpTemplateConfig(kpNodeTemplateRef)
	if err != nil {
		panic(err.Error())
	}

	scaler.kpNodeTemplateConfig = kpNodeTemplateConfig

	return scaler
}

func (scaler *KProximateScaler) updateState() {
	scaler.numKpNodes = len(scaler.pCluster.GetKpNodes())
}

func (scaler *KProximateScaler) Start() {
	for {
		scaler.updateState()

		requiredResources, err := scaler.kCluster.GetUnschedulableResources()
		if err != nil {
			errorLog.Fatalf("Unable to get unschedulable resources: %s", err.Error())
		}

		if requiredResources.Memory != 0 || requiredResources.Cpu != 0 {
			// define scaleEvent{}, instantiated by assessScaleRequirements() which registers scaleEvents in scaler{}
			// scaleEvent contains: type (pos/neg), requirements that triggered it, KpNodeName, targetNode etc.
			// assessScaleRequirements will assess the amount to scale from the currently registered scaling events' triggering requirements, kpsnodetemplate etc, against requiredResorces. Will also be able to scale down
			// scaleState then is calculated from the number of scaleEvents registered in the scaler

			// TODO calculate scale amount
			// TODO select target node(s)
			// TODO Calculate spare capacity and consider consolidation
			infoLog.Println("Scale up requested")
			go scaler.scaleUp(requiredResources)
		}

		cleanUpErr := scaler.cleanUp()
		if cleanUpErr != nil {
			errorLog.Printf("Cleanup failed: %s", err.Error())
		}
		time.Sleep(5 * time.Second)
	}
}

func (scaler *KProximateScaler) scaleUp(requiredResources *kubernetes.UnschedulableResources) {
	if scaler.scaleState == 0 {
		if scaler.numKpNodes < scaler.maxKpNodes {
			scaler.scaleState++
			infoLog.Printf("ScaleState: %+d", scaler.scaleState)

			infoLog.Println("Provisioning new kp-node on pcluster")
			newKpNodeName, err := scaler.pCluster.NewKpNode(
				scaler.kpNodeTemplateRef,
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
					infoLog.Printf("ScaleState: %+d", scaler.scaleState)

					return
				}

				retry++
				if retry == 60 {
					errorLog.Printf("Timeout waiting for %s to join the cluster: ", newKpNodeName)

					scaler.scaleState--
					infoLog.Printf("ScaleState: %+d", scaler.scaleState)

					infoLog.Printf("Cleaning up failed node from scale attempt: %s", newKpNodeName)

					err := scaler.deleteKpNode(newKpNodeName)
					if err != nil {
						errorLog.Printf("Failed to delete %s: %s", newKpNodeName, err.Error())
					}
					infoLog.Printf("Deleted %s", newKpNodeName)
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

func (scaler *KProximateScaler) deleteKpNode(kpNodeName string) error {
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

func (scaler *KProximateScaler) cleanUp() error {
	emptyKpNodes, err := scaler.kCluster.GetEmptyNodes()
	if err != nil {
		return err
	}

	for _, node := range emptyKpNodes {
		err := scaler.deleteKpNode(node.Name)
		if err != nil {
			return err
		}
		infoLog.Printf("Deleted empty node: %s", node.Name)
	}

	scaler.updateState()

	return err
}
