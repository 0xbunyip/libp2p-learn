package main

import (
	"context"
	fmt "fmt"
	"log"
	"os"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	p2pgrpc "github.com/paralin/go-libp2p-grpc"
	grpc "google.golang.org/grpc"
)

func runServer() {
	ctx := context.Background()
	host := setupHost(ctx, "", 0)

	p := p2pgrpc.NewGRPCProtocol(ctx, host)
	fmt.Println("Sleeping...")
	RegisterHelloServiceServer(p.GetGRPCServer(), &HelloServer2{})
	select {}
}

func dialAndCall(p *p2pgrpc.GRPCProtocol, destID, ourID peer.ID) {
	log.Println("calling", destID)
	ctx := context.Background()
	conn, err := p.Dial(ctx, destID, grpc.WithInsecure(), grpc.WithBlock())
	// No close
	if err != nil {
		log.Fatal("A", err)
	}
	client := NewHelloServiceClient(conn)

	id := peer.IDB58Encode(ourID)
	resp, err := client.SayHello(ctx, &HelloRequest{Name: id})
	if err != nil {
		log.Fatal("B", err)
	}
	fmt.Println("response:", resp.Welcome)
}

func runClient(maddr string) {
	ctx := context.Background()
	host := setupHost(ctx, "", 0)

	// Act as client, connect to public server
	p := p2pgrpc.NewGRPCProtocol(ctx, host)
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

	for {
		dialAndCall(p, addrInfo.ID, host.ID())
		time.Sleep(3 * time.Millisecond)
	}
}

func testCloseConn() {
	if os.Args[1] == "server" {
		runServer()
	} else {
		runClient(os.Args[2])
	}
}

type HelloServer2 struct{}

func (hs *HelloServer2) SayHello(ctx context.Context, req *HelloRequest) (*HelloResponse, error) {
	id, _ := peer.IDB58Decode(req.Name)
	log.Println("hello from", id)

	return &HelloResponse{
		Welcome: "Hello " + req.Name,
	}, nil
}
