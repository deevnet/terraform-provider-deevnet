package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/deevnet/terraform-provider-deevnet/internal/client"
)

// One of a tenant's alert rule groups (ADR-0023 §5, CHG-0071). The group is
// the resource and is replaced whole; its rules are blocks within it.
//
// It holds no secret, so a group the API no longer has is dropped from state
// and planned again, unlike the secret-bearing resources: there is nothing in
// state that a re-create would lose.
type alertRuleGroupResource struct{ client *client.Client }

func NewAlertRuleGroupResource() resource.Resource { return &alertRuleGroupResource{} }

type alertRuleModel struct {
	Alert       types.String `tfsdk:"alert"`
	Expr        types.String `tfsdk:"expr"`
	For         types.String `tfsdk:"for"`
	Labels      types.Map    `tfsdk:"labels"`
	Annotations types.Map    `tfsdk:"annotations"`
}

type alertRuleGroupModel struct {
	Tenant    types.String     `tfsdk:"tenant"`
	Name      types.String     `tfsdk:"name"`
	Type      types.String     `tfsdk:"type"`
	Interval  types.String     `tfsdk:"interval"`
	Topic     types.String     `tfsdk:"topic"`
	Partition types.String     `tfsdk:"partition"`
	NtfyTopic types.String     `tfsdk:"ntfy_topic"`
	Rules     []alertRuleModel `tfsdk:"rule"`
}

func (r *alertRuleGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_rule_group"
}

func (r *alertRuleGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A group of alert rules over the tenant's metrics or its logs.\n\n" +
			"The rules run in the tenant's own vmalert on the site, reading with the tenant's log read " +
			"token, so a rule sees exactly what the tenant's token sees. When a rule fires, and again " +
			"when it clears, a notification goes to the ntfy topic `<tenant>-<topic>`, which the tenant's " +
			"phone reads with `deevnet_tenant`'s `notify_username` and `notify_password`, on site.\n\n" +
			"The API has vmalert check every expression before it records the group, so a rule vmalert " +
			"will not load fails the apply with the parse error. Alerting rules only: there are no " +
			"recording rules. Requires API 0.25.0 and a site with alerting.",
		Attributes: map[string]schema.Attribute{
			"tenant": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The tenant this group belongs to: the `name` of a `deevnet_tenant`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "1-32 lowercase letters, digits or dashes, starting with a letter; unique within the tenant.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "`metrics`: each `expr` is MetricsQL (PromQL works) over the tenant's metrics. " +
					"`logs`: each `expr` is LogsQL with a `stats` pipe, such as " +
					"`\"door open\" | stats count() as hits | filter hits:>0`.",
			},
			"interval": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("1m"),
				MarkdownDescription: "How often the rules are evaluated, 30s to 1h. Default `1m`.",
			},
			"topic": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Where the group's alerts go: the ntfy topic `<tenant>-<topic>`. 1-32 lowercase letters, " +
					"digits or dashes. Several groups may share a topic.",
			},
			"partition": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("workloads"),
				MarkdownDescription: "Which of the tenant's partitions the rules read: `workloads` (default), what the " +
					"tenant's own workloads push; `platform`, what the site publishes about the tenant; or `devices`, " +
					"the devices' logs from MQTT, for a `logs` group only.",
			},
			"ntfy_topic": computedString("`<tenant>-<topic>`, the topic to subscribe to on the phone."),
		},
		Blocks: map[string]schema.Block{
			"rule": schema.ListNestedBlock{
				MarkdownDescription: "An alerting rule. 1 to 50 in a group, and 200 across a tenant's groups.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"alert": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The alert's name: letters, digits and underscores, unique in the group.",
						},
						"expr": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The expression. The rule fires for each series (or `stats` row) it returns.",
						},
						"for": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "How long the expression must hold before the alert fires, such as `5m`.",
						},
						"labels": schema.MapAttribute{
							Optional:    true,
							ElementType: types.StringType,
							MarkdownDescription: "Labels added to the alert, such as `severity`. `tenant`, `deevnet_topic` and " +
								"`alertname` are the platform's and are refused: the first two decide where an alert goes.",
						},
						"annotations": schema.MapAttribute{
							Optional:    true,
							ElementType: types.StringType,
							MarkdownDescription: "Text for the notification, such as `summary`. vmalert templates work: " +
								"`{{ $value }}`, `{{ $labels.host }}`.",
						},
					},
				},
			},
		},
	}
}

func (r *alertRuleGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *alertRuleGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertRuleGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *alertRuleGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertRuleGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.put(ctx, plan, &resp.State)...)
}

func (r *alertRuleGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertRuleGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	g, err := r.client.GetAlertRuleGroup(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	switch {
	case errors.Is(err, client.ErrNotFound):
		// Nothing secret is lost by planning it again.
		resp.State.RemoveResource(ctx)
		return
	case err != nil:
		resp.Diagnostics.AddError("Reading the alert rule group failed", err.Error())
		return
	}
	m, diags := alertRuleGroupFrom(ctx, g)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func (r *alertRuleGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertRuleGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteAlertRuleGroup(ctx, state.Tenant.ValueString(), state.Name.ValueString())
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting the alert rule group failed", err.Error())
	}
}

func (r *alertRuleGroupResource) put(ctx context.Context, m alertRuleGroupModel, state stateSetter) diag.Diagnostics {
	var diags diag.Diagnostics
	req := client.PutAlertRuleGroupRequest{
		Type:      m.Type.ValueString(),
		Interval:  m.Interval.ValueString(),
		Topic:     m.Topic.ValueString(),
		Partition: m.Partition.ValueString(),
	}
	for _, rm := range m.Rules {
		rule := client.AlertRule{Alert: rm.Alert.ValueString(), Expr: rm.Expr.ValueString(), For: rm.For.ValueString()}
		diags.Append(rm.Labels.ElementsAs(ctx, &rule.Labels, false)...)
		diags.Append(rm.Annotations.ElementsAs(ctx, &rule.Annotations, false)...)
		req.Rules = append(req.Rules, rule)
	}
	if diags.HasError() {
		return diags
	}
	g, err := r.client.PutAlertRuleGroup(ctx, m.Tenant.ValueString(), m.Name.ValueString(), req)
	if err != nil {
		diags.AddError("Writing the alert rule group failed", err.Error())
		return diags
	}
	out, d := alertRuleGroupFrom(ctx, g)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	diags.Append(state.Set(ctx, out)...)
	return diags
}

// alertRuleGroupFrom folds an API answer into the model. An absent `for`, and
// empty labels or annotations, are null rather than empty, so a rule written
// without them does not show a perpetual diff.
func alertRuleGroupFrom(ctx context.Context, g client.AlertRuleGroup) (alertRuleGroupModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	m := alertRuleGroupModel{
		Tenant:    types.StringValue(g.Tenant),
		Name:      types.StringValue(g.Name),
		Type:      types.StringValue(g.Type),
		Interval:  types.StringValue(g.Interval),
		Topic:     types.StringValue(g.Topic),
		Partition: types.StringValue(g.Partition),
		NtfyTopic: types.StringValue(g.NtfyTopic),
	}
	for _, r := range g.Rules {
		rm := alertRuleModel{
			Alert:       types.StringValue(r.Alert),
			Expr:        types.StringValue(r.Expr),
			For:         types.StringNull(),
			Labels:      types.MapNull(types.StringType),
			Annotations: types.MapNull(types.StringType),
		}
		if r.For != "" {
			rm.For = types.StringValue(r.For)
		}
		if len(r.Labels) > 0 {
			v, d := types.MapValueFrom(ctx, types.StringType, r.Labels)
			diags.Append(d...)
			rm.Labels = v
		}
		if len(r.Annotations) > 0 {
			v, d := types.MapValueFrom(ctx, types.StringType, r.Annotations)
			diags.Append(d...)
			rm.Annotations = v
		}
		m.Rules = append(m.Rules, rm)
	}
	return m, diags
}
