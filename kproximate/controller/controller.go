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
	go scaleUp(kpScaler, scaleUpChannel, scaleUpQueue, mgmtClient)

	scaleDownChannel := internal.NewChannel(conn)
	defer scaleDownChannel.Close()
	
	scaleDownQueue := internal.DeclareQueue(scaleDownChannel, "scaleDownEvents")
	go scaleDown(kpScaler, scaleDownChannel, scaleDownQueue, mgmtClient)

	var forever chan struct{}
	<-forever
}

func scaleUp(scaler *scaler.KProximateScaler, channel *amqp.Channel, queue *amqp.Queue, mgmtClient *http.Client) {
	for {
		pendingScaleUpEvents := getQueueState(channel, queue.Name)
		runningScaleUpEvents := internal.GetUnackedMessages(mgmtClient, scaler.Config.RabbitMQHost, scaler.Config.RabbitMQUser, scaler.Config.RabbitMQPassword, queue.Name)
		allScaleUpEvents := pendingScaleUpEvents + runningScaleUpEvents

		if scaler.NumKpNodes()+allScaleUpEvents < scaler.Config.MaxKpNodes {
			scaleEvents := scaler.AssessScaleUp(&allScaleUpEvents)

			for _, scaleUpEvent := range scaleEvents {
				msg, err := json.Marshal(scaleUpEvent)
				err = sendScaleEventMsg(msg, channel, queue.Name)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to publish a message: %s", err)
				}
				logger.InfoLog.Printf("Requested scale up event: %s", scaleUpEvent.KpNodeName)
			}
		}

		time.Sleep(time.Duration(scaler.Config.PollInterval) * time.Second)
	}
}

func scaleDown(scaler *scaler.KProximateScaler, channel *amqp.Channel, queue *amqp.Queue, mgmtClient *http.Client) {
	for {
		pendingScaleUpEvents := getQueueState(channel, "scaleUpEvents")
		runningScaleUpEvents := internal.GetUnackedMessages(mgmtClient, scaler.Config.RabbitMQHost, scaler.Config.RabbitMQUser, scaler.Config.RabbitMQPassword, queue.Name)
		allScaleUpEvents := pendingScaleUpEvents + runningScaleUpEvents

		pendingScaleDownEvents := getQueueState(channel, queue.Name)
		runningScaleDownEvents := internal.GetUnackedMessages(mgmtClient, scaler.Config.RabbitMQHost, scaler.Config.RabbitMQUser, scaler.Config.RabbitMQPassword, queue.Name)
		allScaleDownEvents := pendingScaleDownEvents + runningScaleDownEvents

		allEvents := allScaleUpEvents + allScaleDownEvents

		if allEvents == 0 || scaler.NumKpNodes() != 0 {
			scaleDownEvent := scaler.AssessScaleDown()
			if scaleDownEvent != nil {
				msg, err := json.Marshal(scaleDownEvent)
				err = sendScaleEventMsg(msg, channel, queue.Name)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to publish a message: %s", err)
				}
				logger.InfoLog.Printf("Requested scale down event: %s", scaleDownEvent.KpNodeName)
			}
		}

		time.Sleep(time.Duration(scaler.Config.PollInterval) * time.Second)
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

func getQueueState(scaleUpChannel *amqp.Channel, queueName string) int {
	scaleEvents, err := scaleUpChannel.QueueInspect(queueName)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to find queue length: %s", err)
	}

	return scaleEvents.Messages
}
