package kubernetes

import (
	"context"
	"flag"
	"path/filepath"
	"strings"

	apiv1 "k8s.io/api/core/v1"
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
