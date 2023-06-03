package main

import (
	"github.com/lupinelab/kproximate/scaler"
)

func main() {
	kpConfig := scaler.GetConfig()
	kpScaler := scaler.NewScaler(*kpConfig)

	kpScaler.Start()

	// // P
	// kpnodes := kpScaler.pCluster.GetKpNodes()
	// for _, kpnode := range kpnodes {
	// 	fmt.Println(kpnode.Name)
	// }

	// kpScaler.PCluster.GetClusterStats()

	// // K
	// kpScaler.KCluster.GetUnschedulableResources()

	// // New KPNode
	// // fmt.Println(pClient.NewKpNode(kpNodeTemplateName, "qtiny-02"))

	// kpTemplateConfig := kpScaler.PCluster.GetKpTemplateConfig(kpConfig.KpNodeTemplateName)
	// fmt.Println(kpTemplateConfig.Cores)
}
