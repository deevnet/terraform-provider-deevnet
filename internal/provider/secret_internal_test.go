package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// fakeSecrets is the API's secrets routes, held in memory. It answers the way
// the contract says: a value goes in and never comes out, and every reader
// write issues a new secret_id.
type fakeSecrets struct {
	mu      sync.Mutex
	secrets map[string]fakeSecret
	readers map[string]map[string]any
	puts    []fakeSecret
	issued  int
}

type fakeSecret struct {
	Path    string
	Data    map[string]string
	Version int64
}

func newFakeSecrets(t *testing.T) (*fakeSecrets, *httptest.Server) {
	f := &fakeSecrets{secrets: map[string]fakeSecret{}, readers: map[string]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeSecrets) serve(w http.ResponseWriter, r *http.Request) {
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
	const secrets, readers = "/v1/tenants/eds/secrets/", "/v1/tenants/eds/secret-readers/"
	switch p := r.URL.Path; {
	case strings.HasPrefix(p, secrets):
		key := strings.TrimPrefix(p, secrets)
		switch r.Method {
		case http.MethodPut:
			var in struct {
				Data    map[string]string `json:"data"`
				Version int64             `json:"version"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			s := fakeSecret{Path: key, Data: in.Data, Version: in.Version}
			f.secrets[key] = s
			f.puts = append(f.puts, s)
			answer(http.StatusOK, map[string]any{"path": key, "version": in.Version, "present": true})
		case http.MethodGet:
			s, ok := f.secrets[key]
			if !ok {
				answer(http.StatusNotFound, map[string]string{"error": "no such secret"})
				return
			}
			answer(http.StatusOK, map[string]any{"path": key, "version": s.Version, "present": true})
		case http.MethodDelete:
			delete(f.secrets, key)
			answer(http.StatusNoContent, nil)
		}
	case strings.HasPrefix(p, readers):
		name := strings.TrimPrefix(p, readers)
		switch r.Method {
		case http.MethodPut:
			var in struct {
				Workload string   `json:"workload"`
				Paths    []string `json:"paths"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			f.issued++
			rd := map[string]any{
				"name": name, "workload": in.Workload, "paths": in.Paths,
				"role_id": "role-" + name, "namespace": "tenants/eds", "addr": "https://bao.example:8200",
			}
			f.readers[name] = rd
			out := map[string]any{"secret_id": fmt.Sprintf("secret-%d", f.issued)}
			for k, v := range rd {
				out[k] = v
			}
			answer(http.StatusOK, out)
		case http.MethodGet:
			rd, ok := f.readers[name]
			if !ok {
				answer(http.StatusNotFound, map[string]string{"error": "no such reader"})
				return
			}
			answer(http.StatusOK, rd)
		case http.MethodDelete:
			delete(f.readers, name)
			answer(http.StatusNoContent, nil)
		}
	default:
		answer(http.StatusNotFound, map[string]string{"error": "no route"})
	}
}

func (f *fakeSecrets) putCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.puts)
}

// needTerraform skips the tests that drive Terraform when there is none:
// they run the real plan and apply, which is the only way to see what a
// write-only argument does.
func needTerraform(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC_TERRAFORM_PATH") != "" {
		return
	}
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("no terraform on PATH")
	}
}

func unitFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"deevnet": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func secretConfig(endpoint, value string, version int) string {
	return fmt.Sprintf(`
provider "deevnet" {
  endpoint = %q
  token    = "token"
}

resource "deevnet_secret" "weather" {
  tenant          = "eds"
  path            = "weather/api"
  data_wo         = sensitive({ api_key = %q })
  data_wo_version = %d
}
`, endpoint, value, version)
}

// The value reaches the API, never state, and a new value reaches it only
// with a new version. A secret OpenBao lost, or holds at another version, is
// written again by the next apply.
func TestSecretIsWriteOnlyAndResupplied(t *testing.T) {
	needTerraform(t)
	f, srv := newFakeSecrets(t)
	const addr = "deevnet_secret.weather"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: unitFactories(),
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		Steps: []resource.TestStep{
			// Checked at plan, before anything is written.
			{
				Config:      strings.Replace(secretConfig(srv.URL, "x", 1), `"weather/api"`, `"Weather/API"`, 1),
				ExpectError: regexp.MustCompile(`Invalid secret path`),
				PlanOnly:    true,
			},
			{
				Config: secretConfig(srv.URL, "first-key", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "id", "weather/api"),
					resource.TestCheckResourceAttr(addr, "data_wo_version", "1"),
					resource.TestCheckNoResourceAttr(addr, "data_wo"),
					resource.TestCheckNoResourceAttr(addr, "data_wo.%"),
					noValueInState("first-key"),
					func(*terraform.State) error {
						if n := f.putCount(); n != 1 {
							return fmt.Errorf("%d writes, want 1", n)
						}
						if got := f.puts[0].Data["api_key"]; got != "first-key" {
							return fmt.Errorf("the API got %q", got)
						}
						return nil
					},
				),
			},
			// A new value alone is invisible to Terraform: nothing to do.
			{
				Config: secretConfig(srv.URL, "second-key", 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: func(*terraform.State) error {
					if n := f.putCount(); n != 1 {
						return fmt.Errorf("%d writes, want 1: a value change without a version was written", n)
					}
					return nil
				},
			},
			// A new version writes the value from the configuration.
			{
				Config: secretConfig(srv.URL, "second-key", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "data_wo_version", "2"),
					noValueInState("second-key"),
					func(*terraform.State) error {
						if n := f.putCount(); n != 2 {
							return fmt.Errorf("%d writes, want 2", n)
						}
						if last := f.puts[1]; last.Version != 2 || last.Data["api_key"] != "second-key" {
							return fmt.Errorf("the API got %+v", last)
						}
						return nil
					},
				),
			},
			// OpenBao rebuilt: the secret is gone, and the next apply
			// creates it again.
			{
				PreConfig: func() {
					f.mu.Lock()
					delete(f.secrets, "weather/api")
					f.mu.Unlock()
				},
				Config: secretConfig(srv.URL, "second-key", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate)},
				},
				Check: func(*terraform.State) error {
					if n := f.putCount(); n != 3 {
						return fmt.Errorf("%d writes, want 3: a lost secret was not written again", n)
					}
					return nil
				},
			},
			// OpenBao holds another version: a diff, and the apply writes
			// the configured one.
			{
				PreConfig: func() {
					f.mu.Lock()
					s := f.secrets["weather/api"]
					s.Version = 7
					f.secrets["weather/api"] = s
					f.mu.Unlock()
				},
				Config: secretConfig(srv.URL, "second-key", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: func(*terraform.State) error {
					if n := f.putCount(); n != 4 || f.secrets["weather/api"].Version != 2 {
						return fmt.Errorf("%d writes, version %d", n, f.secrets["weather/api"].Version)
					}
					return nil
				},
			},
		},
	})
}

func noValueInState(value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for name, rs := range s.RootModule().Resources {
			for k, v := range rs.Primary.Attributes {
				if strings.Contains(v, value) {
					return fmt.Errorf("%s.%s holds the secret's value", name, k)
				}
			}
		}
		return nil
	}
}

func readerConfig(endpoint string, paths ...string) string {
	q := make([]string, len(paths))
	for i, p := range paths {
		q[i] = fmt.Sprintf("%q", p)
	}
	return fmt.Sprintf(`
provider "deevnet" {
  endpoint = %q
  token    = "token"
}

resource "deevnet_secret_reader" "lightd" {
  tenant   = "eds"
  name     = "lightd"
  workload = "lightd"
  paths    = [%s]
}
`, endpoint, strings.Join(q, ", "))
}

// Every write of a reader issues a new secret_id, and a reader OpenBao lost is
// issued again.
func TestSecretReaderReissuesOnChange(t *testing.T) {
	needTerraform(t)
	f, srv := newFakeSecrets(t)
	const addr = "deevnet_secret_reader.lightd"
	differs := statecheck.CompareValue(compare.ValuesDiffer())

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: unitFactories(),
		Steps: []resource.TestStep{
			// Checked at plan, before anything is written.
			{
				Config:      strings.Replace(readerConfig(srv.URL, "weather/api"), `name     = "lightd"`, `name     = "Light_D"`, 1),
				ExpectError: regexp.MustCompile(`Invalid reader name`),
				PlanOnly:    true,
			},
			{
				Config: readerConfig(srv.URL, "weather/api"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "secret_id", "secret-1"),
					resource.TestCheckResourceAttr(addr, "role_id", "role-lightd"),
					resource.TestCheckResourceAttr(addr, "namespace", "tenants/eds"),
					resource.TestCheckResourceAttr(addr, "addr", "https://bao.example:8200"),
				),
				ConfigStateChecks: []statecheck.StateCheck{differs.AddStateValue(addr, tfjsonpath.New("secret_id"))},
			},
			// A refresh keeps the secret_id a read does not return.
			{
				Config: readerConfig(srv.URL, "weather/api"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.TestCheckResourceAttr(addr, "secret_id", "secret-1"),
			},
			{
				Config: readerConfig(srv.URL, "weather/api", "mail/smtp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(addr, tfjsonpath.New("secret_id")),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "secret_id", "secret-2"),
					resource.TestCheckResourceAttr(addr, "paths.#", "2"),
				),
				ConfigStateChecks: []statecheck.StateCheck{differs.AddStateValue(addr, tfjsonpath.New("secret_id"))},
			},
			{
				PreConfig: func() {
					f.mu.Lock()
					delete(f.readers, "lightd")
					f.mu.Unlock()
				},
				Config: readerConfig(srv.URL, "weather/api", "mail/smtp"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate)},
				},
				Check: resource.TestCheckResourceAttr(addr, "secret_id", "secret-3"),
			},
		},
	})
}

func TestValidSecretPath(t *testing.T) {
	for _, ok := range []string{"a", "weather/api", "a/b/c/d", "x_y-z/" + strings.Repeat("a", 64)} {
		if err := validSecretPath(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "a/b/c/d/e", "A", "a//b", "/a", "a/", "a.b", strings.Repeat("a", 65),
		strings.Repeat("a", 64) + "/" + strings.Repeat("b", 64)} {
		if validSecretPath(bad) == nil {
			t.Errorf("%q passed", bad)
		}
	}
}
