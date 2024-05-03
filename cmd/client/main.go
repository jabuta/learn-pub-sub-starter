package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/jabuta/learn-pub-sub-starter/internal/gamelogic"
	"github.com/jabuta/learn-pub-sub-starter/internal/pubsub"
	"github.com/jabuta/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	go interruptQuit()
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

	ch, queue, err := pubsub.DeclareAndBind(
		conn,
		routing.ExchangePerilDirect,
		routing.PauseKey+"."+username,
		routing.PauseKey,
		pubsub.Transient,
	)
	if err != nil {
		log.Fatalf("could not subscribe to pause: %x", err)
	}
	fmt.Printf("Queue %v declared and bound", queue.Name)

	gs := gamelogic.NewGameState(username)

	//consume pause
	if err := pubsub.SubscribeJSON(
		conn,
		routing.ExchangePerilDirect,
		routing.PauseKey+"."+username,
		routing.PauseKey,
		pubsub.Transient,
		handlerPause(gs),
	); err != nil {
		log.Fatalln("subscribe to pause failed: ", err)
	}

	//consume war
	if err := pubsub.SubscribeJSON(
		conn,
		routing.ExchangePerilTopic,
		routing.WarRecognitionsPrefix,
		routing.WarRecognitionsPrefix+".*",
		pubsub.Durable,
		handlerWar(gs, ch),
	); err != nil {
		log.Fatalln("subscribe to army moves failed: ", err)
	}

	//consume move
	if err := pubsub.SubscribeJSON(
		conn,
		routing.ExchangePerilTopic,
		routing.ArmyMovesPrefix+"."+username,
		routing.ArmyMovesPrefix+".*",
		pubsub.Transient,
		handlerMove(gs, ch),
	); err != nil {
		log.Fatalln("subscribe to army moves failed: ", err)
	}

	gamelogic.PrintClientHelp()

	for {
		cmds := gamelogic.GetInput()
		if len(cmds) == 0 {
			continue
		}

		switch cmds[0] {
		case "spawn":
			if err := gs.CommandSpawn(cmds); err != nil {
				fmt.Println(err)
				continue
			}
		case "move":
			am, err := gs.CommandMove(cmds)
			if err != nil {
				fmt.Println(err)
				continue
			}
			if err := pubsub.PublishJSON(
				ch,
				routing.ExchangePerilTopic,
				routing.ArmyMovesPrefix+"."+username,
				am,
			); err != nil {
				fmt.Println(err)
				continue
			}
		case "status":
			gs.CommandStatus()
		case "spam":
			if len(cmds) != 2 {
				fmt.Println("need second argument int")
				continue
			}
			numSpam, err := strconv.Atoi(cmds[1])
			if err != nil {
				fmt.Println("second argument is not an int")
				continue
			}
			for i := 0; i < numSpam; i++ {
				if err := publishGameLog(ch,
					username,
					gamelogic.GetMaliciousLog(),
				); err != nil {
					fmt.Println("error sending spam: ", err)
				}
			}
			continue
		case "quit":
			gamelogic.PrintQuit()
			return
		default:
			fmt.Println("learn to play breh")
			continue
		}
	}

}

func interruptQuit() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)
	<-signalChan
	fmt.Println("Closing Client down")
	os.Exit(0)
}

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckType {
	return func(ps routing.PlayingState) pubsub.AckType {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return pubsub.Ack
	}
}

func handlerMove(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.ArmyMove) pubsub.AckType {
	return func(am gamelogic.ArmyMove) pubsub.AckType {
		defer fmt.Print("> ")
		switch gs.HandleMove(am) {
		case gamelogic.MoveOutComeSafe:
			return pubsub.Ack
		case gamelogic.MoveOutcomeMakeWar:
			if err := pubsub.PublishJSON(
				ch,
				routing.ExchangePerilTopic,
				routing.WarRecognitionsPrefix+"."+gs.GetUsername(),
				gamelogic.RecognitionOfWar{
					Attacker: am.Player,
					Defender: gs.GetPlayerSnap(),
				},
			); err != nil {
				fmt.Println("Error: ", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.MoveOutcomeSamePlayer:
			return pubsub.NackDiscard
		}
		fmt.Println("Error, unknown move outcome")
		return pubsub.NackDiscard
	}
}

func handlerWar(gs *gamelogic.GameState, ch *amqp.Channel) func(gamelogic.RecognitionOfWar) pubsub.AckType {
	return func(row gamelogic.RecognitionOfWar) pubsub.AckType {
		defer fmt.Print("> ")
		outcome, winner, loser := gs.HandleWar(row)

		switch outcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.NackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.NackDiscard
		case gamelogic.WarOutcomeOpponentWon:
			if err := publishGameLog(
				ch,
				gs.GetUsername(),
				fmt.Sprintf("%s won a war against %s", winner, loser),
			); err != nil {
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.WarOutcomeYouWon:
			if err := publishGameLog(
				ch,
				gs.GetUsername(),
				fmt.Sprintf("%s won a war against %s", winner, loser),
			); err != nil {
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.WarOutcomeDraw:
			if err := publishGameLog(
				ch,
				gs.GetUsername(),
				fmt.Sprintf("A war between %s and %s resulted in a draw", winner, loser),
			); err != nil {
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		}
		fmt.Println("Outcome unknown")
		return pubsub.NackDiscard
	}
}

func publishGameLog(ch *amqp.Channel, username, msg string) error {
	return pubsub.PublishGob(
		ch,
		routing.ExchangePerilTopic,
		routing.GameLogSlug+"."+username,
		routing.GameLog{
			CurrentTime: time.Now(),
			Message:     msg,
			Username:    username,
		},
	)
}
