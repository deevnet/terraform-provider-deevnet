package provider

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// A server certificate for one of the tenant's own services, for a name in
// the tenant's own zone (ADR-0037). The tenant makes the key - with the tls
// provider, say - and this resource takes a signing request made with it. The
// key itself is never an argument here, and never goes to the API.
//
// Unlike everything else a tenant declares, a certificate goes wrong by
// itself: it expires. So this resource plans a change with nothing changed in
// the configuration, once the certificate is within sixty days of expiring,
// and the apply that follows renews it. Nothing renews it for a tenant that
// never applies.
type certificateResource struct {
	client *client.Client
	// now is the clock, so a test can stand near an expiry.
	now func() time.Time
}

func NewCertificateResource() resource.Resource { return &certificateResource{now: time.Now} }

type certificateModel struct {
	Tenant types.String `tfsdk:"tenant"`
	Name   types.String `tfsdk:"name"`
	CSR    types.String `tfsdk:"csr_pem"`
	Names  types.List   `tfsdk:"names"`

	FQDN           types.String `tfsdk:"fqdn"`
	DNSNames       types.List   `tfsdk:"dns_names"`
	Certificate    types.String `tfsdk:"certificate_pem"`
	CAChain        types.String `tfsdk:"ca_chain_pem"`
	FullChain      types.String `tfsdk:"fullchain_pem"`
	Serial         types.String `tfsdk:"serial"`
	KeyFingerprint types.String `tfsdk:"key_fingerprint"`
	NotBefore      types.String `tfsdk:"not_before"`
	NotAfter       types.String `tfsdk:"not_after"`
	RenewAfter     types.String `tfsdk:"renew_after"`
	Present        types.Bool   `tfsdk:"present"`
}

func (r *certificateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_certificate"
}

func (r *certificateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A server certificate for one of the tenant's own services, for a name in the tenant's own zone, " +
			"signed by the site's Tenant Service CA. It chains to the Deevnet Root CA, which every client on a site already trusts.\n\n" +
			"**You make the key and keep it.** This resource takes a signing request, not a key: make both with the `tls` provider " +
			"(`tls_private_key`, `tls_cert_request`). The key stays in your state and on your workload, and never goes to the API.\n\n" +
			"**The request is read for its public key alone.** Its subject and its names are not used. The certificate is for " +
			"`<name>.<tenant zone>`, and for each of `names` in the same zone, whatever the request says.\n\n" +
			"**It expires after a year, and renewing is an apply.** Once fewer than sixty days remain, a plan shows this resource " +
			"changing, and the apply issues a new certificate for the same key. Nothing renews it if you do not apply: run a plan " +
			"at least every ten months. Whatever serves the certificate then needs the new one.\n\n" +
			"**A certificate cannot be withdrawn.** Destroying this resource, or replacing its key, does not revoke the old " +
			"certificate, which stays valid until it expires wherever its key still is.",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant this certificate belongs to: the `name` of a `deevnet_tenant`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The certificate's own name: one DNS label under the tenant's zone, such as `console`. " +
					"A label, not a full name: the API appends the zone.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"csr_pem": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "A PEM signing request made with your key: `tls_cert_request.<name>.cert_request_pem`. " +
					"The key must be RSA of 2048 bits or more, or ECDSA P-256. A request for a new key issues a new certificate.",
			},
			"names": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				MarkdownDescription: "Further labels in the tenant's zone for the certificate to be valid for, beside `name`. " +
					"Up to seven. No dots and no wildcards.",
			},
			"fqdn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The certificate's own name in full, and its common name.",
			},
			"dns_names": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Every DNS name the certificate is valid for, in full, its own first.",
			},
			"certificate_pem": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The certificate, PEM.",
			},
			"ca_chain_pem": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "What a server presents after the certificate, PEM: the Tenant Service CA, then the Site CA. " +
					"Not the root, which a client holds.",
			},
			"fullchain_pem": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`certificate_pem` followed by `ca_chain_pem`: the one file most servers want beside the key.",
			},
			"serial": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The certificate's serial number, as hex pairs.",
			},
			"key_fingerprint": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The SHA-256 of the public key certified, in hex.",
			},
			"not_before": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the certificate becomes valid, RFC 3339.",
			},
			"not_after": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the certificate expires, RFC 3339.",
			},
			"renew_after": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When a plan begins to show a renewal: sixty days before `not_after`, RFC 3339.",
			},
			"present": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the API still holds this certificate.",
			},
		},
	}
}

func (r *certificateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// ModifyPlan is where a certificate differs from every other resource here.
//
// None of the computed attributes keeps its value in a plan, so a new key or
// a changed name already plans them as unknown. What that leaves is the two
// cases in which nothing in the configuration changed and a new certificate
// is still due: the API no longer holds it, or it is close to expiring. Both
// are planned as an update, which is what makes the next apply ask again.
func (r *certificateResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !reissueDue(state, r.now()) {
		return
	}
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.FQDN = types.StringUnknown()
	plan.DNSNames = types.ListUnknown(types.StringType)
	plan.Certificate = types.StringUnknown()
	plan.CAChain = types.StringUnknown()
	plan.FullChain = types.StringUnknown()
	plan.Serial = types.StringUnknown()
	plan.KeyFingerprint = types.StringUnknown()
	plan.NotBefore = types.StringUnknown()
	plan.NotAfter = types.StringUnknown()
	plan.RenewAfter = types.StringUnknown()
	plan.Present = types.BoolUnknown()
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}

// reissueDue reports whether a certificate in state needs asking for again
// with nothing changed: the API has lost it, or its renewal day has come. A
// date that cannot be read counts as due. Asking is an ensure, so being wrong
// that way costs one call that returns the same certificate.
func reissueDue(state certificateModel, now time.Time) bool {
	if state.Present.Equal(types.BoolValue(false)) {
		return true
	}
	if state.RenewAfter.IsNull() || state.RenewAfter.IsUnknown() {
		return false
	}
	renew, err := time.Parse(time.RFC3339, state.RenewAfter.ValueString())
	if err != nil {
		return true
	}
	return !now.Before(renew)
}

func (r *certificateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *certificateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.client.GetCertificate(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		// Kept, and marked: the certificate in state may still be the one a
		// workload is serving, and the next apply asks for it again.
		state.Present = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the certificate failed", err.Error())
		return
	}
	next, diags := certificateFrom(ctx, state, c)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *certificateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan certificateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *certificateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state certificateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteCertificate(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting the certificate failed", err.Error())
	}
}

func (r *certificateResource) put(ctx context.Context, m certificateModel, state stateSetter) diagList {
	names, diags := stringsFrom(ctx, m.Names)
	if diags.HasError() {
		return diags
	}
	c, err := r.client.PutCertificate(ctx, m.Tenant.ValueString(), m.Name.ValueString(),
		client.PutCertificateRequest{CSR: m.CSR.ValueString(), Names: names})
	if err != nil {
		diags.AddError("Issuing the certificate failed", err.Error())
		return diags
	}
	next, more := certificateFrom(ctx, m, c)
	diags.Append(more...)
	if diags.HasError() {
		return diags
	}
	diags.Append(state.Set(ctx, next)...)
	return diags
}

// certificateFrom folds the API's answer into the model. The arguments are
// kept as they were written: the API answers with full names, and the
// configuration holds labels.
func certificateFrom(ctx context.Context, prior certificateModel, c client.Certificate) (certificateModel, diagList) {
	dnsNames, diags := types.ListValueFrom(ctx, types.StringType, c.Names)
	return certificateModel{
		Tenant: prior.Tenant,
		Name:   prior.Name,
		CSR:    prior.CSR,
		Names:  prior.Names,

		FQDN:           types.StringValue(c.FQDN),
		DNSNames:       dnsNames,
		Certificate:    types.StringValue(c.Certificate),
		CAChain:        types.StringValue(c.CAChain),
		FullChain:      types.StringValue(c.Certificate + c.CAChain),
		Serial:         types.StringValue(c.Serial),
		KeyFingerprint: types.StringValue(c.KeyFingerprint),
		NotBefore:      types.StringValue(c.NotBefore),
		NotAfter:       types.StringValue(c.NotAfter),
		RenewAfter:     types.StringValue(c.RenewAfter),
		Present:        types.BoolValue(true),
	}, diags
}

// schemaAttributes is the schema's attributes, for the test that holds this
// resource to carrying no key.
func (r *certificateResource) schemaAttributes() map[string]schema.Attribute {
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema.Attributes
}
