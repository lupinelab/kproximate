package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	apiv1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Kubernetes interface {
	GetUnschedulableResources() (*UnschedulableResources, error)
	IsFailedSchedulingDueToControlPlaneTaint() (bool, error)
	GetWorkerNodes() (*apiv1.NodeList, error)
	GetWorkerNodesAllocatableResources() (WorkerNodesAllocatableResources, error)
	GetKpNodes(kpNodeName regexp.Regexp) ([]apiv1.Node, error)
	GetAllocatedResources(kpNodeName regexp.Regexp) (map[string]*AllocatedResources, error)
	CheckForNodeJoin(ctx context.Context, ok chan<- bool, newKpNodeName string)
	DeleteKpNode(kpNodeName string) error
	CordonKpNode(KpNodeName string) error
}

type KubernetesClient struct {
	client *kubernetes.Clientset
}

type UnschedulableResources struct {
	Cpu    float64
	Memory int64
}

type WorkerNodesAllocatableResources struct {
	Cpu    int64
	Memory int64
}

type AllocatedResources struct {
	Cpu    float64
	Memory float64
}

func NewKubernetesClient() (KubernetesClient, error) {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		flag.Parse()
	}

	var config *rest.Config

	if _, err := os.Stat(*kubeconfig); err == nil {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return KubernetesClient{}, err
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			panic(err.Error())
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	kubernetes := KubernetesClient{
		client: clientset,
	}

	return kubernetes, nil
}

func (k *KubernetesClient) GetUnschedulableResources() (*UnschedulableResources, error) {
	var rCpu float64
	var rMemory float64

	pods, err := k.client.CoreV1().Pods("").List(
		context.TODO(),
		metav1.ListOptions{},
	)
	if err != nil {
		return nil, err
	}

	for _, pod := range pods.Items {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == apiv1.PodScheduled && condition.Status == apiv1.ConditionFalse && condition.Reason == "Unschedulable" {
				if strings.Contains(condition.Message, "Insufficient cpu") {
					for _, container := range pod.Spec.Containers {
						rCpu += container.Resources.Requests.Cpu().AsApproximateFloat64()
					}
				}
				if strings.Contains(condition.Message, "Insufficient memory") {
					for _, container := range pod.Spec.Containers {
						rMemory += container.Resources.Requests.Memory().AsApproximateFloat64()
					}
				}
			}
		}
	}

	unschedulableResources := &UnschedulableResources{
		Cpu:    rCpu,
		Memory: int64(rMemory),
	}

	return unschedulableResources, err
}

func (k *KubernetesClient) IsFailedSchedulingDueToControlPlaneTaint() (bool, error) {
	pods, err := k.client.CoreV1().Pods("").List(
		context.TODO(),
		metav1.ListOptions{},
	)
	if err != nil {
		return false, err
	}

	for _, pod := range pods.Items {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == apiv1.PodScheduled && condition.Status == apiv1.ConditionFalse && condition.Reason == "Unschedulable" {
				if strings.Contains(condition.Message, "untolerated taint {node-role.kubernetes.io/control-plane:") {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// Worker nodes should comprise of all kpNodes and any additional worker nodes
// in the cluster that are not managed by kproximate
func (k *KubernetesClient) GetWorkerNodes() (*apiv1.NodeList, error) {

	noControlPlaneLabel, err := labels.NewRequirement(
		"node-role.kubernetes.io/control-plane",
		selection.DoesNotExist,
		[]string{},
	)
	if err != nil {
		return nil, err
	}

	noMasterLabel, err := labels.NewRequirement(
		"node-role.kubernetes.io/master",
		selection.DoesNotExist,
		[]string{},
	)
	if err != nil {
		return nil, err
	}

	labelSelector := labels.NewSelector()
	labelSelector = labelSelector.Add(
		*noControlPlaneLabel,
		*noMasterLabel,
	)

	nodes, err := k.client.CoreV1().Nodes().List(
		context.TODO(),
		metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		},
	)
	if err != nil {
		return nil, err
	}

	return nodes, err
}

func (k *KubernetesClient) GetWorkerNodesAllocatableResources() (WorkerNodesAllocatableResources, error) {
	var workerNodesAllocatableResources WorkerNodesAllocatableResources
	workerNodes, err := k.GetWorkerNodes()
	if err != nil {
		return workerNodesAllocatableResources, err
	}

	for _, workerNode := range workerNodes.Items {
		workerNodesAllocatableResources.Cpu += int64(workerNode.Status.Allocatable.Cpu().AsApproximateFloat64())
		workerNodesAllocatableResources.Memory += int64(workerNode.Status.Allocatable.Memory().AsApproximateFloat64())
	}

	return workerNodesAllocatableResources, err
}

func (k *KubernetesClient) GetKpNodes(kpNodeName regexp.Regexp) ([]apiv1.Node, error) {
	nodes, err := k.GetWorkerNodes()
	if err != nil {
		return nil, err
	}

	var kpNodes []apiv1.Node

	for _, kpNode := range nodes.Items {
		if kpNodeName.MatchString(kpNode.Name) {
			kpNodes = append(kpNodes, kpNode)
		}
	}

	return kpNodes, err
}

func (k *KubernetesClient) GetAllocatedResources(kpNodeName regexp.Regexp) (map[string]*AllocatedResources, error) {
	kpNodes, err := k.GetKpNodes(kpNodeName)
	if err != nil {
		return nil, err
	}

	allocatedResources := map[string]*AllocatedResources{}

	for _, kpNode := range kpNodes {
		allocatedResources[kpNode.Name] = &AllocatedResources{
			Cpu:    0,
			Memory: 0,
		}

		pods, err := k.client.CoreV1().Pods("").List(
			context.TODO(),
			metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", kpNode.Name),
			},
		)
		if err != nil {
			return nil, err
		}

		for _, pod := range pods.Items {
			for _, container := range pod.Spec.Containers {
				allocatedResources[kpNode.Name].Cpu += container.Resources.Requests.Cpu().AsApproximateFloat64()
				allocatedResources[kpNode.Name].Memory += container.Resources.Requests.Memory().AsApproximateFloat64()
			}
		}
	}

	return allocatedResources, err
}

func (k *KubernetesClient) CheckForNodeJoin(ctx context.Context, ok chan<- bool, newKpNodeName string) {
	for {
		newkpNode, _ := k.client.CoreV1().Nodes().Get(
			context.TODO(),
			newKpNodeName,
			metav1.GetOptions{},
		)

		for _, condition := range newkpNode.Status.Conditions {
			if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
				ok <- true
				return
			}
		}
	}
}

func (k *KubernetesClient) DeleteKpNode(kpNodeName string) error {
	err := k.CordonKpNode(kpNodeName)
	if err != nil {
		return err
	}

	pods, err := k.client.CoreV1().Pods("").List(
		context.TODO(),
		metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", kpNodeName),
		},
	)
	if err != nil {
		return err
	}

	for _, pod := range pods.Items {
		k.client.PolicyV1().Evictions(pod.Namespace).Evict(
			context.TODO(),
			&policy.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pod.Name,
					Namespace: pod.Namespace,
				},
			},
		)
	}

	err = k.client.CoreV1().Nodes().Delete(
		context.TODO(),
		kpNodeName,
		metav1.DeleteOptions{},
	)
	if err != nil {
		return err
	}

	return err
}

func (k *KubernetesClient) CordonKpNode(kpNodeName string) error {
	kpNode, err := k.client.CoreV1().Nodes().Get(
		context.TODO(),
		kpNodeName,
		metav1.GetOptions{},
	)
	if err != nil {
		return err
	}

	kpNode.Spec.Unschedulable = true

	_, err = k.client.CoreV1().Nodes().Update(
		context.TODO(),
		kpNode, metav1.UpdateOptions{},
	)

	return err
}
