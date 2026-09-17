package provider_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/deevnet/terraform-provider-deevnet/internal/provider"
)

// Acceptance tests drive real Terraform against a real Deevnet API, which
// builds real objects. They run only with TF_ACC=1 and DEEVNET_API_ENDPOINT,
// DEEVNET_API_TOKEN (an operator token) and optionally DEEVNET_API_CACERT.
var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"deevnet": providerserver.NewProtocol6WithError(provider.New("test")()),
}

func preCheck(t *testing.T) {
	t.Helper()
	for _, k := range []string{"DEEVNET_API_ENDPOINT", "DEEVNET_API_TOKEN"} {
		if os.Getenv(k) == "" {
			t.Skipf("%s is not set", k)
		}
	}
}

// The tenant name these tests build and destroy. It must not be a live tenant.
func testTenant() string {
	if v := os.Getenv("DEEVNET_TEST_TENANT"); v != "" {
		return v
	}
	return "tfacc"
}

func TestAccTenantWithARecord(t *testing.T) {
	name := testTenant()
	config := fmt.Sprintf(`
resource "deevnet_tenant" "this" {
  name = %q
}

resource "deevnet_dns_record" "alias" {
  tenant  = deevnet_tenant.this.name
  name    = "alias"
  address = cidrhost(deevnet_tenant.this.subnet, 10)
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("deevnet_tenant.this", "name", name),
					resource.TestCheckResourceAttr("deevnet_tenant.this", "status", "ready"),
					resource.TestCheckResourceAttr("deevnet_tenant.this", "present", "true"),
					// Everything below is the API's to decide, so the test
					// asserts shape rather than value.
					resource.TestMatchResourceAttr("deevnet_tenant.this", "index", regexpDigits),
					resource.TestMatchResourceAttr("deevnet_tenant.this", "subnet", regexpCIDR),
					resource.TestCheckResourceAttrSet("deevnet_tenant.this", "tsig_secret"),
					resource.TestCheckResourceAttrSet("deevnet_tenant.this", "state_secret_key"),
					resource.TestCheckResourceAttrSet("deevnet_tenant.this", "api_token"),
					resource.TestCheckResourceAttr("deevnet_tenant.this", "dns_zone", name+".mobile.deevnet.net"),
					resource.TestCheckResourceAttr("deevnet_dns_record.alias", "fqdn", "alias."+name+".mobile.deevnet.net"),
				),
			},
			// The plan is empty afterwards: nothing the API issued drifts.
			{Config: config, PlanOnly: true},
		},
	})
}

func TestAccWorkload(t *testing.T) {
	if os.Getenv("DEEVNET_TEST_WORKLOADS") == "" {
		t.Skip("DEEVNET_TEST_WORKLOADS is not set; this builds a VM on the tenant hypervisor")
	}
	name := testTenant()
	config := fmt.Sprintf(`
resource "deevnet_tenant" "this" {
  name = %q
}

resource "deevnet_workload" "web" {
  tenant    = deevnet_tenant.this.name
  name      = "web"
  cores     = 1
  memory_mb = 1024
}
`, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { preCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("deevnet_workload.web", "status", "ready"),
					resource.TestCheckResourceAttr("deevnet_workload.web", "ordinal", "0"),
					resource.TestCheckResourceAttr("deevnet_workload.web", "cores", "1"),
					resource.TestCheckResourceAttrSet("deevnet_workload.web", "vmid"),
					resource.TestCheckResourceAttrSet("deevnet_workload.web", "mac"),
					resource.TestCheckResourceAttr("deevnet_workload.web", "fqdn", "web."+name+".mobile.deevnet.net"),
				),
			},
			{Config: config, PlanOnly: true},
		},
	})
}
