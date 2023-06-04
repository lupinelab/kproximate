package scaler

import (
	"fmt"
	"log"
	"math"
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
	kpNodeTemplateConfig kproxmox.VMConfig
	kpNodeTemplateRef    proxmox.VmRef
	maxKpNodes           int
	pCluster             kproxmox.Proxmox
	scaleEvents          map[string]scaleEvent
	newNodes             map[string]scaleEvent
}

type scaleEvent struct {
	scaleType   int
	kpNodeName  string
	targetPNode kproxmox.NodeInformation
}

func NewScaler(config Config) *KProximateScaler {
	kClient := kubernetes.NewKubernetesClient()
	pClient := kproxmox.NewProxmoxClient(config.PMUrl, config.AllowInsecure, config.PMUserID, config.PMToken)

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
		pCluster:          *pClient,
		scaleEvents:       map[string]scaleEvent{},
		newNodes:          map[string]scaleEvent{},
	}

	kpNodeTemplateConfig, err := scaler.pCluster.GetKpTemplateConfig(kpNodeTemplateRef)
	if err != nil {
		panic(err.Error())
	}

	scaler.kpNodeTemplateConfig = kpNodeTemplateConfig

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
	return len(scaler.pCluster.GetKpNodes())
}

func (scaler *KProximateScaler) Start() {
	for {
		unschedulableResources, err := scaler.kCluster.GetUnschedulableResources()
		if err != nil {
			errorLog.Fatalf("Unable to get unschedulable resources: %s", err.Error())
		}

		requiredScaleEvents := scaler.getRequiredScaleEvents(unschedulableResources)

		if scaler.numKpNodes() < scaler.maxKpNodes {
			for _, scaleEvent := range requiredScaleEvents {
				infoLog.Printf("Scale %+d requested", scaleEvent.scaleType)
				scaler.scaleEvents[scaleEvent.kpNodeName] = scaleEvent

				infoLog.Printf("Scale state: %+d", scaler.scaleState())
				go scaler.scale(&scaleEvent)

				// Kick off the scaling events 1 second apart to allow proxmox some time to start the first 
				// scaleEvent, otherwise we try to duplicate the VMID
				time.Sleep(time.Second)
			}
		} else if len(requiredScaleEvents) > 0 {
			warningLog.Printf("Already at max nodes: %v", scaler.maxKpNodes)
		}

		cleanUpErr := scaler.cleanUp()
		if cleanUpErr != nil {
			errorLog.Printf("Cleanup failed: %s", err.Error())
		}

		// TODO Calculate spare capacity and consider consolidation
		
		time.Sleep(5 * time.Second)
	}
}

func (scaler *KProximateScaler) getRequiredScaleEvents(requiredResources *kubernetes.UnschedulableResources) []scaleEvent {
	var scaleEvents []scaleEvent
	var numCpuNodesRequired int
	var numMemoryNodesRequired int

	if requiredResources.Cpu != 0 {
		expectedCpu := float64(scaler.kpNodeTemplateConfig.Cores) * float64(scaler.scaleState() + len(scaler.newNodes))
		unaccountedCpu := requiredResources.Cpu - expectedCpu
		numCpuNodesRequired = int(math.Ceil(unaccountedCpu / float64(scaler.kpNodeTemplateConfig.Cores)))
	}

	if requiredResources.Memory != 0 {
		expectedMemory := int64(scaler.kpNodeTemplateConfig.Memory << 20) * int64(scaler.scaleState() + len(scaler.newNodes))
		unaccountedMemory := requiredResources.Memory - int64(expectedMemory)
		numMemoryNodesRequired = int(math.Ceil(float64(unaccountedMemory) / float64(scaler.kpNodeTemplateConfig.Memory << 20)))
	}

	numNodesRequired := int(math.Max(float64(numCpuNodesRequired), float64(numMemoryNodesRequired)))

	for node := 1; node <= numNodesRequired; node++ {
		newName := fmt.Sprintf("kp-node-%s", uuid.NewUUID())

		scaleEvents = append(scaleEvents, scaleEvent{
			scaleType:   1,
			kpNodeName:  newName,
			targetPNode: scaler.selectTargetPNode(),
		})
	}

	return scaleEvents
}

func (scaler *KProximateScaler) selectTargetPNode() kproxmox.NodeInformation {
	pNodes := scaler.pCluster.GetClusterStats()
	kpNodes := scaler.pCluster.GetKpNodes()

	// Select a pnode without a kpnode on it
	for _, node := range pNodes {
		for _, kpNode := range kpNodes {
			if strings.Contains(kpNode.Name, node.Node) {
				continue
			}
			return node
		}
	}

	// Else select a node with the most available memory
	var maxAvailMemNode kproxmox.NodeInformation

	for i, node := range pNodes {
		if i == 0 || (node.Maxmem - node.Mem) > maxAvailMemNode.Mem {
			maxAvailMemNode = node
		}
	}

	return maxAvailMemNode
}

func (scaler *KProximateScaler) scale(scaleEvent *scaleEvent) {
	infoLog.Println("Provisioning new kp-node on pcluster")
	err := scaler.pCluster.NewKpNode(
		scaler.kpNodeTemplateRef,
		scaleEvent.kpNodeName,
		scaleEvent.targetPNode.Node,
	)
	if err != nil {
		errorLog.Printf("Cloud not provision new kp-node %s: %s", scaleEvent.kpNodeName, err.Error())
		delete(scaler.scaleEvents, scaleEvent.kpNodeName)
		
		return
	}

	infoLog.Printf("Waiting for %s to join kcluster", scaleEvent.kpNodeName)
	retry := 0
	for retry < 60 {
		if scaler.kCluster.CheckKpNodeReady(scaleEvent.kpNodeName) {
			infoLog.Printf("%s joined kcluster", scaleEvent.kpNodeName)

			scaler.newNodes[scaleEvent.kpNodeName] = *scaleEvent
			delete(scaler.scaleEvents, scaleEvent.kpNodeName)

			go scaler.newNodeBackOff(scaleEvent)
			infoLog.Printf("Scale state: %+d", scaler.scaleState())

			return
		}

		retry++
		if retry == 60 {
			errorLog.Printf("Timeout waiting for %s to join the cluster: ", scaleEvent.kpNodeName)

			delete(scaler.scaleEvents, scaleEvent.kpNodeName)
			infoLog.Printf("Scale state: %+d", scaler.scaleState())

			infoLog.Printf("Cleaning up failed node from scale attempt: %s", scaleEvent.kpNodeName)

			err := scaler.deleteKpNode(scaleEvent.kpNodeName)
			if err != nil {
				errorLog.Printf("Failed to delete node: %s", err.Error())
				return
			}
			infoLog.Printf("Deleted %s", scaleEvent.kpNodeName)
			return
		}

		time.Sleep(1 * time.Second)
	}
	delete(scaler.scaleEvents, scaleEvent.kpNodeName)
}

func (scaler *KProximateScaler) newNodeBackOff(scaleEvent *scaleEvent) {
	time.Sleep(time.Second * 60)
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

func (scaler *KProximateScaler) cleanUp() error {
	kpNodes := scaler.pCluster.GetKpNodes()

	for _, pnode := range kpNodes {
		emptyKpNodes, err := scaler.kCluster.GetEmptyNodes()
		if err != nil {
			return err
		}

		for _, node := range emptyKpNodes {
			if pnode.Name == node.Name && pnode.Uptime < 120 {
				continue
			}

			err := scaler.deleteKpNode(node.Name)
			if err != nil {
				return err
			}

			infoLog.Printf("Deleted empty node: %s", node.Name)
			
		}
	}

	return nil
}
