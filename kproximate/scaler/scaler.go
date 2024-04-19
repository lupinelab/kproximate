package scaler

import (
	"context"

	"github.com/lupinelab/kproximate/proxmox"
)

type Scaler interface {
	RequiredScaleEvents(numCurrentEvents int) ([]*ScaleEvent, error)
	SelectTargetHosts(scaleEvents []*ScaleEvent) error
	ScaleUp(ctx context.Context, scaleEvent *ScaleEvent) error
	NumReadyNodes() (int, error)
	NumNodes() (int, error)
	AssessScaleDown() (*ScaleEvent, error)
	ScaleDown(ctx context.Context, scaleEvent *ScaleEvent) error
	DeleteNode(kpNodeName string) error
	GetResourceStatistics() (ResourceStatistics, error)
}

type ScaleEvent struct {
	ScaleType  int
	NodeName   string
	TargetHost proxmox.HostInformation
}

type AllocatedResources struct {
	Cpu    float64
	Memory float64
}

type AllocatableResources struct {
	Cpu    float64
	Memory float64
}

type ResourceStatistics struct {
	Allocatable AllocatableResources
	Allocated   AllocatedResources
}
