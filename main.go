package main

import (
	"fmt"
	"github.com/weAutomateEverything/go2hal/database"
	"github.com/weAutomateEverything/go2hal/remoteTelegramCommands"
	"github.com/weAutomateEverything/goFidoGo/monitor"
	monitor2 "github.com/weAutomateEverything/prognosisHalBot/monitor"
	"google.golang.org/grpc"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	log.Println("Starting Go Fido Go")
	conn, err := grpc.Dial(os.Getenv("HAL")+":8080", grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	c := remoteTelegramCommands.NewRemoteCommandClient(conn)
	log.Println("GRPC Connection Done")

	db := database.NewConnection()

	monitorStore := monitor2.NewMongoStore(db)

	monitor.NewService(c, monitorStore)
	log.Println("Service Started")
	errs := make(chan error, 2)

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errs <- fmt.Errorf("%s", <-c)
	}()
	<-errs
	log.Printf("System Exit")

}
