package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	apiv1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Kubernetes struct {
	client *kubernetes.Clientset
}

type UnschedulableResources struct {
	Cpu    float64
	Memory int64
}

func NewKubernetesClient() *Kubernetes {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
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

func (k *Kubernetes) GetUnschedulableResources() *UnschedulableResources {
	var rCpu float64
	var rMemory float64

	pods, err := k.client.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
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

	return unschedulableResources
}

func (k *Kubernetes) GetNodes() ([]apiv1.Node, error) {
	nodes, err := k.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nodes.Items, err
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

func (k *Kubernetes) CheckKpNodeReady(newKpNodeName string) bool {
	newkpNode, _ := k.client.CoreV1().Nodes().Get(context.TODO(), newKpNodeName, metav1.GetOptions{})

	for _, condition := range newkpNode.Status.Conditions {
		if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
			return true
		}
	}

	// nodes, err := k.GetNodes()
	// if err != nil {
	// 	return false, err
	// }

	// for _, node := range nodes {
	// 	if strings.Contains(node.Name, newKpNodeName) {
	// 		for _, condition := range node.Status.Conditions {
	// 			if condition.Type == apiv1.NodeReady && condition.Status == apiv1.ConditionTrue {
	// 				return true, err
	// 			}
	// 		}
	// 	}
	// }

	return false
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

	err = k.client.CoreV1().Nodes().Delete(context.TODO(), kpNodeName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}

	return err
}

func (k *Kubernetes) CordonKpNode(KpNodeName string) error {
	kpNode, err := k.client.CoreV1().Nodes().Get(context.TODO(), KpNodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	kpNode.Spec.Unschedulable = true

	_, err = k.client.CoreV1().Nodes().Update(context.TODO(), kpNode, metav1.UpdateOptions{})

	return err
}

func (k *Kubernetes) EvictPod(podName, namespace string) error {
	return k.client.PolicyV1().Evictions(namespace).Evict(context.TODO(), &policy.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
	})
}
