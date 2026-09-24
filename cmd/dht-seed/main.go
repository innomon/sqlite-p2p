package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-pear/pkg/hyperswarm"
	internalp2p "sqlite-p2p/internal/p2p"
	"sqlite-p2p/pkg/p2p"
)

func main() {
	port := flag.Int("port", 49737, "UDP and TCP port for the HyperDHT seed server")
	enableLANBeacon := flag.Bool("beacon", true, "Enable periodic LAN UDP broadcast for local zero-config discovery")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	swarm, err := hyperswarm.New(hyperswarm.SwarmOptions{
		Port: *port,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start HyperDHT seed on port %d: %v\n", *port, err)
		os.Exit(1)
	}
	defer swarm.Close()

	assignedPort := swarm.Port()
	routableIP, _ := internalp2p.ResolveRoutableIP()
	dhtEndpoint := internalp2p.FormatDHTEndpoint(routableIP, assignedPort)

	fmt.Println("=================================================================")
	fmt.Println("  HyperDHT Bootstrap Seed Node Online")
	fmt.Printf("  Listening Port:   %d (UDP / TCP)\n", assignedPort)
	fmt.Printf("  LAN Endpoint:     %s\n", dhtEndpoint)
	fmt.Printf("  Cloud Deployment: Point your peers to '<YOUR_VPS_PUBLIC_IP>:%d'\n", assignedPort)
	fmt.Println("  Service:          Routing DHT announcements and peer lookups")
	fmt.Println("=================================================================")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *enableLANBeacon {
		beaconMsg := internalp2p.BeaconMessage{
			Cluster: "sqlite-p2p",
			Topic:   "*",
			DHTAddr: dhtEndpoint,
			NodeID:  "dht-seed",
		}
		_ = p2p.StartBeaconBroadcaster(ctx, beaconMsg, internalp2p.DefaultBeaconPort, 3*time.Second)
		logger.Info("LAN discovery beacon broadcaster active", "port", internalp2p.DefaultBeaconPort)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nReceived shutdown signal. Stopping HyperDHT seed server...")
}
