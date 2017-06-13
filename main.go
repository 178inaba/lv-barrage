package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix(fmt.Sprintf("%s: ", os.Args[0]))
	os.Exit(run())
}

func run() int {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	_, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigCh
		cancel()
	}()

	return 0
}
