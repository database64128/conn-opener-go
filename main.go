package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
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
	noDrainRead    bool
	fwmark         int
	concurrency    int
	packetInterval time.Duration
	logLevel       slog.Level
)

func init() {
	flag.StringVar(&tcpNetwork, "tcpNetwork", "tcp", "TCP network type (e.g., tcp, tcp4, tcp6)")
	flag.StringVar(&udpNetwork, "udpNetwork", "udp", "UDP network type (e.g., udp, udp4, udp6)")
	flag.StringVar(&endpoint, "endpoint", "", "Network endpoint address")
	flag.StringVar(&payload, "payload", "", "TCP payload or UDP message in base64 encoding")
	flag.BoolVar(&useTCP, "tcp", false, "Use TCP transport")
	flag.BoolVar(&useUDP, "udp", false, "Use UDP transport")
	flag.BoolVar(&noDrainRead, "noDrainRead", false, "Do not drain read TCP connections until EOF; close immediately after writing payload")
	flag.IntVar(&fwmark, "fwmark", 0, "Set the fwmark on Linux or user cookie on FreeBSD")
	flag.IntVar(&concurrency, "concurrency", 1, "Number of concurrent connections to maintain")
	flag.DurationVar(&packetInterval, "packetInterval", backoffDuration, "Interval for sending UDP packets")
	flag.TextVar(&logLevel, "logLevel", slog.LevelInfo, "Log level (e.g., debug, info, warn, error)")
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

	switch tcpNetwork {
	case "tcp", "tcp4", "tcp6":
	default:
		badFlagValue("Invalid TCP network type. Must be one of: tcp, tcp4, tcp6.")
	}

	switch udpNetwork {
	case "udp", "udp4", "udp6":
	default:
		badFlagValue("Invalid UDP network type. Must be one of: udp, udp4, udp6.")
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
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	if useTCP {
		for i := range concurrency {
			logger := logger.With("network", tcpNetwork, "index", i)
			wg.Go(func() {
				for {
					if !doTCP(ctx, logger, &dialer, tcpNetwork, endpoint, b) {
						time.Sleep(backoffDuration)
					}
				}
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

func doTCP(ctx context.Context, logger *slog.Logger, dialer *net.Dialer, network, endpoint string, b []byte) bool {
	c, err := dialer.DialContext(ctx, network, endpoint)
	if err != nil {
		logger.Warn("Failed to dial endpoint", "endpoint", endpoint, "error", err)
		return false
	}
	defer c.Close()

	if len(b) > 0 {
		if _, err = c.Write(b); err != nil {
			logger.Warn("Failed to write payload", "error", err)
			return false
		}
	}

	if noDrainRead {
		// Without draining read, we have to force a connection reset,
		// or we'd run out of local ports because the connection would
		// linger in TIME_WAIT state.
		if err := c.(*net.TCPConn).SetLinger(0); err != nil {
			logger.Warn("Failed to set linger option", "error", err)
			return false
		}
		logger.Info("Closing connection without draining read")
		return true
	}

	n, err := io.Copy(io.Discard, c)
	logger.Info("Read bytes", "bytes", n, "error", err)
	return err == nil
}

func doUDP(ctx context.Context, logger *slog.Logger, dialer *net.Dialer, network, endpoint string, b []byte, interval time.Duration) {
	c, err := dialer.DialContext(ctx, network, endpoint)
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
