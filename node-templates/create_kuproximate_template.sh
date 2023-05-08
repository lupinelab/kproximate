#!/bin/bash
set -xe

# Installing libguestfs-tools only required once, prior to first run
apt update -y
apt install libguestfs-tools -y

# Set the CODENAME from $2, this is the lowercase short version... ie jammy from Jammy Jellyfish
CODENAME=$1

# Set the VMID from $1, this should be the next available 9xxx number, check pmx-01 gui.
VMID=$2

## If it doesn't already exist download a new CODENAME image ie. https://cloud-images.ubuntu.com/kinetic/current/kinetic-server-cloudimg-amd64.img
## Links for newer/other images can be found here: https://cloud-images.ubuntu.com/, the .img you need should match the naming convention from the above line.
if [ ! -f $CODENAME-server-cloudimg-amd64.img ]
then
    wget https://cloud-images.ubuntu.com/$CODENAME/current/$CODENAME-server-cloudimg-amd64.img
fi

# Grab the name of the file
IMG=$(ls | grep ^$CODENAME-server-cloudimg-amd64.img$)

# A name for the new img
NEWDISK=$CODENAME.img

# Expand the image
virt-filesystems --long -h --all --format=raw -a $IMG
truncate -r $IMG $NEWDISK
truncate -s 10G $NEWDISK
virt-resize --format raw --expand /dev/sda1 $IMG $NEWDISK

# Configure the new img 
virt-customize \
        -a $NEWDISK \   
        --install qemu-guest-agent,nfs-common,containerd,runc \
        --mkdir /home/ubuntu/.ssh \
        --append-line "/home/ubuntu/.ssh/authorized_keys:<add_ssh_pub_key>" \
        --firstboot-command "curl -sfL https://get.k3s.io | K3S_URL=<K3S URL>:6443 K3S_TOKEN=<K3S_TOKEN> sh -" ## Add your K3s URL and token

# Build a vm from which to create a proxmox template
# Ensure you tag you template to the required vlan or remove the tag
qm create $VMID --name kuproximate-template --memory 2048 --balloon 1024 --cpu cputype=host --cores 2 --net0 virtio,bridge=vmbr0,firewall=1,tag= --bios ovmf
qm set $VMID --efidisk0 local-zfs:0
qm importdisk $VMID $NEWDISK local-zfs
qm set $VMID --scsihw virtio-scsi-single --scsi0 local-zfs:vm-$VMID-disk-1,cache=writeback,discard=on,iothread=1,ssd=1
qm set $VMID --boot order=scsi0
qm set $VMID --scsi1 local-zfs:cloudinit
qm set $VMID --ipconfig0 ip=dhcp
qm set $VMID --agent enabled=1
qm set $VMID --tags template
qm template $VMID  

# Remove the image 
rm $NEWDISK
