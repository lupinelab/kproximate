package kubernetes

import (
	"fmt"
	"regexp"
	"testing"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func NewKubernetes(objects ...runtime.Object) *KubernetesClient {
	k := KubernetesClient{}
	k.client = testclient.NewSimpleClientset(objects...)

	return &k
}

func TestGetUnschedulableResourcesIgnoresUnsatisfiableCpu(t *testing.T) {
	satisfiablePodRequest, _ := resource.ParseQuantity("1")
	unsatisfiablePodRequest, _ := resource.ParseQuantity("3")

	k := NewKubernetes(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pickle",
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					{
						Resources: apiv1.ResourceRequirements{
							Requests: apiv1.ResourceList{
								apiv1.ResourceCPU: satisfiablePodRequest,
							},
						},
					},
				},
			},
			Status: apiv1.PodStatus{
				Conditions: []apiv1.PodCondition{
					{
						Type:    apiv1.PodScheduled,
						Status:  apiv1.ConditionFalse,
						Reason:  apiv1.PodReasonUnschedulable,
						Message: "Insufficient cpu",
					},
				},
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sausage",
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					{
						Resources: apiv1.ResourceRequirements{
							Requests: apiv1.ResourceList{
								apiv1.ResourceCPU: satisfiablePodRequest,
							},
						},
					},
				},
			},
			Status: apiv1.PodStatus{
				Conditions: []apiv1.PodCondition{
					{
						Type:    apiv1.PodScheduled,
						Status:  apiv1.ConditionFalse,
						Reason:  apiv1.PodReasonUnschedulable,
						Message: "Insufficient cpu",
					},
				},
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "mustard",
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					{
						Resources: apiv1.ResourceRequirements{
							Requests: apiv1.ResourceList{
								apiv1.ResourceCPU: unsatisfiablePodRequest,
							},
						},
					},
				},
			},
			Status: apiv1.PodStatus{
				Conditions: []apiv1.PodCondition{
					{
						Type:    apiv1.PodScheduled,
						Status:  apiv1.ConditionFalse,
						Reason:  apiv1.PodReasonUnschedulable,
						Message: "Insufficient cpu",
					},
				},
			},
		},
	)

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))
	kpNodeCores := 2
	unschedulableResources, err := k.GetUnschedulableResources(int64(kpNodeCores), kpNodeNameRegex)
	if err != nil {
		t.Error(err)
	}

	if unschedulableResources.Cpu != 2.0 {
		t.Errorf("Expected 2.0 cpu, got %f", unschedulableResources.Cpu)
	}
}

func TestGetUnschedulableResourcesIgnoresUnsatisfiableMemory(t *testing.T) {
	maxMemorySatisfiable, _ := resource.ParseQuantity("2048Mi")
	satisfiablePodRequest, _ := resource.ParseQuantity("1024Mi")
	unsatisfiablePodRequest, _ := resource.ParseQuantity("3072Mi")

	k := NewKubernetes(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
			Status: apiv1.NodeStatus{
				Allocatable: apiv1.ResourceList{
					apiv1.ResourceMemory: maxMemorySatisfiable,
				},
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pickle",
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					{
						Resources: apiv1.ResourceRequirements{
							Requests: apiv1.ResourceList{
								apiv1.ResourceMemory: satisfiablePodRequest,
							},
						},
					},
				},
			},
			Status: apiv1.PodStatus{
				Conditions: []apiv1.PodCondition{
					{
						Type:    apiv1.PodScheduled,
						Status:  apiv1.ConditionFalse,
						Reason:  apiv1.PodReasonUnschedulable,
						Message: "Insufficient memory",
					},
				},
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sausage",
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					{
						Resources: apiv1.ResourceRequirements{
							Requests: apiv1.ResourceList{
								apiv1.ResourceMemory: satisfiablePodRequest,
							},
						},
					},
				},
			},
			Status: apiv1.PodStatus{
				Conditions: []apiv1.PodCondition{
					{
						Type:    apiv1.PodScheduled,
						Status:  apiv1.ConditionFalse,
						Reason:  apiv1.PodReasonUnschedulable,
						Message: "Insufficient memory",
					},
				},
			},
		},
		&apiv1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "mustard",
			},
			Spec: apiv1.PodSpec{
				Containers: []apiv1.Container{
					{
						Resources: apiv1.ResourceRequirements{
							Requests: apiv1.ResourceList{
								apiv1.ResourceMemory: unsatisfiablePodRequest,
							},
						},
					},
				},
			},
			Status: apiv1.PodStatus{
				Conditions: []apiv1.PodCondition{
					{
						Type:    apiv1.PodScheduled,
						Status:  apiv1.ConditionFalse,
						Reason:  apiv1.PodReasonUnschedulable,
						Message: "Insufficient memory",
					},
				},
			},
		},
	)

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))
	unschedulableResources, err := k.GetUnschedulableResources(2, kpNodeNameRegex)
	if err != nil {
		t.Error(err)
	}

	if unschedulableResources.Memory != 2147483648 {
		t.Errorf("Expected 2147483648 memory, got %d", unschedulableResources.Memory)
	}
}

func TestGetKpNodesOnlyReturnsKpNodes(t *testing.T) {
	k := NewKubernetes(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a",
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pickle",
			},
		},
	)

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))

	nodes, err := k.GetKpNodes(kpNodeNameRegex)

	if err != nil {
		t.Error(err)
	}

	if len(nodes) != 2 {
		t.Errorf("Expected 2, got %d", len(nodes))
	}
}

func TestGetWorkerNodes(t *testing.T) {
	k := NewKubernetes(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "not-a-kp-worker-node",
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "k3s-master",
				Labels: map[string]string{
					"node-role.kubernetes.io/master": "true",
				},
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "k3s-control-plane",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "true",
				},
			},
		},
	)

	workerNodes, err := k.GetWorkerNodes()
	if err != nil {
		t.Error(err)
	}

	for _, node := range workerNodes {
		for _, label := range []string{
			"node-role.kubernetes.io/master",
			"node-role.kubernetes.io/control-plane",
		} {
			if _, exists := node.Labels[label]; exists {
				t.Errorf("Did not expect to find node %s with %s label", node.Name, node.Labels[label])
			}
		}
	}

	if len(workerNodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(workerNodes))
	}
}

func TestGetKpNodes(t *testing.T) {
	k := NewKubernetes(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "not-a-kp-worker-node",
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "k3s-master",
				Labels: map[string]string{
					"node-role.kubernetes.io/master": "true",
				},
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "k3s-control-plane",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "true",
				},
			},
		},
	)

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))
	kpNodes, err := k.GetKpNodes(kpNodeNameRegex)
	if err != nil {
		t.Error(err)
	}

	for _, node := range kpNodes {
		if node.Name == "not-a-kp-worker-node" {
			t.Errorf("Did not expect %s to be in kp node list", node.Name)
		}
	}

	if len(kpNodes) != 1 {
		t.Errorf("Expected 1 node, got %d", len(kpNodes))
	}
}

func TestCordonKpNode(t *testing.T) {
	k := NewKubernetes(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a",
			},
		},
	)

	err := k.CordonKpNode("kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a")
	if err != nil {
		t.Error(err)
	}

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))
	nodes, err := k.GetKpNodes(kpNodeNameRegex)
	if err != nil {
		t.Error(err)
	}

	for _, node := range nodes {
		if node.Name == "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd" && node.Spec.Unschedulable {
			t.Errorf("Expected 'kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd' not to be cordoned.")
		}

		if node.Name == "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a" && !node.Spec.Unschedulable {
			t.Errorf("Expected 'kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a' to be cordoned.")
		}
	}
}

func TestDeleteKpNode(t *testing.T) {
	k := NewKubernetes(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a",
			},
		},
	)

	err := k.DeleteKpNode("kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a")
	if err != nil {
		t.Error(err)
	}

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))
	nodes, err := k.GetKpNodes(kpNodeNameRegex)
	if err != nil {
		t.Error(err)
	}

	for _, node := range nodes {
		if node.Name == "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a" {
			t.Errorf("Expected 'kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a' to be deleted.")
		}
	}
}
