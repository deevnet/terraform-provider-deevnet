package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// A read does not return the psk. Folding that answer into the model must keep
// the one already in state: it is the credential the devices were flashed with,
// and this state holds the authoritative copy (ADR-0012 §4).
func TestReadKeepsTheKeyAlreadyInState(t *testing.T) {
	prior := iotWiFiKeyModel{PSK: types.StringValue("theKeyDevicesHold")}
	got := wifiKeyFrom(client.WiFiKey{
		Tenant: "eds", Name: "devices", TrustClass: "iot",
		SSID: "DVNTM-IOT", VLAN: 30, Status: "ready", SecretsStored: true,
	}, prior)

	if got.PSK.ValueString() != "theKeyDevicesHold" {
		t.Fatalf("psk = %q; a read must not blank the key", got.PSK.ValueString())
	}
	if got.SSID.ValueString() != "DVNTM-IOT" || got.VLAN.ValueInt64() != 30 {
		t.Errorf("ssid/vlan = %s/%d", got.SSID.ValueString(), got.VLAN.ValueInt64())
	}
	if !got.Present.ValueBool() {
		t.Error("a key that was read should be present")
	}
}

// A create does return it, and that one wins.
func TestCreateTakesTheIssuedKey(t *testing.T) {
	prior := iotWiFiKeyModel{PSK: types.StringNull()}
	got := wifiKeyFrom(client.WiFiKey{
		Tenant: "eds", Name: "devices", TrustClass: "iot",
		SSID: "DVNTM-IOT", VLAN: 30, PSK: "freshlyIssued", SecretsStored: true,
	}, prior)
	if got.PSK.ValueString() != "freshlyIssued" {
		t.Fatalf("psk = %q", got.PSK.ValueString())
	}
}

// An API that can no longer read its copy is reported, so ModifyPlan turns it
// into an apply that resupplies the key rather than minting a new one.
func TestSecretsStoredFalseIsCarried(t *testing.T) {
	got := wifiKeyFrom(client.WiFiKey{
		Tenant: "eds", Name: "devices", TrustClass: "iot",
		SSID: "DVNTM-IOT", VLAN: 30, SecretsStored: false,
	}, iotWiFiKeyModel{PSK: types.StringValue("held")})
	if got.SecretsStored.ValueBool() {
		t.Error("secrets_stored should be false")
	}
	if got.PSK.ValueString() != "held" {
		t.Error("the key must survive so the next apply can resupply it")
	}
}

// State keeps the MAC as the tenant wrote it: the API answers with its own
// spelling, and a state that differed from the configuration would fail the
// apply as an inconsistent result.
func TestWiFiKeyKeepsTheMACAsWritten(t *testing.T) {
	prior := iotWiFiKeyModel{PSK: types.StringValue("held"), MAC: types.StringValue("AA-BB-CC-00-11-22")}
	got := wifiKeyFrom(client.WiFiKey{Tenant: "eds", Name: "laptop", MAC: "aa:bb:cc:00:11:22"}, prior)
	if got.MAC.ValueString() != "AA-BB-CC-00-11-22" {
		t.Errorf("mac = %s", got.MAC)
	}
	// An import has nothing written, so it takes the API's.
	got = wifiKeyFrom(client.WiFiKey{Tenant: "eds", Name: "laptop", MAC: "aa:bb:cc:00:11:22"},
		iotWiFiKeyModel{MAC: types.StringNull()})
	if got.MAC.ValueString() != "aa:bb:cc:00:11:22" {
		t.Errorf("imported mac = %s", got.MAC)
	}
	// No binding stays null, not "".
	got = wifiKeyFrom(client.WiFiKey{Tenant: "eds", Name: "devices"}, iotWiFiKeyModel{MAC: types.StringNull()})
	if !got.MAC.IsNull() {
		t.Errorf("unbound mac = %s, want null", got.MAC)
	}
}
