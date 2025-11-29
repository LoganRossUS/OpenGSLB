package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	fmt.Println("OpenGSLB starting...")
	fmt.Println("DNS server will listen on :53")
	fmt.Println("Metrics server will listen on :9090")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("OpenGSLB running. Press Ctrl+C to stop.")
	<-sigChan

	fmt.Println("OpenGSLB shutting down...")
}
