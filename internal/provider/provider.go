// Package provider is the deevnet/deevnet Terraform provider (ADR-0012 §7,
// ADR-0015). A tenant declares itself, its workloads and its names here, and
// holds no substrate credential: its only secret is its Deevnet token.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

type deevnetProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &deevnetProvider{version: version} }
}

func (p *deevnetProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "deevnet"
	resp.Version = p.version
}

type providerModel struct {
	Endpoint      types.String `tfsdk:"endpoint"`
	Token         types.String `tfsdk:"token"`
	CACertificate types.String `tfsdk:"ca_certificate"`
}

func (p *deevnetProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Builds tenants through the Deevnet API: their network, workloads and published names.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The API's base URL, e.g. `https://api.mobile.deevnet.net:8080`. Defaults to `DEEVNET_API_ENDPOINT`.",
			},
			"token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "The bearer token: the tenant's own token, or the single-use enrollment token it was admitted with. " +
					"Defaults to `DEEVNET_API_TOKEN`.",
			},
			"ca_certificate": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Path to the site CA certificate the API's TLS certificate comes from. " +
					"Defaults to `DEEVNET_API_CACERT`; empty uses the system trust store.",
			},
		},
	}
}

func (p *deevnetProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := orEnv(cfg.Endpoint, "DEEVNET_API_ENDPOINT")
	token := orEnv(cfg.Token, "DEEVNET_API_TOKEN")
	caCert := orEnv(cfg.CACertificate, "DEEVNET_API_CACERT")

	if endpoint == "" {
		resp.Diagnostics.AddError("No API endpoint", "Set `endpoint` in the provider block or DEEVNET_API_ENDPOINT.")
	}
	if token == "" {
		resp.Diagnostics.AddError("No API token", "Set `token` in the provider block or DEEVNET_API_TOKEN.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c, err := client.New(client.Config{Endpoint: endpoint, Token: token, CACertificate: caCert})
	if err != nil {
		resp.Diagnostics.AddError("Cannot reach the Deevnet API", err.Error())
		return
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *deevnetProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewTenantResource,
		NewWorkloadResource,
		NewRecordResource,
	}
}

func (p *deevnetProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func orEnv(v types.String, key string) string {
	if !v.IsNull() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(key)
}

// clientFrom pulls the configured client out of a resource's provider data.
func clientFrom(data any, diags interface{ AddError(string, string) }) *client.Client {
	if data == nil {
		return nil
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", "The provider did not configure a Deevnet API client.")
		return nil
	}
	return c
}
