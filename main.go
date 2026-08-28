package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/database64128/conn-opener-go/conn"
)

const backoffDuration = 5 * time.Second

var (
	tcpNetwork     string
	udpNetwork     string
	endpoint       string
	payload        string
	useTCP         bool
	useUDP         bool
	fwmark         int
	concurrency    int
	packetInterval time.Duration
)

func init() {
	flag.StringVar(&tcpNetwork, "tcpNetwork", "tcp", "TCP network type (e.g., tcp, tcp4, tcp6)")
	flag.StringVar(&udpNetwork, "udpNetwork", "udp", "UDP network type (e.g., udp, udp4, udp6)")
	flag.StringVar(&endpoint, "endpoint", "", "Network endpoint address")
	flag.StringVar(&payload, "payload", "", "TCP payload or UDP message in base64 encoding")
	flag.BoolVar(&useTCP, "tcp", false, "Use TCP transport")
	flag.BoolVar(&useUDP, "udp", false, "Use UDP transport")
	flag.IntVar(&fwmark, "fwmark", 0, "Set the fwmark on Linux or user cookie on FreeBSD")
	flag.IntVar(&concurrency, "concurrency", 1, "Number of concurrent connections to maintain")
	flag.DurationVar(&packetInterval, "packetInterval", backoffDuration, "Interval for sending UDP packets")
}

func main() {
	flag.Parse()

	if !useTCP && !useUDP {
		badFlagValue("Use of either TCP or UDP is required.")
	}

	if concurrency < 1 {
		badFlagValue("Concurrency must be at least 1.")
	}

	if packetInterval <= 0 {
		badFlagValue("Packet interval must be greater than 0.")
	}

	if endpoint == "" {
		badFlagValue("Endpoint is required.")
	}

	var b []byte
	if payload != "" {
		var err error
		b, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to decode payload: %v\n", err)
			os.Exit(1)
		}
	}

	dso := conn.DialerSocketOptions{
		Fwmark: fwmark,
	}
	dialer := dso.Dialer()

	var wg sync.WaitGroup
	ctx := context.Background()
	logger := slog.Default()

	if useTCP {
		for i := range concurrency {
			logger := logger.With("network", tcpNetwork, "index", i)
			wg.Go(func() {
				doTCP(ctx, logger, &dialer, tcpNetwork, endpoint, b)
			})
		}
	}

	if useUDP {
		for i := range concurrency {
			logger := logger.With("network", udpNetwork, "index", i)
			wg.Go(func() {
				doUDP(ctx, logger, &dialer, udpNetwork, endpoint, b, packetInterval)
			})
		}
	}

	wg.Wait()
}

func badFlagValue(a ...any) {
	fmt.Fprintln(os.Stderr, a...)
	flag.Usage()
	os.Exit(1)
}

func doTCP(ctx context.Context, logger *slog.Logger, dialer *conn.Dialer, network, endpoint string, b []byte) {
	for {
		c, err := dialer.Dial(ctx, network, endpoint)
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

func doUDP(ctx context.Context, logger *slog.Logger, dialer *conn.Dialer, network, endpoint string, b []byte, interval time.Duration) {
	c, err := dialer.Dial(ctx, network, endpoint)
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
