// Package server serves the editor on a loopback address: the embedded app over HTTP,
// internal/session's messages over a WebSocket, and outside changes from fsnotify (FR-13).
package server

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/nvcnvn/dg/internal/fsys"
	"github.com/nvcnvn/dg/internal/session"
	"github.com/nvcnvn/dg/web"
)

const writeTimeout = 10 * time.Second

// Server serves the session of the project in a directory.
type Server struct {
	dir     string
	session *session.Session
}

// New loads the project in dir.
func New(dir string) *Server {
	return &Server{dir: dir, session: session.New(fsys.Disk(dir))}
}

// Handler serves the app at / and the WebSocket at /ws, only to requests whose Host is a
// loopback address, so a page on another site cannot reach it by rebinding its name to
// 127.0.0.1.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", web.Handler())
	mux.HandleFunc("/ws", s.serveWS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsLoopback(r.Host) {
			http.Error(w, "dg serve answers only on a loopback address", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// IsLoopback reports whether hostport, with or without a port, names a loopback address.
func IsLoopback(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = strings.Trim(hostport, "[]")
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Reload loads the project again and updates every client.
func (s *Server) Reload(ctx context.Context) {
	s.session.Reload(ctx)
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	c := s.session.Connect(func(ctx context.Context, data []byte) {
		ctx, cancel := context.WithTimeout(ctx, writeTimeout)
		defer cancel()
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			_ = conn.CloseNow()
		}
	})
	defer func() {
		s.session.Disconnect(c)
		_ = conn.CloseNow()
	}()
	ctx := r.Context()
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		s.session.Receive(ctx, c, data)
	}
}
