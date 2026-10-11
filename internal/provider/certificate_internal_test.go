package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// A certificate is the one thing here that goes wrong by itself. A plan has to
// show it changing when nothing in the configuration did.
func TestAReissueIsDueWhenLostOrCloseToExpiry(t *testing.T) {
	renew := time.Date(2027, 8, 12, 12, 0, 0, 0, time.UTC)
	held := certificateModel{Present: types.BoolValue(true), RenewAfter: types.StringValue(renew.Format(time.RFC3339))}

	if reissueDue(held, renew.Add(-time.Hour)) {
		t.Error("due an hour before its renewal day")
	}
	if !reissueDue(held, renew) {
		t.Error("not due on its renewal day")
	}
	if !reissueDue(held, renew.Add(90*24*time.Hour)) {
		t.Error("not due after it has expired")
	}

	lost := held
	lost.Present = types.BoolValue(false)
	if !reissueDue(lost, renew.Add(-300*24*time.Hour)) {
		t.Error("a certificate the API no longer holds is not asked for again")
	}

	// A date that cannot be read is asked about, not trusted: asking is an
	// ensure, and costs one call.
	unreadable := held
	unreadable.RenewAfter = types.StringValue("next summer")
	if !reissueDue(unreadable, renew.Add(-300*24*time.Hour)) {
		t.Error("an unreadable renewal day was taken to mean never")
	}
	// Before the first apply there is nothing to renew.
	if reissueDue(certificateModel{Present: types.BoolValue(true), RenewAfter: types.StringNull()}, renew) {
		t.Error("due with no renewal day at all")
	}
}

// The configuration holds labels and the API answers with full names. Writing
// the answer over the arguments would make every apply report an
// inconsistent result.
func TestTheAnswerIsFoldedAndTheArgumentsKept(t *testing.T) {
	prior := certificateModel{
		Tenant: types.StringValue("eds"),
		Name:   types.StringValue("console"),
		CSR:    types.StringValue("-----BEGIN CERTIFICATE REQUEST-----\n..."),
		Names:  list(t, "www"),
	}
	got, diags := certificateFrom(context.Background(), prior, client.Certificate{
		Tenant: "eds", Name: "console", FQDN: "console.eds.mobile.deevnet.net",
		Names:       []string{"console.eds.mobile.deevnet.net", "www.eds.mobile.deevnet.net"},
		Certificate: "LEAF\n", CAChain: "ISSUING\nSITE\n",
		Serial: "0a:0b", KeyFingerprint: "abc",
		NotBefore: "2026-10-11T12:00:00Z", NotAfter: "2027-10-11T12:00:00Z", RenewAfter: "2027-08-12T12:00:00Z",
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !got.CSR.Equal(prior.CSR) || !got.Names.Equal(prior.Names) || !got.Name.Equal(prior.Name) {
		t.Errorf("the arguments were rewritten: names %v", strsOf(t, got.Names))
	}
	if names := strsOf(t, got.DNSNames); len(names) != 2 || names[1] != "www.eds.mobile.deevnet.net" {
		t.Errorf("dns_names = %v", names)
	}
	// What a server is configured with: the leaf, then the chain, and no root.
	if got.FullChain.ValueString() != "LEAF\nISSUING\nSITE\n" {
		t.Errorf("fullchain = %q", got.FullChain.ValueString())
	}
	if got.FQDN.ValueString() != "console.eds.mobile.deevnet.net" || !got.Present.ValueBool() ||
		got.RenewAfter.ValueString() != "2027-08-12T12:00:00Z" {
		t.Errorf("got %+v", got)
	}
}

// No names in the configuration is null, and stays null: an empty list would
// be a difference from what was written.
func TestNoFurtherNamesStaysUnset(t *testing.T) {
	prior := certificateModel{Names: types.ListNull(types.StringType)}
	got, diags := certificateFrom(context.Background(), prior, client.Certificate{Names: []string{"console.eds.mobile.deevnet.net"}})
	if diags.HasError() {
		t.Fatal(diags)
	}
	if !got.Names.IsNull() {
		t.Errorf("names = %v", got.Names)
	}
}

// Nothing about a certificate is a secret, and the model has nowhere to put a
// key: the tenant's key is another resource's, and never an argument here.
func TestTheCertificateModelHoldsNoKey(t *testing.T) {
	for name, attr := range (&certificateResource{}).schemaAttributes() {
		if attr.IsSensitive() {
			t.Errorf("%s is sensitive; a certificate holds nothing that is", name)
		}
		for _, banned := range []string{"private", "password", "secret"} {
			if strings.Contains(name, banned) {
				t.Errorf("attribute %q looks like it carries a key", name)
			}
		}
	}
}
