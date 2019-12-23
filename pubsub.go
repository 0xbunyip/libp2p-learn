package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pubsub "github.com/0xbunyip/libp2p-learn/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
)

func testPubSub() {
	n := 3
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

	ps, err := createPubSubs(hosts) // Must create pubsub before connecting hosts
	if err != nil {
		log.Println(err)
		return
	}

	if err := connectHosts(hosts); err != nil {
		log.Println(err)
		return
	}

	if err = subscribe(ps); err != nil {
		log.Println(err)
		return
	}

	data := []byte("xabc")
	if err := publish(ps, data, 3, time.Second); err != nil {
		log.Println(err)
		return
	}
}

func createHosts(n int) ([]host.Host, error) {
	log.Println("Creating hosts")
	ctx := context.Background()
	hosts := []host.Host{}
	for i := 0; i < n; i++ {
		node, err := libp2p.New(
			ctx,
			libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		)
		if err != nil {
			return hosts, err
		}

		hosts = append(hosts, node)

		// Print out listening address
		peerInfo := &peer.AddrInfo{
			ID:    node.ID(),
			Addrs: node.Addrs(),
		}
		multiAddrs, err := peer.AddrInfoToP2pAddrs(peerInfo)
		if err != nil {
			return hosts, err
		}
		log.Printf("multiAddr %d: %v\n", i, multiAddrs[0])
	}
	return hosts, nil
}

func createPubSubs(hosts []host.Host) ([]*pubsub.PubSub, error) {
	ps := []*pubsub.PubSub{}
	ctx := context.Background()
	for _, h := range hosts {
		// p, err := pubsub.NewGossipSub(ctx, h)
		p, err := pubsub.NewFloodSub(ctx, h)
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	return ps, nil
}

func subscribe(ps []*pubsub.PubSub) error {
	log.Println("Subscribing hosts")
	topic := "art"
	for i, p := range ps {
		// if i == 1 {
		// 	continue
		// }

		sub, err := p.Subscribe(topic)
		if err != nil {
			return err
		}
		go processSubscriptionMessage(i, sub)
	}
	return nil
}

func processSubscriptionMessage(i int, sub *pubsub.Subscription) {
	log.Printf("host[%d] processing message\n", i)
	ctx := context.Background()
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			log.Println(err)
			continue
		}
		log.Printf("host[%d] received msg %v %v %v\n", i, msg.TopicIDs, msg.GetFrom(), msg.Seqno)
	}
}

func connectHosts(hosts []host.Host) error {
	log.Println("Connecting hosts")
	// Connect hosts i and i-1
	ctx := context.Background()
	n := len(hosts)
	for i := 1; i < n; i++ {
		id := hosts[i-1].ID()
		mas := hosts[i-1].Addrs()
		addrInfo := peer.AddrInfo{id, mas}
		// fmt.Println(i, id, mas, addrInfo)
		if err := hosts[i].Connect(ctx, addrInfo); err != nil {
			return err
		}
	}
	return nil
}

func publish(ps []*pubsub.PubSub, data []byte, cnt int, delay time.Duration) error {
	time.Sleep(1 * time.Second)
	log.Println("Publishing hosts")
	for i := 0; i < cnt; i++ {
		log.Println("Publish ...")
		data[0] = byte(i % 255)
		if err := ps[0].Publish("art", data); err != nil {
			log.Println(err)
		}
		time.Sleep(delay)
	}
	return nil
}
