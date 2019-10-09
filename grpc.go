package main

import (
	"context"
	fmt "fmt"
	"log"
	"os"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	p2pgrpc "github.com/paralin/go-libp2p-grpc"
	grpc "google.golang.org/grpc"
)

func setupHost(ctx context.Context) host.Host {
	host, err := libp2p.New(ctx)
	if err != nil {
		log.Fatal(err)
	}

	peerInfo := &peer.AddrInfo{
		ID:    host.ID(),
		Addrs: host.Addrs(),
	}
	multiAddrs, err := peer.AddrInfoToP2pAddrs(peerInfo)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Address:", multiAddrs[0])
	return host
}

func setupServer(p *p2pgrpc.GRPCProtocol, c chan peer.ID) {
	RegisterHelloServiceServer(p.GetGRPCServer(), &HelloServer{c})
	fmt.Println("Public serving...")
}

func runPublic() {
	ctx := context.Background()
	host := setupHost(ctx)

	p := p2pgrpc.NewGRPCProtocol(ctx, host)
	c := make(chan peer.ID, 10)
	setupServer(p, c)

	for {
		select {
		case id := <-c:
			call(p, id, host.ID())
		}
	}
}

func call(p *p2pgrpc.GRPCProtocol, destID, ourID peer.ID) {
	log.Println("calling", destID)
	ctx := context.Background()
	conn, err := p.Dial(ctx, destID, grpc.WithInsecure(), grpc.WithBlock())
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

func runPrivate(maddr string) {
	ctx := context.Background()
	host := setupHost(ctx)

	// Run its own server
	p := p2pgrpc.NewGRPCProtocol(ctx, host)
	setupServer(p, nil)

	// Act as client, connect to public server
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

	call(p, addrInfo.ID, host.ID())
	select {}
}

func testGRPC() {
	if os.Args[1] == "public" {
		runPublic()
	} else {
		runPrivate(os.Args[2])
	}
}

type HelloServer struct {
	c chan peer.ID
}

func (hs *HelloServer) SayHello(ctx context.Context, req *HelloRequest) (*HelloResponse, error) {
	id, err := peer.IDB58Decode(req.Name)
	log.Println("hello from", id)

	if err == nil && hs.c != nil {
		go func(id peer.ID) {
			hs.c <- id
		}(id)
	}

	return &HelloResponse{
		Welcome: "Hello " + req.Name,
	}, nil
}
