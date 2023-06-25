package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/internal"
	"github.com/lupinelab/kproximate/scaler"
)

var (
	infoLog    *log.Logger
	warningLog *log.Logger
	errorLog   *log.Logger
)

func init() {
	workerName, err := os.Hostname()
	if err != nil {
		log.Panicf("Could not get worker name: %s", err)
	}
	infoLog = log.New(os.Stdout, fmt.Sprintf("%s INFO: ", workerName), log.Ldate|log.Ltime)
	warningLog = log.New(os.Stdout, fmt.Sprintf("%s WARNING: ", workerName), log.Ldate|log.Ltime)
	errorLog = log.New(os.Stdout, fmt.Sprintf("%s ERROR: ", workerName), log.Ldate|log.Ltime)
}

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
		log.Panicf("Failed to set QoS: %s", err)
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
		log.Panicf("Failed to register a consumer: %s", err)
	}

	var forever chan struct{}

	go func() {
		for scaleUpMsg := range scaleUpMsgs {
			var scaleEvent *scaler.ScaleEvent
			json.Unmarshal(scaleUpMsg.Body, &scaleEvent)

			ctx := context.Background()
			kpScaler.ScaleUp(ctx, scaleEvent)
			scaleUpMsg.Ack(false)
		}
	}()

	infoLog.Printf("Listening for messages")
	<-forever
}
