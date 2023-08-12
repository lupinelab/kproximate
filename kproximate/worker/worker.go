package main

import (
	"context"
	"encoding/json"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/rabbitmq"
	"github.com/lupinelab/kproximate/logger"
	"github.com/lupinelab/kproximate/scaler"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	kpConfig, err := config.GetKpConfig()
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to get config: %s", err.Error())
	}

	scaler, err := scaler.NewScaler(kpConfig)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to initialise scaler: %s", err.Error())
	}

	rabbitConfig, err := config.GetRabbitConfig()
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to get rabbit config: %s", err.Error())
	}

	conn, _ := rabbitmq.NewRabbitmqConnection(rabbitConfig)
	defer conn.Close()

	scaleUpChannel := rabbitmq.NewChannel(conn)
	defer scaleUpChannel.Close()
	scaleUpQueue := rabbitmq.DeclareQueue(scaleUpChannel, "scaleUpEvents")
	err = scaleUpChannel.Qos(
		1,
		0,
		false,
	)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to set scale up QoS: %s", err)
	}

	scaleDownChannel := rabbitmq.NewChannel(conn)
	defer scaleDownChannel.Close()
	scaleDownQueue := rabbitmq.DeclareQueue(scaleUpChannel, "scaleDownEvents")
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

	ctx := context.Background()
	go consumeScaleUpMsgs(ctx, scaler, scaleUpMsgs)
	go consumeScaleDownMsgs(ctx, scaler, scaleDownMsgs)

	logger.InfoLog.Println("Listening for scale events")

	<-ctx.Done()
}

func consumeScaleUpMsgs(ctx context.Context, kpScaler *scaler.Scaler, scaleUpMsgs <-chan amqp.Delivery) {
	for scaleUpMsg := range scaleUpMsgs {
		var scaleUpEvent *scaler.ScaleEvent
		json.Unmarshal(scaleUpMsg.Body, &scaleUpEvent)

		if scaleUpMsg.Redelivered {
			kpScaler.DeleteKpNode(scaleUpEvent.NodeName)
			logger.InfoLog.Printf("Retrying scale up event: %s", scaleUpEvent.NodeName)
		} else {
			logger.InfoLog.Printf("Triggered scale up event: %s", scaleUpEvent.NodeName)
		}

		scaleCtx := context.Background()
		err := kpScaler.ScaleUp(scaleCtx, scaleUpEvent)
		if err != nil {
			logger.WarningLog.Printf("Scale up event failed: %s", err.Error())
			kpScaler.DeleteKpNode(scaleUpEvent.NodeName)
			scaleUpMsg.Reject(true)
			continue
		}

		scaleUpMsg.Ack(false)
	}
}

func consumeScaleDownMsgs(ctx context.Context, kpScaler *scaler.Scaler, scaleDownMsgs <-chan amqp.Delivery) {
	for scaleDownMsg := range scaleDownMsgs {
		var scaleDownEvent *scaler.ScaleEvent
		json.Unmarshal(scaleDownMsg.Body, &scaleDownEvent)

		if scaleDownMsg.Redelivered {
			logger.InfoLog.Printf("Retrying scale down event: %s", scaleDownEvent.NodeName)
		} else {
			logger.InfoLog.Printf("Triggered scale down event: %s", scaleDownEvent.NodeName)
		}

		scaleCtx := context.Background()
		err := kpScaler.ScaleDown(scaleCtx, scaleDownEvent)
		if err != nil {
			logger.WarningLog.Printf("Scale down event failed: %s", err.Error())
			scaleDownMsg.Reject(true)
			continue
		}
		logger.InfoLog.Printf("Deleted %s", scaleDownEvent.NodeName)

		scaleDownMsg.Ack(false)
	}
}
