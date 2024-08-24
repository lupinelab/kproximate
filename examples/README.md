# Configuration & Installation

There are four main requirements for configuring and deploying kproximate:
* [Proxmox API Access](#proxmox-api-access)
* [Proxmox Template](#proxmox-template)
* [Networking](#networking)
* [Installing kproximate](#)

## Proxmox API Access

The [create_kproximate_api_token.sh](https://github.com/lupinelab/kproximate/tree/main/examples/create_proxmox_api_token.sh) script can be run on a Proxmox host to create a token with the required privileges, it will return the token ID and token which will be required later.

A custom user/token can be used, however the below privileges must be granted to it:

* Datastore.AllocateSpace
* Datastore.Audit
* Sys.Audit
* SDN.Use
* VM.Allocate
* VM.Audit
* VM.Clone
* VM.Config.Cloudinit
* VM.Config.CPU
* VM.Config.Disk
* VM.Config.Memory
* VM.Config.Network
* VM.Config.Options
* VM.Monitor
* VM.PowerMgmt

Alternatively you may use password authentication with a user that has the above permissions granted to it.

## Proxmox Template

There are two types of template that may be created, each using a different method to join the cluster, qemu-exec and first boot.

Both methods require a script to be run on a host in the Proxmox cluster after configuring the the variables at the top with values appropriate for your cluster.

Special consideration should be given to the STORAGE variable whose value should be the name of the storage on which you wish to store the template.

When running the script it requires two args. First the codename of an ubuntu release (e.g. "jammy") and then the VMID to assign to the template. 

Consideration should be given to the VMID as all kproximate nodes created will be assigned to the next available VMID after that of the template.

Example:

```./create_kproximate_template_exec.sh jammy 600```

### Join by qemu-exec (recommended)

The [create_kproximate_template_exec.sh](https://github.com/lupinelab/kproximate/tree/main/examples/create_kproximate_template_exec.sh) script creates a template that joins the Kubernetes cluster using a command that is executed by qemu-exec.

The benefits of this are:
  - The template does not contain secrets
  - The template can be re-used across mutliple k3s clusters

If using this method you must supply the following values:

```yaml
kproximate:
  config:
    kpQemuExecJoin: true
  secrets:
    kpJoinCommand: "<your-join-command>"
```
 
The value of `kpJoinCommand` is executed on the new node as follows: `bash -c <your-join-command>`.

### Join on first boot

The [create_kproximate_template.sh](https://github.com/lupinelab/kproximate/tree/main/examples/create_kproximate_template.sh) script creates a template that joins the Kubernetes cluster automatically on first boot.

### Using local storage

Those wishing to use local storage can set `kpLocalTemplateStorage: true` in their configuration and create a template on each Proxmox node with an identical name but a different VMID.

### Custom templates

If creating your own template please consider the following:

* It should have `qemu-guest-agent` installed.
* It should be a cloud-init enabled image in order for ssh key injection to work.
* The final template should have a cloudinit disk added to it.
* Ensure that each time it is booted the template will generate a new machine-id. I found that this was only achieveable when truncating (and not removing) `/etc/machine-id` with the `virt-customize --truncate` command at the end of the configuration steps.
* It should be configured to receive a DHCP IP lease.
* If you are using VLANs ensure it is tagged appropriately, ie the one your kubernetes cluster resides in.

## Networking

The template should be configured to reside in the same network as your Kubernetes cluster, this can be done after it's creation in the Proxmox web gui.

This network should also provide a DHCP server so that new kproximate nodes can aquire an IP address.

Your Proxmox API endpoint should also be accessible from this network. For example, in my case I have a firewall rule between the two VLANS that my Proxmox and Kubernetes clusters are in.

## Installing kproximate

*NOTE: All charts have now been moved to oci://gchr.io/lupinelab/kproximate, all versions up to 0.1.9 have been duplicated here but new versions will NOT be published on the old repository https://charts.lupinelab.co.uk*

A helm chart is provided at oci://ghcr.io/lupinelab for installing the application into your kubernetes cluster. See [example-values.yaml](https://github.com/lupinelab/kproximate/tree/main/examples/example-values.yaml) for a basic configuraton example.

Install kproximate:

`helm install kproximate oci://ghcr.io/lupinelab/kproximate -f your-values.yaml -n kproximate --create-namespace`

See [values.yaml](https://github.com/lupinelab/kproximate/tree/main/chart/kproximate/values.yaml) in the helm chart for the full set of configuration options and defaults.
