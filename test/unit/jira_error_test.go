// Package unit contains tests that verify Jira resource error messages are
// clear, user-friendly, and consistent across all resource types.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	automationresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/automation"
	boardresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/board"
	customdomainresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/custom_domain"
	customfieldresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/custom_field"
	mailresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/mail_handler"
	permschemeresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/permission_scheme"
	spaceresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/space"
	workflowresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/workflow"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// errorClient creates a mock server returning a fixed HTTP status and message, then returns a configured client.
func errorClient(t *testing.T, statusCode int, message string) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{message},
			"errors":        map[string]string{},
		})
	}))
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	c, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)
	return c
}

// --- Space: not found, duplicate key, permission denied ---

func TestSpaceNotFoundRead(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 404, "Project not found")
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "nonexistent-id"),
		"key":                      tftypes.NewValue(tftypes.String, "GONE"),
		"name":                     tftypes.NewValue(tftypes.String, "Gone Space"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: emptyState(ctx, s)}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	// 404 on Read should remove the resource from state (no error)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected 404 Read to silently remove resource, but got error")
	}
}

func TestSpaceDuplicateKey(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 409, "A project with this key already exists")
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "DUP"),
		"name":                     tftypes.NewValue(tftypes.String, "Duplicate"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected duplicate key error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Duplicate space key") &&
			strings.Contains(d.Detail(), "DUP") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the duplicate key. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestSpacePermissionDeniedCreate(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "NOPERM"),
		"name":                     tftypes.NewValue(tftypes.String, "NoPerms"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "admin privileges") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention permission denied and admin privileges. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Workflow: invalid transition (BadRequest) ---

func TestWorkflowInvalidTransitionCreate(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 400, "Invalid workflow transition configuration")
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Bad Workflow"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error for invalid workflow")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid workflow configuration") &&
			strings.Contains(d.Detail(), "transitions") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should describe invalid workflow configuration. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestWorkflowInvalidTransitionUpdate(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 400, "Invalid workflow transition")
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "wf-123"),
		"name":        tftypes.NewValue(tftypes.String, "Existing Workflow"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error for invalid workflow update")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid workflow configuration") &&
			strings.Contains(d.Detail(), "wf-123") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the workflow ID. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Automation: rule validation failure ---

func TestAutomationRuleValidationFailure(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 400, "Invalid trigger type")
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":           tftypes.NewValue(tftypes.String, "Bad Rule"),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, "invalid_trigger"),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"send_email"}]`),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected validation error for invalid automation rule")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid automation rule configuration") &&
			strings.Contains(d.Detail(), "trigger type") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should describe the invalid trigger. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestAutomationRulePermissionDenied(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":           tftypes.NewValue(tftypes.String, "Forbidden Rule"),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"send_email"}]`),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "automation rules") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention automation rules. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Mail handler: connection failure (BadRequest) ---

func TestIncomingMailHandlerConnectionFailure(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 400, "Connection refused to mail server")
	ctx := context.Background()
	r := mailresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Bad Mail"),
		"enabled":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"server":        tftypes.NewValue(tftypes.String, "badhost.invalid"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAPS"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error for mail handler")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid incoming mail handler configuration") &&
			strings.Contains(d.Detail(), "server") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention server configuration. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestOutgoingMailHandlerConnectionFailure(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 400, "SMTP connection failed")
	ctx := context.Background()
	r := mailresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":         tftypes.NewValue(tftypes.String, "Bad SMTP"),
		"from_address": tftypes.NewValue(tftypes.String, "noreply@test.com"),
		"prefix":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_host":    tftypes.NewValue(tftypes.String, "badsmtp.invalid"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"password":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"tls":          tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error for outgoing mail handler")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid outgoing mail handler configuration") &&
			strings.Contains(d.Detail(), "SMTP") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention SMTP configuration. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Custom domain: verification failure, duplicate ---

func TestCustomDomainDuplicate(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 409, "Domain already registered")
	ctx := context.Background()
	r := customdomainresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "dup.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"mx_records":          tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "priority": tftypes.Number, "value": tftypes.String}}}, []tftypes.Value{}),
		"txt_records":         tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
		"dkim_records":        tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
		"cname_records":       tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected duplicate domain error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Domain already registered") &&
			strings.Contains(d.Detail(), "dup.example.com") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the duplicate domain. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestCustomDomainInvalidName(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 400, "Invalid domain name")
	ctx := context.Background()
	r := customdomainresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "not a valid domain"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"mx_records":          tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "priority": tftypes.Number, "value": tftypes.String}}}, []tftypes.Value{}),
		"txt_records":         tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
		"dkim_records":        tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
		"cname_records":       tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected invalid domain name error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid domain name") &&
			strings.Contains(d.Detail(), "fully qualified domain name") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should suggest providing a valid FQDN. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestCustomDomainPermissionDenied(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := customdomainresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "noperm.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"mx_records":          tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "priority": tftypes.Number, "value": tftypes.String}}}, []tftypes.Value{}),
		"txt_records":         tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
		"dkim_records":        tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
		"cname_records":       tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"host": tftypes.String, "value": tftypes.String}}}, []tftypes.Value{}),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "admin privileges") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention admin privileges. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Permission denied across resources ---

func TestPermissionDeniedBoard(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := boardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	ccType := tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "status_ids": tftypes.List{ElementType: tftypes.String}}}}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Board"),
		"type":          tftypes.NewValue(tftypes.String, "scrum"),
		"space_id":      tftypes.NewValue(tftypes.String, "proj-1"),
		"column_config": tftypes.NewValue(ccType, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") {
			found = true
		}
	}
	if !found {
		t.Fatal("Expected Permission denied in error summary")
	}
}

func TestPermissionDeniedCustomField(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := customfieldresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Custom"),
		"type":        tftypes.NewValue(tftypes.String, "string"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"options":     tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "custom fields") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error should mention custom fields. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestPermissionDeniedPermissionScheme(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := permschemeresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Scheme"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "permission schemes") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error should mention permission schemes. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Board: BadRequest for invalid type ---

func TestBoardInvalidTypeCreate(t *testing.T) {
	t.Parallel()
	client := errorClient(t, 400, "Invalid board type")
	ctx := context.Background()
	r := boardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	ccType := tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "status_ids": tftypes.List{ElementType: tftypes.String}}}}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Bad Board"),
		"type":          tftypes.NewValue(tftypes.String, "invalid"),
		"space_id":      tftypes.NewValue(tftypes.String, "proj-1"),
		"column_config": tftypes.NewValue(ccType, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error for invalid board type")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid board configuration") &&
			strings.Contains(d.Detail(), "scrum") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should suggest valid board types. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Error messages never expose raw API JSON ---

func TestErrorDoesNotExposeRawJSON(t *testing.T) {
	t.Parallel()
	// Return a raw JSON error body that should be translated
	client := errorClient(t, 500, "Internal server error")
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "ERRTEST"),
		"name":                     tftypes.NewValue(tftypes.String, "Error Test"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for 500 response")
	}
	for _, d := range resp.Diagnostics.Errors() {
		detail := d.Detail()
		// The error message should not contain raw JSON structures
		if strings.Contains(detail, `"errorMessages"`) ||
			strings.Contains(detail, `"errors":{`) {
			t.Fatalf("Error message exposes raw API JSON: %s", detail)
		}
		// Should contain the space key for identification
		if !strings.Contains(detail, "ERRTEST") {
			t.Fatalf("Error message should identify the affected resource. Got: %s", detail)
		}
	}
}

// --- Error messages have consistent format ---

func TestErrorMessageConsistency(t *testing.T) {
	t.Parallel()
	// Verify that permission denied errors across different resources follow the same pattern
	client := errorClient(t, 403, "Forbidden")
	ctx := context.Background()

	// Test space
	sr := spaceresource.NewResource()
	configureResource(t, sr, client)
	ss := getResourceSchema(t, sr)
	stfType := ss.Type().TerraformType(ctx)
	sPlan := tfsdk.Plan{Schema: ss, Raw: tftypes.NewValue(stfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "CONS"),
		"name":                     tftypes.NewValue(tftypes.String, "Consistent"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	sResp := &resource.CreateResponse{State: emptyState(ctx, ss)}
	sr.Create(ctx, resource.CreateRequest{Plan: sPlan}, sResp)

	// Test workflow
	wr := workflowresource.NewResource()
	configureResource(t, wr, client)
	ws := getResourceSchema(t, wr)
	wtfType := ws.Type().TerraformType(ctx)
	wPlan := tfsdk.Plan{Schema: ws, Raw: tftypes.NewValue(wtfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Consistent WF"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	wResp := &resource.CreateResponse{State: emptyState(ctx, ws)}
	wr.Create(ctx, resource.CreateRequest{Plan: wPlan}, wResp)

	// Both should have "Permission denied" summary and mention "admin privileges"
	for _, resp := range []*resource.CreateResponse{sResp, wResp} {
		if !resp.Diagnostics.HasError() {
			t.Fatal("Expected permission denied error")
		}
		for _, d := range resp.Diagnostics.Errors() {
			if d.Summary() != "Permission denied" {
				t.Fatalf("Inconsistent error summary: expected 'Permission denied', got %q", d.Summary())
			}
			if !strings.Contains(d.Detail(), "admin privileges") {
				t.Fatalf("Error message should mention admin privileges: %s", d.Detail())
			}
		}
	}
}
