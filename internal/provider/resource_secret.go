package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// One of the tenant's own runtime secrets, written to its OpenBao namespace
// (ADR-0021, CHG-0067).
//
// The value is write-only: it is read from the configuration on each apply
// that writes it and never enters a plan or state. The tenant's configuration
// - encrypted in its own repository - is the authoritative copy, and OpenBao
// holds a runtime copy. So a secret OpenBao no longer holds, which is what a
// rebuild leaves, is removed from state on refresh, and the next apply
// writes it again from the configuration. That is the resupply INC-0003
// needed, and it is why this resource, unlike the API-issued ones, forgets
// rather than marking itself absent: there is nothing here to restore from
// state.
type secretResource struct {
	client *client.Client
}

func NewSecretResource() resource.Resource { return &secretResource{} }

type secretModel struct {
	ID            types.String `tfsdk:"id"`
	Tenant        types.String `tfsdk:"tenant"`
	Path          types.String `tfsdk:"path"`
	DataWO        types.Map    `tfsdk:"data_wo"`
	DataWOVersion types.Int64  `tfsdk:"data_wo_version"`
}

var (
	secretSegment = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	readerName    = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
)

// validSecretPath is the API's rule for a secret's path, checked here so a
// plan fails rather than an apply.
func validSecretPath(p string) error {
	if len(p) == 0 || len(p) > 128 {
		return fmt.Errorf("a path is 1-128 characters; %q is %d", p, len(p))
	}
	segs := strings.Split(p, "/")
	if len(segs) > 4 {
		return fmt.Errorf("a path has at most four segments; %q has %d", p, len(segs))
	}
	for _, s := range segs {
		if !secretSegment.MatchString(s) {
			return fmt.Errorf("each segment of a path is 1-64 of a-z, 0-9, _ and -; %q in %q is not", s, p)
		}
	}
	return nil
}

func (r *secretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *secretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One of your own runtime secrets, such as a key a third-party API gave you, " +
			"written to your OpenBao namespace for your workloads to read (ADR-0021). The service is optional: " +
			"a site that does not offer it answers with an error saying so. Needs API v0.22.0.\n\n" +
			"**The value never enters your plan or state.** `data_wo` is a write-only argument, which needs " +
			"Terraform 1.11 or later. Feed it from a sensitive variable or `sensitive(...)`, and keep it encrypted in " +
			"your own repository: that copy is the authoritative one, and OpenBao holds a runtime copy.\n\n" +
			"**A new value needs a new version.** Terraform cannot see a write-only value change, so changing `data_wo` " +
			"alone does nothing. Raise `data_wo_version` with it, and the apply writes the new value.\n\n" +
			"**A lost secret is written again.** When OpenBao no longer holds the secret, as after a rebuild, the next " +
			"plan shows it being created, and the apply writes it from your configuration. The same happens when the " +
			"version OpenBao holds is not the one in your configuration.\n\n" +
			"A workload reads it through a `deevnet_secret_reader`. This resource cannot be imported: there is no way to " +
			"import a value that is never read back.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The secret's path.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant this secret belongs to: the `name` of a `deevnet_tenant`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"path": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Where the secret lives in your namespace, such as `weather/api`: one to four segments " +
					"joined by `/`, each 1-64 of `a-z`, `0-9`, `_` and `-`, and 128 characters at most.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"data_wo": schema.MapAttribute{
				ElementType: types.StringType,
				Required:    true,
				WriteOnly:   true,
				Sensitive:   true,
				MarkdownDescription: "The secret's value, as keys and string values, such as `{ api_key = var.weather_key }`. " +
					"Write-only: it is sent when the secret is written and never stored in plan or state. " +
					"Needs Terraform 1.11 or later. Changing it alone does nothing: raise `data_wo_version` with it.",
			},
			"data_wo_version": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "Your own counter for `data_wo`, from 1. Raising it writes the value again, which is " +
					"how a new value reaches OpenBao.",
			},
		},
	}
}

func (r *secretResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var p types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("path"), &p)...)
	if !p.IsNull() && !p.IsUnknown() {
		if err := validSecretPath(p.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("path"), "Invalid secret path", err.Error())
		}
	}
	var v types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("data_wo_version"), &v)...)
	if !v.IsNull() && !v.IsUnknown() && v.ValueInt64() < 1 {
		resp.Diagnostics.AddAttributeError(path.Root("data_wo_version"), "Invalid version",
			fmt.Sprintf("data_wo_version starts at 1; got %d", v.ValueInt64()))
	}
}

func (r *secretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *secretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config secretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, config.DataWO, &resp.State)...)
}

func (r *secretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config secretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, config.DataWO, &resp.State)...)
}

func (r *secretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state secretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetSecret(ctx, state.Tenant.ValueString(), state.Path.ValueString())
	if errors.Is(err, client.ErrNotFound) || (err == nil && !s.Present) {
		// OpenBao does not hold it. The configuration does, so forgetting it
		// is what makes the next apply write it again.
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading the secret failed", err.Error())
		return
	}
	next, changed := secretFrom(state, s)
	if changed {
		resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
	}
}

func (r *secretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state secretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteSecret(ctx, state.Tenant.ValueString(), state.Path.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting the secret failed", err.Error())
	}
}

func (r *secretResource) put(ctx context.Context, m secretModel, data types.Map, state stateSetter) diagList {
	var diags diagList
	if data.IsNull() || data.IsUnknown() {
		// Unknown here is a value Terraform cannot have yet; write-only
		// arguments must be known at apply.
		diags.AddAttributeError(path.Root("data_wo"), "No secret value",
			"data_wo has no value at apply. Write-only arguments need Terraform 1.11 or later, and a value known at apply.")
		return diags
	}
	values := map[string]string{}
	diags.Append(data.ElementsAs(ctx, &values, false)...)
	if diags.HasError() {
		return diags
	}
	_, err := r.client.PutSecret(ctx, m.Tenant.ValueString(), m.Path.ValueString(),
		client.PutSecretRequest{Data: values, Version: m.DataWOVersion.ValueInt64()})
	if err != nil {
		diags.AddError("Writing the secret failed", err.Error())
		return diags
	}
	// State records what was configured. A write-only attribute is always
	// null in state, whatever the plan carried.
	diags.Append(state.Set(ctx, secretModel{
		ID:            m.Path,
		Tenant:        m.Tenant,
		Path:          m.Path,
		DataWO:        types.MapNull(types.StringType),
		DataWOVersion: m.DataWOVersion,
	})...)
	return diags
}

// secretFrom folds a read into state. The version is the one thing a read
// can say, and it is taken only when it differs: then a plan shows the
// configured version against the held one, and the apply writes the value
// again.
func secretFrom(prior secretModel, s client.Secret) (secretModel, bool) {
	if prior.DataWOVersion.ValueInt64() == s.Version {
		return prior, false
	}
	next := prior
	next.DataWOVersion = types.Int64Value(s.Version)
	return next, true
}
