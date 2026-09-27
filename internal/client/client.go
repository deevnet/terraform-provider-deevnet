// Package client talks to the Deevnet API (ADR-0015).
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Client calls the Deevnet API with one bearer token: an operator token, a
// tenant's own token, or a single-use enrollment token.
type Client struct {
	base string
	mu   sync.RWMutex
	// Guarded because Terraform applies resources concurrently, and the token
	// changes mid-run: see UseToken.
	token string
	http  *http.Client
}

// UseToken switches the token every later call carries. A first apply is
// configured with the single-use enrollment token, which creating the tenant
// spends; the tenant's own token comes back in that response, and its workloads
// and names are created with it in the same run. Without this the apply would
// get as far as the tenant and then fail 401 on everything after it.
func (c *Client) UseToken(token string) {
	if token == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

func (c *Client) bearer() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

type Config struct {
	Endpoint string
	Token    string
	// CACertificate is the site CA the API's certificate comes from. Empty
	// means the system trust store.
	CACertificate string
}

func New(cfg Config) (*Client, error) {
	if cfg.Endpoint == "" || cfg.Token == "" {
		return nil, errors.New("endpoint and token are required")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.CACertificate != "" {
		pem, err := os.ReadFile(cfg.CACertificate)
		if err != nil {
			return nil, fmt.Errorf("ca_certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("ca_certificate holds no certificate")
		}
		tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &Client{
		base:  strings.TrimRight(cfg.Endpoint, "/"),
		token: cfg.Token,
		http:  &http.Client{Timeout: 15 * time.Minute, Transport: tr},
	}, nil
}

// ErrNotFound is a 404. The provider answers it by restoring rather than
// forgetting (ADR-0012 §5, ADR-0015 §5).
var ErrNotFound = errors.New("not found")

// Tenant is what the API returns for a tenant.
type Tenant struct {
	Name    string `json:"name"`
	Index   int64  `json:"index"`
	Status  string `json:"status"`
	Outcome string `json:"outcome,omitempty"`
	Network struct {
		VRFVNI      int64  `json:"vrf_vni"`
		VNetVNIBase int64  `json:"vnet_vni_base"`
		Subnet      string `json:"subnet"`
		Gateway     string `json:"gateway"`
	} `json:"network"`
	Fabric struct {
		ControllerID string `json:"controller_id"`
		Node         string `json:"node"`
	} `json:"fabric"`
	DNS struct {
		Zone          string `json:"zone"`
		ReverseZone   string `json:"reverse_zone"`
		UpdateServer  string `json:"update_server"`
		TSIGKeyName   string `json:"tsig_key_name"`
		TSIGAlgorithm string `json:"tsig_algorithm"`
		TSIGSecret    string `json:"tsig_secret,omitempty"`
	} `json:"dns"`
	State struct {
		Endpoint  string `json:"endpoint"`
		Bucket    string `json:"bucket"`
		KeyPrefix string `json:"key_prefix"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key,omitempty"`
	} `json:"state"`
	// Log is the tenant's access to the log store (ADR-0027). The tokens
	// appear on create and on reconcile; the rest on every read.
	Log struct {
		Endpoint     string `json:"endpoint,omitempty"`
		AccountID    int64  `json:"account_id"`
		SelectHeader string `json:"select_header,omitempty"`
		IngestToken  string `json:"ingest_token,omitempty"`
		ReadToken    string `json:"read_token,omitempty"`
	} `json:"log"`
	// Dashboard is the tenant's Grafana login (ADR-0024, CHG-0024): its own
	// organisation, where it is an Editor. The password appears on create and
	// on reconcile; the rest on every read, once the organisation exists.
	Dashboard struct {
		URL      string `json:"url,omitempty"`
		OrgID    int64  `json:"org_id,omitempty"`
		Username string `json:"username,omitempty"`
		Password string `json:"password,omitempty"`
	} `json:"dashboard"`
	APIToken string `json:"api_token,omitempty"`
	// SecretsStored is false when the API holds secrets for this tenant that it
	// can no longer read, which is what a rebuilt or rotated Transit key leaves
	// behind. The tenant's state is the authoritative copy, so the answer is to
	// send them again. An API that does not report the field at all is older than
	// it: see tenantFrom.
	SecretsStored *bool `json:"secrets_stored,omitempty"`
}

// CreateTenantRequest creates a tenant, or restores one: a restore carries the
// index and all three secrets, and the API keeps or reissues the index.
type CreateTenantRequest struct {
	Name        string `json:"name"`
	Index       int64  `json:"index,omitempty"`
	TSIGSecret  string `json:"tsig_secret,omitempty"`
	StateSecret string `json:"state_secret,omitempty"`
	APIToken    string `json:"api_token,omitempty"`
}

func (c *Client) CreateTenant(ctx context.Context, req CreateTenantRequest) (Tenant, error) {
	var out Tenant
	err := c.do(ctx, http.MethodPost, "/v1/tenants", req, &out)
	return out, err
}

func (c *Client) GetTenant(ctx context.Context, name string) (Tenant, error) {
	var out Tenant
	err := c.do(ctx, http.MethodGet, "/v1/tenants/"+name, nil, &out)
	return out, err
}

func (c *Client) DeleteTenant(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tenants/"+name, nil, nil)
}

// Workload is a tenant VM.
type Workload struct {
	Tenant   string `json:"tenant"`
	Name     string `json:"name"`
	FQDN     string `json:"fqdn"`
	Status   string `json:"status"`
	Ordinal  int64  `json:"ordinal"`
	VMID     int64  `json:"vmid"`
	MAC      string `json:"mac"`
	Address  string `json:"address"`
	Cores    int64  `json:"cores"`
	MemoryMB int64  `json:"memory_mb"`
	DiskGB   int64  `json:"disk_gb,omitempty"`
}

type CreateWorkloadRequest struct {
	Name     string   `json:"name"`
	Cores    int64    `json:"cores,omitempty"`
	MemoryMB int64    `json:"memory_mb,omitempty"`
	DiskGB   int64    `json:"disk_gb,omitempty"`
	SSHKeys  []string `json:"ssh_keys,omitempty"`
}

func (c *Client) PutWorkload(ctx context.Context, tenant string, req CreateWorkloadRequest) (Workload, error) {
	var out Workload
	err := c.do(ctx, http.MethodPost, "/v1/tenants/"+tenant+"/workloads", req, &out)
	return out, err
}

func (c *Client) GetWorkload(ctx context.Context, tenant, name string) (Workload, error) {
	var out Workload
	err := c.do(ctx, http.MethodGet, "/v1/tenants/"+tenant+"/workloads/"+name, nil, &out)
	return out, err
}

func (c *Client) DeleteWorkload(ctx context.Context, tenant, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tenants/"+tenant+"/workloads/"+name, nil, nil)
}

// WiFiKey is a tenant's PPSK key for one IoT trust class (ADR-0012 §3).
type WiFiKey struct {
	Tenant     string `json:"tenant"`
	Name       string `json:"name"`
	TrustClass string `json:"trust_class"`
	// SSID and VLAN are reported by the API, never chosen by the tenant.
	SSID   string `json:"ssid"`
	VLAN   int64  `json:"vlan"`
	MAC    string `json:"mac,omitempty"`
	Status string `json:"status"`
	// PSK comes back on a create only. A read answers SecretsStored instead.
	PSK           string `json:"psk,omitempty"`
	SecretsStored bool   `json:"secrets_stored"`
}

type PutWiFiKeyRequest struct {
	Name       string `json:"name"`
	TrustClass string `json:"trust_class"`
	// PSK is sent only to restore a key the tenant already holds, so the
	// controller is made to match devices already flashed (ADR-0012 §5).
	PSK string `json:"psk,omitempty"`
	// MAC binds the key to one client (ADR-0029). Empty unbinds.
	MAC string `json:"mac,omitempty"`
}

func (c *Client) PutWiFiKey(ctx context.Context, tenant string, req PutWiFiKeyRequest) (WiFiKey, error) {
	var out WiFiKey
	err := c.do(ctx, http.MethodPost, "/v1/tenants/"+tenant+"/wifi-keys", req, &out)
	return out, err
}

func (c *Client) GetWiFiKey(ctx context.Context, tenant, name string) (WiFiKey, error) {
	var out WiFiKey
	err := c.do(ctx, http.MethodGet, "/v1/tenants/"+tenant+"/wifi-keys/"+name, nil, &out)
	return out, err
}

func (c *Client) DeleteWiFiKey(ctx context.Context, tenant, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tenants/"+tenant+"/wifi-keys/"+name, nil, nil)
}

// Device is one of a tenant's edge devices in the registry (ADR-0012 §3).
// The entry is the device's identity; it carries no credential and grants
// nothing (ADR-0020 §2).
type Device struct {
	Tenant     string `json:"tenant"`
	Name       string `json:"name"`
	TrustClass string `json:"trust_class"`
	// MAC is a label for the owner's own inventory. The substrate enforces
	// nothing with it.
	MAC    string `json:"mac,omitempty"`
	Status string `json:"status"`
}

type PutDeviceRequest struct {
	Name       string `json:"name"`
	TrustClass string `json:"trust_class"`
	MAC        string `json:"mac,omitempty"`
}

func (c *Client) PutDevice(ctx context.Context, tenant string, req PutDeviceRequest) (Device, error) {
	var out Device
	err := c.do(ctx, http.MethodPost, "/v1/tenants/"+tenant+"/devices", req, &out)
	return out, err
}

func (c *Client) GetDevice(ctx context.Context, tenant, name string) (Device, error) {
	var out Device
	err := c.do(ctx, http.MethodGet, "/v1/tenants/"+tenant+"/devices/"+name, nil, &out)
	return out, err
}

func (c *Client) DeleteDevice(ctx context.Context, tenant, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tenants/"+tenant+"/devices/"+name, nil, nil)
}

// BrokerAccount is a tenant's MQTT account on the platform broker
// (ADR-0012 §3, §8, §10).
type BrokerAccount struct {
	Tenant string `json:"tenant"`
	Name   string `json:"name"`
	Device string `json:"device,omitempty"`
	// Username is derived by the API: <tenant>-<name>. Reported so a tenant can
	// configure a device without being able to choose it.
	Username string `json:"username"`
	// Publish and Subscribe come back ABSOLUTE - the API writes the tenant
	// prefix itself (§10) - which is deliberately not what was sent.
	Publish   []string `json:"publish"`
	Subscribe []string `json:"subscribe"`
	Status    string   `json:"status"`
	// Password comes back on a create that minted or was given one, and never
	// on a read. The API holds only a hash, so this state is the only copy.
	Password string `json:"password,omitempty"`
}

type PutBrokerAccountRequest struct {
	Name   string `json:"name"`
	Device string `json:"device,omitempty"`
	// Publish and Subscribe are RELATIVE to the tenant: "lightstand/+/scene",
	// never "eds/lightstand/+/scene". The API writes the prefix.
	Publish   []string `json:"publish,omitempty"`
	Subscribe []string `json:"subscribe,omitempty"`
	// Password is sent only to restore an account the tenant already holds, so
	// the broker is made to match devices already flashed (ADR-0012 §5).
	Password string `json:"password,omitempty"`
}

// PutBrokerAccount issues or re-applies an account.
//
// On a 502 the API has written its own record but the broker has not been
// reached, and the answer carries the account INCLUDING its password. That is
// returned alongside the error rather than discarded: the API cannot reproduce
// the plaintext later, so losing it here would leave an account that nothing
// can authenticate as and that only a delete could clear.
func (c *Client) PutBrokerAccount(ctx context.Context, tenant string, req PutBrokerAccountRequest) (BrokerAccount, error) {
	var out BrokerAccount
	err := c.do(ctx, http.MethodPost, "/v1/tenants/"+tenant+"/broker-accounts", req, &out)
	if err != nil {
		if partial, ok := PartialBrokerAccount(err); ok {
			return partial, err
		}
		return BrokerAccount{}, err
	}
	return out, nil
}

// PartialBrokerAccount digs a partially applied account out of a failed
// create. The API sends {"error": ..., "broker_account": {...}} when its own
// record was written and a later step was not.
func PartialBrokerAccount(err error) (BrokerAccount, bool) {
	var ae *APIError
	if !errors.As(err, &ae) {
		return BrokerAccount{}, false
	}
	var body struct {
		Account BrokerAccount `json:"broker_account"`
	}
	if json.Unmarshal(ae.Body, &body) != nil || body.Account.Name == "" {
		return BrokerAccount{}, false
	}
	return body.Account, true
}

func (c *Client) GetBrokerAccount(ctx context.Context, tenant, name string) (BrokerAccount, error) {
	var out BrokerAccount
	err := c.do(ctx, http.MethodGet, "/v1/tenants/"+tenant+"/broker-accounts/"+name, nil, &out)
	return out, err
}

func (c *Client) DeleteBrokerAccount(ctx context.Context, tenant, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tenants/"+tenant+"/broker-accounts/"+name, nil, nil)
}

// Record is a name the tenant publishes beside its workloads'.
type Record struct {
	Name    string `json:"name"`
	FQDN    string `json:"fqdn"`
	Address string `json:"address"`
}

func (c *Client) PutRecord(ctx context.Context, tenant, name, address string) (Record, error) {
	var out Record
	err := c.do(ctx, http.MethodPut, "/v1/tenants/"+tenant+"/records/"+name,
		map[string]string{"address": address}, &out)
	return out, err
}

func (c *Client) GetRecord(ctx context.Context, tenant, name string) (Record, error) {
	var out struct {
		Records []Record `json:"records"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/tenants/"+tenant+"/records", nil, &out); err != nil {
		return Record{}, err
	}
	for _, r := range out.Records {
		if r.Name == name {
			return r, nil
		}
	}
	return Record{}, ErrNotFound
}

func (c *Client) DeleteRecord(ctx context.Context, tenant, name string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tenants/"+tenant+"/records/"+name, nil, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.bearer())
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 300:
		return &APIError{Method: method, Path: path, Status: resp.StatusCode, Body: raw}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%s %s: decoding response: %w", method, path, err)
		}
	}
	return nil
}

// APIError is a non-2xx answer, keeping the raw body.
//
// The body is kept because some failures carry a partially applied object the
// caller must not lose. A broker account's password is the case that forced
// this: the API stores only a bcrypt hash, so the plaintext in a 502 body
// exists nowhere else, and discarding it would leave an account nothing can
// ever authenticate as (ADR-0012 §4).
type APIError struct {
	Method, Path string
	Status       int
	Body         []byte
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: %s", e.Method, e.Path, apiError(e.Status, e.Body))
}

// apiError uses the API's own message when it sent one. The API never puts a
// secret or a backend address in an error body.
func apiError(status int, raw []byte) string {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err == nil && body.Error != "" {
		return fmt.Sprintf("%d %s", status, body.Error)
	}
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	return fmt.Sprintf("%d %s", status, msg)
}
