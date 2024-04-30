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

	go quitOnInterrupt()

	gamelogic.PrintServerHelp()

	for loop := true; loop; {
		command := gamelogic.GetInput()
		if len(command) == 0 {
			continue
		}
		switch command[0] {
		case "pause":
			fmt.Println("sending pause message")
			pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
				IsPaused: true,
			})
		case "resume":
			fmt.Println("sending resume message")
			pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
				IsPaused: false,
			})
		case "quit":
			fmt.Println("quitting the game")
			loop = false
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
