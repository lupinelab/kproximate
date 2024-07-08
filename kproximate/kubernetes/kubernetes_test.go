package kubernetes

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	testclient "k8s.io/client-go/kubernetes/fake"
)

func NewKubernetesMock(objects ...runtime.Object) *KubernetesClient {
	return &KubernetesClient{
		client: testclient.NewSimpleClientset(objects...),
	}
}

func TestGetUnschedulableResourcesIgnoresUnsatisfiableCpu(t *testing.T) {
	satisfiablePodRequest, _ := resource.ParseQuantity("1")
	unsatisfiablePodRequest, _ := resource.ParseQuantity("3")

	k := NewKubernetesMock(
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

	k := NewKubernetesMock(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
				},
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
	k := NewKubernetesMock(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
				},
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a",
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
				},
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pickle",
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
				},
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
	k := NewKubernetesMock(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
				},
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "not-a-kp-worker-node",
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
				},
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "k3s-master",
				Labels: map[string]string{
					"node-role.kubernetes.io/master": "true",
				},
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
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
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
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
	k := NewKubernetesMock(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
				},
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "not-a-kp-worker-node",
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
				},
			},
		},
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "k3s-master",
				Labels: map[string]string{
					"node-role.kubernetes.io/master": "true",
				},
			},
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
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
			Status: apiv1.NodeStatus{
				Conditions: []apiv1.NodeCondition{
					{
						Type:   apiv1.NodeReady,
						Status: "True",
					},
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
	k := NewKubernetesMock(
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

	err := k.cordonKpNode(context.TODO(), "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a")
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
	k := NewKubernetesMock(
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

	err := k.DeleteKpNode(context.TODO(), "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a")
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

func TestLabelNode(t *testing.T) {
	kpNodeName := "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd"
	k := NewKubernetesMock(
		&apiv1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: kpNodeName,
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "true",
				},
			},
		},
	)

	newKpNodeLabels := map[string]string{
		"topology.kubernetes.io/region": "tc",
		"topology.kubernetes.io/zone2":  "tc-01",
	}

	err := k.LabelKpNode(kpNodeName, newKpNodeLabels)
	if err != nil {
		t.Error(err)
	}

	kpNode, err := k.client.CoreV1().Nodes().Get(
		context.TODO(),
		kpNodeName,
		metav1.GetOptions{},
	)
	if err != nil {
		t.Error(err)
	}

	for key, value := range newKpNodeLabels {
		labelvalue, ok := kpNode.Labels[key]
		if ok {
			if labelvalue != value {
				t.Errorf("Expected %s label: %s:%s", kpNodeName, key, value)
			}
		}
	}

	value, ok := kpNode.Labels["node-role.kubernetes.io/control-plane"]
	if !ok {
		t.Errorf("Expected %s label node-role.kubernetes.io/control-plane to exist", kpNodeName)
		return
	}
	if value != "true" {
		t.Errorf("Expected %s label: %s:%s", kpNodeName, "node-role.kubernetes.io/control-plane", "true")
	}
}
