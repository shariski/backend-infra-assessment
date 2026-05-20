package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllama_Generate_Success(t *testing.T) {
	var gotAuthID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthID = r.Header.Get("CF-Access-Client-Id")
		if r.URL.Path != "/api/generate" {
			t.Errorf("path = %q, want /api/generate", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"This account looks compromised."}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "llama3.2:1b", Timeout: 5 * time.Second, CFAccessClientID: "cf-id", CFAccessClientSecret: "cf-secret"})
	out, err := c.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.Contains(out, "compromised") {
		t.Errorf("Generate() = %q, want it to contain 'compromised'", out)
	}
	if gotAuthID != "cf-id" {
		t.Errorf("CF-Access-Client-Id header = %q, want cf-id", gotAuthID)
	}
}

func TestOllama_Generate_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second})
	if _, err := c.Generate(context.Background(), "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrUnavailable", err)
	}
}

func TestOllama_Generate_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"response":"too late"}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "m", Timeout: 10 * time.Millisecond})
	if _, err := c.Generate(context.Background(), "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrUnavailable on timeout", err)
	}
}

func TestNew_DisabledWhenNoURL(t *testing.T) {
	c := New(Config{BaseURL: ""})
	if _, err := c.Generate(context.Background(), "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("disabled client error = %v, want ErrUnavailable", err)
	}
}

func TestOllama_Generate_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":"   "}`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second})
	if _, err := c.Generate(context.Background(), "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrUnavailable for empty response", err)
	}
}

func TestOllama_Generate_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	c := New(Config{BaseURL: srv.URL, Model: "m", Timeout: 5 * time.Second})
	if _, err := c.Generate(context.Background(), "p"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Generate() error = %v, want ErrUnavailable for malformed JSON", err)
	}
}
