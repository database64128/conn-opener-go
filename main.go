package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

const backoffDuration = 5 * time.Second

var (
	endpoint       = flag.String("endpoint", "", "Network endpoint address")
	payload        = flag.String("payload", "", "TCP payload or UDP message in base64 encoding")
	useTCP         = flag.Bool("tcp", false, "Use TCP transport")
	useUDP         = flag.Bool("udp", false, "Use UDP transport")
	concurrency    = flag.Int("concurrency", 1, "Number of concurrent connections to maintain")
	packetInterval = flag.Duration("packetInterval", backoffDuration, "Interval for sending UDP packets")
)

func main() {
	flag.Parse()

	if !*useTCP && !*useUDP {
		fmt.Fprintln(os.Stderr, "Use of either TCP or UDP is required.")
		flag.Usage()
		os.Exit(1)
	}

	if *concurrency < 1 {
		fmt.Fprintln(os.Stderr, "Concurrency must be at least 1.")
		flag.Usage()
		os.Exit(1)
	}

	var b []byte

	if *payload != "" {
		var err error
		b, err = base64.StdEncoding.DecodeString(*payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to decode payload: %v\n", err)
			os.Exit(1)
		}
	}

	var wg sync.WaitGroup
	logger := slog.Default()

	if *useTCP {
		for i := range *concurrency {
			logger := logger.With("network", "tcp", "index", i)
			wg.Go(func() {
				doTCP(logger, *endpoint, b)
			})
		}
	}

	if *useUDP {
		for i := range *concurrency {
			logger := logger.With("network", "udp", "index", i)
			wg.Go(func() {
				doUDP(logger, *endpoint, b, *packetInterval)
			})
		}
	}

	wg.Wait()
}

func doTCP(logger *slog.Logger, endpoint string, b []byte) {
	for {
		c, err := net.Dial("tcp", endpoint)
		if err != nil {
			logger.Warn("Failed to dial endpoint", "endpoint", endpoint, "error", err)
			time.Sleep(backoffDuration)
			continue
		}

		if len(b) > 0 {
			if _, err = c.Write(b); err != nil {
				logger.Warn("Failed to write payload", "error", err)
				c.Close()
				time.Sleep(backoffDuration)
				continue
			}
		}

		n, err := io.Copy(io.Discard, c)
		logger.Info("Read bytes", "bytes", n, "error", err)
		c.Close()
		if err != nil {
			time.Sleep(backoffDuration)
		}
	}
}

func doUDP(logger *slog.Logger, endpoint string, b []byte, interval time.Duration) {
	c, err := net.Dial("udp", endpoint)
	if err != nil {
		logger.Warn("Failed to dial endpoint", "endpoint", endpoint, "error", err)
		return
	}
	defer c.Close()

	for {
		if _, err = c.Write(b); err != nil {
			logger.Warn("Failed to write message", "error", err)
		}
		time.Sleep(interval)
	}
}
