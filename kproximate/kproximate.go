package main

import (
	"fmt"

	"github.com/lupinelab/kproximate/scaler"
)

func main() {
	kpsConfig := scaler.GetConfig()
	kpsScaler := scaler.NewScaler(*kpsConfig)

	kpsScaler.Start()

	// P
	kpnodes := kpsScaler.PCluster.GetKpsNodes()
	for _, kpnode := range kpnodes {
		fmt.Println(kpnode.Name)
	}

	kpsScaler.PCluster.GetClusterStats()

	// K
	kpsScaler.KCluster.GetUnschedulableResources()

	// New KPNode
	// fmt.Println(pClient.NewKpNode(kpNodeTemplateName, "qtiny-02"))

	kpsTemplateConfig := kpsScaler.PCluster.GetKpsTemplateConfig(kpsConfig.KpsNodeTemplateName)
	fmt.Println(kpsTemplateConfig.Cores)
}
