package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// The tenant itself: its index, network numbering, DNS zone and key, and its
// state-store credential (ADR-0015 §2).
//
// Read does NOT forget a tenant the API no longer has. The framework's own
// advice is to call RemoveResource and let the next plan recreate, but that
// would mint new secrets and cost every device a visit (ADR-0012 §5). Instead
// `present` goes false, which makes the next plan an update, and Update sends
// the index and secrets back from state. The API keeps that index when it is
// free, and issues a new one when another tenant took it.
type tenantResource struct{ client *client.Client }

func NewTenantResource() resource.Resource { return &tenantResource{} }

type tenantModel struct {
	Name    types.String `tfsdk:"name"`
	Present types.Bool   `tfsdk:"present"`
	// SecretsStored false means the API cannot read the secrets it holds for this
	// tenant, and this state's copies are the ones that put it right.
	SecretsStored types.Bool   `tfsdk:"secrets_stored"`
	Status        types.String `tfsdk:"status"`
	Index         types.Int64  `tfsdk:"index"`

	VRFVNI      types.Int64  `tfsdk:"vrf_vni"`
	VNetVNIBase types.Int64  `tfsdk:"vnet_vni_base"`
	Subnet      types.String `tfsdk:"subnet"`
	Gateway     types.String `tfsdk:"gateway"`

	ControllerID types.String `tfsdk:"controller_id"`
	Node         types.String `tfsdk:"node"`

	DNSZone         types.String `tfsdk:"dns_zone"`
	DNSReverseZone  types.String `tfsdk:"dns_reverse_zone"`
	DNSUpdateServer types.String `tfsdk:"dns_update_server"`
	TSIGKeyName     types.String `tfsdk:"tsig_key_name"`
	TSIGAlgorithm   types.String `tfsdk:"tsig_algorithm"`
	TSIGSecret      types.String `tfsdk:"tsig_secret"`

	StateEndpoint  types.String `tfsdk:"state_endpoint"`
	StateBucket    types.String `tfsdk:"state_bucket"`
	StateKeyPrefix types.String `tfsdk:"state_key_prefix"`
	StateAccessKey types.String `tfsdk:"state_access_key"`
	StateSecretKey types.String `tfsdk:"state_secret_key"`

	APIToken types.String `tfsdk:"api_token"`
}

// computedString and computedInt are the attributes the API issues: they keep
// their value in a plan, so a restore is the only thing that changes them.
func computedString(desc string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: desc,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func computedInt(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Computed:            true,
		MarkdownDescription: desc,
		PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
	}
}

func (r *tenantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant"
}

func (r *tenantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A tenant: its index and numbering, its DNS zone and key, its state-store credential, " +
			"and its own API token. Everything but the name is issued by the API.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "1-8 lowercase alphanumerics, starting with a letter: the SDN zone id, a DNS label and a state-store user.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"present": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether the API still holds this tenant. False after the API's registry is lost, " +
					"which makes the next apply restore it from this state rather than create a new one.",
			},
			"secrets_stored": schema.BoolAttribute{
				Computed: true,
				MarkdownDescription: "Whether the API can still read the secrets it holds for this tenant. " +
					"False after its Transit key is rebuilt or rotated past them, which makes the next apply " +
					"send this state's copies again - they are the authoritative ones.",
			},
			"status":            computedString("`provisioning` or `ready`."),
			"index":             computedInt("The tenant index every identifier derives from (ADR-0002)."),
			"vrf_vni":           computedInt("VRF VXLAN id."),
			"vnet_vni_base":     computedInt("First VNet VXLAN id."),
			"subnet":            computedString("Overlay subnet."),
			"gateway":           computedString("Anycast gateway."),
			"controller_id":     computedString("EVPN controller the tenant's zone attaches to."),
			"node":              computedString("Hypervisor the tenant's workloads land on."),
			"dns_zone":          computedString("Forward zone."),
			"dns_reverse_zone":  computedString("Reverse zone."),
			"dns_update_server": computedString("Where RFC 2136 updates go, if the tenant publishes names itself."),
			"tsig_key_name":     computedString("TSIG key name."),
			"tsig_algorithm":    computedString("TSIG algorithm."),
			"tsig_secret": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "TSIG secret. This state is its authoritative copy (ADR-0015 §4).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"state_endpoint":   computedString("State store endpoint."),
			"state_bucket":     computedString("State store bucket."),
			"state_key_prefix": computedString("The prefix this tenant may write."),
			"state_access_key": computedString("State store access key."),
			"state_secret_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "State store secret key.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"api_token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The tenant's own API token, for every later call it makes.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *tenantResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// ModifyPlan turns the two conditions a Read can discover into an apply that
// fixes them: the tenant is gone from the API, or the API cannot read the secrets
// it holds for it. Both are answered from this state, which holds the
// authoritative copies (ADR-0015 §4), and neither produces a diff on its own.
//
// Every issued
// attribute keeps its state value in a plan (UseStateForUnknown), so a tenant
// the API no longer has produced no diff at all: `terraform apply` reported "no
// changes" while the substrate held no tenant. Marking them unknown gives
// Terraform an update to make, and Update sends the index and secrets back.
//
// They are ALL marked unknown, not just `present`: a restore onto an index
// another tenant took returns different numbering, and a computed attribute that
// was not unknown in the plan and changes in apply is an error rather than a
// new value.
func (r *tenantResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return // creating or destroying
	}
	var state tenantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	gone := state.Present.Equal(types.BoolValue(false))
	unreadable := state.SecretsStored.Equal(types.BoolValue(false))
	if !gone && !unreadable {
		return
	}
	var plan tenantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The API holds this tenant but cannot read its secrets - a Transit key
	// rebuilt or rotated past them. Only the secrets need sending again; the index
	// and everything derived from it are unchanged, because the registry row is
	// still there. Marking the whole tenant unknown here would show a plan that
	// implies the network is about to be rebuilt, which it is not.
	if unreadable && !gone {
		plan.SecretsStored = types.BoolUnknown()
		plan.TSIGSecret = types.StringUnknown()
		plan.StateSecretKey = types.StringUnknown()
		plan.APIToken = types.StringUnknown()
		resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
		return
	}

	plan.SecretsStored = types.BoolUnknown()
	plan.Present = types.BoolUnknown()
	plan.Status = types.StringUnknown()
	plan.Index = types.Int64Unknown()
	plan.VRFVNI = types.Int64Unknown()
	plan.VNetVNIBase = types.Int64Unknown()
	plan.Subnet = types.StringUnknown()
	plan.Gateway = types.StringUnknown()
	plan.ControllerID = types.StringUnknown()
	plan.Node = types.StringUnknown()
	plan.DNSZone = types.StringUnknown()
	plan.DNSReverseZone = types.StringUnknown()
	plan.DNSUpdateServer = types.StringUnknown()
	plan.TSIGKeyName = types.StringUnknown()
	plan.TSIGAlgorithm = types.StringUnknown()
	plan.TSIGSecret = types.StringUnknown()
	plan.StateEndpoint = types.StringUnknown()
	plan.StateBucket = types.StringUnknown()
	plan.StateKeyPrefix = types.StringUnknown()
	plan.StateAccessKey = types.StringUnknown()
	plan.StateSecretKey = types.StringUnknown()
	plan.APIToken = types.StringUnknown()
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}

func (r *tenantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tenantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.CreateTenant(ctx, client.CreateTenantRequest{Name: plan.Name.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Creating the tenant failed", err.Error())
		return
	}
	// The enrollment token this apply was configured with has just been spent.
	// Everything after this - workloads, names - goes with the tenant's own.
	r.client.UseToken(t.APIToken)
	resp.Diagnostics.Append(resp.State.Set(ctx, tenantFrom(t, plan))...)
}

func (r *tenantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state tenantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.client.GetTenant(ctx, state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		// Kept, not forgotten: the next apply restores it (see the type comment).
		state.Present = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the tenant failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tenantFrom(t, state))...)
}

func (r *tenantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state, plan tenantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// The restore: the index and the secrets go back as they are.
	t, err := r.client.CreateTenant(ctx, client.CreateTenantRequest{
		Name:        state.Name.ValueString(),
		Index:       state.Index.ValueInt64(),
		TSIGSecret:  state.TSIGSecret.ValueString(),
		StateSecret: state.StateSecretKey.ValueString(),
		APIToken:    state.APIToken.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Restoring the tenant failed", err.Error())
		return
	}
	r.client.UseToken(t.APIToken)
	if t.Index != state.Index.ValueInt64() {
		resp.Diagnostics.AddWarning("The tenant was issued a new index",
			"Another tenant holds the index this one had, so the API issued index "+
				t.DNS.Zone+" a new one. Its network and workloads are rebuilt on the new numbering (ADR-0015 §5).")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tenantFrom(t, plan))...)
}

func (r *tenantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state tenantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteTenant(ctx, state.Name.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting the tenant failed", err.Error())
	}
}

// tenantFrom maps an API tenant onto the model. Secrets the API did not return
// are kept from prior state: a read never carries them.
func tenantFrom(t client.Tenant, prior tenantModel) tenantModel {
	m := tenantModel{
		Name:    types.StringValue(t.Name),
		Present: types.BoolValue(true),
		// An API that does not report the field is older than it and has no way to
		// say otherwise; assuming readable keeps it usable rather than replanning
		// a resupply on every apply.
		SecretsStored:   types.BoolValue(t.SecretsStored == nil || *t.SecretsStored),
		Status:          types.StringValue(t.Status),
		Index:           types.Int64Value(t.Index),
		VRFVNI:          types.Int64Value(t.Network.VRFVNI),
		VNetVNIBase:     types.Int64Value(t.Network.VNetVNIBase),
		Subnet:          types.StringValue(t.Network.Subnet),
		Gateway:         types.StringValue(t.Network.Gateway),
		ControllerID:    types.StringValue(t.Fabric.ControllerID),
		Node:            types.StringValue(t.Fabric.Node),
		DNSZone:         types.StringValue(t.DNS.Zone),
		DNSReverseZone:  types.StringValue(t.DNS.ReverseZone),
		DNSUpdateServer: types.StringValue(t.DNS.UpdateServer),
		TSIGKeyName:     types.StringValue(t.DNS.TSIGKeyName),
		TSIGAlgorithm:   types.StringValue(t.DNS.TSIGAlgorithm),
		StateEndpoint:   types.StringValue(t.State.Endpoint),
		StateBucket:     types.StringValue(t.State.Bucket),
		StateKeyPrefix:  types.StringValue(t.State.KeyPrefix),
		StateAccessKey:  types.StringValue(t.State.AccessKey),
		TSIGSecret:      keep(t.DNS.TSIGSecret, prior.TSIGSecret),
		StateSecretKey:  keep(t.State.SecretKey, prior.StateSecretKey),
		APIToken:        keep(t.APIToken, prior.APIToken),
	}
	return m
}

func keep(fresh string, prior types.String) types.String {
	if fresh != "" {
		return types.StringValue(fresh)
	}
	if prior.IsNull() || prior.IsUnknown() {
		return types.StringNull()
	}
	return prior
}
