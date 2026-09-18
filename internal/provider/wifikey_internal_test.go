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
