package main

import "testing"

func TestStripOuterFence(t *testing.T) {
	wrapped := "```\n---\ntitle: \"x\"\n---\nbody\n```"
	if got := stripOuterFence(wrapped); got != "---\ntitle: \"x\"\n---\nbody" {
		t.Fatalf("got %q", got)
	}
	plain := "---\ntitle: \"x\"\n---\nbody"
	if got := stripOuterFence(plain); got != plain {
		t.Fatalf("plain mangled: %q", got)
	}
	// fence without closing fence: leave untouched
	if got := stripOuterFence("```\nbody only"); got != "```\nbody only" {
		t.Fatalf("got %q", got)
	}
}
