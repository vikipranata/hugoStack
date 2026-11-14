package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func canned(t *testing.T, body string, code int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q, want /chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k1" {
			t.Errorf("Authorization = %q", got)
		}
		var req struct {
			Model     string    `json:"model"`
			Messages  []Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Model != "m1" {
			t.Errorf("model = %q", req.Model)
		}
		if len(req.Messages) == 0 {
			t.Errorf("messages empty")
		}
		w.WriteHeader(code)
		io.WriteString(w, body)
	}))
}

func TestChatReturnsContent(t *testing.T) {
	srv := canned(t, `{"choices":[{"message":{"content":"Halo, dunia!"}}]}`, 200)
	defer srv.Close()
	out, err := New(srv.URL, "k1", "m1").Chat(context.Background(),
		[]Message{{Role: "system", Content: "s"}, {Role: "user", Content: "u"}})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Halo, dunia!" {
		t.Fatalf("out = %q", out)
	}
}

func TestChatNon2xxStructuredError(t *testing.T) {
	srv := canned(t, `{"error":"boom"}`, 500)
	defer srv.Close()
	out, err := New(srv.URL, "k1", "m1").Chat(context.Background(),
		[]Message{{Role: "user", Content: "u"}})
	if err == nil {
		t.Fatal("want error on 500")
	}
	if out != "" {
		t.Fatalf("out = %q, want empty", out)
	}
	for _, want := range []string{"500", "boom"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to contain %q", err, want)
		}
	}
}
