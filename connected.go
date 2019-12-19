package main

import (
	"context"
	"fmt"
	"log"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func testConnected() {
	n := 2
	hosts, err := createHosts(n)
	if err != nil {
		log.Println(err)
		return
	}

	// Clean up hosts
	defer func() {
		fmt.Println("Shutting down")
		for _, h := range hosts {
			if err := h.Close(); err != nil {
				fmt.Println("error while shutting down:", err)
			}
		}
	}()

	h1 := hosts[0]
	addrInfo := peer.AddrInfo{
		ID:    h1.ID(),
		Addrs: h1.Addrs(),
	}

	h2 := hosts[1]
	err = h2.Connect(context.Background(), addrInfo)
	if err != nil {
		log.Println(err)
		return
	}

	connected := h1.Network().Connectedness(h1.ID())
	switch connected {
	case network.NotConnected:
		log.Println("network.NotConnected")
	case network.Connected:
		log.Println("network.Connected")
	case network.CanConnect:
		log.Println("network.CanConnect")
	case network.CannotConnect:
		log.Println("network.CannotConnect")
	}

	fmt.Println(addrInfo.Addrs[0].ValueForProtocol(ma.P_IP4))
}
