package provider

import (
	"reflect"
	"strings"
	"testing"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

func TestDeviceFoldsTheAPIAnswer(t *testing.T) {
	got := deviceFrom(client.Device{
		Tenant: "eds", Name: "stand-1", TrustClass: "iot",
		MAC: "aa:bb:cc:dd:ee:ff", Status: "ready",
	})

	if got.Tenant.ValueString() != "eds" || got.Name.ValueString() != "stand-1" {
		t.Errorf("identity = %s/%s", got.Tenant.ValueString(), got.Name.ValueString())
	}
	if got.TrustClass.ValueString() != "iot" {
		t.Errorf("trust_class = %s", got.TrustClass.ValueString())
	}
	if got.MAC.ValueString() != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("mac = %s", got.MAC.ValueString())
	}
	if !got.Present.ValueBool() {
		t.Error("a device that was read should be present")
	}
}

// A device with no MAC must fold to null, not to "". An empty string in state
// against a null in config is a diff that never converges.
func TestDeviceWithoutMACFoldsToNull(t *testing.T) {
	got := deviceFrom(client.Device{
		Tenant: "eds", Name: "stand-1", TrustClass: "iot", Status: "ready",
	})
	if !got.MAC.IsNull() {
		t.Errorf("mac = %q, want null", got.MAC.ValueString())
	}
}

// The registry holds no credential. If this fails, a secret has been added to
// the resource, and the restore path in ModifyPlan needs the treatment the
// Wi-Fi key's psk gets - carried through on a restore, never blanked - or the
// first apply after a registry loss will mint a new one and strand the device
// (ADR-0020 §2, ADR-0012 §5).
func TestDeviceModelHoldsNoSecret(t *testing.T) {
	ty := reflect.TypeOf(iotDeviceModel{})
	for i := 0; i < ty.NumField(); i++ {
		tag := ty.Field(i).Tag.Get("tfsdk")
		for _, secret := range []string{"psk", "token", "secret", "key", "password", "credential"} {
			if strings.Contains(tag, secret) {
				t.Errorf("the device model gained a secret-looking field %q; see this test's comment", tag)
			}
		}
	}
}
