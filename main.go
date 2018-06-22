package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/weAutomateEverything/go2hal/database"
	"github.com/weAutomateEverything/go2hal/remoteTelegramCommands"
	"github.com/weAutomateEverything/goFidoGo/monitor"
	monitor2 "github.com/weAutomateEverything/prognosisHalBot/monitor"
	"google.golang.org/grpc"
	"log"
	"net/http"
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

	http.Handle("/api/metrics", promhttp.Handler())

	monitor.NewService(c, monitorStore)
	log.Println("Service Started")
	errs := make(chan error, 2)

	go func() {
		log.Println("transport", "http", "address", ":8000", "msg", "listening")
		errs <- http.ListenAndServe(":8000", nil)
	}()

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errs <- fmt.Errorf("%s", <-c)
	}()

	<-errs
	log.Printf("System Exit")

}

func accessControl(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			return
		}

		h.ServeHTTP(w, r)
	})
}
