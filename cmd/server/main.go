package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/jabuta/learn-pub-sub-starter/internal/gamelogic"
	"github.com/jabuta/learn-pub-sub-starter/internal/pubsub"
	"github.com/jabuta/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	fmt.Println("Starting Peril server...")
	const rmquri = "amqp://guest:guest@localhost:5672/"

	conn, err := amqp.Dial(rmquri)
	if err != nil {
		log.Fatalf("Client connection failed: %x", err)
	}
	defer conn.Close()
	fmt.Println("Server connection succsesful")

	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}

	if err := initRabbitStuff(conn, ch); err != nil {
		log.Fatalf("Failed to init: %x", err)
	}

	_, queue, err := pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilTopic,
		routing.GameLogSlug,
		routing.GameLogSlug+".*",
		pubsub.Durable,
	)
	if err != nil {
		log.Fatalf("could not subscribe to pause: %x", err)
	}
	fmt.Printf("Queue %v declared and bound", queue.Name)

	go quitOnInterrupt()

	gamelogic.PrintServerHelp()

	for {
		command := gamelogic.GetInput()
		if len(command) == 0 {
			continue
		}
		switch command[0] {
		case "pause":
			fmt.Println("sending pause message")
			pubsub.PublishJSON(ch,
				routing.ExchangePerilDirect,
				routing.PauseKey,
				routing.PlayingState{
					IsPaused: true,
				},
			)
		case "resume":
			fmt.Println("sending resume message")
			pubsub.PublishJSON(ch,
				routing.ExchangePerilDirect,
				routing.PauseKey,
				routing.PlayingState{
					IsPaused: false,
				},
			)
		case "quit":
			fmt.Println("quitting the game")
			return
		default:
			fmt.Println("invalid command")
		}
	}

}

func quitOnInterrupt() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan

	fmt.Println("Closing server down")
	os.Exit(0)
}

// inits the server so i dont have to do it manually when i stop the docker container.
func initRabbitStuff(conn *amqp.Connection, ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(
		routing.ExchangePerilDirect,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		fmt.Println("failed to init ", routing.ExchangePerilDirect)
		return err
	}

	if err := ch.ExchangeDeclare(
		routing.ExchangePerilTopic,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		fmt.Println("failed to init ", routing.ExchangePerilTopic)
		return err
	}

	if err := ch.ExchangeDeclare(
		pubsub.ExchangePerilDlx,
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		fmt.Println("failed to init ", pubsub.ExchangePerilDlx)
		return err
	}

	if _, _, err := pubsub.DeclareAndBind(
		conn,
		pubsub.ExchangePerilDlx,
		pubsub.QueuePerilDlq,
		"",
		pubsub.Durable,
	); err != nil {
		fmt.Println("failed to init ", pubsub.QueuePerilDlq)
		return err
	}

	//declare and subscribe game logs
	if _, _, err := pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilTopic,
		routing.GameLogSlug,
		"",
		pubsub.Durable,
	); err != nil {
		fmt.Println("failed to init game logs queue", pubsub.QueuePerilDlq)
		return err
	}
	if err := pubsub.SubscribeGob(
		conn,
		routing.ExchangePerilTopic,
		routing.GameLogSlug,
		routing.GameLogSlug+".*",
		pubsub.Durable,
		handlerGameLogs(),
	); err != nil {
		fmt.Println("failed to init gamelogs sub", pubsub.QueuePerilDlq)
		return err
	}

	return nil
}

func handlerGameLogs() func(routing.GameLog) pubsub.AckType {
	return func(log routing.GameLog) pubsub.AckType {
		defer fmt.Print("> ")
		if err := gamelogic.WriteLog(log); err != nil {
			fmt.Printf("error writing log: %v\n", err)
			return pubsub.NackRequeue
		}
		return pubsub.Ack
	}
}
