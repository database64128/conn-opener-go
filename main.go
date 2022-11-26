package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
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
		fmt.Println("Use of either TCP or UDP is required.")
		flag.Usage()
		os.Exit(1)
	}

	if *concurrency < 1 {
		fmt.Println("Concurrency must be at least 1.")
		flag.Usage()
		os.Exit(1)
	}

	var b []byte

	if *payload != "" {
		var err error
		b, err = base64.StdEncoding.DecodeString(*payload)
		if err != nil {
			fmt.Printf("Failed to decode payload: %v\n", err)
			os.Exit(1)
		}
	}

	var wg sync.WaitGroup

	if *useTCP {
		for i := 0; i < *concurrency; i++ {
			wg.Add(1)
			go func() {
				doTCP(*endpoint, b)
				wg.Done()
			}()
		}
	}

	if *useUDP {
		for i := 0; i < *concurrency; i++ {
			wg.Add(1)
			go func() {
				doUDP(*endpoint, b, *packetInterval)
				wg.Done()
			}()
		}
	}

	wg.Wait()
}

func doTCP(endpoint string, b []byte) {
	for {
		c, err := net.Dial("tcp", endpoint)
		if err != nil {
			log.Printf("Failed to dial %s: %v", endpoint, err)
			time.Sleep(backoffDuration)
			continue
		}

		if len(b) > 0 {
			if _, err = c.Write(b); err != nil {
				log.Printf("Failed to write payload: %v", err)
				c.Close()
				time.Sleep(backoffDuration)
				continue
			}
		}

		n, err := io.Copy(io.Discard, c)
		log.Printf("Read %d bytes with error: %v", n, err)
		c.Close()
		if err != nil {
			time.Sleep(backoffDuration)
		}
	}
}

func doUDP(endpoint string, b []byte, interval time.Duration) {
	c, err := net.Dial("udp", endpoint)
	if err != nil {
		log.Printf("Failed to dial %s: %v", endpoint, err)
		return
	}
	defer c.Close()

	for {
		if _, err = c.Write(b); err != nil {
			log.Printf("Failed to write message: %v", err)
		}
		time.Sleep(interval)
	}
}
