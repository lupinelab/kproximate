package main

import "github.com/lupinelab/kproximate/scaler"

func main() {
	kpConfig := scaler.GetConfig()
	kpScaler := scaler.NewScaler(*kpConfig)

	kpScaler.Start()
}
