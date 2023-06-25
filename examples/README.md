# Configuration & Installation

There are four main requirements for configuring and deploying kproximate:
* [Proxmox API Access](#proxmox-api-access)
* [Proxmox Template](#proxmox-template)
* [Networking](#networking)
* [Installing kproximate](#)

## Proxmox API Access

The `create_kproximate_api_token.sh` script can be run on a Proxmox host to create a token with the required privileges, it will return the token ID and token which will be required later.

A custom user/token can be used however the below privileges must be granted to it:

* Datastore.AllocateSpace
* Datastore.Audit
* Sys.Audit
* VM.Allocate
* VM.Audit
* VM.Clone
* VM.Config.Cloudinit
* VM.Config.CPU
* VM.Config.Disk
* VM.Config.Memory
* VM.Config.Network
* VM.Config.Options
* VM.PowerMgmt

## Proxmox Template

The [create_kproximate_template.sh](https://github.com/lupinelab/kproximate/tree/main/examples/create_kproximate_template.sh) script shows a working example of how to create a K3S template node that will automatically join the Kubernetes cluster when booted.

This script should be run on a host in the Proxmox cluster after configuring the the variables at the top with values appropiate to your cluster.

Special consideration should be given to the STORAGE variable whose value should be the name of ***shared storage*** that is available to all of your proxmox hosts.

When running the script it requires two args. First the codename of an ubuntu release (e.g. "jammy") and then the VMID to assign to the template. 

The VMID should also be chosen carefully since all kproximate nodes created will be assigned to the next available VMID after that of the template.

Example:

```./create_kproximate_template.sh jammy 600```

### Custom templates

If creating your own template please consider the following:

* As previously stated, the template must join your kubernetes cluster with zero interaction. I found that the `virt-customize` package was the easiest way to do this.
* It should be a cloud-init enabled image in order for ssh key injection to work.
* The final template should have a cloudinit disk added to it.
* Ensure that each time it is booted the template will generate a new machine-id. I found that this was only achieveable when truncating (and not removing) `/etc/machine-id` with the `virt-customize --truncate` command at the end of my configuration steps.
* It should be configured to receive a DHCP IP lease.
* If you are using VLANs ensure it is tagged appropriately, ie the one your kubernetes cluster resides in.
* It should have `qemu-guest-agent` installed.

See [create_kproximate_template.sh](https://github.com/lupinelab/kproximate/tree/main/examples/create_kproximate_template.sh) for examples of the above.

## Networking

The template should be configured to reside in the same network as your Kubernetes cluster, this can be done after it's creation in the Proxmox web gui.

This network should also provide a DHCP server so that new kproximate nodes can aquire an IP address.

Your Proxmox API endpoint should also be accessible from this network. In my case I have a firewall rule between the two VLANS that my Proxmox and Kubernetes clusters are in.

## Installing kproximate

A helm chart is provided [INSERT HELM CHART LINK] for installing the application into your kubernetes cluster. See [example-values.yaml](https://github.com/lupinelab/kproximate/tree/main/examples/example-values.yaml) for a basic configuraton example.

Add the repo:

`helm repo add kproximate https://link-to-helm-chart`

Install kproximate:

`helm install lupinelab/kproximate -f examples-values.yaml -n kproximate --create-namespace`

The full set of configuration options with their default values is below:
```
kproximate:
  // The period after creation that a kproximate node will be given grace to be empty before it is a target for deletion
  emptyGraceSecondsAfterCreation: 120 
  
  // The number of cores assigned to new kproximate nodes
  kpNodeCores: 2
  
  // The required headroom used in scale down calculations expressed as value between 0 and 1
  kpNodeHeadroom: 0.2
  
  // The amount of memory assigned to new kproximate nodes in MiB
  kpNodeMemory: 2048
  
  // The name of the Proxmox template for kproximate nodes
  kpNodeTemplateName: // Required
  
  // The maximum number of kproximate nodes allowable
  maxKPNodes: 3
  
  // If set to true skip TLS checks for the Proxmox API
  pmAllowInsecure: false
  
  // If set to true enable debug output for Proxmox API calls
  pmDebug: false
  
  // The Proxmox API URL
  pmUrl: // Required
  
  // The Proxmox API token ID
  pmUserID: // Required
  
  // The Proxmox API token
  pmToken: // Required
  
  // The number of seconds between polls of the cluster for scaling
  secondsBetweenPolls: 5
  
  // The SSH public key to inject into the kproximate template
  sshKey: None
```