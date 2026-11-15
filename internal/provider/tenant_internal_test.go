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
	t.Log.MetricsEndpoint = "https://metrics.example"
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
	if m.MetricsEndpoint.ValueString() != "https://metrics.example" {
		t.Errorf("metrics endpoint = %q", m.MetricsEndpoint.ValueString())
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

// A tenant created before the site had a log store or dashboards holds no
// credentials for them, and a read never brings them: ModifyPlan has to plan
// the restore that does. One that has them, or a site with neither, plans
// nothing.
func TestLacksIssuedFindsCredentialsStateNeverReceived(t *testing.T) {
	old := tenantFrom(tenantWithLog("", ""), tenantModel{})
	old.DashboardURL = types.StringValue("https://obs.example:3000")
	if log, dash, _ := lacksIssued(old); !log || !dash {
		t.Errorf("tenant older than both: log %v, dashboard %v; want both", log, dash)
	}

	current := tenantFrom(tenantWithLog("i", "r"), tenantModel{})
	current.DashboardURL = types.StringValue("https://obs.example:3000")
	current.DashboardPassword = types.StringValue("pw")
	if log, dash, _ := lacksIssued(current); log || dash {
		t.Errorf("tenant holding both: log %v, dashboard %v; want neither", log, dash)
	}

	bare := tenantFrom(client.Tenant{Name: "eds", Index: 2}, tenantModel{})
	if log, dash, notify := lacksIssued(bare); log || dash || notify {
		t.Errorf("site with none: log %v, dashboard %v, notify %v; want none", log, dash, notify)
	}
}

// A tenant created before the site had alerting has no ntfy password; a read
// brings the endpoint and never the password, so ModifyPlan plans the restore
// that hands it over. A plain read afterwards keeps it.
func TestTheNtfyLoginIsPlannedForAndKept(t *testing.T) {
	var fresh client.Tenant
	fresh.Name, fresh.Index = "eds", 2
	fresh.Notify.Endpoint, fresh.Notify.Username, fresh.Notify.TopicPrefix = "https://ntfy.example", "eds", "eds-"
	old := tenantFrom(fresh, tenantModel{})
	if _, _, notify := lacksIssued(old); !notify {
		t.Error("a tenant with the endpoint and no password is not planned for")
	}
	fresh.Notify.Password = "ntfy-pw"
	m := tenantFrom(fresh, old)
	if _, _, notify := lacksIssued(m); notify {
		t.Error("a tenant holding the password is planned for again")
	}
	fresh.Notify.Password = ""
	if m = tenantFrom(fresh, m); m.NotifyPassword.ValueString() != "ntfy-pw" {
		t.Errorf("a plain read lost the ntfy password: %v", m.NotifyPassword)
	}
	if m.NotifyTopicPrefix.ValueString() != "eds-" || m.NotifyUsername.ValueString() != "eds" {
		t.Errorf("model = %+v", m)
	}
}
