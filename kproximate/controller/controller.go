package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/logger"
	"github.com/lupinelab/kproximate/metrics"
	"github.com/lupinelab/kproximate/rabbitmq"
	"github.com/lupinelab/kproximate/scaler"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	kpConfig, err := config.GetKpConfig()
	if err != nil {
		logger.FatalLog("Failed to get config", err)
	}

	logger.ConfigureLogger("controller", kpConfig.Debug)

	scaler, err := scaler.NewProxmoxScaler(kpConfig)
	if err != nil {
		logger.FatalLog("Failed to initialise scaler", err)
	}

	rabbitConfig, err := config.GetRabbitConfig()
	if err != nil {
		logger.FatalLog("Failed to get rabbit config", err)
	}

	conn, mgmtClient := rabbitmq.NewRabbitmqConnection(rabbitConfig)
	defer conn.Close()

	scaleUpChannel := rabbitmq.NewChannel(conn)
	defer scaleUpChannel.Close()
	scaleUpQueue := rabbitmq.DeclareQueue(scaleUpChannel, "scaleUpEvents")

	scaleDownChannel := rabbitmq.NewChannel(conn)
	defer scaleDownChannel.Close()
	scaleDownQueue := rabbitmq.DeclareQueue(scaleDownChannel, "scaleDownEvents")

	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	go func() {
		<-sigChan
		cancel()
	}()

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go metrics.Serve(ctx, scaler, kpConfig)
	logger.InfoLog("Started")
	for {
		select {
		case <-ctx.Done():
			return
		default:
			time.Sleep(time.Second * time.Duration(kpConfig.PollInterval))

			assessScaleUp(ctx, scaler, kpConfig, rabbitConfig, scaleUpChannel, scaleUpQueue, mgmtClient)
			assessScaleDown(ctx, scaler, rabbitConfig, scaleDownChannel, scaleDownQueue, mgmtClient)
		}
	}

}

func assessScaleUp(
	ctx context.Context,
	scaler scaler.Scaler,
	config config.KproximateConfig,
	rabbitConfig config.RabbitConfig,
	scaleUpChannel *amqp.Channel,
	scaleUpQueue *amqp.Queue,
	mgmtClient *http.Client,
) {

	logger.DebugLog("Assessing for scale up")
	allScaleEvents, err := countScalingEvents(
		[]string{"scaleUpEvents"},
		scaleUpChannel,
		mgmtClient,
		rabbitConfig,
	)
	if err != nil {
		logger.FatalLog("Failed to count scaling events", err)
	}

	numKpNodes, err := scaler.NumReadyNodes()
	if err != nil {
		logger.FatalLog("Failed to get kproximate nodes", err)
	}

	if numKpNodes+allScaleEvents < config.MaxKpNodes {
		logger.DebugLog("Calculating required scale events")
		scaleUpEvents, err := scaler.RequiredScaleEvents(allScaleEvents)
		if err != nil {
			logger.FatalLog("Failed to calculate required scale events", err)
		}

		if len(scaleUpEvents) > 0 {
			maxScaleEvents := config.MaxKpNodes - (numKpNodes + allScaleEvents)
			numScaleEvents := min(maxScaleEvents, len(scaleUpEvents))
			scaleUpEvents = scaleUpEvents[0:numScaleEvents]
			logger.DebugLog("Selecting target hosts")
			err = scaler.SelectTargetHosts(scaleUpEvents)
			if err != nil {
				logger.FatalLog("Failed to select target host", err)
			}
		} else {
			logger.DebugLog("No scale up events required")
		}

		for _, scaleUpEvent := range scaleUpEvents {
			logger.DebugLog("Generated scale event", "scaleEvent", fmt.Sprintf("%+v", scaleUpEvent))
			err = queueScaleEvent(ctx, scaleUpEvent, scaleUpChannel, scaleUpQueue.Name)
			if err != nil {
				logger.ErrorLog("Failed to queue scale up event", err)
			}

			logger.InfoLog(fmt.Sprintf("Requested scale up event: %s", scaleUpEvent.NodeName))

			time.Sleep(time.Second * 1)
		}
	} else {
		logger.DebugLog("Reached maxKpNodes")
	}

}

func assessScaleDown(
	ctx context.Context,
	scaler scaler.Scaler,
	rabbitConfig config.RabbitConfig,
	scaleDownChannel *amqp.Channel,
	scaleDownQueue *amqp.Queue,
	mgmtClient *http.Client,
) {
	logger.DebugLog("Assessing for scale down")
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
		logger.FatalLog("Failed to count scale events", err)
	}

	numKpNodes, err := scaler.NumReadyNodes()
	if err != nil {
		logger.FatalLog("Failed to get kproximate nodes", err)
	}

	if allScaleEvents == 0 && numKpNodes > 0 {
		logger.DebugLog("Calculating required scale events")
		scaleDownEvent, err := scaler.AssessScaleDown()
		if err != nil {
			logger.ErrorLog(fmt.Sprintf("Failed to assess scale down: %s", err))
		}
		if scaleDownEvent != nil {
			err = queueScaleEvent(ctx, scaleDownEvent, scaleDownChannel, scaleDownQueue.Name)
			if err != nil {
				logger.ErrorLog(fmt.Sprintf("Failed to queue scale down event: %s", err))
			}

			logger.InfoLog(fmt.Sprintf("Requested scale down event: %s", scaleDownEvent.NodeName))
		} else {
			logger.DebugLog("No scale down events required")
		}
	} else {
		logger.DebugLog("Cannot scale down, scale event in progress or 0 kpNodes in cluster")
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

func queueScaleEvent(ctx context.Context, scaleEvent *scaler.ScaleEvent, channel *amqp.Channel, queueName string) error {
	msg, err := json.Marshal(scaleEvent)
	if err != nil {
		return err
	}

	queueCtx, queueCancel := context.WithTimeout(ctx, 5*time.Second)
	defer queueCancel()
	return channel.PublishWithContext(
		queueCtx,
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
