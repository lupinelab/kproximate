package scaler

import (
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/lupinelab/kproximate/kubernetes"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
	"k8s.io/apimachinery/pkg/util/uuid"
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
	kpNodeParams         map[string]interface{}
	kpNodeTemplateConfig kproxmox.VMConfig
	kpNodeTemplateRef    proxmox.VmRef
	maxKpNodes           int
	newNodes             map[string]scaleEvent
	pCluster             kproxmox.Proxmox
	scaleEvents          map[string]scaleEvent
}

type scaleEvent struct {
	scaleType   int
	kpNodeName  string
	targetPNode kproxmox.NodeInformation
}

func NewScaler(config Config) *KProximateScaler {
	kClient := kubernetes.NewKubernetesClient()
	pClient := kproxmox.NewProxmoxClient(config.PmUrl, config.AllowInsecure, config.PmUserID, config.PmToken)

	pNodeTemplateRef, err := pClient.Client.GetVmRefByName(config.KpNodeTemplateName)
	if err != nil {
		panic(err.Error())
	}

	pNodeTemplateConfig, err := pClient.GetKpTemplateConfig(pNodeTemplateRef)
	if err != nil {
		panic(err.Error())
	}

	kpNodeParams := map[string]interface{}{
		"autostart": true,
		"sshkeys":   strings.Replace(url.QueryEscape(config.SshKey), "+", "%20", 1),
		"ipconfig0": "ip=dhcp",
	}

	scaler := &KProximateScaler{
		kCluster:             *kClient,
		kpNodeParams:         kpNodeParams,
		kpNodeTemplateConfig: pNodeTemplateConfig,
		kpNodeTemplateRef:    *pNodeTemplateRef,
		maxKpNodes:           config.MaxKpNodes,
		newNodes:             map[string]scaleEvent{},
		pCluster:             *pClient,
		scaleEvents:          map[string]scaleEvent{},
	}

	return scaler
}

func (scaler *KProximateScaler) scaleState() int {
	var scaleState int

	for _, event := range scaler.scaleEvents {
		scaleState += event.scaleType
	}

	return scaleState
}

func (scaler *KProximateScaler) numKpNodes() int {
	currentKpNodes := len(scaler.pCluster.GetKpNodes())
	numScaleEvents := len(scaler.scaleEvents)
	return currentKpNodes + numScaleEvents
}

func (scaler *KProximateScaler) Start() {
	for {
		unschedulableResources, err := scaler.kCluster.GetUnschedulableResources()
		if err != nil {
			errorLog.Fatalf("Unable to get unschedulable resources: %s", err.Error())
		}

		requiredScaleEvents := scaler.requiredScaleEvents(unschedulableResources)

		if scaler.numKpNodes() < scaler.maxKpNodes {
			for _, scaleEvent := range requiredScaleEvents {
				scaleEvent.targetPNode = scaler.selectTargetPNode()

				scaler.scaleEvents[scaleEvent.kpNodeName] = scaleEvent
				infoLog.Printf("Scale state: %+d", scaler.scaleState())

				go scaler.scale(scaler.scaleEvents[scaleEvent.kpNodeName])

				// Kick off the scaling events 1 second apart to allow proxmox some time to start the first
				// scaleEvent, otherwise it can attempt to duplicate the VMID
				time.Sleep(time.Second)
			}
		} else if len(requiredScaleEvents) > 0 {
			warningLog.Printf("Already at max nodes: %v", scaler.maxKpNodes)
		}

		cleanUpErr := scaler.cleanUpEmptyNodes()
		if cleanUpErr != nil {
			errorLog.Printf("Cleanup failed: %s", err.Error())
		}

		// TODO Calculate spare capacity and consider consolidation

		time.Sleep(5 * time.Second)
	}
}

func (scaler *KProximateScaler) requiredScaleEvents(requiredResources *kubernetes.UnschedulableResources) map[string]scaleEvent {
	requiredScaleEvents := make(map[string]scaleEvent)
	var numCpuNodesRequired int
	var numMemoryNodesRequired int

	if requiredResources.Cpu != 0 {
		expectedCpu := float64(scaler.kpNodeTemplateConfig.Cores) * float64(scaler.scaleState()+len(scaler.newNodes))
		unaccountedCpu := requiredResources.Cpu - expectedCpu
		numCpuNodesRequired = int(math.Ceil(unaccountedCpu / float64(scaler.kpNodeTemplateConfig.Cores)))
	}

	if requiredResources.Memory != 0 {
		expectedMemory := int64(scaler.kpNodeTemplateConfig.Memory<<20) * int64(scaler.scaleState()+len(scaler.newNodes))
		unaccountedMemory := requiredResources.Memory - int64(expectedMemory)
		numMemoryNodesRequired = int(math.Ceil(float64(unaccountedMemory) / float64(scaler.kpNodeTemplateConfig.Memory<<20)))
	}

	numNodesRequired := int(math.Max(float64(numCpuNodesRequired), float64(numMemoryNodesRequired)))

	for kpNode := 1; kpNode <= numNodesRequired; kpNode++ {
		newName := fmt.Sprintf("kp-node-%s", uuid.NewUUID())

		requiredEvent := scaleEvent{
			scaleType:  1,
			kpNodeName: newName,
		}

		requiredScaleEvents[requiredEvent.kpNodeName] = requiredEvent
	}

	return requiredScaleEvents
}

func (scaler *KProximateScaler) selectTargetPNode() kproxmox.NodeInformation {
	pNodes := scaler.pCluster.GetClusterStats()
	kpNodes := scaler.pCluster.GetKpNodes()

	// Select a pNode without a kpNode on it or currently being provisioned on it
	for _, pNode := range pNodes {
		for _, kpNode := range kpNodes {
			if kpNode.Node == pNode.Id {
				continue
			}

			for _, event := range scaler.scaleEvents {
				if event.targetPNode.Node == pNode.Id {
					continue
				}
			}

			return pNode
		}
	}

	// Else select a node with the most available memory
	var maxAvailMemNode kproxmox.NodeInformation

	for i, node := range pNodes {
		if i == 0 || (node.Maxmem-node.Mem) > maxAvailMemNode.Mem {
			maxAvailMemNode = node
		}
	}

	return maxAvailMemNode
}

func (scaler *KProximateScaler) scale(scaleEvent scaleEvent) {
	infoLog.Printf("Provisioning %s on pcluster", scaleEvent.kpNodeName)
	err := scaler.pCluster.NewKpNode(
		scaler.kpNodeTemplateRef,
		scaleEvent.kpNodeName,
		scaleEvent.targetPNode.Node,
		scaler.kpNodeParams,
	)
	if err != nil {
		errorLog.Printf("Cloud not provision %s: %s", scaleEvent.kpNodeName, err.Error())

		delete(scaler.scaleEvents, scaleEvent.kpNodeName)
		infoLog.Printf("Scale state: %+d", scaler.scaleState())

		infoLog.Printf("Cleaning up failed scale attempt: %s", scaleEvent.kpNodeName)

		err := scaler.pCluster.DeleteKpNode(scaleEvent.kpNodeName)
		if err != nil {
			errorLog.Printf("Cleanup failed for %s: %s", scaleEvent.kpNodeName, err.Error())
			return
		}
		infoLog.Printf("Deleted %s", scaleEvent.kpNodeName)

		return
	}

	infoLog.Printf("Waiting for %s to join kcluster", scaleEvent.kpNodeName)
	retry := 0
	for retry < 60 {
		if scaler.kCluster.CheckKpNodeReady(scaleEvent.kpNodeName) {
			infoLog.Printf("%s joined kcluster", scaleEvent.kpNodeName)

			scaler.newNodes[scaleEvent.kpNodeName] = scaleEvent

			delete(scaler.scaleEvents, scaleEvent.kpNodeName)
			infoLog.Printf("Scale state: %+d", scaler.scaleState())

			// Allow new kNodes a grace period of emptiness before they are targets for cleanup
			go scaler.newNodeBackOff(scaleEvent)

			return
		}

		retry++
		if retry == 60 {
			scaler.cleaupFailedScaleEvent(scaleEvent)
		}

		time.Sleep(1 * time.Second)
	}
}

func (scaler *KProximateScaler) cleaupFailedScaleEvent(scaleEvent scaleEvent) {
	errorLog.Printf("Timeout waiting for %s to join the cluster: ", scaleEvent.kpNodeName)

	delete(scaler.scaleEvents, scaleEvent.kpNodeName)
	infoLog.Printf("Scale state: %+d", scaler.scaleState())

	infoLog.Printf("Cleaning up failed scale attempt: %s", scaleEvent.kpNodeName)

	err := scaler.deleteKpNode(scaleEvent.kpNodeName)
	if err != nil {
		errorLog.Printf("Cleanup failed for %s: %s", scaleEvent.kpNodeName, err.Error())
		return
	}
	infoLog.Printf("Deleted %s", scaleEvent.kpNodeName)
}

// Allow new kNodes a grace period before they are targets for cleanUp
func (scaler *KProximateScaler) newNodeBackOff(scaleEvent scaleEvent) {
	time.Sleep(time.Second * 120)
	delete(scaler.newNodes, scaleEvent.kpNodeName)
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

func (scaler *KProximateScaler) cleanUpEmptyNodes() error {
	emptyKpNodes, err := scaler.kCluster.GetEmptyNodes()
	if err != nil {
		return err
	}

	for _, emptyNode := range emptyKpNodes {
		for _, newNode := range scaler.newNodes {
			if emptyNode.Name == newNode.kpNodeName {
				continue
			}
		}

		emptyPNode, err := scaler.pCluster.GetKpNode(emptyNode.Name)
		if err != nil {
			return err
		}

		// in a situation where kproximate has crashed or been restarted during a scaling event,
		// after restart allow any pnodes a grace period before they are considered targets for cleanUp
		if emptyPNode.Uptime < 180 {
			continue
		}

		err = scaler.deleteKpNode(emptyNode.Name)
		if err != nil {
			return err
		}

		infoLog.Printf("Deleted empty node: %s", emptyNode.Name)

	}

	return nil
}
