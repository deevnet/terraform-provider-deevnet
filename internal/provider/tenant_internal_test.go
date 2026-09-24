package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

func tenantWithLog(ingest, read string) client.Tenant {
	var t client.Tenant
	t.Name = "eds"
	t.Index = 2
	t.Status = "ready"
	t.Log.Endpoint = "https://obs.example:8427"
	t.Log.AccountID = 2
	t.Log.SelectHeader = "X-Deevnet-Partition"
	t.Log.IngestToken = ingest
	t.Log.ReadToken = read
	return t
}

// A create carries the tokens, and they land in state.
func TestTenantFromTakesTheLogTokensACreateReturns(t *testing.T) {
	m := tenantFrom(tenantWithLog("ingest-token", "read-token"), tenantModel{})
	if m.LogIngestToken.ValueString() != "ingest-token" || m.LogReadToken.ValueString() != "read-token" {
		t.Fatalf("tokens = %q and %q", m.LogIngestToken.ValueString(), m.LogReadToken.ValueString())
	}
	if m.LogAccountID.ValueInt64() != 2 || m.LogSelectHeader.ValueString() != "X-Deevnet-Partition" {
		t.Errorf("account id = %d, select header = %q", m.LogAccountID.ValueInt64(), m.LogSelectHeader.ValueString())
	}
}

// A read carries none, and state keeps the ones it has. Losing them here would
// leave the tenant's workloads configured with a credential nothing in state
// records.
func TestTenantFromKeepsTheTokensAReadDoesNotCarry(t *testing.T) {
	prior := tenantModel{
		LogIngestToken: types.StringValue("ingest-token"),
		LogReadToken:   types.StringValue("read-token"),
	}
	m := tenantFrom(tenantWithLog("", ""), prior)
	if m.LogIngestToken.ValueString() != "ingest-token" || m.LogReadToken.ValueString() != "read-token" {
		t.Fatalf("a read dropped the tokens: %q and %q",
			m.LogIngestToken.ValueString(), m.LogReadToken.ValueString())
	}
}

// A tenant that has never had them, at a site with no store, holds null rather
// than an empty string: nothing was issued.
func TestTenantFromLeavesTheTokensNullAtASiteWithNoStore(t *testing.T) {
	m := tenantFrom(client.Tenant{Name: "eds", Index: 2}, tenantModel{})
	if !m.LogIngestToken.IsNull() || !m.LogReadToken.IsNull() {
		t.Fatalf("tokens = %v and %v, want null", m.LogIngestToken, m.LogReadToken)
	}
	if m.LogEndpoint.ValueString() != "" {
		t.Errorf("endpoint = %q, want empty", m.LogEndpoint.ValueString())
	}
}

// The dashboard login arrives like the log tokens: the password on create and
// reconcile, kept from state on every other read, and the rest always.
func TestTenantFromTakesTheDashboardLoginAndKeepsItsPassword(t *testing.T) {
	fresh := tenantWithLog("i", "r")
	fresh.Dashboard.URL = "https://obs.example:3000"
	fresh.Dashboard.OrgID = 4
	fresh.Dashboard.Username = "eds"
	fresh.Dashboard.Password = "dash-pw"
	m := tenantFrom(fresh, tenantModel{})
	if m.DashboardPassword.ValueString() != "dash-pw" || m.DashboardOrgID.ValueInt64() != 4 ||
		m.DashboardUsername.ValueString() != "eds" || m.DashboardURL.ValueString() != "https://obs.example:3000" {
		t.Fatalf("dashboard = %v %v %v %v", m.DashboardURL, m.DashboardOrgID, m.DashboardUsername, m.DashboardPassword)
	}

	read := fresh
	read.Dashboard.Password = ""
	m = tenantFrom(read, m)
	if m.DashboardPassword.ValueString() != "dash-pw" {
		t.Fatalf("a plain read lost the password: %v", m.DashboardPassword)
	}
}
