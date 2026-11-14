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

// A first apply names no address; a restore names the one state holds. And a
// 502 that carries the record hands it back with the error, so the caller keeps
// the address it was given.
func TestDeviceAddressRequestAndPartialFailure(t *testing.T) {
	var gotPath, gotBody string
	status := http.StatusCreated
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotPath, gotBody = r.URL.Path, strings.TrimSpace(string(b))
		w.WriteHeader(status)
		if status == http.StatusBadGateway {
			_, _ = w.Write([]byte(`{"error":"backend step \"device-address\" failed","address":{"tenant":"eds","device":"stand-1","address":"10.20.30.25","status":"provisioning"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"tenant":"eds","device":"stand-1","address":"10.20.30.25","mac":"aa:bb:cc:dd:ee:ff","fqdn":"stand-1.eds.mobile.deevnet.net","status":"ready"}`))
	}))
	defer srv.Close()
	c, err := New(Config{Endpoint: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	a, err := c.PutDeviceAddress(ctx, "eds", "stand-1", PutDeviceAddressRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/tenants/eds/devices/stand-1/address" || gotBody != "{}" {
		t.Errorf("first apply sent %s %s, want no address named", gotPath, gotBody)
	}
	if a.Address != "10.20.30.25" || a.FQDN != "stand-1.eds.mobile.deevnet.net" || a.Status != "ready" {
		t.Errorf("answer = %+v", a)
	}

	if _, err := c.PutDeviceAddress(ctx, "eds", "stand-1", PutDeviceAddressRequest{Address: "10.20.30.25"}); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"address":"10.20.30.25"}` {
		t.Errorf("restore sent %s", gotBody)
	}

	status = http.StatusBadGateway
	a, err = c.PutDeviceAddress(ctx, "eds", "stand-1", PutDeviceAddressRequest{})
	if err == nil {
		t.Fatal("a 502 must be an error")
	}
	if a.Address != "10.20.30.25" || a.Status != "provisioning" {
		t.Errorf("partial = %+v, want the address the API recorded", a)
	}
}

// A certificate request carries a signing request and labels, by name, and
// nothing else: there is no lifetime or usage to send.
func TestCertificateRequests(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(raw)
		if strings.HasSuffix(r.URL.Path, "/missing") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"tenant":"eds","name":"console","fqdn":"console.eds.mobile.deevnet.net",
		  "names":["console.eds.mobile.deevnet.net","www.eds.mobile.deevnet.net"],
		  "certificate":"LEAF","ca_chain":"CHAIN","serial":"0a","key_fingerprint":"abc",
		  "not_before":"2026-10-11T12:00:00Z","not_after":"2027-10-11T12:00:00Z","renew_after":"2027-08-12T12:00:00Z"}`))
	}))
	defer srv.Close()
	c, err := New(Config{Endpoint: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	got, err := c.PutCertificate(ctx, "eds", "console", PutCertificateRequest{CSR: "CSR", Names: []string{"www"}})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotPath != "/v1/tenants/eds/certificates/console" {
		t.Errorf("called %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"csr":"CSR","names":["www"]}` {
		t.Errorf("sent %s", gotBody)
	}
	if got.FQDN != "console.eds.mobile.deevnet.net" || len(got.Names) != 2 || got.RenewAfter != "2027-08-12T12:00:00Z" {
		t.Errorf("got %+v", got)
	}

	// Sites go as they are written.
	if _, err := c.PutCertificate(ctx, "eds", "console", PutCertificateRequest{CSR: "CSR", Sites: []string{"mobile", "home"}}); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"csr":"CSR","sites":["mobile","home"]}` {
		t.Errorf("sent %s", gotBody)
	}

	// With no further names the field is left out, not sent empty.
	if _, err := c.PutCertificate(ctx, "eds", "console", PutCertificateRequest{CSR: "CSR"}); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"csr":"CSR"}` {
		t.Errorf("sent %s", gotBody)
	}

	if _, err := c.GetCertificate(ctx, "eds", "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a missing certificate: %v", err)
	}
	if err := c.DeleteCertificate(ctx, "eds", "console"); err != nil || gotMethod != http.MethodDelete {
		t.Errorf("delete: %s %v", gotMethod, err)
	}
}

// A secret's path keeps its slashes as separators; each segment is escaped by
// itself. The value goes out, and nothing about it comes back.
func TestSecretRequests(t *testing.T) {
	var gotMethod, gotRawPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotMethod, gotRawPath, gotBody = r.Method, r.URL.EscapedPath(), string(raw)
		switch {
		case strings.HasSuffix(r.URL.Path, "/missing"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/secret-readers/"):
			_, _ = w.Write([]byte(`{"name":"lightd","workload":"lightd","paths":["weather/api"],
			  "role_id":"r","secret_id":"s","namespace":"tenants/eds","addr":"https://bao:8200"}`))
		case strings.HasSuffix(r.URL.Path, "/offline"):
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"tenant secrets are not offered at this site"}`))
		default:
			_, _ = w.Write([]byte(`{"path":"weather/api","version":3,"present":true}`))
		}
	}))
	defer srv.Close()
	c, err := New(Config{Endpoint: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	s, err := c.PutSecret(ctx, "eds", "weather/api", PutSecretRequest{Data: map[string]string{"k": "v"}, Version: 3})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut || gotRawPath != "/v1/tenants/eds/secrets/weather/api" {
		t.Errorf("called %s %s", gotMethod, gotRawPath)
	}
	if gotBody != `{"data":{"k":"v"},"version":3}` {
		t.Errorf("sent %s", gotBody)
	}
	if s.Version != 3 || !s.Present || s.Path != "weather/api" {
		t.Errorf("got %+v", s)
	}
	if _, err := c.GetSecret(ctx, "eds", "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a 404 read is %v", err)
	}
	if err := c.DeleteSecret(ctx, "eds", "weather/api"); err != nil || gotMethod != http.MethodDelete {
		t.Errorf("delete: %v, %s", err, gotMethod)
	}
	// A site without the service says so, and its words are what surface.
	if _, err := c.PutSecret(ctx, "eds", "offline", PutSecretRequest{Version: 1}); err == nil ||
		!strings.Contains(err.Error(), "not offered at this site") {
		t.Errorf("err = %v", err)
	}

	rd, err := c.PutSecretReader(ctx, "eds", "lightd", PutSecretReaderRequest{Workload: "lightd", Paths: []string{"weather/api"}})
	if err != nil {
		t.Fatal(err)
	}
	if gotRawPath != "/v1/tenants/eds/secret-readers/lightd" || gotBody != `{"workload":"lightd","paths":["weather/api"]}` {
		t.Errorf("called %s with %s", gotRawPath, gotBody)
	}
	if rd.SecretID != "s" || rd.RoleID != "r" || rd.Namespace != "tenants/eds" {
		t.Errorf("got %+v", rd)
	}
	if _, err := c.GetSecretReader(ctx, "eds", "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a 404 read is %v", err)
	}
}

func TestSecretPathEscapesEachSegment(t *testing.T) {
	if got := secretPath("eds", "a b/c?d"); got != "/v1/tenants/eds/secrets/a%20b/c%3Fd" {
		t.Errorf("got %s", got)
	}
}
