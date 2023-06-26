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

	scaleUpChannel := internal.NewScaleUpChannel(conn)
	defer scaleUpChannel.Close()

	scaleUpQueue := internal.DeclareQueue(scaleUpChannel, "scaleUpEvents")

	var forever chan struct{}

	go scaleUp(kpScaler, scaleUpChannel, scaleUpQueue, rhconn)

	<-forever
}

func scaleUp(scaler *scaler.KProximateScaler, channel *amqp.Channel, queue *amqp.Queue, rhconn *rabbithole.Client) {
	for {
		pendingEvents := GetQueueState(channel, queue.Name)
		runningEvents := GetUnackedMessage(rhconn, queue.Name)
		queuedEvents := pendingEvents + runningEvents

		if scaler.NumKpNodes()+queuedEvents < scaler.Config.MaxKpNodes {
			scaleEvents := scaler.AssessScaleUp(&queuedEvents)

			for _, scaleEvent := range scaleEvents {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				msg, err := json.Marshal(scaleEvent)
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
				logger.InfoLog.Printf("Requested scale up event: %s", scaleEvent.KpNodeName)
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
