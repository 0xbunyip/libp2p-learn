package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/ping"
	"github.com/multiformats/go-multiaddr"
)

func basic() {
	ctx := context.Background()

	node, err := libp2p.New(
		ctx,
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Ping(false),
	)
	if err != nil {
		panic(err)
	}

	defer func() {
		fmt.Println("Shutting down")
		if err := node.Close(); err != nil {
			fmt.Println("error while shutting down:", err)
		}
	}()

	pingService := ping.PingService{Host: node}
	node.SetStreamHandler(ping.ID, pingService.PingHandler)

	peerInfo := &peer.AddrInfo{
		ID:    node.ID(),
		Addrs: node.Addrs(),
	}
	multiAddrs, err := peer.AddrInfoToP2pAddrs(peerInfo)
	if err != nil {
		panic(err)
	}
	fmt.Println("multiAddr:", multiAddrs[0])

	if len(os.Args) > 1 {
		addr, err := multiaddr.NewMultiaddr(os.Args[1])
		if err != nil {
			panic(err)
		}

		addrInfo, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			panic(err)
		}

		err = node.Connect(ctx, *addrInfo)
		if err != nil {
			panic(err)
		}

		n := 5
		fmt.Printf("sending %d ping messages to %v\n", n, addr)
		resp := pingService.Ping(ctx, addrInfo.ID)
		for i := 0; i < n; i++ {
			val := <-resp
			fmt.Printf("got response, RTT: %v\n", val.RTT)
		}

	} else {
		fmt.Println("waiting for CTRL-C")
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
	}
}

func main() {
	// basic()
	// testPubSub()
	testGRPC()
}
