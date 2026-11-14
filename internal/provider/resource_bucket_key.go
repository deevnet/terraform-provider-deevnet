package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// A further access key to one of the tenant's buckets (ADR-0026, CHG-0069).
// Nothing about a key changes in place: the API has no update, so every
// argument replaces it. The secret comes only in the create's answer, and a
// key the API lost is marked absent and replaced, as deevnet_bucket is.
type bucketKeyResource struct{ client *client.Client }

func NewBucketKeyResource() resource.Resource { return &bucketKeyResource{} }

type bucketKeyModel struct {
	Tenant    types.String `tfsdk:"tenant"`
	Bucket    types.String `tfsdk:"bucket"`
	Name      types.String `tfsdk:"name"`
	Access    types.String `tfsdk:"access"`
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
	Present   types.Bool   `tfsdk:"present"`
}

func (r *bucketKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bucket_key"
}

func (r *bucketKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "A further access key to one of your buckets, beside the `default` key every " +
			"`deevnet_bucket` has. The key reaches its one bucket and nothing else. Needs API v0.23.0.\n\n" +
			"Give each thing that uses a bucket its own key, and give a reader a `read` key, so revoking one " +
			"touches nothing else. Destroying the resource revokes the key.\n\n" +
			"**The API returns `secret_key` only when it creates the key**, so this state is the only copy you can " +
			"read. Any change replaces the key, with a new secret. When the API no longer holds the key, the next " +
			"plan replaces it. This resource cannot be imported: an import could never recover the secret.",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant the bucket belongs to: the `name` of a `deevnet_tenant`.",
				PlanModifiers:       replace,
			},
			"bucket": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The bucket this key reaches: the `name` of a `deevnet_bucket`, not its " +
					"`<tenant>-<name>` store name.",
				PlanModifiers: replace,
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The key's name, unique within the bucket: 1-20 characters, `a-z` first, then " +
					"`a-z`, `0-9` and `-`. `default` is the bucket's own key.",
				PlanModifiers: replace,
			},
			"access": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "`read` to list and get objects; `write` to list, get, put and delete objects and " +
					"their versions.",
				PlanModifiers: replace,
			},
			"access_key": computedString("The key's access key ID."),
			"secret_key": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				MarkdownDescription: "The key's secret access key. The API returns it only when it creates the key, " +
					"so **this state is the only copy you can read**.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"present": schema.BoolAttribute{
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the API still holds this key. When it does not, the next plan replaces it.",
			},
		},
	}
}

func (r *bucketKeyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m bucketKeyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Bucket.IsNull() && !m.Bucket.IsUnknown() && !bucketName.MatchString(m.Bucket.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("bucket"), "Invalid bucket name",
			fmt.Sprintf("a bucket's name is 3-32 characters: a-z first, then a-z, 0-9 and -, ending in a-z or 0-9; %q is not", m.Bucket.ValueString()))
	}
	if !m.Name.IsNull() && !m.Name.IsUnknown() && !bucketKeyName.MatchString(m.Name.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid key name",
			fmt.Sprintf("a key's name is 1-20 characters: a-z first, then a-z, 0-9 and -; %q is not", m.Name.ValueString()))
	}
	if !m.Access.IsNull() && !m.Access.IsUnknown() {
		if a := m.Access.ValueString(); a != "read" && a != "write" {
			resp.Diagnostics.AddAttributeError(path.Root("access"), "Invalid access",
				fmt.Sprintf("access is read or write; got %q", a))
		}
	}
}

func (r *bucketKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// ModifyPlan replaces a key the API no longer holds, as deevnet_bucket does.
func (r *bucketKeyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var present types.Bool
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("present"), &present)...)
	if present.Equal(types.BoolValue(false)) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("present"))
	}
}

func (r *bucketKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan bucketKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	k, err := r.client.CreateBucketKey(ctx, plan.Tenant.ValueString(), plan.Bucket.ValueString(),
		client.CreateBucketKeyRequest{Name: plan.Name.ValueString(), Access: plan.Access.ValueString()})
	if err != nil {
		resp.Diagnostics.AddError("Creating the bucket key failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, bucketKeyFrom(plan, k))...)
}

// Update is never reached with a change the API could make - every argument
// replaces - but present flipping back is one, and it records the plan.
func (r *bucketKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan bucketKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read never removes the key from state: it holds the only readable copy of
// the secret.
func (r *bucketKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state bucketKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	k, err := r.client.GetBucketKey(ctx, state.Tenant.ValueString(), state.Bucket.ValueString(), state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		state.Present = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the bucket key failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, bucketKeyFrom(state, k))...)
}

func (r *bucketKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state bucketKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteBucketKey(ctx, state.Tenant.ValueString(), state.Bucket.ValueString(), state.Name.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Revoking the bucket key failed", err.Error())
	}
}

// bucketKeyFrom folds the API's answer into the model. A read carries no
// secret, so the one in prior is kept; access is taken, so a key whose access
// changed in the API shows as a replacement.
func bucketKeyFrom(prior bucketKeyModel, k client.BucketKey) bucketKeyModel {
	next := prior
	next.Name = types.StringValue(k.Name)
	next.Access = types.StringValue(k.Access)
	next.AccessKey = types.StringValue(k.AccessKey)
	next.Present = types.BoolValue(true)
	if k.SecretKey != "" {
		next.SecretKey = types.StringValue(k.SecretKey)
	}
	if next.SecretKey.IsUnknown() {
		next.SecretKey = types.StringNull()
	}
	return next
}
