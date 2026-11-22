package main

import (
	"fmt"
	"net"
	"testing"
	"time"

	"loris-tunnel/internal/conf"
	"loris-tunnel/internal/model"
)

func TestGetStateDoesNotWaitForConfigGate(t *testing.T) {
	a, _ := syncTestApp(t)
	_, err := a.storage.Update(func(c *conf.Config) error {
		c.Tunnels = []model.Tunnel{{ID: 1, Status: "running", ManuallyStopped: true}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.configGate.Lock()
	defer a.configGate.Unlock()
	type result struct {
		state model.State
		err   error
	}
	ready := make(chan result, 1)
	go func() { state, err := a.GetState(); ready <- result{state, err} }()
	select {
	case got := <-ready:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if len(got.state.Jumpers) != 1 || len(got.state.Groups) != 1 || len(got.state.Tunnels) != 1 {
			t.Fatal("incomplete configuration snapshot")
		}
		if got.state.Tunnels[0].Status != "stopped" || !got.state.Tunnels[0].ManuallyStopped {
			t.Fatal("snapshot did not overlay runtime state")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("page data waited for a network operation's gate")
	}
}

func TestToggleTunnelStartsFourthWithoutLicense(t *testing.T) {
	a, _ := syncTestApp(t)
	t.Cleanup(a.tunnel.Shutdown)

	// Hold the SSH handshakes open so all four starts compete for slots.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()
	t.Cleanup(func() { <-closed })
	_, err = a.storage.Update(func(c *conf.Config) error {
		c.Jumpers[0].Host = "127.0.0.1"
		c.Jumpers[0].Port = listener.Addr().(*net.TCPAddr).Port
		c.Jumpers[0].BypassHostVerification = true
		for id := 1; id <= 4; id++ {
			c.Tunnels = append(c.Tunnels, model.Tunnel{
				ID: id, Name: fmt.Sprintf("Tunnel %d", id), Mode: "local", Status: "stopped",
				JumperIDs: []int{1}, LocalHost: "127.0.0.1", LocalPort: 18000 + id,
				RemoteHost: "127.0.0.1", RemotePort: 5432,
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 4; id++ {
		tunnel, err := a.ToggleTunnel(id)
		if err != nil {
			t.Fatalf("start tunnel %d: %v", id, err)
		}
		if tunnel.Status != "busy" {
			t.Fatalf("start tunnel %d: expected busy, got %s", id, tunnel.Status)
		}
	}
	state, err := a.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveSlotLimit != 0 || state.ActiveSlots != 4 {
		t.Fatalf("expected four active slots with no limit, got %d with limit %d", state.ActiveSlots, state.ActiveSlotLimit)
	}
}
