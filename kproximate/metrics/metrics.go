package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/logger"
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
	ctx context.Context,
	scaler scaler.Scaler,
	config config.KproximateConfig,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(5 * time.Second)

			numKpNodes, _ := scaler.NumNodes()
			totalKpNodes.Set(float64(numKpNodes))

			runningNodes, _ := scaler.NumReadyNodes()
			runningKpNodes.Set(float64(runningNodes))

			totalProvisionedCpu.Set(float64(runningNodes * config.KpNodeCores))
			totalProvisionedMemory.Set(float64(runningNodes * (config.KpNodeMemory << 20)))

			resourceStats, err := scaler.GetResourceStatistics()
			if err != nil {
				logger.ErrorLog("Failed to get resource stats", "error", err)
				continue
			}

			totalAllocatableCpu.Set(resourceStats.Allocatable.Cpu)
			totalAllocatableMemory.Set(resourceStats.Allocatable.Memory)

			totalAllocatedCpu.Set(resourceStats.Allocated.Cpu)
			totalAllocatedMemory.Set(resourceStats.Allocated.Memory)
		}
	}
}

func Serve(
	ctx context.Context,
	scaler scaler.Scaler,
	config config.KproximateConfig,
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

	go recordMetrics(ctx, scaler, config)

	http.Handle(
		"/metrics",
		promhttp.HandlerFor(
			registry,
			promhttp.HandlerOpts{},
		),
	)

	http.ListenAndServe(":80", nil)
}
