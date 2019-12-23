package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/libp2p/go-libp2p-core/host"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
)

func runPublisher() {
	ctx := context.Background()
	h := setupHost(ctx, "CAMSeTB3AgEBBCBOKmqCVHP98tkPVrlS/OESNKg6eJp2iPgQ5Zuf9rm/j6AKBggqhkjOPQMBB6FEA0IABABisRYCDewo66C1TdfPdF6I3Ag9FTEvcOU+Zk/ghLvd03bQ8BOt8AVro1TKCa59IGlP/A1Rav1J1Ci8ah+nL5Y=", 51234)
	// Clean up hosts
	defer func() {
		fmt.Println("Shutting down")
		if err := h.Close(); err != nil {
			fmt.Println("error while shutting down:", err)
		}
	}()

	ps, err := createPubSubs([]host.Host{h}) // Must create pubsub before connecting hosts
	if err != nil {
		log.Println(err)
		return
	}

	time.Sleep(10 * time.Second) // For subscriber to run

	data := make([]byte, 10000)
	for {
		if err := publish(ps, data, 1000, 5*time.Millisecond); err != nil {
			log.Println(err)
			continue
		}
	}
}

func runSubscriber(maddr string) {
	ctx := context.Background()
	h := setupHost(ctx, "", 0)

	addr, err := multiaddr.NewMultiaddr(maddr)
	if err != nil {
		log.Fatal(err)
	}

	addrInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		log.Fatal(err)
	}

	ps, err := createPubSubs([]host.Host{h}) // Must create pubsub before connecting hosts
	if err != nil {
		log.Println(err)
		return
	}

	if err := h.Connect(ctx, *addrInfo); err != nil {
		log.Fatal(err)
	}

	if err := subscribe(ps); err != nil {
		log.Println(err)
		return
	}

	select {}
}

func testDisconnected() {
	if os.Args[1] == "publisher" {
		runPublisher()
	} else {
		runSubscriber(os.Args[2])
	}
}
