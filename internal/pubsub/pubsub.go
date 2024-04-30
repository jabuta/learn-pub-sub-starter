package pubsub

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
)

type QueueType int

const (
	Durable QueueType = iota
	Transient
)

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	valb, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        valb,
	})
}

func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	simpleQueueType QueueType,
) (*amqp.Channel, amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, err
	}
	q, err := ch.QueueDeclare(
		queueName,
		simpleQueueType.durable(),
		simpleQueueType.autoDelete(),
		simpleQueueType.exclusive(),
		false,
		nil,
	)
	if err != nil {
		return nil, amqp.Queue{}, err
	}
	if err := ch.QueueBind(queueName, key, exchange, false, nil); err != nil {
		return nil, amqp.Queue{}, err
	}
	return ch, q, nil
}

func (q QueueType) durable() bool {
	switch q {
	case Durable:
		return true
	case Transient:
		return false
	}
	return false
}
func (q QueueType) autoDelete() bool {
	switch q {
	case Durable:
		return false
	case Transient:
		return true
	}
	return false
}
func (q QueueType) exclusive() bool {
	switch q {
	case Durable:
		return false
	case Transient:
		return true
	}
	return false
}
