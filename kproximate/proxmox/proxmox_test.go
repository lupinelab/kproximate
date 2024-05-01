package proxmox

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/Telmate/proxmox-api-go/proxmox"
)

func NewProxmoxMock(clientMock ProxmoxClientMock) *ProxmoxClient {
	return &ProxmoxClient{
		client: &clientMock,
	}
}

func TestGetAllKpNodes(t *testing.T) {
	p := NewProxmoxMock(ProxmoxClientMock{
		VmList: map[string]interface{}{
			"Data": []map[string]string{
				{"Name": "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd"},
				{"Name": "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a"},
				{"Name": "not-a-kp-node"},
			},
		},
	})

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))
	kpNodes, err := p.GetAllKpNodes(kpNodeNameRegex)
	if err != nil {
		t.Error(err)
	}

	for _, node := range kpNodes {
		if node.Name == "not-a-kp-node" {
			t.Errorf("Did not expect %s", node.Name)
		}
	}

	if len(kpNodes) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(kpNodes))
	}
}

func TestGetRunningKpNodes(t *testing.T) {
	p := NewProxmoxMock(ProxmoxClientMock{
		VmList: map[string]interface{}{
			"Data": []map[string]string{
				{
					"Name":   "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
					"Status": "running",
				},
				{
					"Name":   "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a",
					"Status": "stopped",
				},
			},
		},
	})

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))
	kpNodes, err := p.GetRunningKpNodes(kpNodeNameRegex)
	if err != nil {
		t.Error(err)
	}

	for _, node := range kpNodes {
		if node.Name == "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a" {
			t.Errorf("Did not expect %s", node.Name)
		}
	}
}

func TestGetKpNode(t *testing.T) {
	p := NewProxmoxMock(ProxmoxClientMock{
		VmList: map[string]interface{}{
			"Data": []map[string]string{
				{
					"Name": "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd",
				},
				{
					"Name": "not-a-kp-node",
				},
			},
		},
	})

	kpNodeNameRegex := *regexp.MustCompile(fmt.Sprintf(`^%s-\w{8}-\w{4}-\w{4}-\w{4}-\w{12}$`, "kp-node"))
	kpNode, err := p.GetKpNode("kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd", kpNodeNameRegex)
	if err != nil {
		t.Error(err)
	}

	if kpNode.Name != "kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd" {
		t.Errorf("Expected kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd, got %s", kpNode.Name)
	}
}

func TestGetKpNodeTemplateRefReturnsCorrectTemplateRef(t *testing.T) {
	kproximateTemplateName := "kproximate-template"
	cloneTargetNode := "doesnt-matter"

	expectedVmRef := &proxmox.VmRef{}
	expectedVmRef.SetNode("kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd")

	unwantedVmRef := &proxmox.VmRef{}
	unwantedVmRef.SetNode("the-wrong-template")

	p := NewProxmoxMock(ProxmoxClientMock{
		VmRefsByName: map[string][]*proxmox.VmRef{
			kproximateTemplateName: {
				expectedVmRef,
			},
			"not-a-kproximate-template": {
				unwantedVmRef,
			},
		},
	})

	vmRef, err := p.GetKpNodeTemplateRef(kproximateTemplateName, false, cloneTargetNode)
	if err != nil {
		t.Error(err)
	}

	if vmRef.Node() != expectedVmRef.Node() {
		t.Errorf("Expected %s, got %s", expectedVmRef.Node(), vmRef.Node())
	}
}

func TestGetKpNodeTemplateRefReturnsCorrectTemplateRefForLocalStorage(t *testing.T) {
	kproximateTemplateName := "kproximate-template"
	cloneTargetNode := "kp-node-a4f77d63-a944-425d-a980-e7be925b8a6a"

	vmRef1 := &proxmox.VmRef{}
	vmRef1.SetNode("kp-node-163c3d58-4c4d-426d-baef-e0c30ecb5fcd")
	vmRef2 := &proxmox.VmRef{}
	vmRef2.SetNode(cloneTargetNode)

	p := NewProxmoxMock(ProxmoxClientMock{
		VmRefsByName: map[string][]*proxmox.VmRef{
			kproximateTemplateName: {
				vmRef1,
				vmRef2,
			},
		},
	})

	vmRef, err := p.GetKpNodeTemplateRef(kproximateTemplateName, true, cloneTargetNode)
	if err != nil {
		t.Error(err)
	}

	if vmRef.Node() != cloneTargetNode {
		t.Errorf("Expected %s, got %s", cloneTargetNode, vmRef.Node())
	}
}
