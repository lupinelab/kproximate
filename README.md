# kproximate

A node autoscaler project for Proxmox allowing Kubernetes clusters to dynamically scale across a Proxmox cluster.

* Polls for unschedulable pods
* Assesses the resource requirements from the requests of the unschedulable pods
* Provisions VMs from a user defined template
* User configurable cpu, memory and ssh key for provisioned VMs
* Removes nodes when requested resources can be contained on fewer VMs

The user is required to create a Proxmox template configured to join the kubernetes cluster automatically on it's first boot. All examples show how to do this with K3S, however any kubernetes cluster theoretically could be used if you are prepared to put in the work to build an appropriate template. 

## Configuration and Installation

See [here](https://github.com/lupinelab/kproximate/tree/main/examples) for example setup scripts and configuration.

## Leadership

The default replicas value for the app deployment is 3 and kproximate pods will only be scheduled onto control plane nodes at 1 pod per node. It is recommended to set the number of replicas to 3 or up to the number of control plane nodes in your Kubernetes cluster. A single replica is also a supported configuration.

Leadership election is attempted by each pod every 2 seconds, if the leader pod has not renewed it's claim on the leadership lease for 10 seconds the first pod to make a claim will be elected as leader and will take over responsibility of scaling the cluster.

Currently the application is stateless so if a pod is iterrupted during a scale event the event will be abandoned immediately in the state it is in. The pod taking leadership will attempt to clean up from any abandoned scale events if a VM is left stopped or running but not joined to the kubernetes cluster. This may change in the future.

## Polling

Kproximate polls the kubernetes cluster by default every 5 seconds, this is configurable but not recommended. If this value is set to below 5 seconds it will be ignored and the default will be used.

## Scaling - ***Important***

Scaling is calculated based on pod requests. Resource requests must be set for all pods in the cluster which are not fixed to control plane nodes else the cluster may be left with continually pending pods.

*Note: A minimum kproximate node setting will soon be made available in order to provide a base pool of nodes to satisfy pods without resource requests set.*

## Scaling Up

Kproximate will scale upwards as fast as it can provision VMs in the cluster. As soon as an unschedulable pod is found kproximate will assess the resource requirements and provision Proxmox VMs in order to satisfy them. Scaling events are asyncronous so if more resources are deemed to be required while a scale event is still in progress then an additional scaling event might be triggered if the first scaling event will not be able to satisfy the new requirements.

## Scaling Down

Scaling down is very agressive. Whenever the cluster is not scaling and the cluster's load is calculated to be satisfiable by n-1 node while also being able to provide a user configured headroom percentage, a negative scale event is triggerd. The node with the least allocated resources is selected and all pods are evicted from it before it is removed from the cluster and deleted.

*Note: This feature is very much a work in progress and will soon be far more graceful in it's handling of negative scale events*