// Package notificationscheme implements the atlassian_jira_notification_scheme read-only data source.
//
// This data source reads Jira notification schemes by ID from the Atlassian Cloud REST API.
package notificationscheme

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiNotificationEvent represents a single notification event in a scheme.
type apiNotificationEvent struct {
	EventType     string `json:"event_type"`
	RecipientType string `json:"recipient_type"`
	RecipientID   string `json:"recipient_id"`
}

// apiNotificationScheme represents the JSON structure returned by the Atlassian notification scheme API.
type apiNotificationScheme struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	Self               string                 `json:"self"`
	NotificationEvents []apiNotificationEvent `json:"notification_events,omitempty"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	NotificationEvents types.List   `tfsdk:"notification_events"`
}

// DataSource implements the atlassian_jira_notification_scheme data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_notification_scheme"
}

// Schema defines the schema for the jira notification scheme data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira notification scheme from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the notification scheme.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the notification scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the notification scheme.",
				Computed:    true,
			},
			"notification_events": schema.ListNestedAttribute{
				Description: "Notification events within the scheme.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"event_type": schema.StringAttribute{
							Description: "The type of event that triggers the notification.",
							Computed:    true,
						},
						"recipient_type": schema.StringAttribute{
							Description: "The type of recipient for the notification.",
							Computed:    true,
						},
						"recipient_id": schema.StringAttribute{
							Description: "The identifier of the notification recipient.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// Read retrieves notification scheme data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var ns apiNotificationScheme
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/notificationscheme/%s", identifier), &ns)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Notification scheme not found",
				fmt.Sprintf("Jira notification scheme %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read notification scheme",
			fmt.Sprintf("Could not read Jira notification scheme %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(ns.ID)
	config.Name = types.StringValue(ns.Name)
	config.Description = types.StringValue(ns.Description)
	config.NotificationEvents = eventsToState(ctx, ns.NotificationEvents)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// notificationEventObjectType is the attr.Type for notification event nested objects.
var notificationEventObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"event_type":     types.StringType,
		"recipient_type": types.StringType,
		"recipient_id":   types.StringType,
	},
}

// eventsToState converts API notification events to the Terraform state list.
func eventsToState(ctx context.Context, events []apiNotificationEvent) types.List {
	if len(events) == 0 {
		return types.ListNull(notificationEventObjectType)
	}
	var elems []attr.Value
	for _, e := range events {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"event_type":     types.StringType,
				"recipient_type": types.StringType,
				"recipient_id":   types.StringType,
			},
			map[string]attr.Value{
				"event_type":     types.StringValue(e.EventType),
				"recipient_type": types.StringValue(e.RecipientType),
				"recipient_id":   types.StringValue(e.RecipientID),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(notificationEventObjectType, elems)
	return list
}
