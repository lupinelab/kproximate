package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/lupinelab/kproximate/scaler"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	totalKpNodes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "kpnodes_total",
		Help: "The total number of kproximate nodes",
	})

	runningKpNodes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "kpnodes_running",
		Help: "The number of running kproximate nodes",
	})

	totalProvisionedCpu = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cpu_provisioned_total",
		Help: "The total provisioned cpus",
	})

	totalProvisionedMemory = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "memory_provisioned_total",
		Help: "The total memory provisioned",
	})

	totalAllocatableCpu = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cpu_allocatable_total",
		Help: "The total cpus allocatable",
	})

	totalAllocatableMemory = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "memory_allocatable_total",
		Help: "The total memory allocatable",
	})

	totalAllocatedCpu = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cpu_allocated_total",
		Help: "The total cpu allocated",
	})

	totalAllocatedMemory = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "memory_allocated_total",
		Help: "The total memory allocated",
	})


)

func recordMetrics(
	scaler *scaler.Scaler,
) {
	go func() {
		for {
			numKpNodes, _ := scaler.Proxmox.GetAllKpNodes(scaler.Config.KpNodeNameRegex)
			totalKpNodes.Set(float64(len(numKpNodes)))

			runningNodes, _ := scaler.Kubernetes.GetKpNodes(scaler.Config.KpNodeNameRegex)
			runningKpNodes.Set(float64(len(runningNodes)))

			totalProvisionedCpu.Set(float64(len(runningNodes) * scaler.Config.KpNodeCores))
			totalProvisionedMemory.Set(float64(len(runningNodes) * (scaler.Config.KpNodeMemory << 20)))

			var allocatableCpu float64
			var allocatableMemory float64
			kpNodes, _ := scaler.Kubernetes.GetKpNodes(scaler.Config.KpNodeNameRegex)
			for _, kpNode := range kpNodes {
				allocatableCpu += kpNode.Status.Allocatable.Cpu().AsApproximateFloat64()
				allocatableMemory += kpNode.Status.Allocatable.Memory().AsApproximateFloat64()
			}

			totalAllocatableCpu.Set(allocatableCpu)
			totalAllocatableMemory.Set(allocatableMemory)

			allocatedResources, _ := scaler.Kubernetes.GetAllocatedResources(scaler.Config.KpNodeNameRegex)
			var allocatedCpu float64
			var allocatedMemory float64
			for _, kpNode := range allocatedResources {
				allocatedCpu += kpNode.Cpu
				allocatedMemory += kpNode.Memory
			}

			totalAllocatedCpu.Set(allocatedCpu)
			totalAllocatedMemory.Set(allocatedMemory)
			
			time.Sleep(5 * time.Second)
		}
	}()
}

func Serve(
	ctx context.Context,
	scaler *scaler.Scaler,
) {
	registry := prometheus.NewRegistry()

	registry.MustRegister(
		totalKpNodes,
		runningKpNodes,
		totalProvisionedCpu,
		totalProvisionedMemory,
		totalAllocatableCpu,
		totalAllocatableMemory,
		totalAllocatedCpu,
		totalAllocatedMemory,
	)

	recordMetrics(scaler)

	http.Handle(
		"/metrics",
		promhttp.HandlerFor(
			registry,
			promhttp.HandlerOpts{},
		),
	)

	http.ListenAndServe(":80", nil)
}
