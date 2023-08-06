package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/internal"
	"github.com/lupinelab/kproximate/logger"
	"github.com/lupinelab/kproximate/scaler"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	config := config.GetConfig()
	kpScaler := scaler.NewScaler(config)

	conn, mgmtClient := internal.NewRabbitmqConnection(
		kpScaler.Config.RabbitMQHost,
		kpScaler.Config.RabbitMQPort,
		kpScaler.Config.RabbitMQUser,
		kpScaler.Config.RabbitMQPassword,
	)
	defer conn.Close()

	scaleUpChannel := internal.NewChannel(conn)
	defer scaleUpChannel.Close()
	scaleUpQueue := internal.DeclareQueue(scaleUpChannel, "scaleUpEvents")
	go AssessScaleUp(kpScaler, scaleUpChannel, scaleUpQueue, mgmtClient)

	scaleDownChannel := internal.NewChannel(conn)
	defer scaleDownChannel.Close()
	scaleDownQueue := internal.DeclareQueue(scaleDownChannel, "scaleDownEvents")
	go AssessScaleDown(kpScaler, scaleDownChannel, scaleDownQueue, mgmtClient)

	logger.InfoLog.Println("Controller started")

	var forever chan struct{}
	<-forever
}

func AssessScaleUp(scaler *scaler.Scaler, scaleUpChannel *amqp.Channel, scaleUpQueue *amqp.Queue, mgmtClient *http.Client) {
	for {
		pendingScaleUpEvents := scaleUpQueue.Messages
		runningScaleUpEvents := internal.GetUnAckedMessages(mgmtClient, scaler.Config.RabbitMQHost, scaler.Config.RabbitMQUser, scaler.Config.RabbitMQPassword, scaleUpQueue.Name)
		allScaleUpEvents := pendingScaleUpEvents + runningScaleUpEvents

		if scaler.NumKpNodes()+allScaleUpEvents < scaler.Config.MaxKpNodes {
			unschedulableResources, err := scaler.KCluster.GetUnschedulableResources()
			if err != nil {
				logger.ErrorLog.Fatalf("Assess scale up failed, unable to get unschedulable resources: %s", err.Error())
			}

			scaleUpEvents := scaler.RequiredScaleEvents(unschedulableResources, allScaleUpEvents)
			if len(scaleUpEvents) > 0 {
				scaler.SelectTargetPHosts(scaleUpEvents)
			}

			for _, scaleUpEvent := range scaleUpEvents {
				msg, err := json.Marshal(scaleUpEvent)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to marshal scale up event: %s", err)
				}
				err = sendScaleEventMsg(msg, scaleUpChannel, scaleUpQueue.Name)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to publish a message: %s", err)
				}
				logger.InfoLog.Printf("Requested scale up event: %s", scaleUpEvent.KpNodeName)
				time.Sleep(time.Second * 1)
			}
		}

		time.Sleep(time.Second * time.Duration(scaler.Config.PollInterval))
	}
}

func AssessScaleDown(scaler *scaler.Scaler, scaleDownChannel *amqp.Channel, scaleDownQueue *amqp.Queue, mgmtClient *http.Client) {
	for {
		pendingScaleUpEvents := internal.GetQueueState(scaleDownChannel, "scaleUpEvents")
		runningScaleUpEvents := internal.GetUnAckedMessages(mgmtClient, scaler.Config.RabbitMQHost, scaler.Config.RabbitMQUser, scaler.Config.RabbitMQPassword, "scaleUpEvents")
		allScaleUpEvents := pendingScaleUpEvents + runningScaleUpEvents

		pendingScaleDownEvents := scaleDownQueue.Messages
		runningScaleDownEvents := internal.GetUnAckedMessages(mgmtClient, scaler.Config.RabbitMQHost, scaler.Config.RabbitMQUser, scaler.Config.RabbitMQPassword, scaleDownQueue.Name)
		allScaleDownEvents := pendingScaleDownEvents + runningScaleDownEvents

		allEvents := allScaleUpEvents + allScaleDownEvents
		numKpNodes := scaler.NumKpNodes()

		if (allEvents == 0 && numKpNodes > 0) {
			allocatedResources, err := scaler.KCluster.GetAllocatedResources()
			if err != nil {
				logger.ErrorLog.Fatalf("Unable to get allocated resources: %s", err.Error())
			}

			scaleDownEvent := scaler.AssessScaleDown(allocatedResources, numKpNodes)
			if scaleDownEvent != nil {
				kpNodes, err := scaler.KCluster.GetKpNodes()
				if err != nil {
					logger.ErrorLog.Fatalf("Unable get kp-nodes: %s", err.Error())
				}

				scaler.SelectScaleDownTarget(scaleDownEvent, allocatedResources, kpNodes)
				msg, err := json.Marshal(scaleDownEvent)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to marshal scale down event: %s", err)
				}

				err = sendScaleEventMsg(msg, scaleDownChannel, scaleDownQueue.Name)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to publish a message: %s", err)
				}
				logger.InfoLog.Printf("Requested scale down event: %s", scaleDownEvent.KpNodeName)
			}
		}

		time.Sleep(time.Second * time.Duration(scaler.Config.PollInterval))
	}
}

func sendScaleEventMsg(msg []byte, channel *amqp.Channel, queueName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return channel.PublishWithContext(ctx,
		"",
		queueName,
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         []byte(msg),
		})
}
