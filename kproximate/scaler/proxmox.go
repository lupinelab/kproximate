package scaler

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/kubernetes"
	"github.com/lupinelab/kproximate/logger"
	"github.com/lupinelab/kproximate/proxmox"
	"k8s.io/apimachinery/pkg/util/uuid"
)

type ProxmoxScaler struct {
	config     config.KproximateConfig
	Kubernetes kubernetes.Kubernetes
	Proxmox    proxmox.Proxmox
}

func NewProxmoxScaler(config config.KproximateConfig) (Scaler, error) {
	kubernetes, err := kubernetes.NewKubernetesClient()
	if err != nil {
		return nil, err
	}

	proxmox, err := proxmox.NewProxmoxClient(config.PmUrl, config.PmAllowInsecure, config.PmUserID, config.PmToken, config.PmPassword, config.PmDebug)
	if err != nil {
		return nil, err
	}

	config.KpNodeNameRegex = *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, config.KpNodeNamePrefix))

	config.KpNodeParams = map[string]interface{}{
		"agent":     "enabled=1",
		"balloon":   0,
		"cores":     config.KpNodeCores,
		"ipconfig0": "ip=dhcp",
		"memory":    config.KpNodeMemory,
		"onboot":    1,
	}

	if !config.KpNodeDisableSsh {
		config.KpNodeParams["sshkeys"] = strings.Replace(url.QueryEscape(config.SshKey), "+", "%20", 1)
	}

	scaler := ProxmoxScaler{
		config:     config,
		Kubernetes: &kubernetes,
		Proxmox:    &proxmox,
	}

	return &scaler, err
}

func (scaler *ProxmoxScaler) newKpNodeName() string {
	return fmt.Sprintf("%s-%s", scaler.config.KpNodeNamePrefix, uuid.NewUUID())
}

func (scaler *ProxmoxScaler) requiredScaleEvents(requiredResources kubernetes.UnschedulableResources, numCurrentEvents int) ([]*ScaleEvent, error) {
	requiredScaleEvents := []*ScaleEvent{}
	var numCpuNodesRequired int
	var numMemoryNodesRequired int

	if requiredResources.Cpu != 0 {
		// The expected cpu resources after in-progress scaling events complete
		expectedCpu := float64(scaler.config.KpNodeCores) * float64(numCurrentEvents)
		// The expected amount of cpu resources still required after in-progress scaling events complete
		unaccountedCpu := requiredResources.Cpu - expectedCpu
		// The least amount of nodes that will satisfy the unaccountedMemory
		numCpuNodesRequired = int(math.Ceil(unaccountedCpu / float64(scaler.config.KpNodeCores)))
	}

	if requiredResources.Memory != 0 {
		// Bit shift mebibytes to bytes
		kpNodeMemoryBytes := scaler.config.KpNodeMemory << 20
		// The expected memory resources after in-progress scaling events complete
		expectedMemory := int64(kpNodeMemoryBytes) * int64(numCurrentEvents)
		// The expected amount of memory resources still required after in-progress scaling events complete
		unaccountedMemory := requiredResources.Memory - expectedMemory
		// The least amount of nodes that will satisfy the unaccountedMemory
		numMemoryNodesRequired = int(math.Ceil(float64(unaccountedMemory) / float64(kpNodeMemoryBytes)))
	}

	// The largest of the above two node requirements
	numNodesRequired := int(math.Max(float64(numCpuNodesRequired), float64(numMemoryNodesRequired)))

	for kpNode := 1; kpNode <= numNodesRequired; kpNode++ {
		newName := scaler.newKpNodeName()

		scaleEvent := ScaleEvent{
			ScaleType: 1,
			NodeName:  newName,
		}

		requiredScaleEvents = append(requiredScaleEvents, &scaleEvent)
	}

	// If there are no worker nodes then pods can fail to schedule due to a control-plane taint, trigger a scaling event
	if len(requiredScaleEvents) == 0 && numCurrentEvents == 0 {
		schedulingFailed, err := scaler.Kubernetes.IsFailedSchedulingDueToControlPlaneTaint()
		if err != nil {
			return nil, err
		}

		if schedulingFailed {
			newName := scaler.newKpNodeName()
			scaleEvent := ScaleEvent{
				ScaleType: 1,
				NodeName:  newName,
			}

			requiredScaleEvents = append(requiredScaleEvents, &scaleEvent)
		}
	}

	return requiredScaleEvents, nil
}

func (scaler *ProxmoxScaler) RequiredScaleEvents(allScaleEvents int) ([]*ScaleEvent, error) {
	unschedulableResources, err := scaler.Kubernetes.GetUnschedulableResources(int64(scaler.config.KpNodeCores), int64(scaler.config.KpNodeMemory), scaler.config.KpNodeNameRegex)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to get unschedulable resources: %s", err.Error())
	}

	return scaler.requiredScaleEvents(unschedulableResources, allScaleEvents)
}

func selectTargetHost(hosts []proxmox.HostInformation, kpNodes []proxmox.VmInformation, scaleEvents []*ScaleEvent) proxmox.HostInformation {
skipHost:
	for _, host := range hosts {
		// Check for a scaleEvent targeting the pHost
		for _, scaleEvent := range scaleEvents {
			if scaleEvent.TargetHost.Node == host.Node {
				continue skipHost
			}
		}

		for _, kpNode := range kpNodes {
			// Check for an existing kpNode on the pHost
			if kpNode.Node == host.Node {
				continue skipHost
			}
		}

		return host
	}

	return proxmox.HostInformation{}
}

func selectMaxAvailableMemHost(hosts []proxmox.HostInformation) proxmox.HostInformation {
	// Select a node with the most available memory
	maxAvailMemNode := hosts[0]

	for _, host := range hosts {
		if (host.Maxmem - host.Mem) > (maxAvailMemNode.Maxmem - maxAvailMemNode.Mem) {
			maxAvailMemNode = host
		}
	}

	return maxAvailMemNode
}

func (scaler *ProxmoxScaler) SelectTargetHosts(scaleEvents []*ScaleEvent) error {
	hosts, err := scaler.Proxmox.GetClusterStats()
	if err != nil {
		return err
	}

	kpNodes, err := scaler.Proxmox.GetRunningKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return err
	}

	for _, scaleEvent := range scaleEvents {
		scaleEvent.TargetHost = selectTargetHost(hosts, kpNodes, scaleEvents)

		if scaleEvent.TargetHost == (proxmox.HostInformation{}) {
			scaleEvent.TargetHost = selectMaxAvailableMemHost(hosts)
		}
	}

	return nil
}

func waitForNodeStart(ctx context.Context, cancel context.CancelFunc, scaleEvent *ScaleEvent, ok chan (bool), errchan chan (error)) error {
	select {
	case <-ctx.Done():
		cancel()
		return fmt.Errorf("timed out waiting for %s to start", scaleEvent.NodeName)

	case err := <-errchan:
		return err

	case <-ok:
		return nil
	}
}

func waitForNodeReady(ctx context.Context, cancel context.CancelFunc, scaleEvent *ScaleEvent, ok chan (bool), errchan chan (error)) error {
	select {
	case <-ctx.Done():
		cancel()
		return fmt.Errorf("timed out waiting for %s to be ready", scaleEvent.NodeName)

	case err := <-errchan:
		return err

	case <-ok:
		return nil
	}
}

func waitForNodeJoin(ctx context.Context, cancel context.CancelFunc, scaleEvent *ScaleEvent, ok chan (bool)) error {
	select {
	case <-ctx.Done():
		cancel()
		return fmt.Errorf("timed out waiting for %s to join kubernetes cluster", scaleEvent.NodeName)
	case <-ok:
		return nil
	}
}

func (scaler *ProxmoxScaler) ScaleUp(ctx context.Context, scaleEvent *ScaleEvent) error {
	logger.InfoLog.Printf("Provisioning %s on %s", scaleEvent.NodeName, scaleEvent.TargetHost.Node)

	okChan := make(chan bool)
	defer close(okChan)

	errChan := make(chan error)
	defer close(errChan)

	pctx, cancelPCtx := context.WithTimeout(
		ctx,
		time.Duration(
			time.Second*time.Duration(
				scaler.config.WaitSecondsForProvision,
			),
		),
	)
	defer cancelPCtx()

	go scaler.Proxmox.NewKpNode(
		pctx,
		okChan,
		errChan,
		scaleEvent.NodeName,
		scaleEvent.TargetHost.Node,
		scaler.config.KpNodeParams,
		scaler.config.KpLocalTemplateStorage,
		scaler.config.KpNodeTemplateName,
		scaler.config.KpJoinCommand,
	)

	err := waitForNodeStart(pctx, cancelPCtx, scaleEvent, okChan, errChan)
	if err != nil {
		return err
	}

	logger.InfoLog.Printf("Started %s", scaleEvent.NodeName)

	if scaler.config.KpQemuExecJoin {
		go scaler.Proxmox.CheckNodeReady(pctx, okChan, errChan, scaleEvent.NodeName)

		// TODO could this call CheckNodeReady itself?
		err := waitForNodeReady(pctx, cancelPCtx, scaleEvent, okChan, errChan)
		if err != nil {
			return err
		}

		err = scaler.joinByQemuExec(scaleEvent.NodeName)
		if err != nil {
			return err
		}
	}

	logger.InfoLog.Printf("Waiting for %s to join kubernetes cluster", scaleEvent.NodeName)

	kctx, cancelKCtx := context.WithTimeout(
		ctx,
		time.Duration(
			time.Second*time.Duration(
				scaler.config.WaitSecondsForJoin,
			),
		),
	)
	defer cancelKCtx()

	go scaler.Kubernetes.CheckForNodeJoin(
		kctx,
		okChan,
		scaleEvent.NodeName,
	)

	err = waitForNodeJoin(kctx, cancelKCtx, scaleEvent, okChan)
	if err != nil {
		return err
	}

	logger.InfoLog.Printf("%s joined kubernetes cluster", scaleEvent.NodeName)

	return nil
}

func (scaler *ProxmoxScaler) joinByQemuExec(nodeName string) error {
	logger.InfoLog.Printf("Executing join command on %s", nodeName)
	joinExecPid, err := scaler.Proxmox.QemuExecJoin(nodeName, scaler.config.KpJoinCommand)
	if err != nil {
		return err
	}

	var status proxmox.QemuExecStatus

	for status.Exited != 1 {
		status, err = scaler.Proxmox.GetQemuExecJoinStatus(nodeName, joinExecPid)
		if err != nil {
			return err
		}

		time.Sleep(time.Second * 1)
	}

	if status.ExitCode != 0 {
		return fmt.Errorf("join command for %s failed:\n%s", nodeName, status.OutData)
	} else {
		logger.InfoLog.Printf("Join command for %s executed successfully", nodeName)
		return nil
	}
}

func (scaler *ProxmoxScaler) NumReadyNodes() (int, error) {
	kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return 0, err
	}

	return len(kpNodes), err
}

func (scaler *ProxmoxScaler) AssessScaleDown() (*ScaleEvent, error) {
	totalAllocatedResources, err := scaler.GetAllocatedResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get allocated resources: %w", err)
	}

	workerNodesAllocatable, err := scaler.Kubernetes.GetWorkerNodesAllocatableResources()
	if err != nil {
		return nil, fmt.Errorf("failed to get worker nodes capacity: %w", err)
	}

	totalCpuAllocatable := workerNodesAllocatable.Cpu
	totalMemoryAllocatable := workerNodesAllocatable.Memory

	acceptCpuScaleDown := scaler.assessScaleDownForResourceType(totalAllocatedResources.Cpu, totalCpuAllocatable, int64(scaler.config.KpNodeCores))
	acceptMemoryScaleDown := scaler.assessScaleDownForResourceType(totalAllocatedResources.Memory, totalMemoryAllocatable, int64(scaler.config.KpNodeMemory<<20))

	if !(acceptCpuScaleDown && acceptMemoryScaleDown) {
		return nil, nil
	}

	scaleEvent := ScaleEvent{
		ScaleType: -1,
	}

	err = scaler.SelectScaleDownTarget(&scaleEvent)
	if err != nil {
		return nil, err
	}

	return &scaleEvent, nil
}

// func (scaler *ProxmoxScaler) assessScaleDownForResourceType(currentResourceAllocated float64, totalResourceAllocatable int64, kpNodeResourceCapacity int64) bool {
// 	if currentResourceAllocated == 0 {
// 		return false
// 	}

// 	// The proportion of the cluster's total allocatable resources currently allocated
// 	// represented as a float between 0 and 1
// 	totalResourceLoad := currentResourceAllocated / float64(totalResourceAllocatable)
// 	// The expected allocatable resources of the cluster after scaledown minus the
// 	// requested load headroom.
// 	acceptableResourceLoadForScaleDown := (float64(totalResourceAllocatable-int64(kpNodeResourceCapacity)) / float64(totalResourceAllocatable)) -
// 		(totalResourceLoad * scaler.config.LoadHeadroom)

// 	return totalResourceLoad < acceptableResourceLoadForScaleDown
// }

func (scaler *ProxmoxScaler) assessScaleDownForResourceType(currentResourceAllocated float64, totalResourceAllocatable int64, kpNodeResourceCapacity int64) bool {
	if currentResourceAllocated == 0 {
		return false
	}

	postScaledownCapacity := totalResourceAllocatable - kpNodeResourceCapacity
	fmt.Println(postScaledownCapacity)
	postScaleDownLoad := int64(math.Ceil(currentResourceAllocated) / float64(postScaledownCapacity) * 100)
	fmt.Println(postScaleDownLoad)
	postScaleDownHeadroom := 100 - postScaleDownLoad
	fmt.Println(postScaleDownHeadroom)

	return postScaleDownHeadroom > int64(scaler.config.LoadHeadroom*100)
}

func (scaler *ProxmoxScaler) SelectScaleDownTarget(scaleEvent *ScaleEvent) error {
	if scaleEvent.ScaleType != -1 {
		return fmt.Errorf("expected ScaleEvent ScaleType to be '-1' but got: %d", scaleEvent.ScaleType)
	}

	kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return err
	}

	if len(kpNodes) == 0 {
		return fmt.Errorf("no nodes to scale down, how did we get here?")
	}

	allocatedResources, err := scaler.Kubernetes.GetAllocatedResources(scaler.config.KpNodeNameRegex)
	if err != nil {
		return err
	}

	nodeLoads := make(map[string]float64)

	// Calculate the combined load on each kpNode
	for _, node := range kpNodes {
		nodeLoads[node.Name] =
			(allocatedResources[node.Name].Cpu / float64(scaler.config.KpNodeCores)) +
				(allocatedResources[node.Name].Memory / float64(scaler.config.KpNodeMemory))
	}

	targetNode := kpNodes[0].Name
	// Choose the kpnode with the lowest combined load
	for node := range nodeLoads {
		if nodeLoads[node] < nodeLoads[targetNode] {
			targetNode = node
		}
	}

	scaleEvent.NodeName = targetNode
	return nil
}

func (scaler *ProxmoxScaler) NumNodes() (int, error) {
	nodes, err := scaler.Proxmox.GetAllKpNodes(scaler.config.KpNodeNameRegex)
	return len(nodes), err
}

func (scaler *ProxmoxScaler) ScaleDown(ctx context.Context, scaleEvent *ScaleEvent) error {
	err := scaler.Kubernetes.DeleteKpNode(scaleEvent.NodeName)
	if err != nil {
		return err
	}

	return scaler.Proxmox.DeleteKpNode(scaleEvent.NodeName, scaler.config.KpNodeNameRegex)
}

// This function is only used when it is unclear whether a node has joined the kubernetes cluster
// ie when cleaning up after a failed scaling event
func (scaler *ProxmoxScaler) DeleteNode(kpNodeName string) error {
	_ = scaler.Kubernetes.DeleteKpNode(kpNodeName)

	return scaler.Proxmox.DeleteKpNode(kpNodeName, scaler.config.KpNodeNameRegex)
}

func (scaler *ProxmoxScaler) GetAllocatableResources() (AllocatableResources, error) {
	var allocatableResources AllocatableResources
	kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.config.KpNodeNameRegex)
	if err != nil {
		return allocatableResources, err
	}

	for _, kpNode := range kpNodes {
		allocatableResources.Cpu += kpNode.Status.Allocatable.Cpu().AsApproximateFloat64()
		allocatableResources.Memory += kpNode.Status.Allocatable.Memory().AsApproximateFloat64()
	}

	return allocatableResources, nil
}

func (scaler *ProxmoxScaler) GetAllocatedResources() (AllocatedResources, error) {
	var allocatedResources AllocatedResources
	resources, err := scaler.Kubernetes.GetAllocatedResources(scaler.config.KpNodeNameRegex)
	if err != nil {
		return allocatedResources, err
	}

	for _, kpNode := range resources {
		allocatedResources.Cpu += kpNode.Cpu
		allocatedResources.Memory += kpNode.Memory
	}

	return allocatedResources, nil
}

func (scaler *ProxmoxScaler) GetResourceStatistics() (ResourceStatistics, error) {
	allocatableResources, err := scaler.GetAllocatableResources()
	if err != nil {
		return ResourceStatistics{}, err
	}

	allocatedResources, err := scaler.GetAllocatedResources()
	if err != nil {
		return ResourceStatistics{}, err
	}

	return ResourceStatistics{
		Allocatable: allocatableResources,
		Allocated:   allocatedResources,
	}, nil
}
