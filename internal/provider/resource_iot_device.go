package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// One of a tenant's edge devices in the registry (ADR-0012 §3). The entry is
// the device's identity. It carries no credential and grants no access: what a
// device may consume is carried by something it proves (ADR-0020 §2), and that
// is a later layer.
type iotDeviceResource struct{ client *client.Client }

func NewIoTDeviceResource() resource.Resource { return &iotDeviceResource{} }

type iotDeviceModel struct {
	Tenant     types.String `tfsdk:"tenant"`
	Name       types.String `tfsdk:"name"`
	TrustClass types.String `tfsdk:"trust_class"`
	MAC        types.String `tfsdk:"mac"`
	Status     types.String `tfsdk:"status"`
	Present    types.Bool   `tfsdk:"present"`
}

func (r *iotDeviceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iot_device"
}

func (r *iotDeviceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A device in the tenant's registry.\n\n" +
			"The entry **is** the device's identity: an application-owned device takes no " +
			"substrate host record and is named in its owner's own zone. It leases from its " +
			"trust class's pool unless a `deevnet_iot_address` reserves it a fixed address.\n\n" +
			"**Registering a device grants it nothing.** It is identity, not authorization. " +
			"Flash devices with a `deevnet_iot_wifi_key` to put them on the air; that key is " +
			"per tenant, and does not reference this resource.",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant this device belongs to: the `name` of a `deevnet_tenant`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A label for the device, unique within the tenant.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"trust_class": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "`iot` for devices whose firmware the tenant controls, " +
					"`iot_vendor` for devices whose vendor does. Changing it replaces the " +
					"device, because its SSID, its VLAN and any later service grants would " +
					"otherwise all move under an unchanged name.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"mac": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The device's hardware address. **It is never an " +
					"authorization input** — a MAC is trivially spoofed on a shared segment. " +
					"It is needed only by a device that is to hold a fixed address " +
					"(`deevnet_iot_address`), which is reserved for it. It is mutable: swapping " +
					"the hardware behind a name is an inventory change, not a new device, and a " +
					"device that holds an address keeps it. Accepted in any usual spelling and " +
					"stored lowercase, colon-separated.",
			},
			"status": computedString("`ready` once the entry exists. Registering calls no backend, so it is ready at once."),
			"present": schema.BoolAttribute{
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the API still holds this device.",
			},
		},
	}
}

func (r *iotDeviceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// ModifyPlan turns "the API no longer has this device" into a diff, the way the
// Wi-Fi key resource does. Without it an apply after the registry was lost
// reports no changes and the device stays unregistered.
//
// A device holds no secret today, so Read could drop it from state instead and
// let Terraform plan a create. This shape is built now on purpose: ADR-0020 §2
// attaches a per-device credential to this resource later, and at that point
// dropping the resource would discard the credential in state and cost a visit
// to the device. Retrofitting the restore path after the first credential has
// been issued is exactly the sequence that would lose one.
func (r *iotDeviceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var state iotDeviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !state.Present.Equal(types.BoolValue(false)) {
		return
	}
	var plan iotDeviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Present = types.BoolUnknown()
	plan.Status = types.StringUnknown()
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}

func (r *iotDeviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan iotDeviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *iotDeviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan iotDeviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

// Read records that the API no longer has the device rather than removing the
// resource, and ModifyPlan turns that into an apply that puts it back. See
// ModifyPlan for why this is the shape even though nothing is lost today.
func (r *iotDeviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state iotDeviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.client.GetDevice(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		state.Present = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the device failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, deviceFrom(d))...)
}

func (r *iotDeviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state iotDeviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteDevice(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deregistering the device failed", err.Error())
	}
}

func (r *iotDeviceResource) put(ctx context.Context, m iotDeviceModel, state stateSetter) diagList {
	req := client.PutDeviceRequest{
		Name:       m.Name.ValueString(),
		TrustClass: m.TrustClass.ValueString(),
	}
	if !m.MAC.IsUnknown() && !m.MAC.IsNull() {
		req.MAC = m.MAC.ValueString()
	}
	d, err := r.client.PutDevice(ctx, m.Tenant.ValueString(), req)
	if err != nil {
		var diags diagList
		diags.AddError("Registering the device failed", err.Error())
		return diags
	}
	return state.Set(ctx, deviceFrom(d))
}

// deviceFrom folds an API answer into the model. An absent MAC is null rather
// than the empty string, so a device with none planned does not show a
// perpetual diff against "".
func deviceFrom(d client.Device) iotDeviceModel {
	mac := types.StringNull()
	if d.MAC != "" {
		mac = types.StringValue(d.MAC)
	}
	return iotDeviceModel{
		Tenant:     types.StringValue(d.Tenant),
		Name:       types.StringValue(d.Name),
		TrustClass: types.StringValue(d.TrustClass),
		MAC:        mac,
		Status:     types.StringValue(d.Status),
		Present:    types.BoolValue(true),
	}
}
