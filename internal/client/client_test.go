package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotFoundAndErrorMessages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		switch r.URL.Path {
		case "/v1/tenants/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/tenants/broken":
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"backend step \"state\" failed"}`))
		default:
			_, _ = w.Write([]byte(`{"name":"tdemo","index":2,"status":"ready","dns":{"zone":"tdemo.mobile.deevnet.net"}}`))
		}
	}))
	defer srv.Close()

	c, err := New(Config{Endpoint: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	got, err := c.GetTenant(ctx, "tdemo")
	if err != nil || got.Index != 2 || got.DNS.Zone != "tdemo.mobile.deevnet.net" {
		t.Fatalf("get = %+v %v", got, err)
	}
	if _, err := c.GetTenant(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing tenant: %v, want ErrNotFound", err)
	}
	_, err = c.GetTenant(ctx, "broken")
	if err == nil || !strings.Contains(err.Error(), `backend step "state" failed`) {
		t.Fatalf("failed step: %v, want the API's message", err)
	}

	bad, _ := New(Config{Endpoint: srv.URL, Token: "wrong"})
	if _, err := bad.GetTenant(ctx, "tdemo"); err == nil || !strings.Contains(err.Error(), "401 unauthorized") {
		t.Fatalf("bad token: %v", err)
	}

	if _, err := New(Config{Endpoint: srv.URL}); err == nil {
		t.Error("a client with no token was accepted")
	}
	if _, err := New(Config{Endpoint: srv.URL, Token: "t", CACertificate: "/nonexistent"}); err == nil {
		t.Error("a missing CA file was accepted")
	}
}
