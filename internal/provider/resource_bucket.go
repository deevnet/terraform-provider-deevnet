package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// One of the tenant's buckets in the site's bucket store (ADR-0026, CHG-0069).
//
// Creating a bucket also creates its "default" key, with write access, and the
// create's answer is the only time the API returns that key's secret. State
// therefore holds the only copy the tenant has, and a read never blanks it.
//
// A bucket the API no longer holds is not forgotten - CLAUDE.md's rule for a
// secret-bearing resource - but marked present = false, and the next plan
// replaces it. The contract has no way to restore a bucket with the key it had,
// so a replacement (with a new key) is the honest answer, and showing it as
// one says so.
type bucketResource struct{ client *client.Client }

func NewBucketResource() resource.Resource { return &bucketResource{} }

type bucketModel struct {
	Tenant          types.String `tfsdk:"tenant"`
	Name            types.String `tfsdk:"name"`
	QuotaGB         types.Int64  `tfsdk:"quota_gb"`
	Versioning      types.Bool   `tfsdk:"versioning"`
	ObjectLifecycle types.Object `tfsdk:"object_lifecycle"`
	PublicRead      types.Bool   `tfsdk:"public_read"`
	ForceDestroy    types.Bool   `tfsdk:"force_destroy"`

	Bucket    types.String `tfsdk:"bucket"`
	Endpoint  types.String `tfsdk:"endpoint"`
	Status    types.String `tfsdk:"status"`
	AccessKey types.String `tfsdk:"access_key"`
	SecretKey types.String `tfsdk:"secret_key"`
	Present   types.Bool   `tfsdk:"present"`
}

type bucketLifecycleModel struct {
	ExpireDays           types.Int64 `tfsdk:"expire_days"`
	NoncurrentExpireDays types.Int64 `tfsdk:"noncurrent_expire_days"`
}

var bucketLifecycleAttrTypes = map[string]attr.Type{
	"expire_days":            types.Int64Type,
	"noncurrent_expire_days": types.Int64Type,
}

var (
	// The contract's patterns (CHG-0069).
	bucketName    = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)
	bucketKeyName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}$`)
)

// maxStoreBucket is S3's limit on a bucket's name, which "<tenant>-<name>" must
// fit.
const maxStoreBucket = 63

func validBucketName(tenant, name string) error {
	if !bucketName.MatchString(name) {
		return fmt.Errorf("a bucket's name is 3-32 characters: a-z first, then a-z, 0-9 and -, ending in a-z or 0-9; %q is not", name)
	}
	if tenant != "" && len(tenant)+1+len(name) > maxStoreBucket {
		return fmt.Errorf("the store's bucket %q is %d characters; it must be %d or fewer",
			tenant+"-"+name, len(tenant)+1+len(name), maxStoreBucket)
	}
	return nil
}

func (r *bucketResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bucket"
}

func (r *bucketResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A bucket in your site's bucket store: S3-compatible object storage, reached at `endpoint` " +
			"path-style (ADR-0026). Needs API v0.23.0, at a site that offers buckets; a site that does not answers " +
			"with an error saying so.\n\n" +
			"**Creating a bucket also creates its `default` key**, with write access, in `access_key` and `secret_key`. " +
			"The API returns the secret only when it creates the key, so **this state is the only copy you can read**. " +
			"Use `deevnet_bucket_key` for further keys, such as a read-only one.\n\n" +
			"**The substrate stores your bucket's contents and does not back them up.** Keep your own copy of anything " +
			"you cannot re-derive.\n\n" +
			"**The quota is enforced about a minute behind.** The store checks usage periodically, so writes can run " +
			"past `quota_gb` briefly before they are refused. A single-part upload larger than the quota is refused at once, " +
			"but a multipart upload, which is how most clients send a large object, is counted only after it completes, " +
			"so one large object can take the bucket past its quota. Writes after it are then refused. " +
			"The site has a fixed capacity, and quotas are allocated from it, never overcommitted: a create or a quota " +
			"increase that would exceed it is refused with a message saying how much is left.\n\n" +
			"**`public_read` serves objects to the site network, never the internet:** anything on the site network " +
			"that can reach the store can read them by name, without a key.\n\n" +
			"**Destroying a bucket that holds anything is refused** unless `force_destroy` is true, and that must be in " +
			"state before the destroy: set it and apply first.\n\n" +
			"When the API no longer holds the bucket, the next plan replaces it, with a new `default` key. This resource " +
			"cannot be imported: an import could never recover the key's secret.",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant this bucket belongs to: the `name` of a `deevnet_tenant`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The bucket's name, unique within the tenant: 3-32 characters, `a-z` first, then " +
					"`a-z`, `0-9` and `-`, ending in a letter or digit. The store's bucket is `<tenant>-<name>`, which " +
					"must be 63 characters or fewer. Changing it replaces the bucket.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"quota_gb": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "The bucket's quota, in GB, at least 1. Changes in place. Enforced about a minute " +
					"behind; a multipart upload is counted only after it completes.",
			},
			"versioning": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Keep every version of each object. Defaults to `false`. Setting it back to `false` " +
					"suspends versioning: the versions already kept stay, because S3 cannot un-version a bucket.",
			},
			"public_read": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Serve the bucket's objects without a key, by name: anonymous `GetObject` and " +
					"nothing else, so not a listing. Defaults to `false`. **Public here means the site network:** " +
					"anything on it that can reach the store can read the objects. The store is never reachable from " +
					"the internet.",
			},
			"force_destroy": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Let a destroy delete everything in the bucket, every version included. Defaults " +
					"to `false`, and then a destroy of a bucket that holds anything is refused. Kept by the provider, " +
					"not the API: it takes effect from state, so apply it before the destroy.",
			},
			"bucket":   computedString("The store's name for the bucket, `<tenant>-<name>`: what an S3 client calls it."),
			"endpoint": computedString("The bucket store's S3 endpoint, such as `https://s3.mobile.deevnet.net`. Use path-style addressing."),
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`ready` once the bucket is in the store; `provisioning` or `deleting` while the API is working on it.",
			},
			"access_key": computedString("The `default` key's access key ID."),
			"secret_key": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				MarkdownDescription: "The `default` key's secret access key. The API returns it only when it creates " +
					"the key, so **this state is the only copy you can read**.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"present": schema.BoolAttribute{
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the API still holds this bucket. When it does not, the next plan replaces it.",
			},
		},
		Blocks: map[string]schema.Block{
			// Not "lifecycle": Terraform reserves that name in every resource
			// block for its own meta-argument.
			"object_lifecycle": schema.SingleNestedBlock{
				MarkdownDescription: "When the store expires objects by itself. Omit it, or a value in it, for none.",
				Attributes: map[string]schema.Attribute{
					"expire_days": schema.Int64Attribute{
						Optional: true,
						MarkdownDescription: "Expire each current object this many days after it was written, at " +
							"least 1. In a versioned bucket, the object becomes a noncurrent version.",
					},
					"noncurrent_expire_days": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Delete each noncurrent version this many days after it stopped being current, at least 1.",
					},
				},
			},
		},
	}
}

func (r *bucketResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m bucketModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.Name.IsNull() && !m.Name.IsUnknown() {
		tenant := ""
		if !m.Tenant.IsUnknown() {
			tenant = m.Tenant.ValueString()
		}
		if err := validBucketName(tenant, m.Name.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid bucket name", err.Error())
		}
	}
	if !m.QuotaGB.IsNull() && !m.QuotaGB.IsUnknown() && m.QuotaGB.ValueInt64() < 1 {
		resp.Diagnostics.AddAttributeError(path.Root("quota_gb"), "Invalid quota",
			fmt.Sprintf("quota_gb is at least 1; got %d", m.QuotaGB.ValueInt64()))
	}
	if m.ObjectLifecycle.IsNull() || m.ObjectLifecycle.IsUnknown() {
		return
	}
	var lc bucketLifecycleModel
	resp.Diagnostics.Append(m.ObjectLifecycle.As(ctx, &lc, objectAsOptions)...)
	for name, v := range map[string]types.Int64{"expire_days": lc.ExpireDays, "noncurrent_expire_days": lc.NoncurrentExpireDays} {
		if !v.IsNull() && !v.IsUnknown() && v.ValueInt64() < 1 {
			resp.Diagnostics.AddAttributeError(path.Root("object_lifecycle").AtName(name), "Invalid expiry",
				fmt.Sprintf("%s is at least 1, or omitted for none; got %d", name, v.ValueInt64()))
		}
	}
}

func (r *bucketResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// ModifyPlan replaces a bucket the API no longer holds. present's default puts
// true in the plan against false in state, so naming it here is a replacement.
func (r *bucketResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var present types.Bool
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("present"), &present)...)
	if present.Equal(types.BoolValue(false)) {
		resp.RequiresReplace = append(resp.RequiresReplace, path.Root("present"))
	}
}

func (r *bucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan bucketModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	settings, diags := bucketSettings(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.CreateBucket(ctx, plan.Tenant.ValueString(),
		client.CreateBucketRequest{Name: plan.Name.ValueString(), BucketSettings: settings})
	if err != nil {
		resp.Diagnostics.AddError("Creating the bucket failed", err.Error())
		return
	}
	next, diags := bucketFrom(ctx, plan, b)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *bucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state bucketModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	settings, diags := bucketSettings(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.UpdateBucket(ctx, plan.Tenant.ValueString(), plan.Name.ValueString(), settings)
	if err != nil {
		resp.Diagnostics.AddError("Updating the bucket failed", err.Error())
		return
	}
	// An update returns no key, so the default key's in state is kept.
	plan.AccessKey = state.AccessKey
	plan.SecretKey = state.SecretKey
	next, diags := bucketFrom(ctx, plan, b)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

// Read never removes the bucket from state: it holds the only readable copy of
// the default key's secret. A bucket the API lost is marked absent, and
// ModifyPlan replaces it.
func (r *bucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state bucketModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := r.client.GetBucket(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		state.Present = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the bucket failed", err.Error())
		return
	}
	next, diags := bucketFrom(ctx, state, b)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *bucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state bucketModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteBucket(ctx, state.Tenant.ValueString(), state.Name.ValueString(), state.ForceDestroy.ValueBool())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting the bucket failed", err.Error())
	}
}

// objectAsOptions reads an omitted value inside the block as null.
var objectAsOptions = basetypes.ObjectAsOptions{}

// bucketSettings is what the configuration asks the API for. An omitted
// lifecycle, or an omitted value in it, is 0: none.
func bucketSettings(ctx context.Context, m bucketModel) (client.BucketSettings, diagList) {
	var diags diagList
	s := client.BucketSettings{
		QuotaGB:    m.QuotaGB.ValueInt64(),
		Versioning: m.Versioning.ValueBool(),
		PublicRead: m.PublicRead.ValueBool(),
	}
	if !m.ObjectLifecycle.IsNull() && !m.ObjectLifecycle.IsUnknown() {
		var lc bucketLifecycleModel
		diags.Append(m.ObjectLifecycle.As(ctx, &lc, objectAsOptions)...)
		s.Lifecycle.ExpireDays = lc.ExpireDays.ValueInt64()
		s.Lifecycle.NoncurrentExpireDays = lc.NoncurrentExpireDays.ValueInt64()
	}
	return s, diags
}

// bucketFrom folds the API's view into the model.
//
// The key comes only from a create's answer; otherwise the one in prior is
// kept, which is what keeps a read from wiping the secret. force_destroy is the
// provider's own and is always prior's.
//
// The lifecycle keeps the configuration's shape: 0 from the API is "none",
// which the configuration writes by omitting the block or the value, so a 0
// over a null stays null. Anything else the API reports is taken, so a
// lifecycle changed in the store shows as a diff.
func bucketFrom(ctx context.Context, prior bucketModel, b client.Bucket) (bucketModel, diagList) {
	var diags diagList
	next := prior
	next.Name = types.StringValue(b.Name)
	next.QuotaGB = types.Int64Value(b.QuotaGB)
	next.Versioning = types.BoolValue(b.Versioning)
	next.PublicRead = types.BoolValue(b.PublicRead)
	next.Bucket = types.StringValue(b.Bucket)
	next.Endpoint = types.StringValue(b.Endpoint)
	next.Status = types.StringValue(b.Status)
	next.Present = types.BoolValue(true)
	if next.ForceDestroy.IsNull() || next.ForceDestroy.IsUnknown() {
		next.ForceDestroy = types.BoolValue(false)
	}

	if b.Key != nil {
		next.AccessKey = types.StringValue(b.Key.AccessKey)
		next.SecretKey = types.StringValue(b.Key.SecretKey)
	}
	// Never leave an unknown in state.
	if next.AccessKey.IsUnknown() {
		next.AccessKey = types.StringNull()
	}
	if next.SecretKey.IsUnknown() {
		next.SecretKey = types.StringNull()
	}

	var priorLC bucketLifecycleModel
	if !prior.ObjectLifecycle.IsNull() && !prior.ObjectLifecycle.IsUnknown() {
		diags.Append(prior.ObjectLifecycle.As(ctx, &priorLC, objectAsOptions)...)
	} else {
		priorLC = bucketLifecycleModel{ExpireDays: types.Int64Null(), NoncurrentExpireDays: types.Int64Null()}
	}
	lc := bucketLifecycleModel{
		ExpireDays:           daysFrom(b.Lifecycle.ExpireDays, priorLC.ExpireDays),
		NoncurrentExpireDays: daysFrom(b.Lifecycle.NoncurrentExpireDays, priorLC.NoncurrentExpireDays),
	}
	blockWasNull := prior.ObjectLifecycle.IsNull() || prior.ObjectLifecycle.IsUnknown()
	if blockWasNull && lc.ExpireDays.IsNull() && lc.NoncurrentExpireDays.IsNull() {
		next.ObjectLifecycle = types.ObjectNull(bucketLifecycleAttrTypes)
		return next, diags
	}
	obj, d := types.ObjectValueFrom(ctx, bucketLifecycleAttrTypes, lc)
	diags.Append(d...)
	next.ObjectLifecycle = obj
	return next, diags
}

// daysFrom renders one expiry: 0 is none, which stays null when the
// configuration left it out.
func daysFrom(api int64, prior types.Int64) types.Int64 {
	if api == 0 && (prior.IsNull() || prior.IsUnknown()) {
		return types.Int64Null()
	}
	return types.Int64Value(api)
}
