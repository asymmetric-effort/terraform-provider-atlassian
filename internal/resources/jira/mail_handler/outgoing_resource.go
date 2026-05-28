// Package mailhandler implements the atlassian_jira_outgoing_mail_handler managed resource.
package mailhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the OutgoingResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &OutgoingResource{}
	_ resource.ResourceWithImportState = &OutgoingResource{}
)

// apiOutgoingHandler represents the JSON structure returned by the Atlassian outgoing mail handler API.
type apiOutgoingHandler struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	FromAddress string `json:"fromAddress"`
	Prefix      string `json:"prefix,omitempty"`
	SMTPHost    string `json:"smtpHost"`
	SMTPPort    int64  `json:"smtpPort"`
	Protocol    string `json:"protocol,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	TLS         bool   `json:"tls"`
}

// OutgoingResourceModel describes the outgoing mail handler resource data model.
type OutgoingResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	FromAddress types.String `tfsdk:"from_address"`
	Prefix      types.String `tfsdk:"prefix"`
	SMTPHost    types.String `tfsdk:"smtp_host"`
	SMTPPort    types.Int64  `tfsdk:"smtp_port"`
	Protocol    types.String `tfsdk:"protocol"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	TLS         types.Bool   `tfsdk:"tls"`
}

// OutgoingResource implements the atlassian_jira_outgoing_mail_handler managed resource.
type OutgoingResource struct {
	client *atlassian.Client
}

// NewOutgoingResource returns a new OutgoingResource instance for provider registration.
func NewOutgoingResource() resource.Resource {
	return &OutgoingResource{}
}

// Metadata returns the resource type name.
func (r *OutgoingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_outgoing_mail_handler"
}

// Schema defines the schema for the outgoing mail handler resource.
func (r *OutgoingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira outgoing mail handler in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the outgoing mail handler, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the outgoing mail handler.",
				Required:    true,
			},
			"from_address": schema.StringAttribute{
				Description: "The email address used as the From address for outgoing mail.",
				Required:    true,
			},
			"prefix": schema.StringAttribute{
				Description: "The subject line prefix for outgoing mail.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"smtp_host": schema.StringAttribute{
				Description: "The SMTP server hostname.",
				Required:    true,
			},
			"smtp_port": schema.Int64Attribute{
				Description: "The SMTP server port number.",
				Required:    true,
			},
			"protocol": schema.StringAttribute{
				Description: "The mail protocol (e.g., \"SMTP\", \"SMTPS\").",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"username": schema.StringAttribute{
				Description: "The username for SMTP authentication.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"password": schema.StringAttribute{
				Description: "The password for SMTP authentication.",
				Optional:    true,
				Sensitive:   true,
			},
			"tls": schema.BoolAttribute{
				Description: "Whether to use TLS for the SMTP connection.",
				Optional:    true,
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *OutgoingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create provisions a new Jira outgoing mail handler.
func (r *OutgoingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan OutgoingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiOutgoingHandler{
		Name:        plan.Name.ValueString(),
		FromAddress: plan.FromAddress.ValueString(),
		SMTPHost:    plan.SMTPHost.ValueString(),
		SMTPPort:    plan.SMTPPort.ValueInt64(),
	}
	if !plan.Prefix.IsNull() && !plan.Prefix.IsUnknown() {
		body.Prefix = plan.Prefix.ValueString()
	}
	if !plan.Protocol.IsNull() && !plan.Protocol.IsUnknown() {
		body.Protocol = plan.Protocol.ValueString()
	}
	if !plan.Username.IsNull() && !plan.Username.IsUnknown() {
		body.Username = plan.Username.ValueString()
	}
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		body.Password = plan.Password.ValueString()
	}
	if !plan.TLS.IsNull() && !plan.TLS.IsUnknown() {
		body.TLS = plan.TLS.ValueBool()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiOutgoingHandler
	err := r.client.Post(ctx, "/rest/api/3/mailhandler/outgoing", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid outgoing mail handler configuration",
					fmt.Sprintf("The mail handler configuration is invalid. Verify the SMTP host, port, and from_address settings: %s", err.Error()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create outgoing mail handlers. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create outgoing mail handler",
			fmt.Sprintf("Could not create outgoing mail handler %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	r.mapAPIToState(&plan, &created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *OutgoingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state OutgoingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var handler apiOutgoingHandler
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/mailhandler/outgoing/%s", state.ID.ValueString()), &handler)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read outgoing mail handler",
			fmt.Sprintf("Could not read outgoing mail handler %q: %s. Verify the handler exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	// Preserve password from state since the API does not return it.
	password := state.Password
	r.mapAPIToState(&state, &handler)
	state.Password = password
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira outgoing mail handler.
func (r *OutgoingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan OutgoingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state OutgoingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiOutgoingHandler{
		Name:        plan.Name.ValueString(),
		FromAddress: plan.FromAddress.ValueString(),
		SMTPHost:    plan.SMTPHost.ValueString(),
		SMTPPort:    plan.SMTPPort.ValueInt64(),
	}
	if !plan.Prefix.IsNull() && !plan.Prefix.IsUnknown() {
		body.Prefix = plan.Prefix.ValueString()
	}
	if !plan.Protocol.IsNull() && !plan.Protocol.IsUnknown() {
		body.Protocol = plan.Protocol.ValueString()
	}
	if !plan.Username.IsNull() && !plan.Username.IsUnknown() {
		body.Username = plan.Username.ValueString()
	}
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		body.Password = plan.Password.ValueString()
	}
	if !plan.TLS.IsNull() && !plan.TLS.IsUnknown() {
		body.TLS = plan.TLS.ValueBool()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiOutgoingHandler
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/mailhandler/outgoing/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Outgoing mail handler not found",
					fmt.Sprintf("Outgoing mail handler with ID %q not found. The handler may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid outgoing mail handler configuration",
					fmt.Sprintf("The mail handler configuration is invalid. Verify the SMTP host, port, and from_address settings: %s", err.Error()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update outgoing mail handlers. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update outgoing mail handler",
			fmt.Sprintf("Could not update outgoing mail handler with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	r.mapAPIToState(&plan, &updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira outgoing mail handler.
func (r *OutgoingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state OutgoingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/mailhandler/outgoing/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete outgoing mail handler %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete outgoing mail handler",
			fmt.Sprintf("Could not delete outgoing mail handler with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing outgoing mail handler by ID.
func (r *OutgoingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// mapAPIToState maps API response fields to the Terraform state model.
func (r *OutgoingResource) mapAPIToState(model *OutgoingResourceModel, api *apiOutgoingHandler) {
	model.ID = types.StringValue(api.ID)
	model.Name = types.StringValue(api.Name)
	model.FromAddress = types.StringValue(api.FromAddress)
	model.Prefix = types.StringValue(api.Prefix)
	model.SMTPHost = types.StringValue(api.SMTPHost)
	model.SMTPPort = types.Int64Value(api.SMTPPort)
	model.Protocol = types.StringValue(api.Protocol)
	model.Username = types.StringValue(api.Username)
	model.TLS = types.BoolValue(api.TLS)
}
