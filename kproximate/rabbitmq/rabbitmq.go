package rabbitmq

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/lupinelab/kproximate/config"
	"github.com/lupinelab/kproximate/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type queueInfo struct {
	MessagesUnacknowledged int `json:"messages_unacknowledged,omitempty"`
}

func NewRabbitmqConnection(rabbitConfig config.RabbitConfig) (*amqp.Connection, *http.Client) {
	tls := &tls.Config{InsecureSkipVerify: true}

	rabbitMQUrl := fmt.Sprintf("amqps://%s:%s@%s:%d/", rabbitConfig.User, rabbitConfig.Password, rabbitConfig.Host, rabbitConfig.Port)

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

func GetPendingScaleEvents(ch *amqp.Channel, queueName string) (int, error) {
	args := amqp.Table{
		"x-queue-type":     "quorum",
		"x-delivery-limit": 2,
	}
	scaleEvents, err := ch.QueueDeclarePassive(
		queueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		args,  // arguments
	)
	if err != nil {
		return 0, err
	}

	return scaleEvents.Messages, nil
}

func GetRunningScaleEvents(client *http.Client, rabbitConfig config.RabbitConfig, queueName string) (int, error) {
	endpoint := fmt.Sprintf("http://%s:15672/api/queues/%s/%s", rabbitConfig.Host, url.PathEscape("/"), queueName)
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return 0, err
	}

	req.Close = true
	req.SetBasicAuth(rabbitConfig.User, rabbitConfig.Password)

	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	var queueInfo queueInfo

	err = json.NewDecoder(res.Body).Decode(&queueInfo)
	if err != nil {
		return 0, err
	}

	return queueInfo.MessagesUnacknowledged, nil
}
