package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestAwaitCreditDisabled(t *testing.T) {
	tc := &TunnelClient{}
	if !tc.awaitCredit(context.Background(), nil) {
		t.Fatal("nil flow (disabled) must proceed immediately")
	}
}

func TestAwaitCreditHasCredits(t *testing.T) {
	tc := &TunnelClient{}
	sf := &streamFlow{credits: 2, wake: make(chan struct{}, 1)}
	if !tc.awaitCredit(context.Background(), sf) {
		t.Fatal("positive credits must proceed")
	}
}

func TestAwaitCreditBlocksThenGranted(t *testing.T) {
	tc := &TunnelClient{}
	sf := &streamFlow{credits: 0, wake: make(chan struct{}, 1)}
	done := make(chan bool, 1)
	go func() { done <- tc.awaitCredit(context.Background(), sf) }()

	select {
	case <-done:
		t.Fatal("must block while credits are zero")
	case <-time.After(20 * time.Millisecond):
	}
	// Grant a credit as the read loop would.
	atomic.AddInt64(&sf.credits, 1)
	select {
	case sf.wake <- struct{}{}:
	default:
	}
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("granted credit should let it proceed")
		}
	case <-time.After(time.Second):
		t.Fatal("did not wake after credit granted")
	}
}

func TestAwaitCreditCancelled(t *testing.T) {
	tc := &TunnelClient{}
	sf := &streamFlow{credits: 0, wake: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- tc.awaitCredit(ctx, sf) }()
	cancel()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("cancellation must abort the wait (return false)")
		}
	case <-time.After(time.Second):
		t.Fatal("did not return after cancel")
	}
}
