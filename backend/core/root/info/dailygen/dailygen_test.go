package dailygen

import (
	"encoding/json"
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
	path, err := Render(Request{Guild: "1", Day: 124, Users: []User{{415}, {300}}})
	if err != nil || path != "1/daily_124.png" {
		t.Fatalf("Render() = %q, %v", path, err)
	}
}

func TestRenderErrors(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"guild must be a discord id"}`)) //nolint
	})
	if _, err := Render(Request{}); err == nil || !strings.Contains(err.Error(), "guild must be a discord id") {
		t.Fatalf("err = %v, want the server's reason", err)
	}

	serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>proxy</html>")) //nolint
	})
	if _, err := Render(Request{}); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("non-json err = %v", err)
	}
}

func TestRenderTimeout(t *testing.T) {
	serve(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(200 * time.Millisecond) })
	old := client.Timeout
	client.Timeout = 50 * time.Millisecond
	defer func() { client.Timeout = old }()
	if _, err := Render(Request{}); err == nil {
		t.Fatal("expected a timeout")
	}
}
