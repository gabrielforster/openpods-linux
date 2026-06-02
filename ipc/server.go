package ipc

import (
	"bufio"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	socketName   = "openpods.sock"
	writeTimeout = 5 * time.Second
)

// DefaultSocketPath returns the daemon socket path: $XDG_RUNTIME_DIR/openpods.sock,
// falling back to the OS temp dir when XDG_RUNTIME_DIR is unset.
func DefaultSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, socketName)
	}
	return filepath.Join(os.TempDir(), socketName)
}

// Server streams status snapshots to connected frontends over a Unix socket as
// newline-delimited JSON. On connect a client immediately receives the current
// snapshot, then every subsequent Broadcast. It is read-only.
type Server struct {
	ln net.Listener

	mu          sync.Mutex
	current     Snapshot
	haveCurrent bool
	clients     map[*serverConn]struct{}

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

type serverConn struct {
	conn    net.Conn
	updates chan Snapshot // buffered(1), latest-wins
}

// NewServer listens on the given Unix socket path. A stale socket file from a
// previous run is removed first.
func NewServer(path string) (*Server, error) {
	_ = os.Remove(path) // best effort: clear a leftover socket
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	s := &Server{
		ln:      ln,
		clients: make(map[*serverConn]struct{}),
		done:    make(chan struct{}),
	}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.done: // expected during Close
			default:
				slog.Warn("openpods: accept failed", "err", err)
			}
			return
		}
		s.add(conn)
	}
}

func (s *Server) add(conn net.Conn) {
	sc := &serverConn{conn: conn, updates: make(chan Snapshot, 1)}
	s.mu.Lock()
	s.clients[sc] = struct{}{}
	if s.haveCurrent {
		sc.updates <- s.current // buffer is empty; never blocks
	}
	s.mu.Unlock()

	s.wg.Add(1)
	go s.serve(sc)
}

func (s *Server) serve(sc *serverConn) {
	defer s.wg.Done()
	defer s.remove(sc)
	defer sc.conn.Close()

	for {
		select {
		case <-s.done:
			return
		case snap := <-sc.updates:
			line, err := Encode(snap)
			if err != nil {
				continue
			}
			_ = sc.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if _, err := sc.conn.Write(line); err != nil {
				return // client gone or too slow
			}
		}
	}
}

func (s *Server) remove(sc *serverConn) {
	s.mu.Lock()
	delete(s.clients, sc)
	s.mu.Unlock()
}

// Broadcast records snap as the current status and pushes it to every connected
// client (latest-wins per client, so a slow reader never stalls the daemon).
func (s *Server) Broadcast(snap Snapshot) {
	s.mu.Lock()
	s.current = snap
	s.haveCurrent = true
	for sc := range s.clients {
		pushLatestSnap(sc.updates, snap)
	}
	s.mu.Unlock()
}

func (s *Server) Close() error {
	s.closeOnce.Do(func() { close(s.done) })
	err := s.ln.Close()
	s.mu.Lock()
	for sc := range s.clients {
		sc.conn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}

func pushLatestSnap(ch chan Snapshot, s Snapshot) {
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- s:
	default:
	}
}

// Client reads status snapshots from the daemon socket.
type Client struct {
	conn net.Conn
	r    *bufio.Reader
}

// Dial connects to the daemon socket at path.
func Dial(path string) (*Client, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, r: bufio.NewReader(conn)}, nil
}

// Read returns the next snapshot. Use it once for a one-shot read, or in a loop
// to watch for changes.
func (c *Client) Read() (Snapshot, error) {
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		return Snapshot{}, err
	}
	return Decode(line)
}

func (c *Client) Close() error { return c.conn.Close() }
