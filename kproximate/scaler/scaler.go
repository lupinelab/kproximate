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
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

type Scaler struct {
	Config   config.Config
	KCluster kubernetes.Kubernetes
	PCluster kproxmox.Proxmox
}

type ScaleEvent struct {
	ScaleType   int
	KpNodeName  string
	TargetPHost kproxmox.PHostInformation
}

func NewScaler(config *config.Config) *Scaler {
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

	scaler := &Scaler{
		Config:   *config,
		KCluster: kClient,
		PCluster: pClient,
	}

	return scaler
}

func (scaler *Scaler) RequiredScaleEvents(requiredResources *kubernetes.UnschedulableResources, currentEvents int) []*ScaleEvent {
	requiredScaleEvents := []*ScaleEvent{}
	var numCpuNodesRequired int
	var numMemoryNodesRequired int

	if requiredResources.Cpu != 0 {
		expectedCpu := float64(scaler.Config.KpNodeCores) * float64(currentEvents)
		unaccountedCpu := requiredResources.Cpu - expectedCpu
		numCpuNodesRequired = int(math.Ceil(unaccountedCpu / float64(scaler.Config.KpNodeCores)))
	}

	if requiredResources.Memory != 0 {
		expectedMemory := int64(scaler.Config.KpNodeMemory<<20) * (int64(currentEvents))
		unaccountedMemory := requiredResources.Memory - expectedMemory
		numMemoryNodesRequired = int(math.Ceil(float64(unaccountedMemory) / float64(scaler.Config.KpNodeMemory<<20)))
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

	if len(requiredScaleEvents) == 0 && currentEvents == 0 {
		schedulingFailed, err := scaler.KCluster.IsFailedSchedulingDueToControlPlaneTaint()
		if err != nil {
			logger.WarningLog.Printf("Could not get pods: %s", err.Error())
		}

		if schedulingFailed {
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

func (scaler *Scaler) SelectTargetPHosts(scaleEvents []*ScaleEvent) {
	pHosts := scaler.PCluster.GetClusterStats()
	kpNodes := scaler.PCluster.GetRunningKpNodes()

selected:
	for _, scaleEvent := range scaleEvents {
	skipHost:
		for _, pHost := range pHosts {
			// Check for a scaleEvent targeting the pHost
			for _, allScaleEvent := range scaleEvents {
				if allScaleEvent.TargetPHost.Id == pHost.Id {
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
			if i == 0 || (pHost.Maxmem-pHost.Mem) > maxAvailMemNode.Maxmem-maxAvailMemNode.Mem {
				maxAvailMemNode = pHost
			}
		}

		scaleEvent.TargetPHost = maxAvailMemNode
	}
}

func (scaler *Scaler) ScaleUp(ctx context.Context, scaleEvent *ScaleEvent) error {
	logger.InfoLog.Printf("Provisioning %s on pcluster", scaleEvent.KpNodeName)

	ok := make(chan bool)

	errchan := make(chan error)

	pctx, cancelPCtx := context.WithTimeout(ctx, time.Duration(time.Second*20))
	defer cancelPCtx()

	go scaler.PCluster.NewKpNode(
		pctx,
		ok,
		errchan,
		scaleEvent.KpNodeName,
		scaleEvent.TargetPHost.Node,
		scaler.Config.KpNodeParams,
		scaler.Config.KpNodeTemplateRef,
	)

ptimeout:
	select {
	case <-pctx.Done():
		cancelPCtx()
		return fmt.Errorf("timed out waiting for %s to start", scaleEvent.KpNodeName)

	case err := <-errchan:
		return err

	case <-ok:
		logger.InfoLog.Printf("Started %s", scaleEvent.KpNodeName)
		break ptimeout
	}

	logger.InfoLog.Printf("Waiting for %s to join kcluster", scaleEvent.KpNodeName)

	// TODO: Add wait for join config variable
	kctx, cancelKCtx := context.WithTimeout(
		ctx,
		time.Duration(
			time.Second*time.Duration(
				scaler.Config.WaitSecondsForJoin,
			),
		),
	)
	defer cancelKCtx()

	go scaler.KCluster.WaitForJoin(
		kctx,
		ok,
		scaleEvent.KpNodeName,
	)

ktimeout:
	select {
	case <-kctx.Done():
		cancelKCtx()
		return fmt.Errorf("timed out waiting for %s to join kcluster", scaleEvent.KpNodeName)

	case <-ok:
		break ktimeout
	}

	logger.InfoLog.Printf("%s joined kcluster", scaleEvent.KpNodeName)

	return nil
}

func (scaler *Scaler) ScaleDown(ctx context.Context, scaleEvent *ScaleEvent) error {
	err := scaler.KCluster.DeleteKpNode(scaleEvent.KpNodeName)
	if err != nil {
		return err
	}

	err = scaler.PCluster.DeleteKpNode(scaleEvent.KpNodeName)
	if err != nil {
		return err
	}

	logger.InfoLog.Printf("Deleted %s", scaleEvent.KpNodeName)
	return err
}

func (scaler *Scaler) NumKpNodes() int {
	kpNodes, err := scaler.KCluster.GetKpNodes()
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to get kp nodes: %s", err.Error())
	}

	return len(kpNodes)
}

func (scaler *Scaler) DeleteKpNode(kpNodeName string) error {
	_ = scaler.KCluster.DeleteKpNode(kpNodeName)

	err := scaler.PCluster.DeleteKpNode(kpNodeName)

	return err
}

// func (scaler *KProximateScaler) cleanUpEmptyNodes() {
// 	emptyKpNodes, err := scaler.KCluster.GetEmptyKpNodes()
// 	if err != nil {
// 		logger.ErrorLog.Printf("Could not get emtpy nodes: %s", err.Error())
// 	}

// 	for _, emptyNode := range emptyKpNodes {
// 		emptyPNode, err := scaler.PCluster.GetKpNode(emptyNode.Name)
// 		if err != nil {
// 			logger.ErrorLog.Printf("Could not get emtpy node: %s", err.Error())
// 		}

// 		// Allow new nodes a grace period of emptiness after creation before they are targets for cleanup
// 		if emptyPNode.Uptime < scaler.Config.EmptyGraceSecondsAfterCreation {
// 			continue
// 		}

// 		err = scaler.DeleteKpNode(emptyNode.Name)
// 		if err != nil {
// 			logger.WarningLog.Printf("Failed to delete empty node %s: %s", emptyNode.Name, err.Error())
// 		}

// 		logger.InfoLog.Printf("Deleted empty node: %s", emptyNode.Name)

// 	}
// }

// func (scaler *KProximateScaler) cleanUpStoppedNodes() {
// 	kpNodes, err := scaler.PCluster.GetAllKpNodes()
// 	if err != nil {
// 		logger.ErrorLog.Printf("Could not get pNodes: %s", err.Error())
// 	}

// 	var stoppedNodes []kproxmox.VmInformation
// 	for _, kpNode := range kpNodes {
// 		if kpNode.Status == "stopped" {
// 			stoppedNodes = append(stoppedNodes, kpNode)
// 		}
// 	}

// 	for _, stoppedNode := range stoppedNodes {
// 		err := scaler.PCluster.DeleteKpNode(stoppedNode.Name)
// 		if err != nil {
// 			logger.WarningLog.Printf("Failed to delete stopped node %s: %s", stoppedNode.Name, err.Error())
// 			continue
// 		}

// 		logger.InfoLog.Printf("Deleted stopped node %s", stoppedNode.Name)
// 	}
// }

func (scaler *Scaler) AssessScaleDown(allocatedResources map[string]*kubernetes.AllocatedResources, numKpNodes int) *ScaleEvent {
	totalCpuAllocatable := scaler.Config.KpNodeCores * numKpNodes
	totalMemoryAllocatable := scaler.Config.KpNodeMemory << 20 * numKpNodes

	var currentCpuAllocated float64
	for _, kpNode := range allocatedResources {
		currentCpuAllocated += kpNode.Cpu
	}

	var currentMemoryAllocated float64
	for _, kpNode := range allocatedResources {
		currentMemoryAllocated += kpNode.Memory
	}

	acceptCpuScaleDown := scaler.assessScaleDownForResourceType(currentCpuAllocated, totalCpuAllocatable, numKpNodes)
	acceptMemoryScaleDown := scaler.assessScaleDownForResourceType(currentMemoryAllocated, totalMemoryAllocatable, numKpNodes)

	if acceptCpuScaleDown && acceptMemoryScaleDown {
		scaleEvent := ScaleEvent{
			ScaleType: -1,
		}
		return &scaleEvent
	}

	return nil
}

func (scaler *Scaler) assessScaleDownForResourceType(currentResourceAllocated float64, totalResourceAllocatable int, numKpNodes int) bool {
	if currentResourceAllocated == 0 {
		return false
	}

	totalResourceLoad := currentResourceAllocated / float64(totalResourceAllocatable)
	acceptableResourceLoadForScaleDown := (float64(numKpNodes-1) / float64(numKpNodes)) -
		(totalResourceLoad * scaler.Config.KpLoadHeadroom)

	return totalResourceLoad < acceptableResourceLoadForScaleDown
}

func (scaler *Scaler) SelectScaleDownTarget(scaleEvent *ScaleEvent, allocatedResources map[string]*kubernetes.AllocatedResources, kpNodes []apiv1.Node) {
	if scaleEvent.ScaleType != 0 {
		kpNodeLoads := make(map[string]float64)

		// Calculate the combined load on each kpNode
		for _, kpNode := range kpNodes {
			kpNodeLoads[kpNode.Name] =
				(allocatedResources[kpNode.Name].Cpu / float64(scaler.Config.KpNodeCores)) +
					(allocatedResources[kpNode.Name].Memory / float64(scaler.Config.KpNodeMemory))
		}

		var targetNode string

		// Choose the kpnode with the lowest combined load
		i := 0
		for kpNode := range kpNodeLoads {
			if i == 0 || kpNodeLoads[kpNode] < kpNodeLoads[targetNode] {
				targetNode = kpNode
				i++
			}
		}

		scaleEvent.KpNodeName = targetNode
	}
}

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
