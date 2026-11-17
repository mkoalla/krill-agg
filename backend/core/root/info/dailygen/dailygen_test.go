package dailygen

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func serve(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	old := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { srv.Close(); baseURL = old })
}

func TestRenderSuccess(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if r.URL.Path != "/render" || json.NewDecoder(r.Body).Decode(&req) != nil || req.Guild != "1" || len(req.Users) != 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write([]byte(`{"path":"1/daily_124.png"}`)) //nolint
	})
	path, err := Render(context.Background(), Request{Guild: "1", Day: 124, Users: []User{{415}, {300}}})
	if err != nil || path != "1/daily_124.png" {
		t.Fatalf("Render() = %q, %v", path, err)
	}
}

func TestRenderErrors(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"guild must be a discord id"}`)) //nolint
	})
	if _, err := Render(context.Background(), Request{}); err == nil || !strings.Contains(err.Error(), "guild must be a discord id") {
		t.Fatalf("err = %v, want the server's reason", err)
	}

	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>proxy</html>")) //nolint
	})
	if _, err := Render(context.Background(), Request{}); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("non-json err = %v", err)
	}
}

func TestRenderTimeout(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) })
	old := client.Timeout
	client.Timeout = 50 * time.Millisecond
	defer func() { client.Timeout = old }()
	if _, err := Render(context.Background(), Request{}); err == nil {
		t.Fatal("expected a timeout")
	}
}

func TestRenderGivesUpAtDeadline(t *testing.T) {
	got := make(chan Request, 1) // handed over, not shared: the handler runs on its own goroutine
	release := make(chan struct{})
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		var req Request
		json.NewDecoder(r.Body).Decode(&req) //nolint
		got <- req
		<-release // a render that never finishes in time
	})
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Render(ctx, Request{Guild: "1"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Render() = %v, want a deadline error", err)
	}
	if waited := time.Since(start); waited > time.Second {
		t.Errorf("waited %v past a 50ms deadline", waited)
	}
	deadline, _ := ctx.Deadline()
	sent := <-got
	if sent.Deadline != deadline.UnixMilli() {
		t.Errorf("renderer was told deadline %d, want %d", sent.Deadline, deadline.UnixMilli())
	}
}
