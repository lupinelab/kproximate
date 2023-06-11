package scaler

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
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
	scaleState           int32
}

type scaleEvent struct {
	scaleType   int
	kpNodeName  string
	targetPHost kproxmox.PHostInformation
}

func NewScaler(config Config) *KProximateScaler {
	kClient := kubernetes.NewKubernetesClient()
	pClient := kproxmox.NewProxmoxClient(config.PmUrl, config.AllowInsecure, config.PmUserID, config.PmToken)

	kpNodeTemplateRef, err := pClient.Client.GetVmRefByName(config.KpNodeTemplateName)
	if err != nil {
		panic(err.Error())
	}

	kpNodeTemplateConfig, err := pClient.GetKpTemplateConfig(kpNodeTemplateRef)
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
		kpNodeTemplateConfig: kpNodeTemplateConfig,
		kpNodeTemplateRef:    *kpNodeTemplateRef,
		maxKpNodes:           config.MaxKpNodes,
		newNodes:             map[string]scaleEvent{},
		pCluster:             *pClient,
		scaleEvents:          map[string]scaleEvent{},
		scaleState:           0,
	}

	return scaler
}

// func (scaler *KProximateScaler) scaleState() int {
// 	var scaleState int

// 	for _, event := range scaler.scaleEvents {
// 		scaleState += event.scaleType
// 	}

// 	return scaleState
// }

func (scaler *KProximateScaler) numKpNodes() int {
	currentKpNodes := len(scaler.pCluster.GetKpNodes())
	return currentKpNodes + int(scaler.scaleState)
}

func (scaler *KProximateScaler) Start() {
	for {
		unschedulableResources, err := scaler.kCluster.GetUnschedulableResources()
		if err != nil {
			errorLog.Fatalf("Unable to get unschedulable resources: %s", err.Error())
		}

		requiredScaleEvents := scaler.requiredScaleEvents(unschedulableResources)

		if scaler.numKpNodes() < scaler.maxKpNodes {
			scaler.selectTargetPHosts(requiredScaleEvents)
			
			for _, scaleEvent := range requiredScaleEvents {
				
				ctx := context.Background()

				go scaler.scale(ctx, &scaleEvent)

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

func (scaler *KProximateScaler) requiredScaleEvents(requiredResources *kubernetes.UnschedulableResources) []scaleEvent {
	requiredScaleEvents := []scaleEvent{}
	var numCpuNodesRequired int
	var numMemoryNodesRequired int

	if requiredResources.Cpu != 0 {
		expectedCpu := float64(scaler.kpNodeTemplateConfig.Cores) * float64(int(scaler.scaleState)+len(scaler.newNodes))
		unaccountedCpu := requiredResources.Cpu - expectedCpu
		numCpuNodesRequired = int(math.Ceil(unaccountedCpu / float64(scaler.kpNodeTemplateConfig.Cores)))
	}

	if requiredResources.Memory != 0 {
		expectedMemory := int64(scaler.kpNodeTemplateConfig.Memory<<20) * int64(int(scaler.scaleState)+len(scaler.newNodes))
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

		requiredScaleEvents = append(requiredScaleEvents, requiredEvent)
	}

	return requiredScaleEvents
}

func (scaler *KProximateScaler) selectTargetPHosts(scaleEvents []scaleEvent) []scaleEvent {
	pHosts := scaler.pCluster.GetClusterStats()
	kpNodes := scaler.pCluster.GetKpNodes()	
	
	for _, scaleEvent := range scaleEvents{
		// Select a pNode without a kpNode on it or currently being provisioned on it
		for _, pHost := range pHosts {
				for _, kpNode := range kpNodes {
					// Check for an existing kpNode on the pHost
					if kpNode.Node == pHost.Id {
						continue
					}
				}
				// Check for a scaleEvent targeting the pHost
				for _, event := range scaleEvents {
					if event.targetPHost.Node == pHost.Id {
						continue
					}
					scaleEvent.targetPHost = pHost
				}		
		}
		// Else select a node with the most available memory
		var maxAvailMemNode kproxmox.PHostInformation

		for i, pHost := range pHosts {
			if i == 0 || (pHost.Maxmem-pHost.Mem) > maxAvailMemNode.Mem {
				maxAvailMemNode = pHost
			}
		}

		scaleEvent.targetPHost = maxAvailMemNode
	}

	return scaleEvents
}

func (scaler *KProximateScaler) scale(ctx context.Context, scaleEvent *scaleEvent) {
	atomic.AddInt32(&scaler.scaleState, int32(scaleEvent.scaleType))
	infoLog.Printf("Scale state: %+d", scaler.scaleState)

	infoLog.Printf("Provisioning %s on pcluster", scaleEvent.kpNodeName)

	ok := make(chan bool)

	errchan := make(chan error)

	pctx, cancelCtx := context.WithTimeout(ctx, time.Duration(20*time.Second))
	defer cancelCtx()

	go scaler.pCluster.NewKpNode(
		pctx,
		ok,
		errchan,
		scaler.kpNodeTemplateRef,
		scaleEvent.kpNodeName,
		scaleEvent.targetPHost.Node,
		scaler.kpNodeParams,
	)

ptimeout:
	select {
	case <-pctx.Done():
		cancelCtx()

		errorLog.Printf("Timed out waiting for %s to start", scaleEvent.kpNodeName)

		atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
		infoLog.Printf("Scale state: %+d", scaler.scaleState)

		err := scaler.cleaupFailedVm(scaleEvent.kpNodeName)
		if err != nil {
			errorLog.Printf("Cleanup failed for %s: %s", scaleEvent.kpNodeName, err.Error())
		}

		return

	case err := <-errchan:
		errorLog.Printf("Could not provision %s: %s", scaleEvent.kpNodeName, err.Error())

		atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
		infoLog.Printf("Scale state: %+d", scaler.scaleState)

		return

	case <-ok:
		infoLog.Printf("Started %s", scaleEvent.kpNodeName)
		break ptimeout
	}

	infoLog.Printf("Waiting for %s to join kcluster", scaleEvent.kpNodeName)

	kctx, cancelCtx := context.WithTimeout(ctx, time.Duration(60*time.Second))
	defer cancelCtx()

	go scaler.kCluster.WaitForJoin(	
		kctx,
		ok,
		scaleEvent.kpNodeName,
	)

ktimeout:
	select{
	case <-kctx.Done():
		cancelCtx()
		errorLog.Printf("Timed out waiting for %s to join kcluster", scaleEvent.kpNodeName)

		atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
		infoLog.Printf("Scale state: %+d", scaler.scaleState)

		infoLog.Printf("Cleaning up failed scale attempt: %s", scaleEvent.kpNodeName)

		err := scaler.cleaupFailedKJoin(*scaleEvent)
		if err != nil {
			errorLog.Printf("Cleanup failed for %s: %s", scaleEvent.kpNodeName, err.Error())
		}
		
		infoLog.Printf("Deleted %s", scaleEvent.kpNodeName)

		return
	
	case <- ok:
		break ktimeout
	}

	infoLog.Printf("%s joined kcluster", scaleEvent.kpNodeName)

	atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
	infoLog.Printf("Scale state: %+d", scaler.scaleState)
			
	// Allow new kNodes a grace period of emptiness before they are targets for cleanup
	go scaler.newNodeBackOff(*scaleEvent)
}

func (scaler *KProximateScaler) cleaupFailedVm(kpNodeName string) error {
	kpNodes, err := scaler.kCluster.GetNodes()
	if err != nil {
		return err
	}

	for _, kpNode := range kpNodes {
		if kpNode.Name == kpNodeName {
			err := scaler.kCluster.DeleteKpNode(kpNodeName)
			if err != nil {
				return err
			}
		}
	}

	err = scaler.pCluster.DeleteKpNode(kpNodeName)
	if err != nil {
		return err
	}

	infoLog.Printf("Deleted %s", kpNodeName)
	return err
}

func (scaler *KProximateScaler) cleaupFailedKJoin(scaleEvent scaleEvent) error {
	err := scaler.deleteKpNode(scaleEvent.kpNodeName)

	return err
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

		emptyPNode := scaler.pCluster.GetKpNode(emptyNode.Name)

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
