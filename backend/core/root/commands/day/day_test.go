package day

import "testing"

func TestMessage(t *testing.T) {
	// #125 starts at midnight EST on 2026-11-17 = 05:00 UTC
	if got, want := message(124), "Krillion #124\n-# #125 <t:1794891600:R>"; got != want {
		t.Errorf("message(124) = %q, want %q", got, want)
	}
}
