package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/internal"
	"github.com/lupinelab/kproximate/logger"
	"github.com/lupinelab/kproximate/scaler"
	rabbithole "github.com/michaelklishin/rabbit-hole"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	config := config.GetConfig()
	kpScaler := scaler.NewScaler(config)

	conn, rhconn := internal.NewRabbitmqConnection()
	defer conn.Close()

	scaleUpChannel := internal.NewChannel(conn)
	defer scaleUpChannel.Close()
	scaleUpQueue := internal.DeclareQueue(scaleUpChannel, "scaleUpEvents")
	go scaleUp(kpScaler, scaleUpChannel, scaleUpQueue, rhconn)

	scaleDownChannel := internal.NewChannel(conn)
	defer scaleDownChannel.Close()
	scaleDownQueue := internal.DeclareQueue(scaleDownChannel, "scaleDownEvents")
	go scaleDown(kpScaler, scaleDownChannel, scaleDownQueue, rhconn)
	
	var forever chan struct{}
	<-forever
}

func scaleUp(scaler *scaler.KProximateScaler, channel *amqp.Channel, queue *amqp.Queue, rhconn *rabbithole.Client) {
	for {
		pendingScaleUpEvents := GetQueueState(channel, queue.Name)
		runningScaleUpEvents := GetUnackedMessage(rhconn, queue.Name)
		allScaleUpEvents := pendingScaleUpEvents + runningScaleUpEvents

		if scaler.NumKpNodes()+allScaleUpEvents < scaler.Config.MaxKpNodes {
			scaleEvents := scaler.AssessScaleUp(&allScaleUpEvents)

			for _, scaleUpEvent := range scaleEvents {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				msg, err := json.Marshal(scaleUpEvent)
				err = channel.PublishWithContext(ctx,
					"",         // exchange
					queue.Name, // routing key
					false,      // mandatory
					false,
					amqp.Publishing{
						DeliveryMode: amqp.Persistent,
						ContentType:  "application/json",
						Body:         []byte(msg),
					})
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to publish a message: %s", err)
				}
				logger.InfoLog.Printf("Requested scale up event: %s", scaleUpEvent.KpNodeName)
			}
		}

		time.Sleep(time.Duration(scaler.Config.PollInterval) * time.Second)
	}
}

func scaleDown(scaler *scaler.KProximateScaler, channel *amqp.Channel, queue *amqp.Queue, rhconn *rabbithole.Client) {
	for {
		pendingScaleUpEvents := GetQueueState(channel, "scaleUpEvents")
		runningScaleUpEvents := GetUnackedMessage(rhconn, "scaleUpEvents")
		allScaleUpEvents := pendingScaleUpEvents + runningScaleUpEvents

		pendingScaleDownEvents := GetQueueState(channel, queue.Name)
		runningScaleDownEvents := GetUnackedMessage(rhconn, queue.Name)
		allScaleDownEvents := pendingScaleDownEvents + runningScaleDownEvents

		allEvents := allScaleUpEvents + allScaleDownEvents

		if allEvents == 0 || scaler.NumKpNodes() != 0 {
			scaleDownEvent := scaler.AssessScaleDown()
			if scaleDownEvent != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				msg, err := json.Marshal(scaleDownEvent)
				err = channel.PublishWithContext(ctx,
					"",         // exchange
					queue.Name, // routing key
					false,      // mandatory
					false,
					amqp.Publishing{
						DeliveryMode: amqp.Persistent,
						ContentType:  "application/json",
						Body:         []byte(msg),
					})
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to publish a message: %s", err)
				}
				logger.InfoLog.Printf("Requested scale down event: %s", scaleDownEvent.KpNodeName)
			}
		}

		time.Sleep(time.Duration(scaler.Config.PollInterval) * time.Second)
	}
}

func GetQueueState(scaleUpChannel *amqp.Channel, queueName string) int {
	scaleEvents, err := scaleUpChannel.QueueInspect(queueName)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to find queue length: %s", err)
	}

	return scaleEvents.Messages
}

func GetUnackedMessage(rhconn *rabbithole.Client, queueName string) int {
	queueInfo, err := rhconn.GetQueue("/", queueName)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to find queue info: %s", err)
	}
	return queueInfo.MessagesUnacknowledged
}
