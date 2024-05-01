package proxmox

import "github.com/Telmate/proxmox-api-go/proxmox"

type ProxmoxClientMock struct {
	ExecStatus            map[string]interface{}
	NextID                int
	ResourceList          []interface{}
	VmList                map[string]interface{}
	VmRefByName           map[string]*proxmox.VmRef
	VmRefsByName          map[string][]*proxmox.VmRef
	QemuExecResponse      map[string]interface{}
	QemuAgentPingResponse map[string]interface{}
}

func (m *ProxmoxClientMock) CloneQemuVm(vmr *proxmox.VmRef, vmParams map[string]interface{}) (exitStatus string, err error) {
	return "OK", nil
}

func (m *ProxmoxClientMock) DeleteVm(vmr *proxmox.VmRef) (exitStatus string, err error) {
	return "OK", nil
}

func (m *ProxmoxClientMock) GetExecStatus(vmr *proxmox.VmRef, pid string) (status map[string]interface{}, err error) {
	return m.ExecStatus, nil
}

func (m *ProxmoxClientMock) GetNextID(currentID int) (nextID int, err error) {
	m.NextID++
	return m.NextID, nil
}

func (m *ProxmoxClientMock) GetResourceList(resourceType string) (list []interface{}, err error) {
	return m.ResourceList, nil
}

func (m *ProxmoxClientMock) GetVmList() (map[string]interface{}, error) {
	return m.VmList, nil
}

func (m *ProxmoxClientMock) GetVmRefByName(vmName string) (vmr *proxmox.VmRef, err error) {
	return m.VmRefByName[vmName], nil
}

func (m *ProxmoxClientMock) GetVmRefsByName(vmName string) (vmrs []*proxmox.VmRef, err error) {
	return m.VmRefsByName[vmName], nil

}

func (m *ProxmoxClientMock) QemuAgentExec(vmr *proxmox.VmRef, params map[string]interface{}) (result map[string]interface{}, err error) {
	return m.QemuExecResponse, nil
}

func (m *ProxmoxClientMock) QemuAgentPing(vmr *proxmox.VmRef) (pingRes map[string]interface{}, err error) {
	return m.QemuAgentPingResponse, nil
}

func (m *ProxmoxClientMock) SetVmConfig(vmr *proxmox.VmRef, params map[string]interface{}) (exitStatus interface{}, err error) {
	return "OK", nil
}

func (m *ProxmoxClientMock) StartVm(vmr *proxmox.VmRef) (exitStatus string, err error) {
	return "OK", nil
}

func (m *ProxmoxClientMock) StopVm(vmr *proxmox.VmRef) (exitStatus string, err error) {
	return "OK", nil
}
