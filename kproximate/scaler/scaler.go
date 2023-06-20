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

	"github.com/lupinelab/kproximate/kubernetes"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/leaderelection"
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
	config     Config
	kCluster   kubernetes.Kubernetes
	pCluster   kproxmox.Proxmox
	scaleState int32
}

type scaleEvent struct {
	scaleType   int
	kpNodeName  string
	targetPHost kproxmox.PHostInformation
}

func NewScaler(config Config) *KProximateScaler {
	kClient := kubernetes.NewKubernetesClient()
	pClient := kproxmox.NewProxmoxClient(config.PmUrl, config.AllowInsecure, config.PmUserID, config.PmToken, config.PmDebug)

	kpNodeTemplateRef, err := pClient.Client.GetVmRefByName(config.KpNodeTemplateName)
	if err != nil {
		panic(err.Error())
	}

	config.kpNodeTemplateRef = *kpNodeTemplateRef

	config.kpNodeTemplateConfig, err = pClient.GetKpTemplateConfig(kpNodeTemplateRef)
	if err != nil {
		panic(err.Error())
	}

	config.kpNodeParams = map[string]interface{}{
		"agent":     "enabled=1",
		"balloon":   0,
		"cores":     config.KpNodeCores,
		"ipconfig0": "ip=dhcp",
		"memory":    config.KpNodeMemory,
		"onboot":    1,
		"sshkeys":   strings.Replace(url.QueryEscape(config.SshKey), "+", "%20", 1),
	}

	scaler := &KProximateScaler{
		config:     config,
		kCluster:   *kClient,
		pCluster:   *pClient,
		scaleState: 0,
	}

	return scaler
}

func (scaler *KProximateScaler) Start() {
	podName := os.Getenv("HOSTNAME")

	nameSpace := os.Getenv("NAMESPACE")

	lock := scaler.kCluster.GetNewLock(podName, nameSpace)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   10 * time.Second,
		RenewDeadline:   5 * time.Second,
		RetryPeriod:     2 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				infoLog.Printf("%v elected leader", podName)
				scaler.scaler()
			},
			OnStoppedLeading: func() {
				infoLog.Printf("%v stopped leading", podName)
			},
		},
	})
}

func (scaler *KProximateScaler) scaler() {
	for {
		unschedulableResources, err := scaler.kCluster.GetUnschedulableResources()
		if err != nil {
			errorLog.Fatalf("Unable to get unschedulable resources: %s", err.Error())
		}

		requiredScaleEvents := scaler.requiredScaleEvents(unschedulableResources)

		if scaler.numKpNodes() < scaler.config.MaxKpNodes {
			scaler.selectTargetPHosts(requiredScaleEvents)

			for _, scaleEvent := range requiredScaleEvents {

				ctx := context.Background()

				go scaler.scaleUp(ctx, scaleEvent)

				// Kick off the scaling events 1 second apart to allow proxmox some time to start the first
				// scaleEvent, otherwise it can attempt to duplicate the VMID
				time.Sleep(time.Second)
			}

		} else if len(requiredScaleEvents) > 0 {
			warningLog.Printf("Scale requested but already at max kp-nodes: %v", scaler.config.MaxKpNodes)
		}

		go scaler.cleanUpEmptyNodes()

		if atomic.LoadInt32(&scaler.scaleState) == 0 {
			go scaler.cleanUpStoppedNodes()
		}

		if atomic.LoadInt32(&scaler.scaleState) == 0 {
			scaleEvent := scaler.considerScaleDown()

			if scaleEvent.scaleType != 0 {
				ctx := context.Background()

				go scaler.scaleDown(ctx, scaleEvent)
			}
		}

		time.Sleep(5 * time.Second)
	}
}

func (scaler *KProximateScaler) requiredScaleEvents(requiredResources *kubernetes.UnschedulableResources) []*scaleEvent {
	requiredScaleEvents := []*scaleEvent{}
	var numCpuNodesRequired int
	var numMemoryNodesRequired int

	if requiredResources.Cpu != 0 {
		expectedCpu := float64(scaler.config.KpNodeCores) * float64(atomic.LoadInt32(&scaler.scaleState))
		unaccountedCpu := requiredResources.Cpu - expectedCpu
		numCpuNodesRequired = int(math.Ceil(unaccountedCpu / float64(scaler.config.kpNodeTemplateConfig.Cores)))
	}

	if requiredResources.Memory != 0 {
		expectedMemory := int64(scaler.config.KpNodeMemory<<20) * int64(atomic.LoadInt32(&scaler.scaleState))
		unaccountedMemory := requiredResources.Memory - int64(expectedMemory)
		numMemoryNodesRequired = int(math.Ceil(float64(unaccountedMemory) / float64(scaler.config.kpNodeTemplateConfig.Memory<<20)))
	}

	numNodesRequired := int(math.Max(float64(numCpuNodesRequired), float64(numMemoryNodesRequired)))

	for kpNode := 1; kpNode <= numNodesRequired; kpNode++ {
		newName := fmt.Sprintf("kp-node-%s", uuid.NewUUID())

		requiredEvent := &scaleEvent{
			scaleType:  1,
			kpNodeName: newName,
		}

		requiredScaleEvents = append(requiredScaleEvents, requiredEvent)
	}

	if len(requiredScaleEvents) == 0 && atomic.LoadInt32(&scaler.scaleState) == 0 {
		schedulingFailed, err := scaler.kCluster.GetFailedSchedulingDueToControlPlaneTaint()
		if err != nil {
			warningLog.Printf("Could not get pods: %s", err.Error())
		}

		if schedulingFailed == true {
			newName := fmt.Sprintf("kp-node-%s", uuid.NewUUID())
			requiredEvent := &scaleEvent{
				scaleType:  1,
				kpNodeName: newName,
			}

			requiredScaleEvents = append(requiredScaleEvents, requiredEvent)
		}
	}

	return requiredScaleEvents
}

func (scaler *KProximateScaler) selectTargetPHosts(scaleEvents []*scaleEvent) {
	pHosts := scaler.pCluster.GetClusterStats()
	kpNodes := scaler.pCluster.GetRunningKpNodes()

selected:
	for _, scaleEvent := range scaleEvents {
	skipHost:
		for _, pHost := range pHosts {
			// Check for a scaleEvent targeting the pHost
			for _, scaleEvent := range scaleEvents {
				if scaleEvent.targetPHost.Id == pHost.Id {
					continue skipHost
				}
			}

			for _, kpNode := range kpNodes {
				// Check for an existing kpNode on the pHost
				if strings.Contains(pHost.Id, kpNode.Node) {
					continue skipHost
				}
			}

			scaleEvent.targetPHost = pHost
			continue selected
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

	return
}

func (scaler *KProximateScaler) scaleUp(ctx context.Context, scaleEvent *scaleEvent) {
	atomic.AddInt32(&scaler.scaleState, int32(scaleEvent.scaleType))
	infoLog.Printf("Scale state: %+d", atomic.LoadInt32(&scaler.scaleState))

	infoLog.Printf("Provisioning %s on pcluster", scaleEvent.kpNodeName)

	ok := make(chan bool)

	errchan := make(chan error)

	pctx, cancelCtx := context.WithTimeout(ctx, time.Duration(20*time.Second))
	defer cancelCtx()

	go scaler.pCluster.NewKpNode(
		pctx,
		ok,
		errchan,
		scaler.config.kpNodeTemplateRef,
		scaleEvent.kpNodeName,
		scaleEvent.targetPHost.Node,
		scaler.config.kpNodeParams,
	)

ptimeout:
	select {
	case <-pctx.Done():
		cancelCtx()

		errorLog.Printf("Timed out waiting for %s to start", scaleEvent.kpNodeName)

		atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
		infoLog.Printf("Scale state: %+d", atomic.LoadInt32(&scaler.scaleState))

		err := scaler.deleteKpNode(scaleEvent.kpNodeName)
		if err != nil {
			errorLog.Printf("Cleanup failed for %s: %s", scaleEvent.kpNodeName, err.Error())
		}

		return

	case err := <-errchan:
		errorLog.Printf("Could not provision %s: %s", scaleEvent.kpNodeName, err.Error())

		atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
		infoLog.Printf("Scale state: %+d", atomic.LoadInt32(&scaler.scaleState))

		return

	case <-ok:
		infoLog.Printf("Started %s", scaleEvent.kpNodeName)
		break ptimeout
	}

	infoLog.Printf("Waiting for %s to join kcluster", scaleEvent.kpNodeName)

	kctx, cancelCtx := context.WithTimeout(ctx, time.Duration(60*time.Second))
	// Add wait for join variable
	defer cancelCtx()

	go scaler.kCluster.WaitForJoin(
		kctx,
		ok,
		scaleEvent.kpNodeName,
	)

ktimeout:
	select {
	case <-kctx.Done():
		cancelCtx()
		errorLog.Printf("Timed out waiting for %s to join kcluster", scaleEvent.kpNodeName)

		atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
		infoLog.Printf("Scale state: %+d", atomic.LoadInt32(&scaler.scaleState))

		infoLog.Printf("Cleaning up failed scale attempt: %s", scaleEvent.kpNodeName)

		err := scaler.deleteKpNode(scaleEvent.kpNodeName)
		if err != nil {
			errorLog.Printf("Cleanup failed for %s: %s", scaleEvent.kpNodeName, err.Error())
		}

		infoLog.Printf("Deleted %s", scaleEvent.kpNodeName)

		return

	case <-ok:
		break ktimeout
	}

	infoLog.Printf("%s joined kcluster", scaleEvent.kpNodeName)

	atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
	infoLog.Printf("Scale state: %+d", atomic.LoadInt32(&scaler.scaleState))
}

func (scaler *KProximateScaler) scaleDown(ctx context.Context, scaleEvent *scaleEvent) {
	atomic.AddInt32(&scaler.scaleState, int32(scaleEvent.scaleType))
	infoLog.Printf("Scale state: %+d", atomic.LoadInt32(&scaler.scaleState))

	err := scaler.kCluster.SlowDeleteKpNode(scaleEvent.kpNodeName)
	if err != nil {
		warningLog.Printf("Could not delete kNode %v, scale down failed: %s", scaleEvent.kpNodeName, err.Error())
	}

	err = scaler.pCluster.DeleteKpNode(scaleEvent.kpNodeName)
	if err != nil {
		warningLog.Printf("Could not delete pNode %v, scale down failed: %s", scaleEvent.kpNodeName, err.Error())
	}

	infoLog.Printf("Deleted %s", scaleEvent.kpNodeName)

	atomic.AddInt32(&scaler.scaleState, -int32(scaleEvent.scaleType))
	infoLog.Printf("Scale state: %+d", atomic.LoadInt32(&scaler.scaleState))
}

func (scaler *KProximateScaler) numKpNodes() int {
	kpNodes := scaler.pCluster.GetRunningKpNodes()

	return len(kpNodes) + int(atomic.LoadInt32(&scaler.scaleState))
}

func (scaler *KProximateScaler) deleteKpNode(kpNodeName string) error {
	_ = scaler.kCluster.DeleteKpNode(kpNodeName)

	err := scaler.pCluster.DeleteKpNode(kpNodeName)

	return err
}

func (scaler *KProximateScaler) cleanUpEmptyNodes() {
	emptyKpNodes, err := scaler.kCluster.GetEmptyKpNodes()
	if err != nil {
		errorLog.Printf("Could not get emtpy nodes: %s", err.Error())
	}

	for _, emptyNode := range emptyKpNodes {
		emptyPNode := scaler.pCluster.GetKpNode(emptyNode.Name)

		// Allow new nodes a grace period of emptiness after creation before they are targets for cleanup
		if emptyPNode.Uptime < scaler.config.EmptyGraceSecondsAfterCreation {
			continue
		}

		err := scaler.deleteKpNode(emptyNode.Name)
		if err != nil {
			warningLog.Printf("Failed to delete empty node %s: %s", emptyNode.Name, err.Error())
		}

		infoLog.Printf("Deleted empty node: %s", emptyNode.Name)

	}
}

func (scaler *KProximateScaler) cleanUpStoppedNodes() {
	kpNodes := scaler.pCluster.GetAllKpNodes()
	var stoppedNodes []kproxmox.VmInformation
	for _, kpNode := range kpNodes {
		if kpNode.Status == "stopped" {
			stoppedNodes = append(stoppedNodes, kpNode)
		}
	}

	for _, stoppedNode := range stoppedNodes {
		err := scaler.pCluster.DeleteKpNode(stoppedNode.Name)
		if err != nil {
			warningLog.Printf("Failed to delete stopped node %s: %s", stoppedNode.Name, err.Error())
			continue
		}

		infoLog.Printf("Deleted stopped node %s", stoppedNode.Name)
	}
}

func (scaler *KProximateScaler) considerScaleDown() *scaleEvent {
	allocatedResources, err := scaler.kCluster.GetKpNodesAllocatedResources()
	if err != nil {
		warningLog.Printf("Consider scale down failed, unable to get allocated resources: %s", err.Error())
		return &scaleEvent{}
	}

	numKpNodes := scaler.numKpNodes()

	totalCpuAllocatable := scaler.config.KpNodeCores * numKpNodes
	totalMemoryAllocatable := scaler.config.KpNodeMemory << 20 * numKpNodes

	var totalCpuAllocated float64
	for _, kpNode := range allocatedResources {
		totalCpuAllocated += kpNode.Cpu
	}

	var totalMemoryAllocated float64
	for _, kpNode := range allocatedResources {
		totalMemoryAllocated += kpNode.Memory
	}

	kpNodeHeadroom := scaler.config.KpNodeHeadroom
	if kpNodeHeadroom < 0.2 {
		kpNodeHeadroom = 0.2
	}
	numKpNodesAfterScaleDown := numKpNodes - 1
	acceptableLoadForScaleDown :=
		(float64(numKpNodesAfterScaleDown) / float64(numKpNodes)) -
			kpNodeHeadroom

	scaleEvent := scaleEvent{}

	if totalCpuAllocated != 0 {
		if totalCpuAllocated/float64(totalCpuAllocatable) <= acceptableLoadForScaleDown {
			scaleEvent.scaleType = -1
		}
	}

	if totalMemoryAllocated != 0 {
		if totalMemoryAllocated/float64(totalMemoryAllocatable) <= acceptableLoadForScaleDown {
			scaleEvent.scaleType = -1
		}
	}

	scaler.SelectScaleDownTarget(&scaleEvent, allocatedResources)

	return &scaleEvent
}

func (scaler *KProximateScaler) SelectScaleDownTarget(scaleEvent *scaleEvent, allocatedResources map[string]*kubernetes.AllocatedResources) {
	kpNodes, err := scaler.kCluster.GetKpNodes()
	if err != nil {
		warningLog.Printf("Consider scale down failed, unable get kp-nodes: %s", err.Error())
	}

	if scaleEvent.scaleType != 0 {
		var targetNode string
		kpNodeLoads := make(map[string]float64)

		// Calculate the combined load on each kpNode
		for _, kpNode := range kpNodes {
			kpNodeLoads[kpNode.Name] =
				(allocatedResources[kpNode.Name].Cpu / float64(scaler.config.KpNodeCores)) +
					(allocatedResources[kpNode.Name].Memory / float64(scaler.config.KpNodeMemory))
		}

		// Choose the kpnode with the lowest combined load
		i := 0
		for kpNode := range kpNodeLoads {
			if i == 0 || kpNodeLoads[kpNode] < kpNodeLoads[targetNode] {
				targetNode = kpNode
				i++
			}
		}

		scaleEvent.kpNodeName = targetNode
	}
}