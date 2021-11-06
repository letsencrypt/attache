package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
)

type handler struct {
	redisNodeAddr string
	redisNodePass string
}

func (h *handler) status(w http.ResponseWriter, r *http.Request) {
	client := redis.NewClient(
		&redis.Options{
			Addr:     h.redisNodeAddr,
			Password: h.redisNodePass,
		},
	)

	status, err := client.ClusterInfo(context.Background()).Result()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprint("Unable to connect to node %q", h.redisNodeAddr)))
	} else {
		log.Println(status)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprint("Node %q is role %q", h.redisNodeAddr, status)))
	}
}

func main() {
	checkServAddr := flag.String("check-serv-addr", "", "Address the check server should listen on (example: '0.0.0.0:8080')")
	redisNodeAddr := flag.String("redis-node-addr", "", "Address of the Redis node to be monitored (example: '127.0.0.1:6049')")
	shutdownWait := flag.Duration("shutdown-wait", time.Second*15, "duration to wait for existing connections to finish (example: '1s', '1m', '1h')")
	flag.Parse()

	if *checkServAddr == "" {
		log.Fatalln("Missing required opt 'check-serv-addr'")
	}

	if *redisNodeAddr == "" {
		log.Fatalln("Missing required opt 'redis-node-addr'")
	}

	router := mux.NewRouter()
	handler := handler{redisNodeAddr: *redisNodeAddr}
	router.HandleFunc("/status", handler.status)

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
