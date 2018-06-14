package main

import (
	"gitlab.com/automateEverything/goFidoGo/monitor"
	"os"
	"os/signal"
	"syscall"
	"fmt"
	"log"
)

func main() {
	 monitor.NewService()
	errs := make(chan error, 2)

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errs <- fmt.Errorf("%s", <-c)
	}()
	<-errs
	log.Printf("System Exit")

}


