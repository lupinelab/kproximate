package main

import (
	"context"
	"encoding/json"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/internal"
	"github.com/lupinelab/kproximate/logger"
	"github.com/lupinelab/kproximate/scaler"
	"github.com/rabbitmq/amqp091-go"
)

func main() {
	config := config.GetConfig()
	kpScaler := scaler.NewScaler(config)

	conn, _ := internal.NewRabbitmqConnection()
	defer conn.Close()

	scaleUpChannel := internal.NewScaleUpChannel(conn)
	defer scaleUpChannel.Close()

	scaleUpQueue := internal.DeclareQueue(scaleUpChannel, "scaleUpEvents")

	err := scaleUpChannel.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to set QoS: %s", err)
	}

	scaleUpMsgs, err := scaleUpChannel.Consume(
		scaleUpQueue.Name, // queue
		"",                // consumer
		false,             // auto-ack
		false,             // exclusive
		false,             // no-local
		false,             // no-wait
		nil,               // args
	)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to register a consumer: %s", err)
	}

	var forever chan struct{}

	go consumeScaleUpMsgs(scaleUpMsgs, kpScaler)

	logger.InfoLog.Printf("Listening for messages")
	<-forever
}

func consumeScaleUpMsgs(scaleUpMsgs <-chan amqp091.Delivery, kpScaler *scaler.KProximateScaler) {
	for scaleUpMsg := range scaleUpMsgs {
		var scaleEvent *scaler.ScaleEvent
		json.Unmarshal(scaleUpMsg.Body, &scaleEvent)

		if scaleUpMsg.Redelivered {
			kpScaler.DeleteKpNode(scaleEvent.KpNodeName)
			logger.InfoLog.Printf("Retrying scale up event: %s", scaleEvent.KpNodeName)
		}

		logger.InfoLog.Printf("Triggered scale up event: %s", scaleEvent.KpNodeName)
		ctx := context.Background()
		err := kpScaler.ScaleUp(ctx, scaleEvent)

		if err != nil {
			kpScaler.DeleteKpNode(scaleEvent.KpNodeName)
			scaleUpMsg.Reject(true)
		}

		scaleUpMsg.Ack(false)
	}
}
