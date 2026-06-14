package backends

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

// --- isDeadConn ---

func TestIsDeadConn(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"io.EOF", io.EOF, true},
		{"wrapped io.EOF", fmt.Errorf("wrap: %w", io.EOF), true},
		{"connection closed msg", errors.New("connection closed"), true},
		{"broken pipe msg", errors.New("broken pipe"), true},
		{"closed network msg", errors.New("use of closed network connection"), true},
		{"generic error", errors.New("permission denied"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isDeadConn(tc.err)
			if got != tc.want {
				t.Errorf("isDeadConn(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// fakeNetError satisfies net.Error.
type fakeNetError struct{ msg string }

func (e *fakeNetError) Error() string    { return e.msg }
func (e *fakeNetError) Timeout() bool   { return false }
func (e *fakeNetError) Temporary() bool { return false }

var _ net.Error = (*fakeNetError)(nil)

func TestIsDeadConn_NetError(t *testing.T) {
	err := &fakeNetError{msg: "some network failure"}
	if !isDeadConn(err) {
		t.Errorf("expected net.Error to be treated as dead conn")
	}
}

// --- minimal in-process SSH server for pool tests ---

// startMinimalSSHServer starts a loopback SSH server on a random port.
// Returns a dial function that produces *ssh.Client instances, and a cleanup function.
func startMinimalSSHServer(t *testing.T) (dialFn func() (*ssh.Client, error), cleanup func()) {
	t.Helper()

	hostKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}

	serverCfg := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	serverCfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				sConn, chans, reqs, err := ssh.NewServerConn(c, serverCfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				go func() {
					for range chans {
					}
				}()
				_ = sConn
			}(conn)
		}
	}()

	addr := ln.Addr().String()
	clientCfg := &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // test-only
	}
	dialFn = func() (*ssh.Client, error) {
		return ssh.Dial("tcp", addr, clientCfg)
	}
	cleanup = func() { _ = ln.Close() }
	return dialFn, cleanup
}

// semLen returns the number of available slots in the semaphore.
func semLen(p *clientPool, wsID uuid.UUID) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch, ok := p.sem[wsID]
	if !ok {
		return -1
	}
	return len(ch)
}

// newTestPool creates a bare clientPool without the prune goroutine.
func newTestPool() *clientPool {
	return &clientPool{
		clients:  make(map[uuid.UUID][]*pooledClient),
		circuits: make(map[uuid.UUID]*circuitState),
		sem:      make(map[uuid.UUID]chan struct{}),
		stopCh:   make(chan struct{}),
	}
}

// TestEvict_SemaphoreInvariant verifies that Evict returns the one semaphore slot
// that was consumed when the client was originally dialed.
func TestEvict_SemaphoreInvariant(t *testing.T) {
	dialFn, cleanup := startMinimalSSHServer(t)
	defer cleanup()

	p := newTestPool()
	wsID := uuid.New()

	// Initialise semaphore and consume one slot — simulates a dial.
	p.mu.Lock()
	sem := p.semFor(wsID)
	p.mu.Unlock()
	<-sem

	c, err := dialFn()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	p.mu.Lock()
	p.clients[wsID] = append(p.clients[wsID], &pooledClient{client: c, refCnt: 0})
	p.mu.Unlock()

	slotsBefore := semLen(p, wsID)
	if slotsBefore != maxClientsPerWorkstation-1 {
		t.Fatalf("expected %d slots before evict, got %d", maxClientsPerWorkstation-1, slotsBefore)
	}

	p.Evict(wsID, c)

	slotsAfter := semLen(p, wsID)
	if slotsAfter != maxClientsPerWorkstation {
		t.Fatalf("expected %d slots after evict, got %d (semaphore corrupted)", maxClientsPerWorkstation, slotsAfter)
	}

	p.mu.Lock()
	remaining := len(p.clients[wsID])
	p.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected 0 pool entries after evict, got %d", remaining)
	}
}

// TestEvict_Idempotent verifies a second Evict call is a no-op and does not corrupt
// the semaphore by returning an extra slot.
func TestEvict_Idempotent(t *testing.T) {
	dialFn, cleanup := startMinimalSSHServer(t)
	defer cleanup()

	p := newTestPool()
	wsID := uuid.New()

	p.mu.Lock()
	sem := p.semFor(wsID)
	p.mu.Unlock()
	<-sem

	c, err := dialFn()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	p.mu.Lock()
	p.clients[wsID] = append(p.clients[wsID], &pooledClient{client: c, refCnt: 0})
	p.mu.Unlock()

	p.Evict(wsID, c)
	p.Evict(wsID, c) // second call must be no-op

	slotsAfter := semLen(p, wsID)
	if slotsAfter != maxClientsPerWorkstation {
		t.Fatalf("double-evict: expected %d slots, got %d (semaphore leaked)", maxClientsPerWorkstation, slotsAfter)
	}
}

// TestEvict_RefcountedClient verifies that evicting a borrowed client (refCnt>0)
// still returns exactly one slot — the slot it consumed at dial time.
func TestEvict_RefcountedClient(t *testing.T) {
	dialFn, cleanup := startMinimalSSHServer(t)
	defer cleanup()

	p := newTestPool()
	wsID := uuid.New()

	p.mu.Lock()
	sem := p.semFor(wsID)
	p.mu.Unlock()
	<-sem

	c, err := dialFn()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	p.mu.Lock()
	p.clients[wsID] = append(p.clients[wsID], &pooledClient{client: c, refCnt: 1}) // active borrow
	p.mu.Unlock()

	p.Evict(wsID, c)

	slotsAfter := semLen(p, wsID)
	if slotsAfter != maxClientsPerWorkstation {
		t.Fatalf("evict with refCnt>0: expected %d slots, got %d", maxClientsPerWorkstation, slotsAfter)
	}
}
