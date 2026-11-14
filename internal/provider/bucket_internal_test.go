package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// fakeBuckets is the API's bucket routes for tenant "eds", held in memory, as
// the CHG-0069 contract has them: a key's secret is in the create's answer and
// nowhere else. Every value in it is a placeholder.
type fakeBuckets struct {
	mu      sync.Mutex
	buckets map[string]map[string]any
	keys    map[string]map[string]client.BucketKey
	issued  int
	updates []client.BucketSettings
	creates []client.CreateBucketRequest
	deletes []string // the request URI of every bucket delete
}

func newFakeBuckets(t *testing.T) (*fakeBuckets, *httptest.Server) {
	f := &fakeBuckets{buckets: map[string]map[string]any{}, keys: map[string]map[string]client.BucketKey{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeBuckets) newKey(name, access string) client.BucketKey {
	f.issued++
	return client.BucketKey{
		Name: name, Access: access,
		AccessKey: fmt.Sprintf("FAKEACCESSKEY%07d", f.issued),
		SecretKey: fmt.Sprintf("fake-secret-%d", f.issued),
	}
}

func (f *fakeBuckets) view(name string, s client.BucketSettings) map[string]any {
	return map[string]any{
		"name": name, "bucket": "eds-" + name, "endpoint": "https://s3.example.test",
		"quota_gb": s.QuotaGB, "versioning": s.Versioning, "public_read": s.PublicRead,
		"lifecycle": map[string]any{
			"expire_days": s.Lifecycle.ExpireDays, "noncurrent_expire_days": s.Lifecycle.NoncurrentExpireDays,
		},
		"status": "ready",
	}
}

func (f *fakeBuckets) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	answer := func(status int, body any) {
		w.WriteHeader(status)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}
	if r.Header.Get("Authorization") != "Bearer token" {
		answer(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	const base = "/v1/tenants/eds/buckets"
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, base), "/")
	if !strings.HasPrefix(r.URL.Path, base) {
		answer(http.StatusNotFound, map[string]string{"error": "no route"})
		return
	}
	parts := strings.Split(rest, "/")
	switch {
	case rest == "" && r.Method == http.MethodPost:
		var in client.CreateBucketRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		if _, ok := f.buckets[in.Name]; ok {
			answer(http.StatusConflict, map[string]string{"error": "bucket exists"})
			return
		}
		f.creates = append(f.creates, in)
		f.buckets[in.Name] = f.view(in.Name, in.BucketSettings)
		k := f.newKey("default", "write")
		f.keys[in.Name] = map[string]client.BucketKey{"default": k}
		out := f.view(in.Name, in.BucketSettings)
		out["key"] = k
		answer(http.StatusCreated, out)
	case len(parts) == 1:
		b, ok := f.buckets[parts[0]]
		if !ok {
			answer(http.StatusNotFound, map[string]string{"error": "no such bucket"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			answer(http.StatusOK, b)
		case http.MethodPut:
			var in client.BucketSettings
			_ = json.NewDecoder(r.Body).Decode(&in)
			f.updates = append(f.updates, in)
			f.buckets[parts[0]] = f.view(parts[0], in)
			answer(http.StatusOK, f.buckets[parts[0]])
		case http.MethodDelete:
			f.deletes = append(f.deletes, r.URL.RequestURI())
			delete(f.buckets, parts[0])
			delete(f.keys, parts[0])
			answer(http.StatusNoContent, nil)
		}
	case len(parts) == 2 && parts[1] == "keys" && r.Method == http.MethodPost:
		keys, ok := f.keys[parts[0]]
		if !ok {
			answer(http.StatusNotFound, map[string]string{"error": "no such bucket"})
			return
		}
		var in client.CreateBucketKeyRequest
		_ = json.NewDecoder(r.Body).Decode(&in)
		k := f.newKey(in.Name, in.Access)
		keys[in.Name] = k
		answer(http.StatusCreated, k)
	case len(parts) == 3 && parts[1] == "keys":
		k, ok := f.keys[parts[0]][parts[2]]
		if !ok {
			answer(http.StatusNotFound, map[string]string{"error": "no such key"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			k.SecretKey = "" // never on a read
			answer(http.StatusOK, k)
		case http.MethodDelete:
			delete(f.keys[parts[0]], parts[2])
			answer(http.StatusNoContent, nil)
		}
	default:
		answer(http.StatusNotFound, map[string]string{"error": "no route"})
	}
}

func bucketConfig(endpoint, bucketBody string) string {
	return fmt.Sprintf(`
provider "deevnet" {
  endpoint = %q
  token    = "token"
}

resource "deevnet_bucket" "media" {
  tenant = "eds"
  name   = "media"
%s
}

resource "deevnet_bucket_key" "reader" {
  tenant = "eds"
  bucket = deevnet_bucket.media.name
  name   = "reader"
  access = "read"
}
`, endpoint, bucketBody)
}

// The secrets come from the create and survive every read; a quota or
// lifecycle change is an update in place; a lost bucket or key is replaced;
// force_destroy reaches the delete.
func TestBucketAndKeyLifecycle(t *testing.T) {
	needTerraform(t)
	f, srv := newFakeBuckets(t)
	const b, k = "deevnet_bucket.media", "deevnet_bucket_key.reader"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: unitFactories(),
		CheckDestroy: func(*terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			last := f.deletes[len(f.deletes)-1]
			if last != "/v1/tenants/eds/buckets/media?force=true" {
				return fmt.Errorf("the destroy sent %q; force_destroy did not reach it", last)
			}
			return nil
		},
		Steps: []resource.TestStep{
			// Checked at plan, before anything is created.
			{
				Config:      strings.Replace(bucketConfig(srv.URL, "  quota_gb = 5"), `name   = "media"`, `name   = "Media_1"`, 1),
				ExpectError: regexp.MustCompile(`Invalid bucket name`),
				PlanOnly:    true,
			},
			{
				Config:      bucketConfig(srv.URL, "  quota_gb = 0"),
				ExpectError: regexp.MustCompile(`Invalid quota`),
				PlanOnly:    true,
			},
			{
				Config:      bucketConfig(srv.URL, "  quota_gb = 5\n  object_lifecycle {\n    expire_days = 0\n  }"),
				ExpectError: regexp.MustCompile(`Invalid expiry`),
				PlanOnly:    true,
			},
			{
				Config:      strings.Replace(bucketConfig(srv.URL, "  quota_gb = 5"), `access = "read"`, `access = "admin"`, 1),
				ExpectError: regexp.MustCompile(`Invalid access`),
				PlanOnly:    true,
			},
			{
				Config: bucketConfig(srv.URL, "  quota_gb = 5"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(b, "bucket", "eds-media"),
					resource.TestCheckResourceAttr(b, "endpoint", "https://s3.example.test"),
					resource.TestCheckResourceAttr(b, "status", "ready"),
					resource.TestCheckResourceAttr(b, "versioning", "false"),
					resource.TestCheckResourceAttr(b, "public_read", "false"),
					resource.TestCheckResourceAttr(b, "force_destroy", "false"),
					resource.TestCheckNoResourceAttr(b, "object_lifecycle.expire_days"),
					resource.TestCheckResourceAttr(b, "access_key", "FAKEACCESSKEY0000001"),
					resource.TestCheckResourceAttr(b, "secret_key", "fake-secret-1"),
					resource.TestCheckResourceAttr(k, "access", "read"),
					resource.TestCheckResourceAttr(k, "access_key", "FAKEACCESSKEY0000002"),
					resource.TestCheckResourceAttr(k, "secret_key", "fake-secret-2"),
					func(*terraform.State) error {
						if c := f.creates[0]; c.Name != "media" || c.QuotaGB != 5 || c.Lifecycle.ExpireDays != 0 {
							return fmt.Errorf("the API got %+v", c)
						}
						return nil
					},
				),
			},
			// A refresh reads no secret, and keeps both.
			{
				Config: bucketConfig(srv.URL, "  quota_gb = 5"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(b, "secret_key", "fake-secret-1"),
					resource.TestCheckResourceAttr(k, "secret_key", "fake-secret-2"),
				),
			},
			// Quota, versioning, lifecycle and public_read change in place.
			{
				Config: bucketConfig(srv.URL, "  quota_gb = 10\n  versioning = true\n  public_read = true\n  object_lifecycle {\n    noncurrent_expire_days = 30\n  }"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(b, plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction(k, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(b, "quota_gb", "10"),
					resource.TestCheckResourceAttr(b, "versioning", "true"),
					resource.TestCheckResourceAttr(b, "public_read", "true"),
					resource.TestCheckResourceAttr(b, "object_lifecycle.noncurrent_expire_days", "30"),
					resource.TestCheckNoResourceAttr(b, "object_lifecycle.expire_days"),
					resource.TestCheckResourceAttr(b, "secret_key", "fake-secret-1"),
					func(*terraform.State) error {
						u := f.updates[len(f.updates)-1]
						if u.QuotaGB != 10 || !u.Versioning || !u.PublicRead || u.Lifecycle.NoncurrentExpireDays != 30 {
							return fmt.Errorf("the API got %+v", u)
						}
						return nil
					},
				),
			},
			// The API lost the key: it is replaced, with a new secret.
			{
				PreConfig: func() {
					f.mu.Lock()
					delete(f.keys["media"], "reader")
					f.mu.Unlock()
				},
				Config: bucketConfig(srv.URL, "  quota_gb = 10\n  versioning = true\n  public_read = true\n  object_lifecycle {\n    noncurrent_expire_days = 30\n  }"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(k, plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction(b, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.TestCheckResourceAttr(k, "secret_key", "fake-secret-3"),
			},
			// The API lost the bucket: it is replaced, and so is its key.
			{
				PreConfig: func() {
					f.mu.Lock()
					delete(f.buckets, "media")
					delete(f.keys, "media")
					f.mu.Unlock()
				},
				Config: bucketConfig(srv.URL, "  quota_gb = 10\n  versioning = true\n  public_read = true\n  object_lifecycle {\n    noncurrent_expire_days = 30\n  }"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(b, plancheck.ResourceActionReplace),
						plancheck.ExpectResourceAction(k, plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(b, "present", "true"),
					resource.TestCheckResourceAttr(b, "secret_key", "fake-secret-4"),
					resource.TestCheckResourceAttr(k, "secret_key", "fake-secret-5"),
				),
			},
			// force_destroy is the provider's own: an update that sends the
			// same settings, and the destroy (CheckDestroy) sends force=true.
			{
				Config: bucketConfig(srv.URL, "  quota_gb = 10\n  versioning = true\n  public_read = true\n  force_destroy = true\n  object_lifecycle {\n    noncurrent_expire_days = 30\n  }"),
				Check: func(*terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					for _, d := range f.deletes {
						if strings.Contains(d, "force") {
							return fmt.Errorf("a delete before the destroy sent force: %q", d)
						}
					}
					return nil
				},
			},
		},
	})
}

func TestValidBucketName(t *testing.T) {
	for _, ok := range []string{"abc", "media", "a-1", "a" + strings.Repeat("b", 30) + "c"} {
		if err := validBucketName("eds", ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "ab", "1ab", "ab-", "Abc", "a_b", "a.b", "a" + strings.Repeat("b", 31) + "c"} {
		if validBucketName("eds", bad) == nil {
			t.Errorf("%q passed", bad)
		}
	}
	// The store's name, <tenant>-<name>, is at most 63 characters.
	if validBucketName(strings.Repeat("t", 31), strings.Repeat("n", 32)) == nil {
		t.Error("a 64-character store name passed")
	}
	if err := validBucketName(strings.Repeat("t", 30), strings.Repeat("n", 32)); err != nil {
		t.Errorf("a 63-character store name: %v", err)
	}
}

func TestBucketKeyNamePattern(t *testing.T) {
	for _, ok := range []string{"a", "default", "reader-1", "a" + strings.Repeat("b", 19)} {
		if !bucketKeyName.MatchString(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "1a", "Reader", "a_b", "a" + strings.Repeat("b", 20)} {
		if bucketKeyName.MatchString(bad) {
			t.Errorf("%q passed", bad)
		}
	}
}

// A read carries no key, and folding it in keeps the secret in state.
func TestBucketReadKeepsTheSecret(t *testing.T) {
	prior := bucketModel{
		Tenant: types.StringValue("eds"), Name: types.StringValue("media"),
		AccessKey: types.StringValue("FAKEACCESSKEY0000001"), SecretKey: types.StringValue("fake-secret-1"),
		ForceDestroy:    types.BoolValue(true),
		ObjectLifecycle: types.ObjectNull(bucketLifecycleAttrTypes),
	}
	got, diags := bucketFrom(context.Background(), prior, client.Bucket{Name: "media", Bucket: "eds-media", QuotaGB: 5, Status: "ready"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if got.SecretKey.ValueString() != "fake-secret-1" || got.AccessKey.ValueString() != "FAKEACCESSKEY0000001" {
		t.Fatalf("a read changed the key: %v %v", got.AccessKey, got.SecretKey)
	}
	if !got.ForceDestroy.ValueBool() {
		t.Error("a read reset force_destroy, which only the provider holds")
	}
	if !got.ObjectLifecycle.IsNull() {
		t.Errorf("object_lifecycle = %v; an omitted block with no expiry must stay omitted", got.ObjectLifecycle)
	}
	// An expiry set in the store but not in the configuration is drift.
	got, _ = bucketFrom(context.Background(), prior, client.Bucket{Name: "media", Lifecycle: client.BucketLifecycle{ExpireDays: 7}})
	if got.ObjectLifecycle.IsNull() {
		t.Error("an expiry the configuration does not have was hidden")
	}
}

func TestBucketKeyReadKeepsTheSecret(t *testing.T) {
	got := bucketKeyFrom(bucketKeyModel{SecretKey: types.StringValue("fake-secret-2")},
		client.BucketKey{Name: "reader", Access: "read", AccessKey: "FAKEACCESSKEY0000002"})
	if got.SecretKey.ValueString() != "fake-secret-2" {
		t.Fatalf("secret_key = %v", got.SecretKey)
	}
}
