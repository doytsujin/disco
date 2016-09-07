package node

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"sync"
	"testing"
	"time"

	"gitlab.fg/go/disco/multicast"
)

const testMulticastAddress = "[ff12::9000]:21090"

func TestEqual(t *testing.T) {
	var tests = []struct {
		a        *Node
		b        *Node
		expected bool
	}{
		{&Node{}, &Node{}, true},
		{&Node{IPv4Address: "127.0.0.1"}, &Node{IPv4Address: "127.0.0.1"}, true},
		{&Node{IPv4Address: "127.0.0.1"}, &Node{IPv4Address: ""}, false},
		{&Node{IPv6Address: "fe80::aebc:32ff:fe93:4365"}, &Node{IPv6Address: "fe80::aebc:32ff:fe93:4365"}, true},
		{&Node{IPv6Address: "fe80::aebc:32ff:fe93:4365"}, &Node{IPv6Address: ""}, false},
	}

	for _, test := range tests {
		actual := Equal(test.a, test.b)
		if actual != test.expected {
			t.Errorf("Compare failed %v should equal %v.", test.a, test.b)
		}
	}
}

func TestMulticast(t *testing.T) {
	var tests = []struct {
		n         *Node
		shouldErr bool
	}{
		{&Node{}, true},
		{&Node{IPv4Address: "127.0.0.1"}, false},
		{&Node{IPv4Address: "127.0.0.2"}, false},
		{&Node{IPv6Address: "fe80::aebc:32ff:fe93:4365"}, false},
	}

	// results := make(chan multicast.Response)
	ctx, cancelFunc := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	wg := &sync.WaitGroup{}
	var mu sync.Mutex
	var checkNodes []*Node

	// Listen for nodes
	listener := &multicast.Multicast{Address: testMulticastAddress}
	results, err := listener.Listen(ctx)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			select {
			case resp := <-results:
				buffer := bytes.NewBuffer(resp.Payload)
				rn := &Node{}
				dec := gob.NewDecoder(buffer)
				err := dec.Decode(rn)
				if err != nil {
					errChan <- err
				}

				// Check if any nodes coming in are the ones we are waiting for
				mu.Lock()
				for _, n := range checkNodes {
					if Equal(rn, n) {
						n.Stop() // stop the node from multicasting
						wg.Done()
					}
				}
				mu.Unlock()
			case <-time.After(100 * time.Millisecond):
				errChan <- errors.New("TestMulticast timed out")
			case <-ctx.Done():
				return
			}
		}
	}()

	// Perform our test in a new goroutine so we don't block
	go func() {
		for _, test := range tests {
			// Add to the WaitGroup for each test that should pass and add it to the nodes to verify
			if !test.shouldErr {
				wg.Add(1)
				mu.Lock()
				checkNodes = append(checkNodes, test.n)
				mu.Unlock()

				if err := test.n.Multicast(ctx, testMulticastAddress); err != nil {
					t.Fatal("Multicast error", err)
				}
			} else {
				if err := test.n.Multicast(ctx, testMulticastAddress); err == nil {
					t.Fatal("Multicast of node should fail", err)
				}
			}
		}

		wg.Wait()
		cancelFunc()
	}()

	// Block until the ctx is canceled or we receive an error, such as a timeout
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errChan:
			t.Fatal(err)
		}
	}
}
