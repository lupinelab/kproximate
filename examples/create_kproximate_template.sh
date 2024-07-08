#!/bin/bash
set -xe

NAME= # The name of the resulting template
STORAGE= # The name of the proxmox storage to store the template in
VLAN= # The vlan tag for the template network interface, leave as empty string for no tag
K3S_URL= # Add your k3s URL here ie. https://k3s-server:6443
K3S_TOKEN= # Add your k3s token here, can be found at /var/lib/rancher/k3s/server/node-token on an existing k3s node

# Check the required variables have been set
for var in NAME STORAGE K3S_URL K3S_TOKEN
do
    if [[ -z "${!var}" ]]
    then
        echo "$var is not set"
        exit 1
    fi
done

# Set the CODENAME from $1, this is the lowercase short version... ie jammy from Jammy Jellyfish
CODENAME=$1

# Set the VMID from $2
VMID=$2

# Check for libguestfs-tools
if [[ $(dpkg-query -W --showformat='${Status}\n' libguestfs-tools | grep "install ok installed") == "" ]]; then
    apt update -y
    apt install libguestfs-tools -y
fi

## If it doesn't already exist download a new $CODENAME image ie. https://cloud-images.ubuntu.com/kinetic/current/kinetic-server-cloudimg-amd64.img
## Links for newer/other images can be found here: https://cloud-images.ubuntu.com/, the .img you need should match the naming convention from the above line.
if [[ ! -f $CODENAME-server-cloudimg-amd64.img ]]; then
    wget https://cloud-images.ubuntu.com/${CODENAME}/current/${CODENAME}-server-cloudimg-amd64.img
fi

# Grab the name of the file
IMG=$(ls | grep ^${CODENAME}-server-cloudimg-amd64.img$)

NEWDISK=${CODENAME}.img

# Expand the image
virt-filesystems --long -h --all --format=raw -a $IMG
truncate -r $IMG $NEWDISK
truncate -s 10G $NEWDISK
virt-resize --format raw --expand /dev/sda1 $IMG $NEWDISK

# Configure the new img
virt-customize \
        -a $NEWDISK \
        --install qemu-guest-agent,nfs-common,containerd,runc \
        --firstboot-command "curl -sfL https://get.k3s.io | K3S_URL=${K3S_URL} K3S_TOKEN=${K3S_TOKEN} sh -" \
        --truncate /etc/machine-id

# Build a vm from which to create a proxmox template
qm create $VMID --name $NAME --memory 2048 --cpu cputype=host --cores 2 --net0 virtio,bridge=vmbr0,firewall=1 --bios ovmf
if [[ -n "$VLAN" ]]
then
    qm set $VMID --net0 virtio,bridge=vmbr0,firewall=1,tag=$VLAN
else
    qm set $VMID --net0 virtio,bridge=vmbr0,firewall=1
fi
qm set $VMID --efidisk0 $STORAGE:0
qm set $VMID --scsihw virtio-scsi-single --scsi0 ${STORAGE}:0,import-from=$(realpath ${NEWDISK}),cache=writeback,discard=on,iothread=1,ssd=1
qm set $VMID --boot order=scsi0
qm set $VMID --scsi1 $STORAGE:cloudinit
qm set $VMID --ipconfig0 ip=dhcp
qm set $VMID --agent enabled=1
qm template $VMID

# Remove the image
rm $NEWDISK
