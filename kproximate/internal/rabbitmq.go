package internal

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/lupinelab/kproximate/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type queueInfo struct {
	MessagesUnacknowledged int `json:"messages_unacknowledged,omitempty"`
}

func NewRabbitmqConnection(rabbitMQHost string, rabbitMQPort int, rabbitMQUser string, rabbitMQPassword string) (*amqp.Connection, *http.Client) {
	tls := &tls.Config{InsecureSkipVerify: true}

	rabbitMQUrl := fmt.Sprintf("amqps://%s:%s@%s:%d/", rabbitMQUser, rabbitMQPassword, rabbitMQHost, rabbitMQPort)

	conn, err := amqp.DialTLS(rabbitMQUrl, tls)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to connect to RabbitMQ: %s", err)
	}

	tr := &http.Transport{
		TLSClientConfig: tls,
	}
	mgmtClient := &http.Client{
		Transport: tr,
	}

	return conn, mgmtClient
}

func NewChannel(conn *amqp.Connection) *amqp.Channel {
	ch, err := conn.Channel()
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to open a channel: %s", err)
	}

	return ch
}

func DeclareQueue(ch *amqp.Channel, queueName string) *amqp.Queue {
	args := amqp.Table{
		"x-queue-type":     "quorum",
		"x-delivery-limit": 2,
	}

	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		args,      // arguments
	)
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to declare a queue: %s", err)
	}

	return &q
}

func GetUnackedMessages(client *http.Client, rabbitMQHost string, rabbitMQUser string, rabbitMQPassword string, queueName string) int {
	endpoint := fmt.Sprintf("http://%s:15672/api/queues/%s/%s", rabbitMQHost, url.PathEscape("/"), queueName)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		logger.ErrorLog.Fatalf("Could not build queue query: %s", err.Error())
	}

	req.Close = true
	req.SetBasicAuth(rabbitMQUser, rabbitMQPassword)

	res, err := client.Do(req)
	if err != nil {
		logger.ErrorLog.Fatalf("Could not query queue: %s", err.Error())
	}
	defer res.Body.Close()

	var queueInfo queueInfo

	err = json.NewDecoder(res.Body).Decode(&queueInfo)
	if err != nil {
		logger.ErrorLog.Fatalf("Could not decode queue query response: %s", err.Error())
	}

	return queueInfo.MessagesUnacknowledged
}
