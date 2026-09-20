package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

func list(t *testing.T, vals ...string) types.List {
	t.Helper()
	l, diags := types.ListValueFrom(context.Background(), types.StringType, vals)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return l
}

func strsOf(t *testing.T, l types.List) []string {
	t.Helper()
	out, diags := stringsFrom(context.Background(), l)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return out
}

// Config writes patterns relative to the tenant and the API answers absolute.
// Writing the answer straight back would make every apply report an
// inconsistent result, so the prefix is stripped back off.
func TestTheAnswersPrefixIsStrippedBackOff(t *testing.T) {
	prior := iotBrokerAccountModel{
		Tenant:  types.StringValue("eds"),
		Publish: list(t, "lightstand/+/scene"),
	}
	got, diags := brokerAccountFrom(context.Background(), client.BrokerAccount{
		Tenant: "eds", Name: "lightd", Username: "eds-lightd", Status: "ready",
		Publish: []string{"eds/lightstand/+/scene"},
	}, prior)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if p := strsOf(t, got.Publish); len(p) != 1 || p[0] != "lightstand/+/scene" {
		t.Errorf("publish = %v; state must match what the configuration wrote", p)
	}
	// And the absolute form is still visible, because what the broker enforces
	// is worth being able to read.
	if g := strsOf(t, got.GrantedPublish); len(g) != 1 || g[0] != "eds/lightstand/+/scene" {
		t.Errorf("granted_publish = %v", g)
	}
}

// A grant changed on the broker must show up as a diff against the
// configuration. Recording only the absolute form would not do that: the
// configured attribute would still match and no apply would follow.
func TestAGrantThatDriftedShowsAgainstTheConfiguration(t *testing.T) {
	prior := iotBrokerAccountModel{
		Tenant:  types.StringValue("eds"),
		Publish: list(t, "lightstand/+/scene"),
	}
	got, diags := brokerAccountFrom(context.Background(), client.BrokerAccount{
		Tenant: "eds", Name: "lightd", Status: "ready",
		Publish: []string{"eds/somethingelse/#"},
	}, prior)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if p := strsOf(t, got.Publish); len(p) != 1 || p[0] != "somethingelse/#" {
		t.Fatalf("publish = %v", p)
	}
	if got.Publish.Equal(prior.Publish) {
		t.Error("the drift did not reach the configured attribute, so no apply would correct it")
	}
}

// "#" is the tenant's whole tree and round trips: eds/# strips back to #.
func TestTheWholeTreeRoundTrips(t *testing.T) {
	got, _ := brokerAccountFrom(context.Background(), client.BrokerAccount{
		Tenant: "eds", Name: "all", Status: "ready", Subscribe: []string{"eds/#"},
	}, iotBrokerAccountModel{Tenant: types.StringValue("eds"), Subscribe: list(t, "#")})
	if s := strsOf(t, got.Subscribe); len(s) != 1 || s[0] != "#" {
		t.Errorf("subscribe = %v", s)
	}
}

// An account that declares no subscribe patterns must not gain the attribute.
// A null config and an empty [] in state are different values, and the
// difference is an apply that never settles.
func TestAnOmittedDirectionStaysOmitted(t *testing.T) {
	got, _ := brokerAccountFrom(context.Background(), client.BrokerAccount{
		Tenant: "eds", Name: "sensor", Status: "ready", Publish: []string{"eds/t"},
	}, iotBrokerAccountModel{
		Tenant:    types.StringValue("eds"),
		Publish:   list(t, "t"),
		Subscribe: types.ListNull(types.StringType),
	})
	if !got.Subscribe.IsNull() {
		t.Errorf("subscribe = %v, want null", got.Subscribe)
	}
	// But granted_subscribe is computed, so it is [] rather than null.
	if got.GrantedSubscribe.IsNull() {
		t.Error("granted_subscribe should be an empty list, not null")
	}
}

// A read does not return the password. Folding that answer into the model must
// keep the one already in state: the API holds only a hash, so this state is
// the only copy there is.
func TestReadKeepsThePasswordAlreadyInState(t *testing.T) {
	prior := iotBrokerAccountModel{
		Tenant:   types.StringValue("eds"),
		Password: types.StringValue("whatTheClientsHold"),
		Publish:  list(t, "t"),
	}
	got, _ := brokerAccountFrom(context.Background(), client.BrokerAccount{
		Tenant: "eds", Name: "lightd", Username: "eds-lightd", Status: "ready",
		Publish: []string{"eds/t"},
	}, prior)
	if got.Password.ValueString() != "whatTheClientsHold" {
		t.Fatalf("password = %q; a read must not blank it", got.Password.ValueString())
	}
	if !got.Present.ValueBool() {
		t.Error("an account that was read should be present")
	}
}

// A create does return it, and that one wins.
func TestCreateTakesTheIssuedPassword(t *testing.T) {
	got, _ := brokerAccountFrom(context.Background(), client.BrokerAccount{
		Tenant: "eds", Name: "lightd", Status: "ready", Password: "freshlyIssued",
		Publish: []string{"eds/t"},
	}, iotBrokerAccountModel{Tenant: types.StringValue("eds"), Password: types.StringNull(), Publish: list(t, "t")})
	if got.Password.ValueString() != "freshlyIssued" {
		t.Fatalf("password = %q", got.Password.ValueString())
	}
}

// An unknown password must never reach state - the framework refuses to store
// one - so a create whose answer carried none settles to null rather than
// leaving the plan's unknown in place.
func TestAnUnknownPasswordDoesNotReachState(t *testing.T) {
	got, _ := brokerAccountFrom(context.Background(), client.BrokerAccount{
		Tenant: "eds", Name: "lightd", Status: "ready", Publish: []string{"eds/t"},
	}, iotBrokerAccountModel{Tenant: types.StringValue("eds"), Password: types.StringUnknown(), Publish: list(t, "t")})
	if got.Password.IsUnknown() {
		t.Fatal("an unknown password reached state")
	}
	if !got.Password.IsNull() {
		t.Errorf("password = %v", got.Password)
	}
}

// The 502 case. The API wrote its own record and the writer did not land, and
// the answer carries the password - which exists nowhere else, because the API
// keeps only a hash. Losing it leaves an account nothing can authenticate as.
func TestAFailedWriteKeepsThePasswordItReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "broker: the writer did not answer",
			"broker_account": map[string]any{
				"tenant": "eds", "name": "lightd", "username": "eds-lightd",
				"publish": []string{"eds/t"}, "subscribe": []string{},
				"status": "provisioning", "password": "theOnlyCopy",
			},
		})
	}))
	defer srv.Close()

	c, err := client.New(client.Config{Endpoint: srv.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.PutBrokerAccount(context.Background(), "eds",
		client.PutBrokerAccountRequest{Name: "lightd", Publish: []string{"t"}})
	if err == nil {
		t.Fatal("a 502 was reported as success")
	}
	if a.Password != "theOnlyCopy" {
		t.Fatalf("password = %q; the only copy was discarded", a.Password)
	}
	// And the status says it is not on the broker, which is what makes
	// ModifyPlan retry rather than leave it looking settled.
	if a.Status != "provisioning" {
		t.Errorf("status = %q", a.Status)
	}
	// The error still carries the API's own message.
	if err.Error() == "" {
		t.Error("no message")
	}
}

// An error with no account in it is just an error, and must not be mistaken
// for a partial create.
func TestAPlainErrorIsNotAPartialCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid request: an account must have at least one publish or subscribe pattern"}`))
	}))
	defer srv.Close()

	c, err := client.New(client.Config{Endpoint: srv.URL, Token: "token"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.PutBrokerAccount(context.Background(), "eds", client.PutBrokerAccountRequest{Name: "lightd"})
	if err == nil {
		t.Fatal("a 400 was reported as success")
	}
	if a.Name != "" {
		t.Errorf("a 400 produced an account: %+v", a)
	}
	if _, ok := client.PartialBrokerAccount(err); ok {
		t.Error("a plain error was read as a partial create")
	}
}
