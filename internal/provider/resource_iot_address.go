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

// A fixed address for one of the tenant's registered devices (ADR-0035).
//
// The network is the substrate's and shared by every tenant, so the address is
// allocated by the API rather than derived. This state is what remembers it.
type iotAddressResource struct{ client *client.Client }

func NewIoTAddressResource() resource.Resource { return &iotAddressResource{} }

type iotAddressModel struct {
	Tenant     types.String `tfsdk:"tenant"`
	Device     types.String `tfsdk:"device"`
	Address    types.String `tfsdk:"address"`
	TrustClass types.String `tfsdk:"trust_class"`
	MAC        types.String `tfsdk:"mac"`
	FQDN       types.String `tfsdk:"fqdn"`
	Status     types.String `tfsdk:"status"`
	Present    types.Bool   `tfsdk:"present"`
}

func (r *iotAddressResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iot_address"
}

func (r *iotAddressResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A fixed address for a registered device on its trust class's network.\n\n" +
			"The device gets the same address every time it joins, and the API publishes " +
			"`<device>.<tenant zone>` for it. The device must be a `deevnet_iot_device` with a " +
			"`mac`: the address is reserved for that MAC.\n\n" +
			"The network is shared by every tenant, so the API picks the address: the lowest " +
			"free one in the range set aside for tenants. This state remembers it, and a rebuilt " +
			"API is asked for the same one again.\n\n" +
			"**It is addressing, not authorization, and it changes nothing about what can reach " +
			"the device.**",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant this address reservation belongs to: the `name` of a `deevnet_tenant`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"device": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The `name` of the `deevnet_iot_device` to reserve for. " +
					"Reference the resource rather than repeating the name, so the device is " +
					"registered first and deregistered last.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"address": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The address. Leave it out and the API picks. Set it to ask " +
					"for a particular one in the tenant range; the apply fails if it is taken. " +
					"Changing it replaces the reservation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"trust_class": computedString("The device's trust class, which decides the network the address is on."),
			"mac":         computedString("The MAC the address is reserved for: the device's, as registered."),
			"fqdn":        computedString("The name the API published for the device, in the tenant's zone."),
			"status":      computedString("`ready` once the reservation is on the DHCP server and the name is published."),
			"present": schema.BoolAttribute{
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the API still holds this address.",
			},
		},
	}
}

func (r *iotAddressResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// ModifyPlan turns "the API no longer has this address", or "it never finished
// reserving it", into a diff, the way the Wi-Fi key resource does.
//
// address is NOT blanked, for the reason the Wi-Fi key's psk is not: the value
// in state is what the device, and anything that dials it, already uses.
// Leaving it known carries it into the apply, the POST asks for it, and the
// device stays where it was. Blanking it would let the API hand out whichever
// address is lowest at that moment, which after a lost registry depends on the
// order tenants happen to re-apply.
func (r *iotAddressResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var state iotAddressModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !addressNeedsApply(state) {
		return
	}
	var plan iotAddressModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Present = types.BoolUnknown()
	plan.Status = types.StringUnknown()
	plan.MAC = types.StringUnknown()
	plan.FQDN = types.StringUnknown()
	plan.TrustClass = types.StringUnknown()
	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}

// addressNeedsApply is true when the address in state is not, as far as the
// API last said, fully in place: gone from the registry, or recorded there but
// not yet on the DHCP server.
func addressNeedsApply(state iotAddressModel) bool {
	if state.Present.Equal(types.BoolValue(false)) {
		return true
	}
	return !state.Status.IsNull() && !state.Status.IsUnknown() && state.Status.ValueString() != "ready"
}

func (r *iotAddressResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan iotAddressModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *iotAddressResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan iotAddressModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

// Read never removes the resource from state: this state is what remembers the
// address, so dropping it would let a restore hand the device a different one.
// It records that the API no longer has it, and ModifyPlan turns that into an
// apply that asks for the same address again.
func (r *iotAddressResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state iotAddressModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.client.GetDeviceAddress(ctx, state.Tenant.ValueString(), state.Device.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		state.Present = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the device address failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, addressFrom(a))...)
}

func (r *iotAddressResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state iotAddressModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteDeviceAddress(ctx, state.Tenant.ValueString(), state.Device.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Giving the device address back failed", err.Error())
	}
}

func (r *iotAddressResource) put(ctx context.Context, m iotAddressModel, state stateSetter) diagList {
	var diags diagList
	var req client.PutDeviceAddressRequest
	// Ask for the address already held, or the one the tenant named. Unknown
	// on a first apply with none named, which is when the API picks.
	if !m.Address.IsUnknown() && !m.Address.IsNull() {
		req.Address = m.Address.ValueString()
	}
	a, err := r.client.PutDeviceAddress(ctx, m.Tenant.ValueString(), m.Device.ValueString(), req)
	if err != nil {
		diags.AddError("Reserving the device address failed", err.Error())
		// A failure that still carried the address is the router or the name
		// having failed after the API recorded it. Keep it, so the next apply
		// asks for the same one; its status is not "ready", so ModifyPlan plans
		// the retry rather than leaving it looking settled.
		if a.Address != "" {
			diags.Append(state.Set(ctx, addressFrom(a))...)
		}
		return diags
	}
	return state.Set(ctx, addressFrom(a))
}

func addressFrom(a client.DeviceAddress) iotAddressModel {
	return iotAddressModel{
		Tenant:     types.StringValue(a.Tenant),
		Device:     types.StringValue(a.Device),
		Address:    types.StringValue(a.Address),
		TrustClass: types.StringValue(a.TrustClass),
		MAC:        types.StringValue(a.MAC),
		FQDN:       types.StringValue(a.FQDN),
		Status:     types.StringValue(a.Status),
		Present:    types.BoolValue(true),
	}
}
