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

	conn, _ := internal.NewRabbitmqConnection(
		kpScaler.Config.RabbitMQHost,
		kpScaler.Config.RabbitMQPort,
		kpScaler.Config.RabbitMQUser,
		kpScaler.Config.RabbitMQPassword,
	)
	defer conn.Close()

	scaleUpChannel := internal.NewChannel(conn)
	defer scaleUpChannel.Close()
	scaleUpQueue := internal.DeclareQueue(scaleUpChannel, "scaleUpEvents")
	err := scaleUpChannel.Qos(
		1,
		0,
		false,
	)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to set scale up QoS: %s", err)
	}

	scaleDownChannel := internal.NewChannel(conn)
	defer scaleDownChannel.Close()
	scaleDownQueue := internal.DeclareQueue(scaleUpChannel, "scaleDownEvents")
	err = scaleDownChannel.Qos(
		1,
		0,
		false,
	)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to set scale down QoS: %s", err)
	}

	scaleUpMsgs, err := scaleUpChannel.Consume(
		scaleUpQueue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to register scale up consumer: %s", err)
	}

	scaleDownMsgs, err := scaleDownChannel.Consume(
		scaleDownQueue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to register scale down consumer: %s", err)
	}

	go consumeScaleUpMsgs(scaleUpMsgs, kpScaler)

	go consumeScaleDownMsgs(scaleDownMsgs, kpScaler)

	logger.InfoLog.Printf("Listening for scale events")

	var forever chan struct{}
	<-forever
}

func consumeScaleUpMsgs(scaleUpMsgs <-chan amqp091.Delivery, kpScaler *scaler.Scaler) {
	for scaleUpMsg := range scaleUpMsgs {
		var scaleUpEvent *scaler.ScaleEvent
		json.Unmarshal(scaleUpMsg.Body, &scaleUpEvent)

		if scaleUpMsg.Redelivered {
			kpScaler.DeleteKpNode(scaleUpEvent.KpNodeName)
			logger.InfoLog.Printf("Retrying scale up event: %s", scaleUpEvent.KpNodeName)
		}

		logger.InfoLog.Printf("Triggered scale up event: %s", scaleUpEvent.KpNodeName)
		ctx := context.Background()
		err := kpScaler.ScaleUp(ctx, scaleUpEvent)

		if err != nil {
			logger.WarningLog.Printf("Scale up event failed:, %s", err.Error())
			kpScaler.DeleteKpNode(scaleUpEvent.KpNodeName)
			scaleUpMsg.Reject(true)
		}

		scaleUpMsg.Ack(false)
	}
}

func consumeScaleDownMsgs(scaleDownMsgs <-chan amqp091.Delivery, kpScaler *scaler.Scaler) {
	for scaleDownMsg := range scaleDownMsgs {
		var scaleDownEvent *scaler.ScaleEvent
		json.Unmarshal(scaleDownMsg.Body, &scaleDownEvent)

		if scaleDownMsg.Redelivered {
			logger.InfoLog.Printf("Retrying scale down event: %s", scaleDownEvent.KpNodeName)
		}

		logger.InfoLog.Printf("Triggered scale down event: %s", scaleDownEvent.KpNodeName)
		ctx := context.Background()
		err := kpScaler.ScaleDown(ctx, scaleDownEvent)

		if err != nil {
			logger.WarningLog.Printf("Scale down event failed:, %s", err.Error())
			scaleDownMsg.Reject(true)
		}

		scaleDownMsg.Ack(false)
	}
}
