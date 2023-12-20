#!/bin/bash
set -xe

pveum role add kproximate --privs "VM.Config.Memory VM.Config.Network Datastore.AllocateSpace VM.Audit VM.Clone Sys.Audit Datastore.Audit VM.Config.Cloudinit VM.Config.Disk VM.PowerMgmt VM.Config.Options VM.Allocate VM.Config.CPU VM.Monitor SDN.Use"
pveum user add kproximate@pam
pveum acl modify / --users kproximate@pam --roles kproximate
pveum user token add kproximate@pam kproximate --privsep 0