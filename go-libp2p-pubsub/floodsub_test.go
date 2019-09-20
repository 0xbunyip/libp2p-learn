package pubsub

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	bhost "github.com/libp2p/go-libp2p-blankhost"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	swarmt "github.com/libp2p/go-libp2p-swarm/testing"
)

func checkMessageRouting(t *testing.T, topic string, pubs []*PubSub, subs []*Subscription) {
	data := make([]byte, 16)
	rand.Read(data)

	for _, p := range pubs {
		err := p.Publish(topic, data)
		if err != nil {
			t.Fatal(err)
		}

		for _, s := range subs {
			assertReceive(t, s, data)
		}
	}
}

func getNetHosts(t *testing.T, ctx context.Context, n int) []host.Host {
	var out []host.Host

	for i := 0; i < n; i++ {
		netw := swarmt.GenSwarm(t, ctx)
		h := bhost.NewBlankHost(netw)
		out = append(out, h)
	}

	return out
}

func connect(t *testing.T, a, b host.Host) {
	pinfo := a.Peerstore().PeerInfo(a.ID())
	err := b.Connect(context.Background(), pinfo)
	if err != nil {
		t.Fatal(err)
	}
}

func sparseConnect(t *testing.T, hosts []host.Host) {
	connectSome(t, hosts, 3)
}

func denseConnect(t *testing.T, hosts []host.Host) {
	connectSome(t, hosts, 10)
}

func connectSome(t *testing.T, hosts []host.Host, d int) {
	for i, a := range hosts {
		for j := 0; j < d; j++ {
			n := rand.Intn(len(hosts))
			if n == i {
				j--
				continue
			}

			b := hosts[n]

			connect(t, a, b)
		}
	}
}

func connectAll(t *testing.T, hosts []host.Host) {
	for i, a := range hosts {
		for j, b := range hosts {
			if i == j {
				continue
			}

			connect(t, a, b)
		}
	}
}

func getPubsub(ctx context.Context, h host.Host, opts ...Option) *PubSub {
	ps, err := NewFloodSub(ctx, h, opts...)
	if err != nil {
		panic(err)
	}
	return ps
}

func getPubsubs(ctx context.Context, hs []host.Host, opts ...Option) []*PubSub {
	var psubs []*PubSub
	for _, h := range hs {
		psubs = append(psubs, getPubsub(ctx, h, opts...))
	}
	return psubs
}

func assertReceive(t *testing.T, ch *Subscription, exp []byte) {
	select {
	case msg := <-ch.ch:
		if !bytes.Equal(msg.GetData(), exp) {
			t.Fatalf("got wrong message, expected %s but got %s", string(exp), string(msg.GetData()))
		}
	case <-time.After(time.Second * 5):
		t.Logf("%#v\n", ch)
		t.Fatal("timed out waiting for message of: ", string(exp))
	}
}

func TestBasicFloodsub(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 20)

	psubs := getPubsubs(ctx, hosts)

	var msgs []*Subscription
	for _, ps := range psubs {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs = append(msgs, subch)
	}

	//connectAll(t, hosts)
	sparseConnect(t, hosts)

	time.Sleep(time.Millisecond * 100)

	for i := 0; i < 100; i++ {
		msg := []byte(fmt.Sprintf("%d the flooooooood %d", i, i))

		owner := rand.Intn(len(psubs))

		psubs[owner].Publish("foobar", msg)

		for _, sub := range msgs {
			got, err := sub.Next(ctx)
			if err != nil {
				t.Fatal(sub.err)
			}
			if !bytes.Equal(msg, got.Data) {
				t.Fatal("got wrong message!")
			}
		}
	}

}

func TestMultihops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 6)

	psubs := getPubsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])
	connect(t, hosts[1], hosts[2])
	connect(t, hosts[2], hosts[3])
	connect(t, hosts[3], hosts[4])
	connect(t, hosts[4], hosts[5])

	var subs []*Subscription
	for i := 1; i < 6; i++ {
		ch, err := psubs[i].Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}
		subs = append(subs, ch)
	}

	time.Sleep(time.Millisecond * 100)

	msg := []byte("i like cats")
	err := psubs[0].Publish("foobar", msg)
	if err != nil {
		t.Fatal(err)
	}

	// last node in the chain should get the message
	select {
	case out := <-subs[4].ch:
		if !bytes.Equal(out.GetData(), msg) {
			t.Fatal("got wrong data")
		}
	case <-time.After(time.Second * 5):
		t.Fatal("timed out waiting for message")
	}
}

func TestReconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 3)

	psubs := getPubsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])
	connect(t, hosts[0], hosts[2])

	A, err := psubs[1].Subscribe("cats")
	if err != nil {
		t.Fatal(err)
	}

	B, err := psubs[2].Subscribe("cats")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 100)

	msg := []byte("apples and oranges")
	err = psubs[0].Publish("cats", msg)
	if err != nil {
		t.Fatal(err)
	}

	assertReceive(t, A, msg)
	assertReceive(t, B, msg)

	B.Cancel()

	time.Sleep(time.Millisecond * 50)

	msg2 := []byte("potato")
	err = psubs[0].Publish("cats", msg2)
	if err != nil {
		t.Fatal(err)
	}

	assertReceive(t, A, msg2)
	select {
	case _, ok := <-B.ch:
		if ok {
			t.Fatal("shouldnt have gotten data on this channel")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for B chan to be closed")
	}

	nSubs := len(psubs[2].myTopics["cats"])
	if nSubs > 0 {
		t.Fatal(`B should have 0 subscribers for channel "cats", has`, nSubs)
	}

	ch2, err := psubs[2].Subscribe("cats")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 100)

	nextmsg := []byte("ifps is kul")
	err = psubs[0].Publish("cats", nextmsg)
	if err != nil {
		t.Fatal(err)
	}

	assertReceive(t, ch2, nextmsg)
}

// make sure messages arent routed between nodes who arent subscribed
func TestNoConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 10)

	psubs := getPubsubs(ctx, hosts)

	ch, err := psubs[5].Subscribe("foobar")
	if err != nil {
		t.Fatal(err)
	}

	err = psubs[0].Publish("foobar", []byte("TESTING"))
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-ch.ch:
		t.Fatal("shouldnt have gotten a message")
	case <-time.After(time.Millisecond * 200):
	}
}

func TestSelfReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := getNetHosts(t, ctx, 1)[0]

	psub, err := NewFloodSub(ctx, host)
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("hello world")

	err = psub.Publish("foobar", msg)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 10)

	ch, err := psub.Subscribe("foobar")
	if err != nil {
		t.Fatal(err)
	}

	msg2 := []byte("goodbye world")
	err = psub.Publish("foobar", msg2)
	if err != nil {
		t.Fatal(err)
	}

	assertReceive(t, ch, msg2)
}

func TestOneToOne(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 2)
	psubs := getPubsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])

	sub, err := psubs[1].Subscribe("foobar")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 50)

	checkMessageRouting(t, "foobar", psubs, []*Subscription{sub})
}

func TestRegisterUnregisterValidator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 1)
	psubs := getPubsubs(ctx, hosts)

	err := psubs[0].RegisterTopicValidator("foo", func(context.Context, peer.ID, *Message) bool {
		return true
	})
	if err != nil {
		t.Fatal(err)
	}

	err = psubs[0].UnregisterTopicValidator("foo")
	if err != nil {
		t.Fatal(err)
	}

	err = psubs[0].UnregisterTopicValidator("foo")
	if err == nil {
		t.Fatal("Unregistered bogus topic validator")
	}
}

func TestValidate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 2)
	psubs := getPubsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])
	topic := "foobar"

	err := psubs[1].RegisterTopicValidator(topic, func(ctx context.Context, from peer.ID, msg *Message) bool {
		return !bytes.Contains(msg.Data, []byte("illegal"))
	})
	if err != nil {
		t.Fatal(err)
	}

	sub, err := psubs[1].Subscribe(topic)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 50)

	msgs := []struct {
		msg       []byte
		validates bool
	}{
		{msg: []byte("this is a legal message"), validates: true},
		{msg: []byte("there also is nothing controversial about this message"), validates: true},
		{msg: []byte("openly illegal content will be censored"), validates: false},
		{msg: []byte("but subversive actors will use leetspeek to spread 1ll3g4l content"), validates: true},
	}

	for _, tc := range msgs {
		for _, p := range psubs {
			err := p.Publish(topic, tc.msg)
			if err != nil {
				t.Fatal(err)
			}

			select {
			case msg := <-sub.ch:
				if !tc.validates {
					t.Log(msg)
					t.Error("expected message validation to filter out the message")
				}
			case <-time.After(333 * time.Millisecond):
				if tc.validates {
					t.Error("expected message validation to accept the message")
				}
			}
		}
	}
}

func TestValidateOverload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type msg struct {
		msg       []byte
		validates bool
	}

	tcs := []struct {
		msgs []msg

		maxConcurrency int
	}{
		{
			maxConcurrency: 10,
			msgs: []msg{
				{msg: []byte("this is a legal message"), validates: true},
				{msg: []byte("but subversive actors will use leetspeek to spread 1ll3g4l content"), validates: true},
				{msg: []byte("there also is nothing controversial about this message"), validates: true},
				{msg: []byte("also fine"), validates: true},
				{msg: []byte("still, all good"), validates: true},
				{msg: []byte("this is getting boring"), validates: true},
				{msg: []byte("foo"), validates: true},
				{msg: []byte("foobar"), validates: true},
				{msg: []byte("foofoo"), validates: true},
				{msg: []byte("barfoo"), validates: true},
				{msg: []byte("oh no!"), validates: false},
			},
		},
		{
			maxConcurrency: 2,
			msgs: []msg{
				{msg: []byte("this is a legal message"), validates: true},
				{msg: []byte("but subversive actors will use leetspeek to spread 1ll3g4l content"), validates: true},
				{msg: []byte("oh no!"), validates: false},
			},
		},
	}

	for _, tc := range tcs {

		hosts := getNetHosts(t, ctx, 2)
		psubs := getPubsubs(ctx, hosts)

		connect(t, hosts[0], hosts[1])
		topic := "foobar"

		block := make(chan struct{})

		err := psubs[1].RegisterTopicValidator(topic,
			func(ctx context.Context, from peer.ID, msg *Message) bool {
				<-block
				return true
			},
			WithValidatorConcurrency(tc.maxConcurrency))

		if err != nil {
			t.Fatal(err)
		}

		sub, err := psubs[1].Subscribe(topic)
		if err != nil {
			t.Fatal(err)
		}

		time.Sleep(time.Millisecond * 50)

		if len(tc.msgs) != tc.maxConcurrency+1 {
			t.Fatalf("expected number of messages sent to be maxConcurrency+1. Got %d, expected %d", len(tc.msgs), tc.maxConcurrency+1)
		}

		p := psubs[0]

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			for _, tmsg := range tc.msgs {
				select {
				case msg := <-sub.ch:
					if !tmsg.validates {
						t.Log(msg)
						t.Error("expected message validation to drop the message because all validator goroutines are taken")
					}
				case <-time.After(333 * time.Millisecond):
					if tmsg.validates {
						t.Error("expected message validation to accept the message")
					}
				}
			}
			wg.Done()
		}()

		for i, tmsg := range tc.msgs {
			err := p.Publish(topic, tmsg.msg)
			if err != nil {
				t.Fatal(err)
			}

			// wait a bit to let pubsub's internal state machine start validating the message
			time.Sleep(10 * time.Millisecond)

			// unblock validator goroutines after we sent one too many
			if i == len(tc.msgs)-1 {
				close(block)
			}
		}
		wg.Wait()
	}
}

func assertPeerLists(t *testing.T, hosts []host.Host, ps *PubSub, has ...int) {
	peers := ps.ListPeers("")
	set := make(map[peer.ID]struct{})
	for _, p := range peers {
		set[p] = struct{}{}
	}

	for _, h := range has {
		if _, ok := set[hosts[h].ID()]; !ok {
			t.Fatal("expected to have connection to peer: ", h)
		}
	}
}

func TestTreeTopology(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 10)
	psubs := getPubsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])
	connect(t, hosts[1], hosts[2])
	connect(t, hosts[1], hosts[4])
	connect(t, hosts[2], hosts[3])
	connect(t, hosts[0], hosts[5])
	connect(t, hosts[5], hosts[6])
	connect(t, hosts[5], hosts[8])
	connect(t, hosts[6], hosts[7])
	connect(t, hosts[8], hosts[9])

	/*
		[0] -> [1] -> [2] -> [3]
		 |      L->[4]
		 v
		[5] -> [6] -> [7]
		 |
		 v
		[8] -> [9]
	*/

	var chs []*Subscription
	for _, ps := range psubs {
		ch, err := ps.Subscribe("fizzbuzz")
		if err != nil {
			t.Fatal(err)
		}

		chs = append(chs, ch)
	}

	time.Sleep(time.Millisecond * 50)

	assertPeerLists(t, hosts, psubs[0], 1, 5)
	assertPeerLists(t, hosts, psubs[1], 0, 2, 4)
	assertPeerLists(t, hosts, psubs[2], 1, 3)

	checkMessageRouting(t, "fizzbuzz", []*PubSub{psubs[9], psubs[3]}, chs)
}

func assertHasTopics(t *testing.T, ps *PubSub, exptopics ...string) {
	topics := ps.GetTopics()
	sort.Strings(topics)
	sort.Strings(exptopics)

	if len(topics) != len(exptopics) {
		t.Fatalf("expected to have %v, but got %v", exptopics, topics)
	}

	for i, v := range exptopics {
		if topics[i] != v {
			t.Fatalf("expected %s but have %s", v, topics[i])
		}
	}
}

func TestFloodSubPluggableProtocol(t *testing.T) {
	t.Run("multi-procol router acts like a hub", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hosts := getNetHosts(t, ctx, 3)

		psubA := mustCreatePubSub(ctx, t, hosts[0], "/esh/floodsub", "/lsr/floodsub")
		psubB := mustCreatePubSub(ctx, t, hosts[1], "/esh/floodsub")
		psubC := mustCreatePubSub(ctx, t, hosts[2], "/lsr/floodsub")

		subA := mustSubscribe(t, psubA, "foobar")
		defer subA.Cancel()

		subB := mustSubscribe(t, psubB, "foobar")
		defer subB.Cancel()

		subC := mustSubscribe(t, psubC, "foobar")
		defer subC.Cancel()

		// B --> A, C --> A
		connect(t, hosts[1], hosts[0])
		connect(t, hosts[2], hosts[0])

		time.Sleep(time.Millisecond * 100)

		psubC.Publish("foobar", []byte("bar"))

		assertReceive(t, subA, []byte("bar"))
		assertReceive(t, subB, []byte("bar"))
		assertReceive(t, subC, []byte("bar"))
	})

	t.Run("won't talk to routers with no protocol overlap", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hosts := getNetHosts(t, ctx, 2)

		psubA := mustCreatePubSub(ctx, t, hosts[0], "/esh/floodsub")
		psubB := mustCreatePubSub(ctx, t, hosts[1], "/lsr/floodsub")

		subA := mustSubscribe(t, psubA, "foobar")
		defer subA.Cancel()

		subB := mustSubscribe(t, psubB, "foobar")
		defer subB.Cancel()

		connect(t, hosts[1], hosts[0])

		time.Sleep(time.Millisecond * 100)

		psubA.Publish("foobar", []byte("bar"))

		assertReceive(t, subA, []byte("bar"))

		pass := false
		select {
		case <-subB.ch:
			t.Fatal("different protocols: should not have received message")
		case <-time.After(time.Second * 1):
			pass = true
		}

		if !pass {
			t.Fatal("should have timed out waiting for message")
		}
	})
}

func mustCreatePubSub(ctx context.Context, t *testing.T, h host.Host, ps ...protocol.ID) *PubSub {
	psub, err := NewFloodsubWithProtocols(ctx, h, ps)
	if err != nil {
		t.Fatal(err)
	}

	return psub
}

func mustSubscribe(t *testing.T, ps *PubSub, topic string) *Subscription {
	sub, err := ps.Subscribe(topic)
	if err != nil {
		t.Fatal(err)
	}

	return sub
}

func TestSubReporting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := getNetHosts(t, ctx, 1)[0]
	psub, err := NewFloodSub(ctx, host)
	if err != nil {
		t.Fatal(err)
	}

	fooSub, err := psub.Subscribe("foo")
	if err != nil {
		t.Fatal(err)
	}

	barSub, err := psub.Subscribe("bar")
	if err != nil {
		t.Fatal(err)
	}

	assertHasTopics(t, psub, "foo", "bar")

	_, err = psub.Subscribe("baz")
	if err != nil {
		t.Fatal(err)
	}

	assertHasTopics(t, psub, "foo", "bar", "baz")

	barSub.Cancel()
	assertHasTopics(t, psub, "foo", "baz")
	fooSub.Cancel()
	assertHasTopics(t, psub, "baz")

	_, err = psub.Subscribe("fish")
	if err != nil {
		t.Fatal(err)
	}

	assertHasTopics(t, psub, "baz", "fish")
}

func TestPeerTopicReporting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 4)
	psubs := getPubsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])
	connect(t, hosts[0], hosts[2])
	connect(t, hosts[0], hosts[3])

	_, err := psubs[1].Subscribe("foo")
	if err != nil {
		t.Fatal(err)
	}
	_, err = psubs[1].Subscribe("bar")
	if err != nil {
		t.Fatal(err)
	}
	_, err = psubs[1].Subscribe("baz")
	if err != nil {
		t.Fatal(err)
	}

	_, err = psubs[2].Subscribe("foo")
	if err != nil {
		t.Fatal(err)
	}
	_, err = psubs[2].Subscribe("ipfs")
	if err != nil {
		t.Fatal(err)
	}

	_, err = psubs[3].Subscribe("baz")
	if err != nil {
		t.Fatal(err)
	}
	_, err = psubs[3].Subscribe("ipfs")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 10)

	peers := psubs[0].ListPeers("ipfs")
	assertPeerList(t, peers, hosts[2].ID(), hosts[3].ID())

	peers = psubs[0].ListPeers("foo")
	assertPeerList(t, peers, hosts[1].ID(), hosts[2].ID())

	peers = psubs[0].ListPeers("baz")
	assertPeerList(t, peers, hosts[1].ID(), hosts[3].ID())

	peers = psubs[0].ListPeers("bar")
	assertPeerList(t, peers, hosts[1].ID())
}

func TestSubscribeMultipleTimes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 2)
	psubs := getPubsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])

	sub1, err := psubs[0].Subscribe("foo")
	if err != nil {
		t.Fatal(err)
	}
	sub2, err := psubs[0].Subscribe("foo")
	if err != nil {
		t.Fatal(err)
	}

	// make sure subscribing is finished by the time we publish
	time.Sleep(10 * time.Millisecond)

	psubs[1].Publish("foo", []byte("bar"))

	msg, err := sub1.Next(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v.", err)
	}

	data := string(msg.GetData())

	if data != "bar" {
		t.Fatalf("data is %s, expected %s.", data, "bar")
	}

	msg, err = sub2.Next(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v.", err)
	}
	data = string(msg.GetData())

	if data != "bar" {
		t.Fatalf("data is %s, expected %s.", data, "bar")
	}
}

func TestPeerDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 2)
	psubs := getPubsubs(ctx, hosts)

	connect(t, hosts[0], hosts[1])

	_, err := psubs[0].Subscribe("foo")
	if err != nil {
		t.Fatal(err)
	}

	_, err = psubs[1].Subscribe("foo")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 10)

	peers := psubs[0].ListPeers("foo")
	assertPeerList(t, peers, hosts[1].ID())
	for _, c := range hosts[1].Network().ConnsToPeer(hosts[0].ID()) {
		c.Close()
	}

	time.Sleep(time.Millisecond * 10)

	peers = psubs[0].ListPeers("foo")
	assertPeerList(t, peers)
}

func assertPeerList(t *testing.T, peers []peer.ID, expected ...peer.ID) {
	sort.Sort(peer.IDSlice(peers))
	sort.Sort(peer.IDSlice(expected))

	if len(peers) != len(expected) {
		t.Fatalf("mismatch: %s != %s", peers, expected)
	}

	for i, p := range peers {
		if expected[i] != p {
			t.Fatalf("mismatch: %s != %s", peers, expected)
		}
	}
}

func TestNonsensicalSigningOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hosts := getNetHosts(t, ctx, 1)
	_, err := NewFloodSub(ctx, hosts[0], WithMessageSigning(false), WithStrictSignatureVerification(true))
	if err == nil {
		t.Error("expected constructor to fail on nonsensical options")
	}
}

func TestWithSigning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 2)
	psubs := getPubsubs(ctx, hosts, WithStrictSignatureVerification(true))

	connect(t, hosts[0], hosts[1])

	topic := "foobar"
	data := []byte("this is a message")

	sub, err := psubs[1].Subscribe(topic)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Millisecond * 10)

	err = psubs[0].Publish(topic, data)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := sub.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Signature == nil {
		t.Fatal("no signature in message")
	}
	if string(msg.Data) != string(data) {
		t.Fatalf("unexpected data: %s", string(msg.Data))
	}
}

func TestImproperlySignedMessageRejected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts := getNetHosts(t, ctx, 2)
	adversary := hosts[0]
	honestPeer := hosts[1]

	// The adversary enables signing, but disables verification to let through
	// an incorrectly signed message.
	adversaryPubSub := getPubsub(
		ctx,
		adversary,
		WithMessageSigning(true),
		WithStrictSignatureVerification(false),
	)
	honestPubSub := getPubsub(
		ctx,
		honestPeer,
		WithStrictSignatureVerification(true),
	)

	connect(t, adversary, honestPeer)

	var (
		topic            = "foobar"
		correctMessage   = []byte("this is a correct message")
		incorrectMessage = []byte("this is the incorrect message")
	)

	adversarySubscription, err := adversaryPubSub.Subscribe(topic)
	if err != nil {
		t.Fatal(err)
	}
	honestPeerSubscription, err := honestPubSub.Subscribe(topic)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond * 50)

	// First the adversary sends the correct message.
	err = adversaryPubSub.Publish(topic, correctMessage)
	if err != nil {
		t.Fatal(err)
	}

	// Change the sign key for the adversarial peer, and send the second,
	// incorrectly signed, message.
	adversaryPubSub.signID = honestPubSub.signID
	adversaryPubSub.signKey = honestPubSub.host.Peerstore().PrivKey(honestPubSub.signID)
	err = adversaryPubSub.Publish(topic, incorrectMessage)
	if err != nil {
		t.Fatal(err)
	}

	var adversaryMessages []*Message
	adversaryContext, adversaryCancel := context.WithCancel(ctx)
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := adversarySubscription.Next(ctx)
				if err != nil {
					return
				}
				adversaryMessages = append(adversaryMessages, msg)
			}
		}
	}(adversaryContext)

	<-time.After(1 * time.Second)
	adversaryCancel()

	// Ensure the adversary successfully publishes the incorrectly signed
	// message. If the adversary "sees" this, we successfully got through
	// their local validation.
	if len(adversaryMessages) != 2 {
		t.Fatalf("got %d messages, expected 2", len(adversaryMessages))
	}

	// the honest peer's validation process will drop the message;
	// next will never furnish the incorrect message.
	var honestPeerMessages []*Message
	honestPeerContext, honestPeerCancel := context.WithCancel(ctx)
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := honestPeerSubscription.Next(ctx)
				if err != nil {
					return
				}
				honestPeerMessages = append(honestPeerMessages, msg)
			}
		}
	}(honestPeerContext)

	<-time.After(1 * time.Second)
	honestPeerCancel()

	if len(honestPeerMessages) != 1 {
		t.Fatalf("got %d messages, expected 1", len(honestPeerMessages))
	}
	if string(honestPeerMessages[0].GetData()) != string(correctMessage) {
		t.Fatalf(
			"got %s, expected message %s",
			honestPeerMessages[0].GetData(),
			correctMessage,
		)
	}
}

func TestSubscriptionJoinNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const numLateSubscribers = 10
	const numHosts = 20
	hosts := getNetHosts(t, ctx, numHosts)

	psubs := getPubsubs(ctx, hosts)

	msgs := make([]*Subscription, numHosts)
	subPeersFound := make([]map[peer.ID]struct{}, numHosts)

	// Have some peers subscribe earlier than other peers.
	// This exercises whether we get subscription notifications from
	// existing peers.
	for i, ps := range psubs[numLateSubscribers:] {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs[i] = subch
	}

	connectAll(t, hosts)

	time.Sleep(time.Millisecond * 100)

	// Have the rest subscribe
	for i, ps := range psubs[:numLateSubscribers] {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs[i+numLateSubscribers] = subch
	}

	wg := sync.WaitGroup{}
	for i := 0; i < numHosts; i++ {
		peersFound := make(map[peer.ID]struct{})
		subPeersFound[i] = peersFound
		sub := msgs[i]
		wg.Add(1)
		go func(peersFound map[peer.ID]struct{}) {
			defer wg.Done()
			for len(peersFound) < numHosts-1 {
				event, err := sub.NextPeerEvent(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if event.Type == PeerJoin {
					peersFound[event.Peer] = struct{}{}
				}
			}
		}(peersFound)
	}

	wg.Wait()
	for _, peersFound := range subPeersFound {
		if len(peersFound) != numHosts-1 {
			t.Fatal("incorrect number of peers found")
		}
	}
}

func TestSubscriptionLeaveNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const numHosts = 20
	hosts := getNetHosts(t, ctx, numHosts)

	psubs := getPubsubs(ctx, hosts)

	msgs := make([]*Subscription, numHosts)
	subPeersFound := make([]map[peer.ID]struct{}, numHosts)

	// Subscribe all peers and wait until they've all been found
	for i, ps := range psubs {
		subch, err := ps.Subscribe("foobar")
		if err != nil {
			t.Fatal(err)
		}

		msgs[i] = subch
	}

	connectAll(t, hosts)

	time.Sleep(time.Millisecond * 100)

	wg := sync.WaitGroup{}
	for i := 0; i < numHosts; i++ {
		peersFound := make(map[peer.ID]struct{})
		subPeersFound[i] = peersFound
		sub := msgs[i]
		wg.Add(1)
		go func(peersFound map[peer.ID]struct{}) {
			defer wg.Done()
			for len(peersFound) < numHosts-1 {
				event, err := sub.NextPeerEvent(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if event.Type == PeerJoin {
					peersFound[event.Peer] = struct{}{}
				}
			}
		}(peersFound)
	}

	wg.Wait()
	for _, peersFound := range subPeersFound {
		if len(peersFound) != numHosts-1 {
			t.Fatal("incorrect number of peers found")
		}
	}

	// Test removing peers and verifying that they cause events
	msgs[1].Cancel()
	hosts[2].Close()
	psubs[0].BlacklistPeer(hosts[3].ID())

	leavingPeers := make(map[peer.ID]struct{})
	for len(leavingPeers) < 3 {
		event, err := msgs[0].NextPeerEvent(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if event.Type == PeerLeave {
			leavingPeers[event.Peer] = struct{}{}
		}
	}

	if _, ok := leavingPeers[hosts[1].ID()]; !ok {
		t.Fatal(fmt.Errorf("canceling subscription did not cause a leave event"))
	}
	if _, ok := leavingPeers[hosts[2].ID()]; !ok {
		t.Fatal(fmt.Errorf("closing host did not cause a leave event"))
	}
	if _, ok := leavingPeers[hosts[3].ID()]; !ok {
		t.Fatal(fmt.Errorf("blacklisting peer did not cause a leave event"))
	}
}

func TestSubscriptionManyNotifications(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const topic = "foobar"

	const numHosts = 35
	hosts := getNetHosts(t, ctx, numHosts)

	psubs := getPubsubs(ctx, hosts)

	msgs := make([]*Subscription, numHosts)
	subPeersFound := make([]map[peer.ID]struct{}, numHosts)

	// Subscribe all peers except one and wait until they've all been found
	for i := 1; i < numHosts; i++ {
		subch, err := psubs[i].Subscribe(topic)
		if err != nil {
			t.Fatal(err)
		}

		msgs[i] = subch
	}

	connectAll(t, hosts)

	time.Sleep(time.Millisecond * 100)

	wg := sync.WaitGroup{}
	for i := 1; i < numHosts; i++ {
		peersFound := make(map[peer.ID]struct{})
		subPeersFound[i] = peersFound
		sub := msgs[i]
		wg.Add(1)
		go func(peersFound map[peer.ID]struct{}) {
			defer wg.Done()
			for len(peersFound) < numHosts-2 {
				event, err := sub.NextPeerEvent(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if event.Type == PeerJoin {
					peersFound[event.Peer] = struct{}{}
				}
			}
		}(peersFound)
	}

	wg.Wait()
	for _, peersFound := range subPeersFound[1:] {
		if len(peersFound) != numHosts-2 {
			t.Fatalf("found %d peers, expected %d", len(peersFound), numHosts-2)
		}
	}

	// Wait for remaining peer to find other peers
	for len(psubs[0].ListPeers(topic)) < numHosts-1 {
		time.Sleep(time.Millisecond * 100)
	}

	// Subscribe the remaining peer and check that all the events came through
	sub, err := psubs[0].Subscribe(topic)
	if err != nil {
		t.Fatal(err)
	}

	msgs[0] = sub

	peerState := readAllQueuedEvents(ctx, t, sub)

	if len(peerState) != numHosts-1 {
		t.Fatal("incorrect number of peers found")
	}

	for _, e := range peerState {
		if e != PeerJoin {
			t.Fatal("non Join event occurred")
		}
	}

	// Unsubscribe all peers except one and check that all the events came through
	for i := 1; i < numHosts; i++ {
		msgs[i].Cancel()
	}

	// Wait for remaining peer to disconnect from the other peers
	for len(psubs[0].ListPeers(topic)) != 0 {
		time.Sleep(time.Millisecond * 100)
	}

	peerState = readAllQueuedEvents(ctx, t, sub)

	if len(peerState) != numHosts-1 {
		t.Fatal("incorrect number of peers found")
	}

	for _, e := range peerState {
		if e != PeerLeave {
			t.Fatal("non Leave event occurred")
		}
	}
}

func TestSubscriptionNotificationSubUnSub(t *testing.T) {
	// Resubscribe and Unsubscribe a peers and check the state for consistency
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const topic = "foobar"

	const numHosts = 35
	hosts := getNetHosts(t, ctx, numHosts)
	psubs := getPubsubs(ctx, hosts)

	for i := 1; i < numHosts; i++ {
		connect(t, hosts[0], hosts[i])
	}
	time.Sleep(time.Millisecond * 100)

	notifSubThenUnSub(ctx, t, topic, psubs)
}

func notifSubThenUnSub(ctx context.Context, t *testing.T, topic string,
	psubs []*PubSub) {

	ps := psubs[0]
	msgs := make([]*Subscription, len(psubs))
	checkSize := len(psubs) - 1

	// Subscribe all peers to the topic
	var err error
	for i, ps := range psubs {
		msgs[i], err = ps.Subscribe(topic)
		if err != nil {
			t.Fatal(err)
		}
	}

	sub := msgs[0]

	// Wait for the primary peer to be connected to the other peers
	for len(ps.ListPeers(topic)) < checkSize {
		time.Sleep(time.Millisecond * 100)
	}

	// Unsubscribe all peers except the primary
	for i := 1; i < checkSize+1; i++ {
		msgs[i].Cancel()
	}

	// Wait for the unsubscribe messages to reach the primary peer
	for len(ps.ListPeers(topic)) < 0 {
		time.Sleep(time.Millisecond * 100)
	}

	// read all available events and verify that there are no events to process
	// this is because every peer that joined also left
	peerState := readAllQueuedEvents(ctx, t, sub)

	if len(peerState) != 0 {
		for p, s := range peerState {
			fmt.Println(p, s)
		}
		t.Fatalf("Received incorrect events. %d extra events", len(peerState))
	}
}

func readAllQueuedEvents(ctx context.Context, t *testing.T, sub *Subscription) map[peer.ID]EventType {
	peerState := make(map[peer.ID]EventType)
	for {
		ctx, _ := context.WithTimeout(ctx, time.Millisecond*100)
		event, err := sub.NextPeerEvent(ctx)
		if err == context.DeadlineExceeded {
			break
		} else if err != nil {
			t.Fatal(err)
		}

		e, ok := peerState[event.Peer]
		if !ok {
			peerState[event.Peer] = event.Type
		} else if e != event.Type {
			delete(peerState, event.Peer)
		}
	}
	return peerState
}
