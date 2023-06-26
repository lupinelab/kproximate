package scaler

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/kubernetes"
	"github.com/lupinelab/kproximate/logger"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
	"k8s.io/apimachinery/pkg/util/uuid"
)

type KProximateScaler struct {
	Config   config.Config
	KCluster kubernetes.Kubernetes
	PCluster kproxmox.Proxmox
}

type ScaleEvent struct {
	ScaleType   int
	KpNodeName  string
	TargetPHost kproxmox.PHostInformation
}

func NewScaler(config *config.Config) *KProximateScaler {
	kClient := kubernetes.NewKubernetesClient()
	pClient := kproxmox.NewProxmoxClient(config.PmUrl, config.PmAllowInsecure, config.PmUserID, config.PmToken, config.PmDebug)

	kpNodeTemplateRef, err := pClient.Client.GetVmRefByName(config.KpNodeTemplateName)
	if err != nil {
		panic(err.Error())
	}

	config.KpNodeTemplateRef = *kpNodeTemplateRef

	config.KpNodeTemplateConfig, err = pClient.GetKpTemplateConfig(kpNodeTemplateRef)
	if err != nil {
		panic(err.Error())
	}

	config.KpNodeParams = map[string]interface{}{
		"agent":     "enabled=1",
		"balloon":   0,
		"cores":     config.KpNodeCores,
		"ipconfig0": "ip=dhcp",
		"memory":    config.KpNodeMemory,
		"onboot":    1,
		"sshkeys":   strings.Replace(url.QueryEscape(config.SshKey), "+", "%20", 1),
	}

	scaler := &KProximateScaler{
		Config:   *config,
		KCluster: *kClient,
		PCluster: *pClient,
	}

	return scaler
}

func (scaler *KProximateScaler) GetCurrentScalingEvents() int {
	pNodes, err := scaler.PCluster.GetAllKpNodes()
	if err != nil {
		logger.ErrorLog.Printf("Could not get pNodes: %s", err.Error())
	}

	kNodes, err := scaler.KCluster.GetKpNodes()
	if err != nil {
		logger.ErrorLog.Printf("Could not get kNodes: %s", err.Error())
	}

	events := 0
match:
	for _, pNode := range pNodes {
		for _, kNode := range kNodes {
			if pNode.Name == kNode.Name {
				continue match
			}
		}
		events++
	}

	return events
}

func (scaler *KProximateScaler) AssessScaleUp(queuedEvents *int) []*ScaleEvent {
	unschedulableResources, err := scaler.KCluster.GetUnschedulableResources()
	if err != nil {
		logger.ErrorLog.Fatalf("Unable to get unschedulable resources: %s", err.Error())
	}

	allKpNodes, err := scaler.PCluster.GetAllKpNodes()
	if err != nil {
		logger.ErrorLog.Fatalf("Unable to get kp-nodes: %s", err.Error())
	}

	requiredScaleEvents := scaler.requiredScaleEvents(unschedulableResources, len(allKpNodes), queuedEvents)

	scaler.selectTargetPHosts(requiredScaleEvents)

	return requiredScaleEvents

	// go scaler.cleanUpEmptyNodes()

	// if atomic.LoadInt32(&scaler.ScaleState) == 0 {
	// 	go scaler.cleanUpStoppedNodes()
	// }

	// go scaler.removeUnbackedNodes()
	// // TODO cleanup orphaned VMs

	// if atomic.LoadInt32(&scaler.ScaleState) == 0 {
	// 	scaleEvent := scaler.considerScaleDown()

	// 	if scaleEvent.scaleType != 0 {
	// 		ctx := context.Background()

	// 		go scaler.scaleDown(ctx, scaleEvent)
	// 	}
	// }
}

func (scaler *KProximateScaler) requiredScaleEvents(requiredResources *kubernetes.UnschedulableResources, numKpNodes int, queuedEvents *int) []*ScaleEvent {
	requiredScaleEvents := []*ScaleEvent{}
	var numCpuNodesRequired int
	var numMemoryNodesRequired int

	if requiredResources.Cpu != 0 {
		expectedCpu := float64(scaler.Config.KpNodeCores) * float64(numKpNodes+*queuedEvents)
		unaccountedCpu := requiredResources.Cpu - expectedCpu
		numCpuNodesRequired = int(math.Ceil(unaccountedCpu / float64(scaler.Config.KpNodeTemplateConfig.Cores)))
	}

	if requiredResources.Memory != 0 {
		expectedMemory := int64(scaler.Config.KpNodeMemory<<20) * int64(numKpNodes+*queuedEvents)
		unaccountedMemory := requiredResources.Memory - int64(expectedMemory)
		numMemoryNodesRequired = int(math.Ceil(float64(unaccountedMemory) / float64(scaler.Config.KpNodeTemplateConfig.Memory<<20)))
	}

	numNodesRequired := int(math.Max(float64(numCpuNodesRequired), float64(numMemoryNodesRequired)))

	for kpNode := 1; kpNode <= numNodesRequired; kpNode++ {
		newName := fmt.Sprintf("kp-node-%s", uuid.NewUUID())

		requiredEvent := &ScaleEvent{
			ScaleType:  1,
			KpNodeName: newName,
		}

		requiredScaleEvents = append(requiredScaleEvents, requiredEvent)
	}

	if len(requiredScaleEvents) == 0 && *queuedEvents == 0 {
		schedulingFailed, err := scaler.KCluster.GetFailedSchedulingDueToControlPlaneTaint()
		if err != nil {
			logger.WarningLog.Printf("Could not get pods: %s", err.Error())
		}

		if schedulingFailed == true {
			newName := fmt.Sprintf("kp-node-%s", uuid.NewUUID())
			requiredEvent := &ScaleEvent{
				ScaleType:  1,
				KpNodeName: newName,
			}

			requiredScaleEvents = append(requiredScaleEvents, requiredEvent)
		}
	}

	return requiredScaleEvents
}

func (scaler *KProximateScaler) selectTargetPHosts(scaleEvents []*ScaleEvent) {
	pHosts := scaler.PCluster.GetClusterStats()
	kpNodes := scaler.PCluster.GetRunningKpNodes()

selected:
	for _, scaleEvent := range scaleEvents {
	skipHost:
		for _, pHost := range pHosts {
			// Check for a scaleEvent targeting the pHost
			for _, scaleEvent := range scaleEvents {
				if scaleEvent.TargetPHost.Id == pHost.Id {
					continue skipHost
				}
			}

			for _, kpNode := range kpNodes {
				// Check for an existing kpNode on the pHost
				if strings.Contains(pHost.Id, kpNode.Node) {
					continue skipHost
				}
			}

			scaleEvent.TargetPHost = pHost
			continue selected
		}
		// Else select a node with the most available memory
		var maxAvailMemNode kproxmox.PHostInformation

		for i, pHost := range pHosts {
			if i == 0 || (pHost.Maxmem-pHost.Mem) > maxAvailMemNode.Mem {
				maxAvailMemNode = pHost
			}
		}

		scaleEvent.TargetPHost = maxAvailMemNode
	}

	return
}

func (scaler *KProximateScaler) ScaleUp(ctx context.Context, scaleEvent *ScaleEvent) error {
	logger.InfoLog.Printf("Provisioning %s on pcluster", scaleEvent.KpNodeName)

	ok := make(chan bool)

	errchan := make(chan error)

	pctx, cancelCtx := context.WithTimeout(ctx, time.Duration(20*time.Second))
	defer cancelCtx()

	go scaler.PCluster.NewKpNode(
		pctx,
		ok,
		errchan,
		scaler.Config.KpNodeTemplateRef,
		scaleEvent.KpNodeName,
		scaleEvent.TargetPHost.Node,
		scaler.Config.KpNodeParams,
	)

ptimeout:
	select {
	case <-pctx.Done():
		cancelCtx()

		return fmt.Errorf("Timed out waiting for %s to start", scaleEvent.KpNodeName)

	case err := <-errchan:

		return err

	case <-ok:
		logger.InfoLog.Printf("Started %s", scaleEvent.KpNodeName)
		break ptimeout
	}

	logger.InfoLog.Printf("Waiting for %s to join kcluster", scaleEvent.KpNodeName)

	kctx, cancelCtx := context.WithTimeout(ctx, time.Duration(60*time.Second))
	// Add wait for join variable
	defer cancelCtx()

	go scaler.KCluster.WaitForJoin(
		kctx,
		ok,
		scaleEvent.KpNodeName,
	)

ktimeout:
	select {
	case <-kctx.Done():
		cancelCtx()
		return fmt.Errorf("Timed out waiting for %s to join kcluster", scaleEvent.KpNodeName)

	case <-ok:
		break ktimeout
	}

	logger.InfoLog.Printf("%s joined kcluster", scaleEvent.KpNodeName)

	return nil
}

func (scaler *KProximateScaler) scaleDown(ctx context.Context, scaleEvent *ScaleEvent) {
	err := scaler.KCluster.SlowDeleteKpNode(scaleEvent.KpNodeName)
	if err != nil {
		logger.WarningLog.Printf("Could not delete kNode %v, scale down failed: %s", scaleEvent.KpNodeName, err.Error())
	}

	err = scaler.PCluster.DeleteKpNode(scaleEvent.KpNodeName)
	if err != nil {
		logger.WarningLog.Printf("Could not delete pNode %v, scale down failed: %s", scaleEvent.KpNodeName, err.Error())
	}

	logger.InfoLog.Printf("Deleted %s", scaleEvent.KpNodeName)
}

func (scaler *KProximateScaler) NumKpNodes() int {
	kpNodes := scaler.PCluster.GetRunningKpNodes()

	return len(kpNodes)
}

func (scaler *KProximateScaler) DeleteKpNode(kpNodeName string) error {
	_ = scaler.KCluster.DeleteKpNode(kpNodeName)

	err := scaler.PCluster.DeleteKpNode(kpNodeName)

	return err
}

func (scaler *KProximateScaler) cleanUpEmptyNodes() {
	emptyKpNodes, err := scaler.KCluster.GetEmptyKpNodes()
	if err != nil {
		logger.ErrorLog.Printf("Could not get emtpy nodes: %s", err.Error())
	}

	for _, emptyNode := range emptyKpNodes {
		emptyPNode, err := scaler.PCluster.GetKpNode(emptyNode.Name)
		if err != nil {
			logger.ErrorLog.Printf("Could not get emtpy node: %s", err.Error())
		}

		// Allow new nodes a grace period of emptiness after creation before they are targets for cleanup
		if emptyPNode.Uptime < scaler.Config.EmptyGraceSecondsAfterCreation {
			continue
		}

		err = scaler.DeleteKpNode(emptyNode.Name)
		if err != nil {
			logger.WarningLog.Printf("Failed to delete empty node %s: %s", emptyNode.Name, err.Error())
		}

		logger.InfoLog.Printf("Deleted empty node: %s", emptyNode.Name)

	}
}

func (scaler *KProximateScaler) cleanUpStoppedNodes() {
	kpNodes, err := scaler.PCluster.GetAllKpNodes()
	if err != nil {
		logger.ErrorLog.Printf("Could not get pNodes: %s", err.Error())
	}

	var stoppedNodes []kproxmox.VmInformation
	for _, kpNode := range kpNodes {
		if kpNode.Status == "stopped" {
			stoppedNodes = append(stoppedNodes, kpNode)
		}
	}

	for _, stoppedNode := range stoppedNodes {
		err := scaler.PCluster.DeleteKpNode(stoppedNode.Name)
		if err != nil {
			logger.WarningLog.Printf("Failed to delete stopped node %s: %s", stoppedNode.Name, err.Error())
			continue
		}

		logger.InfoLog.Printf("Deleted stopped node %s", stoppedNode.Name)
	}
}

// func (scaler *KProximateScaler) considerScaleDown() *ScaleEvent {
// 	allocatedResources, err := scaler.KCluster.GetKpNodesAllocatedResources()
// 	if err != nil {
// 		logger.WarningLog.Printf("Consider scale down failed, unable to get allocated resources: %s", err.Error())
// 		return &ScaleEvent{}
// 	}

// 	numKpNodes := scaler.numKpNodes()

// 	totalCpuAllocatable := scaler.Config.KpNodeCores * numKpNodes
// 	totalMemoryAllocatable := scaler.Config.KpNodeMemory << 20 * numKpNodes

// 	var totalCpuAllocated float64
// 	for _, kpNode := range allocatedResources {
// 		totalCpuAllocated += kpNode.Cpu
// 	}

// 	var totalMemoryAllocated float64
// 	for _, kpNode := range allocatedResources {
// 		totalMemoryAllocated += kpNode.Memory
// 	}

// 	kpNodeHeadroom := scaler.Config.KpNodeHeadroom
// 	if kpNodeHeadroom < 0.2 {
// 		kpNodeHeadroom = 0.2
// 	}
// 	numKpNodesAfterScaleDown := numKpNodes - 1
// 	acceptableLoadForScaleDown :=
// 		(float64(numKpNodesAfterScaleDown) / float64(numKpNodes)) -
// 			kpNodeHeadroom

// 	scaleEvent := ScaleEvent{}

// 	if totalCpuAllocated != 0 {
// 		if totalCpuAllocated/float64(totalCpuAllocatable) <= acceptableLoadForScaleDown {
// 			scaleEvent.ScaleType = -1
// 		}
// 	}

// 	if totalMemoryAllocated != 0 {
// 		if totalMemoryAllocated/float64(totalMemoryAllocatable) <= acceptableLoadForScaleDown {
// 			scaleEvent.ScaleType = -1
// 		}
// 	}

// 	scaler.selectScaleDownTarget(&scaleEvent, allocatedResources)

// 	return &scaleEvent
// }

// func (scaler *KProximateScaler) selectScaleDownTarget(scaleEvent *ScaleEvent, allocatedResources map[string]*kubernetes.AllocatedResources) {
// 	kpNodes, err := scaler.KCluster.GetKpNodes()
// 	if err != nil {
// 		logger.WarningLog.Printf("Consider scale down failed, unable get kp-nodes: %s", err.Error())
// 	}

// 	if scaleEvent.ScaleType != 0 {
// 		var targetNode string
// 		kpNodeLoads := make(map[string]float64)

// 		// Calculate the combined load on each kpNode
// 		for _, kpNode := range kpNodes {
// 			kpNodeLoads[kpNode.Name] =
// 				(allocatedResources[kpNode.Name].Cpu / float64(scaler.Config.KpNodeCores)) +
// 					(allocatedResources[kpNode.Name].Memory / float64(scaler.Config.KpNodeMemory))
// 		}

// 		// Choose the kpnode with the lowest combined load
// 		i := 0
// 		for kpNode := range kpNodeLoads {
// 			if i == 0 || kpNodeLoads[kpNode] < kpNodeLoads[targetNode] {
// 				targetNode = kpNode
// 				i++
// 			}
// 		}

// 		scaleEvent.KpNodeName = targetNode
// 	}
// }

// func (scaler *KProximateScaler) removeUnbackedNodes() {
// 	kNodes, err := scaler.KCluster.GetKpNodes()
// 	if err != nil {
// 		logger.ErrorLog.Printf("Cleanup failed, could not get kNodes: %s", err.Error())
// 	}

// 	for _, kNode := range kNodes {
// 		pNode, err := scaler.PCluster.GetKpNode(kNode.Name)
// 		if err != nil {
// 			logger.ErrorLog.Printf("Could not get pNode: %s", err.Error())
// 		}

// 		if pNode.Name == kNode.Name {
// 			continue
// 		} else {
// 			err := scaler.KCluster.DeleteKpNode(kNode.Name)
// 			if err != nil {
// 				logger.WarningLog.Printf("Could not delete %s: %s", kNode.Name, err.Error())
// 			}
// 			logger.InfoLog.Printf("Deleted unbacked node %s", kNode.Name)
// 		}
// 	}
// }
