package ctl

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ctl.sock")
	var seen []string
	srv, err := Serve(path, map[string]Handler{
		"echo": func(args url.Values) (string, error) { return strings.Join(args["v"], ","), nil },
		"fail": func(url.Values) (string, error) { return "", errors.New("nope") },
	}, func(cmd string, _ url.Values, _ string, _ error) { seen = append(seen, cmd) })
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close() //nolint

	if out, err := Call(path, "echo", url.Values{"v": {"a", "b"}}); err != nil || out != "a,b" {
		t.Errorf("echo = %q, %v", out, err)
	}
	if _, err := Call(path, "fail", nil); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("fail error = %v", err)
	}
	if _, err := Call(path, "missing", nil); err == nil {
		t.Error("unknown command should fail")
	}
	if strings.Join(seen, " ") != "echo fail" {
		t.Errorf("reported %v", seen)
	}

	// a restart replaces the stale socket
	srv.Close() //nolint
	srv, err = Serve(path, nil, func(string, url.Values, string, error) {})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	srv.Close() //nolint
	if _, err := Call(path, "echo", nil); err == nil {
		t.Error("call after close should fail")
	}
}
