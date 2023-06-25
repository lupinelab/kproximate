package internal

import (
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/michaelklishin/rabbit-hole"
)

func NewRabbitmqConnection() (*amqp.Connection, *rabbithole.Client) {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Panicf("Failed to connect to RabbitMQ: %s", err)
	}

	rhconn, err := rabbithole.NewClient("http://localhost:15672", "guest", "guest")
	if err != nil {
		log.Panicf("Failed to connect to RabbitMQ: %s", err)
	}

	return conn, rhconn
}

func NewScaleUpChannel(conn *amqp.Connection) *amqp.Channel {
	ch, err := conn.Channel()
	if err != nil {
		log.Panicf("Failed to open a channel: %s", err)
	}

	return ch
}

func DeclareQueue(ch *amqp.Channel, queueName string) *amqp.Queue {
	args := amqp.Table{
		"x-queue-type":   "quorum",
		"delivery-limit": 3,
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
		log.Panicf("Failed to declare a queue: %s", err)
	}

	return &q
}