package kubernetes

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lupinelab/kproximate/logger"
	apiv1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
)

type Kubernetes interface {
	GetUnschedulableResources(kpNodeCores int64, kpNodeNameRegex regexp.Regexp) (UnschedulableResources, error)
	IsUnschedulableDueToControlPlaneTaint() (bool, error)
	GetWorkerNodes() ([]apiv1.Node, error)
	GetWorkerNodesAllocatableResources() (WorkerNodesAllocatableResources, error)
	GetKpNodes(kpNodeNameRegex regexp.Regexp) ([]apiv1.Node, error)
	LabelKpNode(kpNodeName string, kpNodeLabels map[string]string) error
	GetKpNodesAllocatedResources(kpNodeNameRegex regexp.Regexp) (map[string]AllocatedResources, error)
	CheckForNodeJoin(ctx context.Context, ok chan<- bool, newKpNodeName string)
	DeleteKpNode(ctx context.Context, kpNodeName string) error
}

type KubernetesClient struct {
	client kubernetes.Interface
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

func isUnschedulable(condition apiv1.PodCondition) bool {
	return condition.Type == apiv1.PodScheduled && condition.Status == apiv1.ConditionFalse && condition.Reason == apiv1.PodReasonUnschedulable
}

func (k *KubernetesClient) GetUnschedulableResources(kpNodeCores int64, kpNodeNameRegex regexp.Regexp) (UnschedulableResources, error) {
	var rCpu float64
	var rMemory float64

	pods, err := k.client.CoreV1().Pods("").List(
		context.TODO(),
		metav1.ListOptions{},
	)
	if err != nil {
		return UnschedulableResources{}, err
	}

	maxAllocatableMemoryForSinglePod, err := k.getMaxAllocatableMemoryForSinglePod(kpNodeNameRegex)
	if err != nil {
		return UnschedulableResources{}, err
	}

PODLOOP:
	for _, pod := range pods.Items {
		for _, condition := range pod.Status.Conditions {
			if isUnschedulable(condition) {
				if strings.Contains(condition.Message, "Insufficient cpu") {
					for _, container := range pod.Spec.Containers {
						if container.Resources.Requests.Cpu().CmpInt64(kpNodeCores) >= 0 {
							logger.WarnLog(fmt.Sprintf("Ignoring pod (%s) with unsatisfiable Cpu request: %f", pod.Name, container.Resources.Requests.Cpu().AsApproximateFloat64()))
							continue PODLOOP
						}

						rCpu += container.Resources.Requests.Cpu().AsApproximateFloat64()
					}
				}

				if strings.Contains(condition.Message, "Insufficient memory") {
					for _, container := range pod.Spec.Containers {
						if container.Resources.Requests.Memory().AsApproximateFloat64() >= maxAllocatableMemoryForSinglePod {
							logger.WarnLog(fmt.Sprintf("Ignoring pod (%s) with unsatisfiable Memory request: %f", pod.Name, container.Resources.Requests.Memory().AsApproximateFloat64()))
							continue PODLOOP
						}

						rMemory += container.Resources.Requests.Memory().AsApproximateFloat64()
					}
				}
			}
		}
	}

	unschedulableResources := UnschedulableResources{
		Cpu:    rCpu,
		Memory: int64(rMemory),
	}

	return unschedulableResources, err
}

func (k *KubernetesClient) IsUnschedulableDueToControlPlaneTaint() (bool, error) {
	pods, err := k.client.CoreV1().Pods("").List(
		context.TODO(),
		metav1.ListOptions{},
	)
	if err != nil {
		return false, err
	}

	for _, pod := range pods.Items {
		for _, condition := range pod.Status.Conditions {
			if isUnschedulable(condition) {
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
func (k *KubernetesClient) GetWorkerNodes() ([]apiv1.Node, error) {
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

	labelSelector := labels.NewSelector().Add(
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

	workerNodes := []apiv1.Node{}
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
				workerNodes = append(workerNodes, node)
			}
		}
	}

	return workerNodes, err
}

func (k *KubernetesClient) GetWorkerNodesAllocatableResources() (WorkerNodesAllocatableResources, error) {
	var workerNodesAllocatableResources WorkerNodesAllocatableResources
	workerNodes, err := k.GetWorkerNodes()
	if err != nil {
		return workerNodesAllocatableResources, err
	}

	for _, workerNode := range workerNodes {
		workerNodesAllocatableResources.Cpu += int64(workerNode.Status.Allocatable.Cpu().AsApproximateFloat64())
		workerNodesAllocatableResources.Memory += int64(workerNode.Status.Allocatable.Memory().AsApproximateFloat64())
	}

	return workerNodesAllocatableResources, err
}

func (k *KubernetesClient) GetKpNodes(kpNodeNameRegex regexp.Regexp) ([]apiv1.Node, error) {
	workerNodes, err := k.GetWorkerNodes()
	if err != nil {
		return nil, err
	}

	var kpNodes []apiv1.Node

	for _, kpNode := range workerNodes {
		if kpNodeNameRegex.MatchString(kpNode.Name) {
			kpNodes = append(kpNodes, kpNode)
		}
	}

	return kpNodes, err
}

func (k *KubernetesClient) GetKpNodesAllocatedResources(kpNodeNameRegex regexp.Regexp) (map[string]AllocatedResources, error) {
	kpNodes, err := k.GetKpNodes(kpNodeNameRegex)
	if err != nil {
		return nil, err
	}

	allocatedResources := map[string]AllocatedResources{}

	for _, kpNode := range kpNodes {
		nodeResources := AllocatedResources{}

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
				nodeResources.Cpu += container.Resources.Requests.Cpu().AsApproximateFloat64()
				nodeResources.Memory += container.Resources.Requests.Memory().AsApproximateFloat64()
			}
		}

		allocatedResources[kpNode.Name] = nodeResources
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

func (k *KubernetesClient) cordonKpNode(ctx context.Context, kpNodeName string) error {
	kpNode, err := k.client.CoreV1().Nodes().Get(
		ctx,
		kpNodeName,
		metav1.GetOptions{},
	)
	if err != nil {
		return err
	}

	kpNode.Spec.Unschedulable = true

	_, err = k.client.CoreV1().Nodes().Update(
		ctx,
		kpNode,
		metav1.UpdateOptions{},
	)

	return err
}

func (k *KubernetesClient) waitForPodsDelete(ctx context.Context, evictedPods *apiv1.PodList, kpNodeName string) error {
	err := wait.PollUntilContextCancel(
		ctx,
		time.Duration(time.Second*5),
		true,
		func(ctx context.Context) (bool, error) {
			var err error
			deleted := true
			for _, evictedPod := range evictedPods.Items {
				pod, err := k.client.CoreV1().Pods(evictedPod.Namespace).Get(
					ctx,
					evictedPod.Name,
					metav1.GetOptions{},
				)

				if pod.Spec.NodeName != kpNodeName || apierrors.IsNotFound(err) {
					continue
				} else {
					deleted = false
				}
			}

			return deleted, err
		},
	)

	if errors.Is(err, context.DeadlineExceeded) {
		return nil
	}

	return err
}

func (k *KubernetesClient) drainKpNode(ctx context.Context, kpNodeName string) error {
	pods, err := k.client.CoreV1().Pods("").List(
		ctx,
		metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", kpNodeName),
		},
	)
	if err != nil {
		return err
	}

	evictedPods := &apiv1.PodList{}
	for _, pod := range pods.Items {
		if pod.OwnerReferences[0].Kind != "DaemonSet" {
			err = k.client.PolicyV1().Evictions(pod.Namespace).Evict(
				ctx,
				&policyv1.Eviction{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pod.Name,
						Namespace: pod.Namespace,
					},
				},
			)
			if err != nil {
				return err
			}

			evictedPods.Items = append(evictedPods.Items, pod)
		}
	}

	err = k.waitForPodsDelete(ctx, evictedPods, kpNodeName)
	if err != nil {
		return err
	}

	return err
}

func (k *KubernetesClient) DeleteKpNode(ctx context.Context, kpNodeName string) error {
	err := k.cordonKpNode(ctx, kpNodeName)
	if err != nil {
		return err
	}

	err = k.drainKpNode(ctx, kpNodeName)
	if err != nil {
		return err
	}

	err = k.client.CoreV1().Nodes().Delete(
		ctx,
		kpNodeName,
		metav1.DeleteOptions{},
	)
	if err != nil {
		return err
	}

	return err
}

func (k *KubernetesClient) LabelKpNode(kpNodeName string, newKpNodeLabels map[string]string) error {
	return retry.RetryOnConflict(
		retry.DefaultRetry,
		func() error {
			kpNode, err := k.client.CoreV1().Nodes().Get(
				context.TODO(),
				kpNodeName,
				metav1.GetOptions{},
			)
			if err != nil {
				return err
			}

			kpNodeLabels := kpNode.GetLabels()
			for key, value := range newKpNodeLabels {
				kpNodeLabels[key] = value
			}

			kpNode.SetLabels(kpNodeLabels)

			_, err = k.client.CoreV1().Nodes().Update(
				context.TODO(),
				kpNode,
				metav1.UpdateOptions{},
			)

			return err
		},
	)
}

func (k *KubernetesClient) getMaxAllocatableMemoryForSinglePod(kpNodeNameRegex regexp.Regexp) (float64, error) {
	kpNodes, err := k.GetKpNodes(kpNodeNameRegex)
	if err != nil {
		return 0.0, err
	}

	var maxAllocatable float64
	for _, node := range kpNodes {
		if node.Status.Allocatable.Memory().AsApproximateFloat64() > maxAllocatable {
			maxAllocatable = node.Status.Allocatable.Memory().AsApproximateFloat64()
		}
	}

	return maxAllocatable, nil
}
