package kubernetes

import (
	"context"
	"regexp"

	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Mock struct {
	CordonedNodes                          []string
	DeletedNodes                           []string
	AllocatedResources                     map[string]AllocatedResources
	UnschedulableResources                 UnschedulableResources
	WorkerNodesAllocatableResources        WorkerNodesAllocatableResources
	FailedSchedulingDueToControlPlaneTaint bool
	KpNodes                                []apiv1.Node
}

func (m *Mock) GetUnschedulableResources(kpNodeCores int64, kpNodeNameRegex regexp.Regexp) (UnschedulableResources, error) {
	return m.UnschedulableResources, nil
}

func (m *Mock) IsUnschedulableDueToControlPlaneTaint() (bool, error) {
	return m.FailedSchedulingDueToControlPlaneTaint, nil
}

func (m *Mock) GetWorkerNodes() ([]apiv1.Node, error) {
	return nil, nil
}

func (m *Mock) GetWorkerNodesAllocatableResources() (WorkerNodesAllocatableResources, error) {
	return m.WorkerNodesAllocatableResources, nil
}

func (m *Mock) GetKpNodes(kpNodeNameRegex regexp.Regexp) ([]apiv1.Node, error) {
	if m.KpNodes != nil {
		return m.KpNodes, nil
	}

	nodes := make([]apiv1.Node, len(m.AllocatedResources))

	idx := 0
	for k := range m.AllocatedResources {
		nodes[idx] = apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: k,
			},
		}

		idx++
	}

	return nodes, nil
}

func (m *Mock) GetAllocatedKpResources(kpNodeNameRegex regexp.Regexp) (map[string]AllocatedResources, error) {
	return m.AllocatedResources, nil
}

func (m *Mock) CheckForNodeJoin(ctx context.Context, ok chan<- bool, newKpNodeName string) {
}

func (m *Mock) DeleteKpNode(kpNodeName string) error {
	m.DeletedNodes = append(m.DeletedNodes, kpNodeName)
	return nil
}

func (m *Mock) CordonKpNode(KpNodeName string) error {
	m.CordonedNodes = append(m.CordonedNodes, KpNodeName)
	return nil
}
