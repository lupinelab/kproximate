package scaler

import (
	"context"

	"github.com/lupinelab/kproximate/kubernetes"
)

type Mock struct {
}

func (m Mock) RequiredScaleEvents(requiredResources *kubernetes.UnschedulableResources, numCurrentEvents int) ([]*ScaleEvent, error) {
	return nil, nil
}

func (m Mock) SelectTargetHosts(scaleEvents []*ScaleEvent) error {
	return nil
}

func (m Mock) ScaleUp(ctx context.Context, scaleEvent *ScaleEvent) error {
	return nil
}

func (m Mock) NumReadyNodes() (int, error) {
	return 0, nil
}

func (m Mock) AssessScaleDown(allocatedResources map[string]*kubernetes.AllocatedResources, workerNodesAllocatable kubernetes.WorkerNodesAllocatableResources) *ScaleEvent {
	return nil
}

func (m Mock) ScaleDown(ctx context.Context, scaleEvent *ScaleEvent) error {
	return nil
}

func (m Mock) DeleteNode(kpNodeName string) error {
	return nil
}
