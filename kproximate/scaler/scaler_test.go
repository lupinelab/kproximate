package scaler

import (
	"fmt"
	"testing"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/kubernetes"
	kproxmox "github.com/lupinelab/kproximate/proxmox"
	"k8s.io/apimachinery/pkg/util/uuid"
)

func TestRequiredScaleEventsFor1CPU(t *testing.T) {
	unschedulableResources := kubernetes.UnschedulableResources{Cpu: 1.0, Memory: 0}

	s := Scaler{
		Config: config.Config{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			KpNodeTemplateConfig: kproxmox.VMConfig{
				Cores:  2,
				Memory: 2048,
			},
			MaxKpNodes: 3,
		},
	}

	queuedEvents := 0

	requiredScaleEvents := s.requiredScaleEvents(&unschedulableResources, &queuedEvents)

	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor3CPU(t *testing.T) {
	unschedulableResources := kubernetes.UnschedulableResources{Cpu: 3.0, Memory: 0}

	s := Scaler{
		Config: config.Config{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			KpNodeTemplateConfig: kproxmox.VMConfig{
				Cores:  2,
				Memory: 2048,
			},
			MaxKpNodes: 3,
		},
	}

	queuedEvents := 0

	requiredScaleEvents := s.requiredScaleEvents(&unschedulableResources, &queuedEvents)

	if len(requiredScaleEvents) != 2 {
		t.Errorf("Expected exactly 2 scaleEvents, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor1024MBMemory(t *testing.T) {
	unschedulableResources := kubernetes.UnschedulableResources{Cpu: 0, Memory: 1073741824}

	s := Scaler{
		Config: config.Config{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			KpNodeTemplateConfig: kproxmox.VMConfig{
				Cores:  2,
				Memory: 2048,
			},
			MaxKpNodes: 3,
		},
	}

	queuedEvents := 0

	requiredScaleEvents := s.requiredScaleEvents(&unschedulableResources, &queuedEvents)

	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor3072MBMemory(t *testing.T) {
	unschedulableResources := kubernetes.UnschedulableResources{Cpu: 0, Memory: 3221225472}

	s := Scaler{
		Config: config.Config{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			KpNodeTemplateConfig: kproxmox.VMConfig{
				Cores:  2,
				Memory: 2048,
			},
			MaxKpNodes: 3,
		},
	}

	queuedEvents := 0

	requiredScaleEvents := s.requiredScaleEvents(&unschedulableResources, &queuedEvents)

	if len(requiredScaleEvents) != 2 {
		t.Errorf("Expected exactly 2 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor1CPU3072MBMemory(t *testing.T) {
	unschedulableResources := kubernetes.UnschedulableResources{Cpu: 1, Memory: 3221225472}

	s := Scaler{
		Config: config.Config{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			KpNodeTemplateConfig: kproxmox.VMConfig{
				Cores:  2,
				Memory: 2048,
			},
			MaxKpNodes: 3,
		},
	}

	queuedEvents := 0

	requiredScaleEvents := s.requiredScaleEvents(&unschedulableResources, &queuedEvents)

	if len(requiredScaleEvents) != 2 {
		t.Errorf("Expected exactly 2 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestRequiredScaleEventsFor1CPU3072MBMemory1QueuedEvent(t *testing.T) {
	unschedulableResources := kubernetes.UnschedulableResources{Cpu: 1, Memory: 3221225472}

	s := Scaler{
		Config: config.Config{
			KpNodeCores:  2,
			KpNodeMemory: 2048,
			KpNodeTemplateConfig: kproxmox.VMConfig{
				Cores:  2,
				Memory: 2048,
			},
			MaxKpNodes: 3,
		},
	}

	queuedEvents := 1

	requiredScaleEvents := s.requiredScaleEvents(&unschedulableResources, &queuedEvents)

	if len(requiredScaleEvents) != 1 {
		t.Errorf("Expected exactly 1 scaleEvent, got: %d", len(requiredScaleEvents))
	}
}

func TestSelectTargetPHostsWithExistingScalingEvent(t *testing.T) {
	s := Scaler{
		PCluster: &kproxmox.ProxmoxMockClient{},
	}
	
	newName1 := fmt.Sprintf("kp-node-%s", uuid.NewUUID())
	newName2 := fmt.Sprintf("kp-node-%s", uuid.NewUUID())
	newName3 := fmt.Sprintf("kp-node-%s", uuid.NewUUID())

	scaleEvents := []*ScaleEvent{
		{
			ScaleType:  1,
			KpNodeName: newName1,
		},
		{
			ScaleType:  1,
			KpNodeName: newName2,
		},
		{
			ScaleType:  1,
			KpNodeName: newName3,
		},
	}

	s.selectTargetPHosts(scaleEvents)

	if scaleEvents[0].TargetPHost.Id != "node/host-01" {
		t.Errorf("Expected node/host-01 to be selected as target pHost, got: %s", scaleEvents[0].TargetPHost.Id)
	}

	if scaleEvents[1].TargetPHost.Id != "node/host-02" {
		t.Errorf("Expected node/host-02 to be selected as target pHost, got: %s", scaleEvents[1].TargetPHost.Id)
	}

	if scaleEvents[2].TargetPHost.Id != "node/host-03" {
		t.Errorf("Expected node/host-03 to be selected as target pHost, got: %s", scaleEvents[2].TargetPHost.Id)
	}
}

