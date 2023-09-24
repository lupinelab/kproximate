# kproximate

A node autoscaler project for Proxmox allowing a Kubernetes cluster to dynamically scale across a Proxmox cluster.

* Polls for unschedulable pods
* Assesses the resource requirements from the requests of the unschedulable pods
* Provisions VMs from a user defined template
* User configurable cpu, memory and ssh key for provisioned VMs
* Removes nodes when requested resources can be satisfied by fewer VMs

The operator is required to create a Proxmox template configured to join the kubernetes cluster automatically on it's first boot. All examples show how to do this with K3S, however other kubernetes cluster flavours theoretically could be used if you are prepared to put in the work to build an appropriate template.

While it is a pretty niche project, some possible use cases include:
- Providing overflow capacity for a raspberry Pi cluster
- Running multiple k8s clusters each with fluctuating loads on a single proxmox cluster
- Something someone else thinks of that I haven't
- Just because...

## Configuration and Installation
See [here](https://github.com/lupinelab/kproximate/tree/main/examples) for example setup scripts and configuration.

## Scaling
Kproximate polls the kubernetes cluster by default every 10 seconds looking for unschedulable resources.

***Important***\
Scaling is calculated based on pod requests. Resource requests must be set for all pods in the cluster which are not fixed to control plane nodes else the cluster may be left with continually pending pods.

## Scaling Up
Kproximate will scale upwards as fast as it can provision VMs in the cluster limited by the amount of worker replicas deployed. As soon as unschedulable CPU or memory resource requests are found kproximate will assess the resource requirements and provision Proxmox VMs to satisfy them.

Scaling events are asyncronous so if new resources requests are found on the cluster while a scaling event is in progress then an additional scaling event will be triggered if the initial scaling event will not be able to satisfy the new resource requests.

## Proxmox Host Targeting
To select a Proxmox host for a new kproximate node all Proxmox hosts in the cluster are assessed and the following logic is applied:
- Skip host if there is an existing scaling event targeting it
- Skip host if there is an existing kproximate node on it
- Select host as target for scaling event

If no host has been selected after all hosts have been assessed then the host with the most available memory is selected.

## Scaling Down
Scaling down is very agressive. When the cluster is not scaling and the cluster's load is calculated to be satisfiable by n-1 nodes while also remaining within the configured load headroom value then a negative scale event is triggered. The node with the least allocated resources is selected and all pods are evicted from it before it is removed from the cluster and deleted.
