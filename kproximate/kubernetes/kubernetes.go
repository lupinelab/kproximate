package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	apiv1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/homedir"
)

type Kubernetes struct {
	client *kubernetes.Clientset
}

type UnschedulableResources struct {
	Cpu    float64
	Memory int64
}

type AllocatedResources struct {
	Cpu    float64
	Memory float64
}

func NewKubernetesClient() *Kubernetes {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		flag.Parse()
	}

	var config *rest.Config

	if _, err := os.Stat(*kubeconfig); err == nil {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err.Error())
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

	kubernetes := &Kubernetes{
		client: clientset,
	}

	return kubernetes
}

func (k *Kubernetes) GetNewLock(podName string, namespace string) *resourcelock.LeaseLock {
	var lock = &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      "kproximate-leader-lease",
			Namespace: namespace,
		},
		Client: k.client.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: podName,
		},
	}

	return lock
}

func (k *Kubernetes) GetUnschedulableResources() (*UnschedulableResources, error) {
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

func (k *Kubernetes) GetFailedSchedulingDueToControlPlaneTaint() (bool, error) {
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
				if strings.Contains(condition.Message, "untolerated taint {node-role.kubernetes.io/control-plane: }") {

					return true, nil
				}
			}
		}
	}

	return false, nil
}

func (k *Kubernetes) GetKpNodes() ([]apiv1.Node, error) {
	nodes, err := k.client.CoreV1().Nodes().List(
		context.TODO(),
		metav1.ListOptions{},
	)
	if err != nil {
		return nil, err
	}

	var kpNodes []apiv1.Node

	var kpNodeName = regexp.MustCompile(`^kp-node-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`)

	for _, kpNode := range nodes.Items {
		if kpNodeName.MatchString(kpNode.Name) {
			kpNodes = append(kpNodes, kpNode)
		}
	}

	return kpNodes, err
}

func (k *Kubernetes) GetNodePods(kpNodeName string) ([]apiv1.Pod, error) {
	pods, err := k.client.CoreV1().Pods("").List(
		context.TODO(),
		metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", kpNodeName),
		},
	)
	if err != nil {
		return nil, err
	}

	return pods.Items, err
}

func (k *Kubernetes) GetKpNodesAllocatedResources() (map[string]*AllocatedResources, error) {
	kpNodes, err := k.GetKpNodes()
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

func (k *Kubernetes) GetEmptyKpNodes() ([]apiv1.Node, error) {
	nodes, err := k.GetKpNodes()
	if err != nil {
		return nil, err
	}

	var emptyNodes []apiv1.Node

	for _, node := range nodes {
		pods, err := k.client.CoreV1().Pods("").List(
			context.TODO(),
			metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
			},
		)
		if err != nil {
			return nil, err
		}

		if len(pods.Items) == 0 {
			emptyNodes = append(emptyNodes, node)
		}
	}

	return emptyNodes, err
}

func (k *Kubernetes) WaitForJoin(ctx context.Context, ok chan<- bool, newKpNodeName string) {
	for {
		newkpNode, _ := k.client.CoreV1().Nodes().Get(
			context.TODO(),
			newKpNodeName,
			metav1.GetOptions{},
		)

		for _, condition := range newkpNode.Status.Conditions {
			if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
				ok <- true
			}
		}
	}
}

func (k *Kubernetes) DeleteKpNode(kpNodeName string) error {
	err := k.CordonKpNode(kpNodeName)
	if err != nil {
		return err
	}

	pods, err := k.GetNodePods(kpNodeName)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		k.EvictPod(pod.Name, pod.Namespace)
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

func (k *Kubernetes) SlowDeleteKpNode(kpNodeName string) error {
	err := k.CordonKpNode(kpNodeName)
	if err != nil {
		return err
	}

	pods, err := k.GetNodePods(kpNodeName)
	if err != nil {
		return err
	}

	for _, pod := range pods {
		time.Sleep(time.Second * 3)
		k.EvictPod(pod.Name, pod.Namespace)
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

func (k *Kubernetes) CordonKpNode(KpNodeName string) error {
	kpNode, err := k.client.CoreV1().Nodes().Get(
		context.TODO(),
		KpNodeName,
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

func (k *Kubernetes) EvictPod(podName, namespace string) error {
	return k.client.PolicyV1().Evictions(namespace).Evict(
		context.TODO(),
		&policy.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: namespace,
			},
		},
	)
}
