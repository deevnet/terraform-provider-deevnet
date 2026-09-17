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

// A workload: a VM in the tenant's network (ADR-0015 §12). The tenant chooses
// the name, sizing and keys; the API derives the ordinal, VMID, MAC and
// address and keeps them for the workload's life.
//
// Read leaves a workload the API no longer has in state with `present` false,
// for the same reason the tenant resource does: the next apply rebuilds it on
// its own numbering instead of picking a new one.
type workloadResource struct{ client *client.Client }

func NewWorkloadResource() resource.Resource { return &workloadResource{} }

type workloadModel struct {
	Tenant   types.String `tfsdk:"tenant"`
	Name     types.String `tfsdk:"name"`
	Cores    types.Int64  `tfsdk:"cores"`
	MemoryMB types.Int64  `tfsdk:"memory_mb"`
	DiskGB   types.Int64  `tfsdk:"disk_gb"`
	SSHKeys  types.List   `tfsdk:"ssh_keys"`

	Present types.Bool   `tfsdk:"present"`
	Status  types.String `tfsdk:"status"`
	Ordinal types.Int64  `tfsdk:"ordinal"`
	VMID    types.Int64  `tfsdk:"vmid"`
	MAC     types.String `tfsdk:"mac"`
	Address types.String `tfsdk:"address"`
	FQDN    types.String `tfsdk:"fqdn"`
}

func (r *workloadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workload"
}

func (r *workloadResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A VM in the tenant's network. Its name is published in the tenant's zone.",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant this workload belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A DNS label under the tenant's zone, 1-20 lowercase alphanumerics or dashes.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cores": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Defaults to 2.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"memory_mb": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Defaults to 2048.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"disk_gb": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Grows the template's disk. It is never shrunk.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"ssh_keys": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Public keys for the cloud-init account.",
			},
			"present": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the API still holds this workload.",
			},
			"status":  computedString("`provisioning` or `ready`."),
			"ordinal": computedInt("The workload's number within its tenant; the VMID and address derive from it."),
			"vmid":    computedInt("Proxmox VMID."),
			"mac":     computedString("MAC, derived from the VMID."),
			"address": computedString("Address in the tenant's subnet."),
			"fqdn":    computedString("The name published for it."),
		},
	}
}

func (r *workloadResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// See the tenant resource's ModifyPlan: `present: false` has to become a diff,
// or an apply after the registry was lost reports no changes and leaves the
// workload unrecorded. Re-posting it adopts the VM that is already there.
func (r *workloadResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var state workloadModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !state.Present.Equal(types.BoolValue(false)) {
		return
	}
	var plan workloadModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Present = types.BoolUnknown()
	plan.Status = types.StringUnknown()
	plan.Ordinal = types.Int64Unknown()
	plan.VMID = types.Int64Unknown()
	plan.MAC = types.StringUnknown()
	plan.Address = types.StringUnknown()
	plan.FQDN = types.StringUnknown()
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}

func (r *workloadResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan workloadModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, diags := r.put(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workloadFrom(w, plan))...)
}

func (r *workloadResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state workloadModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, err := r.client.GetWorkload(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		state.Present = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the workload failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workloadFrom(w, state))...)
}

func (r *workloadResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan workloadModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	w, diags := r.put(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, workloadFrom(w, plan))...)
}

func (r *workloadResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state workloadModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteWorkload(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting the workload failed", err.Error())
	}
}

func (r *workloadResource) put(ctx context.Context, m workloadModel) (client.Workload, diagList) {
	var diags diagList
	keys := []string{}
	if !m.SSHKeys.IsNull() && !m.SSHKeys.IsUnknown() {
		diags = append(diags, m.SSHKeys.ElementsAs(ctx, &keys, false)...)
		if diags.HasError() {
			return client.Workload{}, diags
		}
	}
	w, err := r.client.PutWorkload(ctx, m.Tenant.ValueString(), client.CreateWorkloadRequest{
		Name:     m.Name.ValueString(),
		Cores:    m.Cores.ValueInt64(),
		MemoryMB: m.MemoryMB.ValueInt64(),
		DiskGB:   m.DiskGB.ValueInt64(),
		SSHKeys:  keys,
	})
	if err != nil {
		diags.AddError("Building the workload failed", err.Error())
	}
	return w, diags
}

func workloadFrom(w client.Workload, prior workloadModel) workloadModel {
	return workloadModel{
		Tenant:   types.StringValue(w.Tenant),
		Name:     types.StringValue(w.Name),
		Cores:    types.Int64Value(w.Cores),
		MemoryMB: types.Int64Value(w.MemoryMB),
		DiskGB:   types.Int64Value(w.DiskGB),
		SSHKeys:  prior.SSHKeys,
		Present:  types.BoolValue(true),
		Status:   types.StringValue(w.Status),
		Ordinal:  types.Int64Value(w.Ordinal),
		VMID:     types.Int64Value(w.VMID),
		MAC:      types.StringValue(w.MAC),
		Address:  types.StringValue(w.Address),
		FQDN:     types.StringValue(w.FQDN),
	}
}
