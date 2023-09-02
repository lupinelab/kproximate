package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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

	conn, mgmtClient := rabbitmq.NewRabbitmqConnection(rabbitConfig)
	defer conn.Close()

	scaleUpChannel := rabbitmq.NewChannel(conn)
	defer scaleUpChannel.Close()
	scaleUpQueue := rabbitmq.DeclareQueue(scaleUpChannel, "scaleUpEvents")

	scaleDownChannel := rabbitmq.NewChannel(conn)
	defer scaleDownChannel.Close()
	scaleDownQueue := rabbitmq.DeclareQueue(scaleDownChannel, "scaleDownEvents")

	ctx := context.Background()
	go AssessScaleUp(ctx, scaler, rabbitConfig, scaleUpChannel, scaleUpQueue, mgmtClient)
	go AssessScaleDown(ctx, scaler, rabbitConfig, scaleDownChannel, scaleDownQueue, mgmtClient)

	logger.InfoLog.Println("Controller started")

	<-ctx.Done()
}

func AssessScaleUp(
	ctx context.Context,
	scaler *scaler.Scaler,
	rabbitConfig config.RabbitConfig,
	scaleUpChannel *amqp.Channel,
	scaleUpQueue *amqp.Queue,
	mgmtClient *http.Client,
) {
	for {
		allScaleEvents, err := countScalingEvents(
			[]string{"scaleUpEvents"}, 
			scaleUpChannel,
			mgmtClient,
			rabbitConfig,
		)
		if err != nil {
			logger.ErrorLog.Fatalf("Failed to count scaling events: %s", err.Error())
		}

		numKpNodes, err := scaler.NumKpNodes()
		if err != nil {
			logger.ErrorLog.Fatalf("Failed to get kproximate nodes: %s", err.Error())
		}

		if numKpNodes+allScaleEvents < scaler.Config.MaxKpNodes {
			unschedulableResources, err := scaler.Kubernetes.GetUnschedulableResources()
			if err != nil {
				logger.ErrorLog.Fatalf("Failed to get unschedulable resources: %s", err.Error())
			}

			scaleUpEvents, err := scaler.RequiredScaleEvents(unschedulableResources, allScaleEvents)
			if err != nil {
				logger.ErrorLog.Fatalf("Failed to calculate required scale events: %s", err.Error())
			}

			if len(scaleUpEvents) > 0 {
				err = scaler.SelectTargetHosts(scaleUpEvents)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to select target host: %s", err.Error())
				}
			}

			for _, scaleUpEvent := range scaleUpEvents {
				err = queueScaleEvent(scaleUpEvent, scaleUpChannel, scaleUpQueue.Name)
				if err != nil {
					logger.ErrorLog.Printf("Failed to queue scale up event: %s", err)
				}

				logger.InfoLog.Printf("Requested scale up event: %s", scaleUpEvent.NodeName)

				time.Sleep(time.Second * 1)
			}
		}

		time.Sleep(time.Second * time.Duration(scaler.Config.PollInterval))
	}
}


func AssessScaleDown(
	ctx context.Context,
	scaler *scaler.Scaler,
	rabbitConfig config.RabbitConfig,
	scaleDownChannel *amqp.Channel,
	scaleDownQueue *amqp.Queue,
	mgmtClient *http.Client,
) {
	for {
		allScaleEvents, err := countScalingEvents(
			[]string{
				"scaleUpEvents",
				"scaleDownEvents",
			}, 
			scaleDownChannel,
			mgmtClient,
			rabbitConfig,
		)
		if err != nil {
			logger.ErrorLog.Fatalf("Failed to count scaling events: %s", err.Error())
		}

		numKpNodes, err := scaler.NumKpNodes()
		if err != nil {
			logger.ErrorLog.Fatalf("Failed to get kproximate nodes: %s", err.Error())
		}

		if allScaleEvents == 0 && numKpNodes > 0 {
			allocatedResources, err := scaler.Kubernetes.GetAllocatedResources(scaler.Config.KpNodeNameRegex)
			if err != nil {
				logger.ErrorLog.Fatalf("Failed to get allocated resources: %s", err.Error())
			}

			workerNodeCapacity, err := scaler.Kubernetes.GetworkerNodesAllocatableResources()
			if err != nil {
				logger.ErrorLog.Fatalf("Failed to get worker nodes capacity: %s", err.Error())
			}

			scaleDownEvent := scaler.AssessScaleDown(allocatedResources, workerNodeCapacity)
			if scaleDownEvent != nil {
				kpNodes, err := scaler.Kubernetes.GetKpNodes(scaler.Config.KpNodeNameRegex)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to get kp-nodes: %s", err.Error())
				}

				err = scaler.SelectScaleDownTarget(scaleDownEvent, allocatedResources, kpNodes)
				if err != nil {
					logger.ErrorLog.Fatalf("Failed to select target host: %s", err.Error())
				}

				err = queueScaleEvent(scaleDownEvent, scaleDownChannel, scaleDownQueue.Name)
				if err != nil {
					logger.ErrorLog.Printf("Failed to queue scale down event: %s", err)
				}

				logger.InfoLog.Printf("Requested scale down event: %s", scaleDownEvent.NodeName)
			}
		}

		time.Sleep(time.Second * time.Duration(scaler.Config.PollInterval))
	}
}

func countScalingEvents(
	queueNames []string,
	channel *amqp.Channel,
	mgmtClient *http.Client,
	rabbitConfig config.RabbitConfig,
) (int, error) {
	numScalingEvents := 0

	for _, queueName := range queueNames {
		pendingScaleEvents, err := rabbitmq.GetPendingScaleEvents(channel, queueName)
		if err != nil {
			return numScalingEvents, err
		}

		numScalingEvents += pendingScaleEvents

		runningScaleEvents, err := rabbitmq.GetRunningScaleEvents(mgmtClient, rabbitConfig, queueName)
		if err != nil {
			return numScalingEvents, err
		}

		numScalingEvents += runningScaleEvents
	}

	return numScalingEvents, nil
}

func queueScaleEvent(scaleEvent *scaler.ScaleEvent, channel *amqp.Channel, queueName string) error {
	msg, err := json.Marshal(scaleEvent)
	if err != nil {
		return err
	}

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
