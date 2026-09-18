package client

import (
	"context"
	"errors"
	"io"
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

// An API that does not report secrets_stored is older than the field, and must
// not be read as "cannot read its secrets" - that would replan a resupply on
// every apply forever.
func TestSecretsStoredIsThreeStated(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string // "absent", "true" or "false"
	}{
		{"older API omits it", `{"name":"tdemo","index":1}`, "absent"},
		{"readable", `{"name":"tdemo","index":1,"secrets_stored":true}`, "true"},
		{"unreadable", `{"name":"tdemo","index":1,"secrets_stored":false}`, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			c, err := New(Config{Endpoint: srv.URL, Token: "t"})
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.GetTenant(context.Background(), "tdemo")
			if err != nil {
				t.Fatal(err)
			}
			switch tc.want {
			case "absent":
				if got.SecretsStored != nil {
					t.Fatalf("want nil, got %v", *got.SecretsStored)
				}
			case "true":
				if got.SecretsStored == nil || !*got.SecretsStored {
					t.Fatalf("want true, got %v", got.SecretsStored)
				}
			case "false":
				if got.SecretsStored == nil || *got.SecretsStored {
					t.Fatalf("want false, got %v", got.SecretsStored)
				}
			}
		})
	}
}
