package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// A name the tenant publishes beside its workloads' own (ADR-0015 §13), such
// as an alias for a service. The address must be in the tenant's subnet.
type recordResource struct{ client *client.Client }

func NewRecordResource() resource.Resource { return &recordResource{} }

type recordModel struct {
	Tenant  types.String `tfsdk:"tenant"`
	Name    types.String `tfsdk:"name"`
	Address types.String `tfsdk:"address"`
	FQDN    types.String `tfsdk:"fqdn"`
	Present types.Bool   `tfsdk:"present"`
}

func (r *recordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *recordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A name in the tenant's zone, with its PTR.",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A DNS label under the tenant's zone.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"address": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "An address in the tenant's own subnet.",
			},
			"fqdn":    computedString("The published name."),
			"present": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the API still holds this name."},
		},
	}
}

func (r *recordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *recordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan recordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *recordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state recordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rec, err := r.client.GetRecord(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		state.Present = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the name failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, recordFrom(state.Tenant.ValueString(), rec))...)
}

func (r *recordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan recordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *recordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state recordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteRecord(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting the name failed", err.Error())
	}
}

type stateSetter interface {
	Set(ctx context.Context, val any) diagList
}

func (r *recordResource) put(ctx context.Context, m recordModel, state stateSetter) diagList {
	rec, err := r.client.PutRecord(ctx, m.Tenant.ValueString(), m.Name.ValueString(), m.Address.ValueString())
	if err != nil {
		var diags diagList
		diags.AddError("Publishing the name failed", err.Error())
		return diags
	}
	return state.Set(ctx, recordFrom(m.Tenant.ValueString(), rec))
}

func recordFrom(tenant string, rec client.Record) recordModel {
	return recordModel{
		Tenant:  types.StringValue(tenant),
		Name:    types.StringValue(rec.Name),
		Address: types.StringValue(rec.Address),
		FQDN:    types.StringValue(rec.FQDN),
		Present: types.BoolValue(true),
	}
}
