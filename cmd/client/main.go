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
	const rmquri = "amqp://guest:guest@localhost:5672/"

	fmt.Println("Starting Peril client...")

	conn, err := amqp.Dial(rmquri)
	if err != nil {
		log.Fatalf("Client connection failed: %x", err)
	}
	defer conn.Close()
	fmt.Println("Client connection succsesful")

retryUsername:
	username, err := gamelogic.ClientWelcome()
	if err != nil {
		goto retryUsername
	}

	pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilDirect,
		routing.PauseKey+"."+username,
		routing.PauseKey,
		pubsub.Transient,
	)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan

	fmt.Println("Closing Client down")
}
