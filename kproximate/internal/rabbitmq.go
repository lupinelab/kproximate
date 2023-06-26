package internal

import (
	"github.com/lupinelab/kproximate/logger"
	rabbithole "github.com/michaelklishin/rabbit-hole"
	amqp "github.com/rabbitmq/amqp091-go"
)

func NewRabbitmqConnection() (*amqp.Connection, *rabbithole.Client) {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to connect to RabbitMQ: %s", err)
	}

	rhconn, err := rabbithole.NewClient("http://localhost:15672", "guest", "guest")
	if err != nil {
		logger.ErrorLog.Fatalf("Failed to connect to RabbitMQ: %s", err)
	}

	return conn, rhconn
}

func NewScaleUpChannel(conn *amqp.Connection) *amqp.Channel {
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
