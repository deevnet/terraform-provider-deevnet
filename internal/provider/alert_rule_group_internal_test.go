package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// fakeAlerts is the API's alert rule group routes, held in memory, answering
// as API 0.25.0 does: defaults filled in, a refused rule a 400.
type fakeAlerts struct {
	mu     sync.Mutex
	groups map[string]client.AlertRuleGroup
	puts   int
}

func newFakeAlerts(t *testing.T) (*fakeAlerts, *httptest.Server) {
	f := &fakeAlerts{groups: map[string]client.AlertRuleGroup{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeAlerts) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	answer := func(status int, body any) {
		w.WriteHeader(status)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}
	const prefix = "/v1/tenants/eds/alert-rule-groups/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		answer(http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	name := strings.TrimPrefix(r.URL.Path, prefix)
	switch r.Method {
	case http.MethodPut:
		var req client.PutAlertRuleGroupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			answer(http.StatusBadRequest, map[string]string{"error": "bad body"})
			return
		}
		for _, rule := range req.Rules {
			if _, ok := rule.Labels["tenant"]; ok {
				answer(http.StatusBadRequest, map[string]string{"error": `label "tenant" is set by the platform, not by a rule`})
				return
			}
			if strings.Contains(rule.Expr, "((") {
				answer(http.StatusBadRequest, map[string]string{"error": `the rules do not load: bad MetricsQL expr`})
				return
			}
		}
		if req.Interval == "" {
			req.Interval = "1m"
		}
		if req.Partition == "" {
			req.Partition = "workloads"
		}
		g := client.AlertRuleGroup{Tenant: "eds", Name: name, Type: req.Type, Interval: req.Interval,
			Topic: req.Topic, NtfyTopic: "eds-" + req.Topic, Partition: req.Partition, Rules: req.Rules}
		f.groups[name] = g
		f.puts++
		answer(http.StatusOK, g)
	case http.MethodGet:
		g, ok := f.groups[name]
		if !ok {
			answer(http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		answer(http.StatusOK, g)
	case http.MethodDelete:
		if _, ok := f.groups[name]; !ok {
			answer(http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		delete(f.groups, name)
		w.WriteHeader(http.StatusNoContent)
	}
}

func alertConfig(endpoint, rules string) string {
	return fmt.Sprintf(`
provider "deevnet" {
  endpoint = %q
  token    = "token"
}

resource "deevnet_alert_rule_group" "backend" {
  tenant = "eds"
  name   = "backend"
  type   = "metrics"
  topic  = "backend"
%s
}
`, endpoint, rules)
}

const plainRule = `
  rule {
    alert = "QueueBacklog"
    expr  = "backend_queue_depth > 100"
  }
`

const fullRule = `
  rule {
    alert       = "QueueBacklog"
    expr        = "backend_queue_depth > 200"
    for         = "5m"
    labels      = { severity = "page" }
    annotations = { summary = "queue is {{ $value }}" }
  }
`

func TestAlertRuleGroupAppliesWithoutAPerpetualDiff(t *testing.T) {
	needTerraform(t)
	f, srv := newFakeAlerts(t)
	const addr = "deevnet_alert_rule_group.backend"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: unitFactories(),
		Steps: []resource.TestStep{
			// A rule with no for, labels or annotations: the API omits them,
			// and the next plan must still be empty.
			{
				Config: alertConfig(srv.URL, plainRule),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "ntfy_topic", "eds-backend"),
					resource.TestCheckResourceAttr(addr, "interval", "1m"),
					resource.TestCheckResourceAttr(addr, "partition", "workloads"),
					resource.TestCheckResourceAttr(addr, "rule.#", "1"),
					resource.TestCheckNoResourceAttr(addr, "rule.0.for"),
				),
			},
			{
				Config: alertConfig(srv.URL, plainRule),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			// Changing a rule replaces the group in place, by name.
			{
				Config: alertConfig(srv.URL, fullRule),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "rule.0.for", "5m"),
					resource.TestCheckResourceAttr(addr, "rule.0.labels.severity", "page"),
					resource.TestCheckResourceAttr(addr, "rule.0.annotations.summary", "queue is {{ $value }}"),
				),
			},
			{
				Config: alertConfig(srv.URL, fullRule),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			// The API's refusals reach the apply with their reason.
			{
				Config:      alertConfig(srv.URL, strings.Replace(fullRule, `severity = "page"`, `tenant = "mabell"`, 1)),
				ExpectError: regexp.MustCompile(`400 label "tenant"`),
			},
			{
				Config:      alertConfig(srv.URL, strings.Replace(fullRule, `backend_queue_depth > 200`, `sum((x`, 1)),
				ExpectError: regexp.MustCompile(`400 the rules do not`),
			},
			// A site that lost the group gets it back on the next apply.
			{
				PreConfig: func() {
					f.mu.Lock()
					delete(f.groups, "backend")
					f.mu.Unlock()
				},
				Config: alertConfig(srv.URL, fullRule),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(addr, plancheck.ResourceActionCreate)},
				},
				Check: func(*terraform.State) error {
					f.mu.Lock()
					defer f.mu.Unlock()
					if _, ok := f.groups["backend"]; !ok {
						return fmt.Errorf("the group was not written again")
					}
					return nil
				},
			},
		},
	})
}
