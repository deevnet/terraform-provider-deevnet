package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// An AppRole in the tenant's OpenBao namespace that one workload logs in with
// to read the secrets it was given (ADR-0021, CHG-0067). The tenant pushes
// role_id and secret_id with its code (ADR-0034).
//
// Every write issues a new secret_id and revokes the old one, so secret_id is
// unknown in any plan that changes the reader. A reader OpenBao no longer
// holds is removed from state on refresh, and the next apply issues it
// again: the old secret_id is dead with the role, so there is nothing in
// state worth restoring.
type secretReaderResource struct {
	client *client.Client
}

func NewSecretReaderResource() resource.Resource { return &secretReaderResource{} }

type secretReaderModel struct {
	Tenant   types.String `tfsdk:"tenant"`
	Name     types.String `tfsdk:"name"`
	Workload types.String `tfsdk:"workload"`
	Paths    types.List   `tfsdk:"paths"`

	RoleID    types.String `tfsdk:"role_id"`
	SecretID  types.String `tfsdk:"secret_id"`
	Namespace types.String `tfsdk:"namespace"`
	Addr      types.String `tfsdk:"addr"`
}

func (r *secretReaderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret_reader"
}

func (r *secretReaderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A credential one of your workloads reads its secrets with: an AppRole in your OpenBao " +
			"namespace that may read the `deevnet_secret` paths you list, and nothing else (ADR-0021). Needs API v0.22.0.\n\n" +
			"**Push `role_id` and `secret_id` with your code** (ADR-0034), and run OpenBao Agent on the workload with " +
			"AppRole auto-auth against `addr`, in `namespace`. The agent logs in, renders your secrets where your service " +
			"reads them, and renews its own token.\n\n" +
			"**The credential works only from the workload's own address.** A copy taken anywhere else cannot log in.\n\n" +
			"**Any change issues a new `secret_id`** and revokes the old one, so push again after an apply that changes " +
			"this resource. When OpenBao no longer holds the reader, as after a rebuild, the next plan shows it being " +
			"created, and the apply issues a new credential to push.",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant this reader belongs to: the `name` of a `deevnet_tenant`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The reader's name: 1-32 of `a-z`, `0-9` and `-`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"workload": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The workload that reads with it: the `name` of a `deevnet_workload`. " +
					"The credential works only from that workload's address.",
			},
			"paths": schema.ListAttribute{
				ElementType: types.StringType,
				Required:    true,
				MarkdownDescription: "The secret paths it may read: the `path` of each `deevnet_secret`, " +
					"1 to 32 of them.",
			},
			"role_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The AppRole's role ID, which the agent logs in with beside `secret_id`.",
			},
			"secret_id": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
				MarkdownDescription: "The AppRole's secret ID. A new one is issued on every change to this resource, " +
					"and the old one stops working.",
			},
			"namespace": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Your OpenBao namespace, where the agent logs in and reads.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"addr": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "OpenBao's address, for the agent's `vault.address`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *secretReaderResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var name types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("name"), &name)...)
	if !name.IsNull() && !name.IsUnknown() && !readerName.MatchString(name.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid reader name",
			fmt.Sprintf("a reader's name is 1-32 of a-z, 0-9 and -; %q is not", name.ValueString()))
	}
	var paths types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("paths"), &paths)...)
	if paths.IsNull() || paths.IsUnknown() {
		return
	}
	elems := paths.Elements()
	if n := len(elems); n < 1 || n > 32 {
		resp.Diagnostics.AddAttributeError(path.Root("paths"), "Invalid paths",
			fmt.Sprintf("a reader has 1 to 32 paths; got %d", n))
	}
	for i, e := range elems {
		s, ok := e.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		if err := validSecretPath(s.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("paths").AtListIndex(i), "Invalid secret path", err.Error())
		}
	}
}

func (r *secretReaderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *secretReaderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan secretReaderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *secretReaderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan secretReaderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *secretReaderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state secretReaderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rd, err := r.client.GetSecretReader(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		// OpenBao lost the role, and its secret_id with it. Forgetting the
		// reader is what makes the next apply issue a new one.
		resp.State.RemoveResource(ctx)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the secret reader failed", err.Error())
		return
	}
	next, diags := secretReaderFrom(ctx, state, rd)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, next)...)
}

func (r *secretReaderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state secretReaderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteSecretReader(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting the secret reader failed", err.Error())
	}
}

func (r *secretReaderResource) put(ctx context.Context, m secretReaderModel, state stateSetter) diagList {
	paths, diags := stringsFrom(ctx, m.Paths)
	if diags.HasError() {
		return diags
	}
	rd, err := r.client.PutSecretReader(ctx, m.Tenant.ValueString(), m.Name.ValueString(),
		client.PutSecretReaderRequest{Workload: m.Workload.ValueString(), Paths: paths})
	if err != nil {
		diags.AddError("Issuing the secret reader failed", err.Error())
		return diags
	}
	next, more := secretReaderFrom(ctx, m, rd)
	diags.Append(more...)
	if diags.HasError() {
		return diags
	}
	diags.Append(state.Set(ctx, next)...)
	return diags
}

// secretReaderFrom folds the API's answer into the model. A read carries no
// secret_id, so the one in state is kept; a write carries the new one. The
// paths keep the configuration's order when the API holds the same set, so a
// reordered answer is not a diff.
func secretReaderFrom(ctx context.Context, prior secretReaderModel, rd client.SecretReader) (secretReaderModel, diagList) {
	next := prior
	next.Workload = types.StringValue(rd.Workload)
	next.RoleID = types.StringValue(rd.RoleID)
	next.Namespace = types.StringValue(rd.Namespace)
	next.Addr = types.StringValue(rd.Addr)
	switch {
	case rd.SecretID != "":
		next.SecretID = types.StringValue(rd.SecretID)
	case prior.SecretID.IsUnknown():
		// Never leave an unknown in state.
		next.SecretID = types.StringNull()
	}

	var diags diagList
	priorPaths, more := stringsFrom(ctx, prior.Paths)
	diags.Append(more...)
	if !sameSet(priorPaths, rd.Paths) {
		apiPaths := rd.Paths
		if apiPaths == nil {
			apiPaths = []string{}
		}
		next.Paths, more = types.ListValueFrom(ctx, types.StringType, apiPaths)
		diags.Append(more...)
	}
	return next, diags
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
