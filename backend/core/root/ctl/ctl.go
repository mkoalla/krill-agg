// Package ctl lets a second invocation of the binary (`main ctl <cmd> ...`,
// e.g. from `docker compose exec`) drive the running bot over a unix socket.
// Only something already inside the container can reach it, so there's no auth.
// The wire format is plain http: POST /<cmd> with form args, text back.
package ctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// inside the container, never on the shared data/ mount (dailygen can see that)
const SOCKET = "/tmp/krill-agg.sock"

// a render can take a while; this only bounds a hung bot
const CALL_TIMEOUT = 5 * time.Minute

// runs one command; the string is printed by the caller on success
type Handler = func(args url.Values) (string, error)

type Server struct {
	srv  *http.Server
	path string
}

// listens on path, replacing a socket left behind by a previous run. report
// sees every call's outcome, for logging
func Serve(path string, handlers map[string]Handler, report func(cmd string, args url.Values, out string, err error)) (*Server, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close() //nolint
		return nil, err
	}
	mux := http.NewServeMux()
	for cmd, h := range handlers {
		mux.HandleFunc("POST /"+cmd, func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			out, err := h(r.PostForm)
			report(cmd, r.PostForm, out, err)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			io.WriteString(w, out) //nolint
		})
	}
	s := &Server{srv: &http.Server{Handler: mux}, path: path}
	go s.srv.Serve(ln) //nolint // returns ErrServerClosed on Close
	return s, nil
}

func (s *Server) Close() error {
	err := s.srv.Close()
	os.Remove(s.path) //nolint
	return err
}

// sends cmd to the bot listening on path; the error carries the bot's reason
func Call(path, cmd string, args url.Values) (string, error) {
	client := &http.Client{
		Timeout: CALL_TIMEOUT,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", path)
			},
		},
	}
	// the host is ignored; the transport always dials the socket
	resp, err := client.PostForm("http://bot/"+url.PathEscape(cmd), args)
	if err != nil {
		return "", fmt.Errorf("is the bot running? %v", err)
	}
	defer resp.Body.Close() //nolint
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(body))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", cmd, text)
	}
	return text, nil
}
