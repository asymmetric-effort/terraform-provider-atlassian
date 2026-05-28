// Package mailhandler implements the atlassian_jira_incoming_mail_handler and
// atlassian_jira_outgoing_mail_handler managed resources.
//
// These resources manage Jira mail handlers through the Atlassian
// Cloud REST API. They support full CRUD operations and state import
// via handler ID.
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

// Ensure the IncomingResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &IncomingResource{}
	_ resource.ResourceWithImportState = &IncomingResource{}
)

// apiIncomingHandler represents the JSON structure returned by the Atlassian incoming mail handler API.
type apiIncomingHandler struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Server      string `json:"server"`
	Port        int64  `json:"port"`
	Protocol    string `json:"protocol"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	Folder      string `json:"folder,omitempty"`
	SpaceID     string `json:"spaceId,omitempty"`
	IssueTypeID string `json:"issueTypeId,omitempty"`
}

// IncomingResourceModel describes the incoming mail handler resource data model.
type IncomingResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Server      types.String `tfsdk:"server"`
	Port        types.Int64  `tfsdk:"port"`
	Protocol    types.String `tfsdk:"protocol"`
	Username    types.String `tfsdk:"username"`
	Password    types.String `tfsdk:"password"`
	Folder      types.String `tfsdk:"folder"`
	SpaceID     types.String `tfsdk:"space_id"`
	IssueTypeID types.String `tfsdk:"issue_type_id"`
}

// IncomingResource implements the atlassian_jira_incoming_mail_handler managed resource.
type IncomingResource struct {
	client *atlassian.Client
}

// NewIncomingResource returns a new IncomingResource instance for provider registration.
func NewIncomingResource() resource.Resource {
	return &IncomingResource{}
}

// Metadata returns the resource type name.
func (r *IncomingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_incoming_mail_handler"
}

// Schema defines the schema for the incoming mail handler resource.
func (r *IncomingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira incoming mail handler in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the incoming mail handler, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the incoming mail handler.",
				Required:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the incoming mail handler is enabled.",
				Optional:    true,
				Computed:    true,
			},
			"server": schema.StringAttribute{
				Description: "The mail server hostname.",
				Required:    true,
			},
			"port": schema.Int64Attribute{
				Description: "The mail server port number.",
				Required:    true,
			},
			"protocol": schema.StringAttribute{
				Description: "The mail protocol. Must be \"IMAP\" or \"POP3\".",
				Required:    true,
			},
			"username": schema.StringAttribute{
				Description: "The username for mail server authentication.",
				Required:    true,
			},
			"password": schema.StringAttribute{
				Description: "The password for mail server authentication.",
				Required:    true,
				Sensitive:   true,
			},
			"folder": schema.StringAttribute{
				Description: "The mail folder to monitor (e.g., INBOX).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"space_id": schema.StringAttribute{
				Description: "The Jira space ID for creating issues from incoming mail.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"issue_type_id": schema.StringAttribute{
				Description: "The issue type ID for creating issues from incoming mail.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *IncomingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira incoming mail handler.
func (r *IncomingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IncomingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiIncomingHandler{
		Name:     plan.Name.ValueString(),
		Server:   plan.Server.ValueString(),
		Port:     plan.Port.ValueInt64(),
		Protocol: plan.Protocol.ValueString(),
		Username: plan.Username.ValueString(),
		Password: plan.Password.ValueString(),
	}
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		body.Enabled = plan.Enabled.ValueBool()
	}
	if !plan.Folder.IsNull() && !plan.Folder.IsUnknown() {
		body.Folder = plan.Folder.ValueString()
	}
	if !plan.SpaceID.IsNull() && !plan.SpaceID.IsUnknown() {
		body.SpaceID = plan.SpaceID.ValueString()
	}
	if !plan.IssueTypeID.IsNull() && !plan.IssueTypeID.IsUnknown() {
		body.IssueTypeID = plan.IssueTypeID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiIncomingHandler
	err := r.client.Post(ctx, "/rest/api/3/mailhandler/incoming", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid incoming mail handler configuration",
					fmt.Sprintf("The mail handler configuration is invalid. Verify the server, port, and protocol settings: %s", err.Error()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create incoming mail handlers. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create incoming mail handler",
			fmt.Sprintf("Could not create incoming mail handler %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	r.mapAPIToState(&plan, &created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *IncomingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IncomingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var handler apiIncomingHandler
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/mailhandler/incoming/%s", state.ID.ValueString()), &handler)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read incoming mail handler",
			fmt.Sprintf("Could not read incoming mail handler %q: %s. Verify the handler exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	// Preserve the password from state since the API does not return it.
	password := state.Password
	r.mapAPIToState(&state, &handler)
	state.Password = password
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira incoming mail handler.
func (r *IncomingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IncomingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state IncomingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiIncomingHandler{
		Name:     plan.Name.ValueString(),
		Server:   plan.Server.ValueString(),
		Port:     plan.Port.ValueInt64(),
		Protocol: plan.Protocol.ValueString(),
		Username: plan.Username.ValueString(),
		Password: plan.Password.ValueString(),
	}
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		body.Enabled = plan.Enabled.ValueBool()
	}
	if !plan.Folder.IsNull() && !plan.Folder.IsUnknown() {
		body.Folder = plan.Folder.ValueString()
	}
	if !plan.SpaceID.IsNull() && !plan.SpaceID.IsUnknown() {
		body.SpaceID = plan.SpaceID.ValueString()
	}
	if !plan.IssueTypeID.IsNull() && !plan.IssueTypeID.IsUnknown() {
		body.IssueTypeID = plan.IssueTypeID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiIncomingHandler
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/mailhandler/incoming/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Incoming mail handler not found",
					fmt.Sprintf("Incoming mail handler with ID %q not found. The handler may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid incoming mail handler configuration",
					fmt.Sprintf("The mail handler configuration is invalid. Verify the server, port, and protocol settings: %s", err.Error()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update incoming mail handlers. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update incoming mail handler",
			fmt.Sprintf("Could not update incoming mail handler with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	r.mapAPIToState(&plan, &updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira incoming mail handler.
func (r *IncomingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IncomingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/mailhandler/incoming/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete incoming mail handler %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete incoming mail handler",
			fmt.Sprintf("Could not delete incoming mail handler with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing incoming mail handler by ID.
func (r *IncomingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// mapAPIToState maps API response fields to the Terraform state model.
func (r *IncomingResource) mapAPIToState(model *IncomingResourceModel, api *apiIncomingHandler) {
	model.ID = types.StringValue(api.ID)
	model.Name = types.StringValue(api.Name)
	model.Enabled = types.BoolValue(api.Enabled)
	model.Server = types.StringValue(api.Server)
	model.Port = types.Int64Value(api.Port)
	model.Protocol = types.StringValue(api.Protocol)
	model.Username = types.StringValue(api.Username)
	model.Folder = types.StringValue(api.Folder)
	model.SpaceID = types.StringValue(api.SpaceID)
	model.IssueTypeID = types.StringValue(api.IssueTypeID)
}
