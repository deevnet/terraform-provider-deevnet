package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

func TestAddressFoldsTheAPIAnswer(t *testing.T) {
	got := addressFrom(client.DeviceAddress{
		Tenant: "eds", Device: "stand-1", TrustClass: "iot", Address: "10.20.30.25",
		MAC: "aa:bb:cc:dd:ee:ff", FQDN: "stand-1.eds.mobile.deevnet.net", Status: "ready",
	})
	if got.Tenant.ValueString() != "eds" || got.Device.ValueString() != "stand-1" {
		t.Errorf("identity = %s/%s", got.Tenant.ValueString(), got.Device.ValueString())
	}
	if got.Address.ValueString() != "10.20.30.25" || got.MAC.ValueString() != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("address = %s for %s", got.Address.ValueString(), got.MAC.ValueString())
	}
	if got.FQDN.ValueString() != "stand-1.eds.mobile.deevnet.net" || got.TrustClass.ValueString() != "iot" {
		t.Errorf("fqdn = %s, class = %s", got.FQDN.ValueString(), got.TrustClass.ValueString())
	}
	if !got.Present.ValueBool() {
		t.Error("an address that was read should be present")
	}
}

// What decides whether a plan re-applies. An address the API lost, or one it
// recorded without finishing, must plan an apply; a settled one must not, or
// every plan would show a change.
func TestAddressNeedsApply(t *testing.T) {
	settled := addressFrom(client.DeviceAddress{Address: "10.20.30.25", Status: "ready"})
	if addressNeedsApply(settled) {
		t.Error("a ready, present address plans no apply")
	}
	lost := settled
	lost.Present = types.BoolValue(false)
	if !addressNeedsApply(lost) {
		t.Error("an address the API no longer holds must be re-applied")
	}
	half := addressFrom(client.DeviceAddress{Address: "10.20.30.25", Status: "provisioning"})
	if !addressNeedsApply(half) {
		t.Error("an address that never reached the DHCP server must be re-applied")
	}
}
