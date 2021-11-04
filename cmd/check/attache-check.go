package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/mux"
	"github.com/letsencrypt/attache/src/check"
)

func main() {
	checkServAddr := flag.String("check-serv-addr", "", "address this utility should listen on")
	shutdownGrace := flag.Duration("shutdown-grace", time.Second*5, "duration to wait before shutting down (e.g. '1s')")
	redisNodeAddr := flag.String("redis-node-addr", "", "redis-server listening address")

	log.Print("Starting...")
	log.Print("Parsing configuration flags")
	flag.Parse()

	if *checkServAddr == "" {
		log.Fatal("Missing required opt 'check-serv-addr'")
	}

	if *redisNodeAddr == "" {
		log.Fatal("Missing required opt 'redis-node-addr'")
	}

	router := mux.NewRouter()
	check := check.NewRedisClient(*redisNodeAddr, "")
	router.HandleFunc("/clusterinfo/state/ok", check.StateOkHandler)
	router.HandleFunc("/clusterinfo/state/new", check.StateNewHandler)

	server := &http.Server{
		Addr:         *checkServAddr,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Print(err)
		}
	}()

	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)
	<-catchSignals

	ctx, cancel := context.WithTimeout(context.Background(), *shutdownGrace)
	defer cancel()
	server.Shutdown(ctx)
	log.Print("shutting down")
	os.Exit(0)
}
