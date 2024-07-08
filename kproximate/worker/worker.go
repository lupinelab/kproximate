package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/logger"
	"github.com/lupinelab/kproximate/rabbitmq"
	"github.com/lupinelab/kproximate/scaler"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	kpConfig, err := config.GetKpConfig()
	if err != nil {
		logger.ErrorLog("Failed to get config", "error", err)
	}

	logger.ConfigureLogger("worker", kpConfig.Debug)

	scaler, err := scaler.NewProxmoxScaler(kpConfig)
	if err != nil {
		logger.ErrorLog("Failed to initialise scaler", "error", err)
	}

	rabbitConfig, err := config.GetRabbitConfig()
	if err != nil {
		logger.ErrorLog("Failed to get rabbit config", "error", err)
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
		logger.ErrorLog("Failed to set scale up QoS", "error", err)
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
		logger.ErrorLog("Failed to set scale down QoS", "error", err)
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
		logger.ErrorLog("Failed to register scale up consumer", "error", err)
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
		logger.ErrorLog("Failed to register scale down consumer", "error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	go func() {
		<-sigChan
		cancel()
	}()

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	logger.InfoLog("Listening for scale events")

	for {
		select {
		case scaleUpMsg := <-scaleUpMsgs:
			consumeScaleUpMsg(ctx, scaler, scaleUpMsg)

		case scaleDownMsg := <-scaleDownMsgs:
			consumeScaleDownMsg(ctx, scaler, scaleDownMsg)

		case <-ctx.Done():
			return
		}
	}
}

func consumeScaleUpMsg(ctx context.Context, kpScaler scaler.Scaler, scaleUpMsg amqp.Delivery) {
	var scaleUpEvent *scaler.ScaleEvent
	json.Unmarshal(scaleUpMsg.Body, &scaleUpEvent)

	if scaleUpMsg.Redelivered {
		kpScaler.DeleteNode(ctx, scaleUpEvent.NodeName)
		logger.InfoLog(fmt.Sprintf("Retrying scale up event: %s", scaleUpEvent.NodeName))
	} else {
		logger.InfoLog(fmt.Sprintf("Triggered scale up event: %s", scaleUpEvent.NodeName))
	}

	err := kpScaler.ScaleUp(ctx, scaleUpEvent)
	if err != nil {
		logger.WarnLog("Scale up event failed", "error", err.Error())
		kpScaler.DeleteNode(ctx, scaleUpEvent.NodeName)
		scaleUpMsg.Reject(true)
		return
	}

	scaleUpMsg.Ack(false)
}

func consumeScaleDownMsg(ctx context.Context, kpScaler scaler.Scaler, scaleDownMsg amqp.Delivery) {
	var scaleDownEvent *scaler.ScaleEvent
	json.Unmarshal(scaleDownMsg.Body, &scaleDownEvent)

	if scaleDownMsg.Redelivered {
		logger.InfoLog(fmt.Sprintf("Retrying scale down event: %s", scaleDownEvent.NodeName))
	} else {
		logger.InfoLog(fmt.Sprintf("Triggered scale down event: %s", scaleDownEvent.NodeName))
	}

	scaleCtx, scaleCancel := context.WithDeadline(ctx, time.Now().Add(time.Second*300))
	defer scaleCancel()

	err := kpScaler.ScaleDown(scaleCtx, scaleDownEvent)
	if err != nil {
		logger.WarnLog(fmt.Sprintf("Scale down event failed: %s", err.Error()))
		scaleDownMsg.Reject(true)
		return
	}

	logger.InfoLog(fmt.Sprintf("Deleted %s", scaleDownEvent.NodeName))
	scaleDownMsg.Ack(false)
}
