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
	redisNodeAddr := flag.String("redis-node-addr", "", "Address of the Redis node to be monitored (example: '127.0.0.1:6049')")
	checkServAddr := flag.String("check-serv-addr", "", "Address the check server should listen on (example: '0.0.0.0:8080')")
	shutdownWait := flag.Duration("shutdown-wait", time.Second*15, "duration to wait for existing connections to finish (example: '1s', '1m', '1h')")
	flag.Parse()

	if *checkServAddr == "" {
		log.Fatalln("Missing required opt 'check-serv-addr'")
	}

	if *redisNodeAddr == "" {
		log.Fatalln("Missing required opt 'redis-node-addr'")
	}

	router := mux.NewRouter()
	handler := check.NewRedisCheckHandler(*redisNodeAddr, "")
	router.HandleFunc("/redis/clusterinfo/state/ok", handler.StateOk)
	router.HandleFunc("/redis/clusterinfo/state/new", handler.StateNew)

	server := &http.Server{
		Addr:         *checkServAddr,
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()

	catchSignals := make(chan os.Signal, 1)
	signal.Notify(catchSignals, os.Interrupt)
	<-catchSignals

	ctx, cancel := context.WithTimeout(context.Background(), *shutdownWait)
	defer cancel()
	server.Shutdown(ctx)
	log.Println("shutting down")
	os.Exit(0)
}
