package main

import (
	"github.com/candacelabs/candace/app/candaceos-core/bootstrap"

	"example.com/candace-external-consumer/customharness"
	"example.com/candace-external-consumer/steering"
)

func main() {
	steeringStore, err := steering.StoreComponent()
	if err != nil {
		panic(err)
	}
	steeringService, err := steering.ServiceComponent(steeringStore)
	if err != nil {
		panic(err)
	}
	if err := bootstrap.Run(
		"external-consumer",
		bootstrap.WithComponent(steeringStore),
		bootstrap.WithComponent(steeringService),
		bootstrap.WithHarnessFactory(customharness.NewFactory(steering.Instance())),
	); err != nil {
		panic(err)
	}
}
