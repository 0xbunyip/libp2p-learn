package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
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

	ps, err := subscribe(hosts)
	if err != nil {
		log.Println(err)
		return
	}

	if err := connectHosts(hosts); err != nil {
		log.Println(err)
		return
	}

	if err := publish(ps, hosts); err != nil {
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
		fmt.Printf("multiAddr %d: %v\n", i, multiAddrs[0])
	}
	return hosts, nil
}

func subscribe(hosts []host.Host) ([]*pubsub.PubSub, error) {
	log.Println("Subscribing hosts")
	// Subscribe
	ctx := context.Background()
	topic := "art"
	ps := []*pubsub.PubSub{}
	for i, h := range hosts {
		p, err := pubsub.NewGossipSub(ctx, h)
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
		// if i == 1 {
		// 	continue
		// }

		sub, err := p.Subscribe(topic)
		if err != nil {
			return nil, err
		}
		go processSubscriptionMessage(i, h, sub)
	}
	return ps, nil
}

func processSubscriptionMessage(i int, h host.Host, sub *pubsub.Subscription) {
	log.Printf("host[%d] processing message\n", i)
	ctx := context.Background()
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			log.Println(err)
			continue
		}
		log.Printf("host[%d] received msg %s %v\n", i, msg.Data, msg.TopicIDs)
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
		if err := hosts[i].Connect(ctx, addrInfo); err != nil {
			return err
		}
	}
	return nil
}

func publish(ps []*pubsub.PubSub, hosts []host.Host) error {
	time.Sleep(1 * time.Second)
	log.Println("Publishing hosts")
	m := 3
	for i := 0; i < m; i++ {
		log.Println("Publish ...")
		if err := ps[0].Publish("art", []byte("abc"+strconv.Itoa(i))); err != nil {
			log.Println(err)
		}
		time.Sleep(3 * time.Second)
	}
	return nil
}
