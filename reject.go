package main

import (
	"context"
	fmt "fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-protocol"
	"github.com/multiformats/go-multiaddr"
)

var ProtocolName = "RejectProtocol"
var Protocol = protocol.ID(ProtocolName)

func runRejector() {
	ctx := context.Background()
	host := setupHost(ctx)
	// host.Mux().AddHandler(ProtocolName, handlerCloser)
	host.SetStreamHandler(Protocol, handler)
	select {}
}

func handlerCloser(protocol string, rwc io.ReadWriteCloser) error {
	log.Println("in handlerCloser")
	rwc.Close()
	return fmt.Errorf("nothing personal")
}

func handler(s network.Stream) {
	log.Println("in handler")

	// err := s.Reset()
	// if err != nil {
	// 	log.Println("reset failed:", err)
	// }

	err := s.Conn().Close()
	if err != nil {
		log.Println("close conn failed:", err)
	}
}

func runConnector(maddr string) {
	ctx := context.Background()
	host := setupHost(ctx)

	addr, err := multiaddr.NewMultiaddr(maddr)
	if err != nil {
		log.Fatal(err)
	}

	addrInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		log.Fatal(err)
	}

	if err := host.Connect(ctx, *addrInfo); err != nil {
		log.Fatal(err)
	}

	log.Println(host.Mux().Protocols())

	s, err := host.NewStream(ctx, addrInfo.ID, Protocol)
	if err != nil {
		log.Fatal(err)
	}

	_, err = s.Write([]byte("wow"))
	if err != nil {
		log.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	_, err = s.Write([]byte("wow"))
	if err != nil {
		log.Fatal(err)
	}

	log.Println(s)
	select {}
}

func testRejectConn() {
	if os.Args[1] == "rejector" {
		runRejector()
	} else {
		runConnector(os.Args[2])
	}
}
