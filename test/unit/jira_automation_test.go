// Package unit contains unit tests for the atlassian_jira_automation_rule
// resource and data source.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	automationdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/automation"
	automationresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/automation"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// automationIDCounter provides unique IDs for automation mock server tests.
var automationIDCounter uint64

func automationNextID() string {
	n := atomic.AddUint64(&automationIDCounter, 1)
	return fmt.Sprintf("rule-%d", n)
}

// automationMockServer creates a mock HTTP server for Jira automation rule endpoints.
func automationMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	rules := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Create rule
	mux.HandleFunc("POST /rest/api/3/automation/rule", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			writeErr(w, 400, "name is required")
			return
		}
		triggerType, _ := req["triggerType"].(string)
		if triggerType == "" {
			writeErr(w, 400, "triggerType is required")
			return
		}
		if triggerType == "invalid_trigger" {
			writeErr(w, 400, "Invalid trigger type: invalid_trigger is not a recognized trigger")
			return
		}
		actions, _ := req["actions"]
		if actions == nil {
			writeErr(w, 400, "actions is required")
			return
		}
		actionsStr := ""
		if ab, err := json.Marshal(actions); err == nil {
			actionsStr = string(ab)
		}
		if actionsStr == `"invalid"` || actionsStr == `"[]"` {
			writeErr(w, 400, "Invalid action configuration: actions must be a valid JSON array")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := automationNextID()
		state, _ := req["state"].(string)
		if state == "" {
			state = "enabled"
		}
		if state != "enabled" && state != "disabled" {
			mu.Unlock()
			writeErr(w, 400, fmt.Sprintf("Invalid state %q: must be \"enabled\" or \"disabled\"", state))
			mu.Lock()
			return
		}
		rule := map[string]interface{}{
			"id":          id,
			"name":        name,
			"state":       state,
			"triggerType": triggerType,
			"actions":     actions,
		}
		if tc, ok := req["triggerConfig"]; ok {
			rule["triggerConfig"] = tc
		}
		if c, ok := req["conditions"]; ok {
			rule["conditions"] = c
		}
		rules[id] = rule
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(rule)
	})

	// Read rule by ID
	mux.HandleFunc("GET /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		rule, ok := rules[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Automation rule not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)
	})

	// Update rule
	mux.HandleFunc("PUT /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		rule, ok := rules[id]
		if !ok {
			mu.Unlock()
			writeErr(w, 404, "Automation rule not found")
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				rule[k] = v
			}
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rule)
	})

	// Delete rule
	mux.HandleFunc("DELETE /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := rules[id]; !ok {
			writeErr(w, 404, "Automation rule not found")
			return
		}
		delete(rules, id)
		w.WriteHeader(204)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, err := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	c, err := atlassian.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return ts, c
}

// ==================== RESOURCE SCHEMA TESTS ====================

// TestAutomationRuleResourceMetadata verifies the resource type name.
func TestAutomationRuleResourceMetadata(t *testing.T) {
	t.Parallel()
	r := automationresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_automation_rule" {
		t.Errorf("expected resource type name 'atlassian_jira_automation_rule', got %q", resp.TypeName)
	}
}

// TestAutomationRuleResourceSchema verifies the resource schema has all expected attributes.
func TestAutomationRuleResourceSchema(t *testing.T) {
	t.Parallel()
	r := automationresource.NewResource()
	s := getResourceSchema(t, r)
	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "state", "trigger_type", "trigger_config", "conditions", "actions"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestAutomationRuleResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestAutomationRuleResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := automationresource.NewResource()
	s := getResourceSchema(t, r)
	expected := 7
	actual := len(s.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestAutomationRuleResourceSchemaRequiredAttributes verifies required attributes.
func TestAutomationRuleResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := automationresource.NewResource()
	s := getResourceSchema(t, r)
	requiredAttrs := []string{"name", "trigger_type", "actions"}
	for _, name := range requiredAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("expected attribute %q to be required", name)
		}
	}
}

// TestAutomationRuleResourceSchemaComputedAttributes verifies computed attributes.
func TestAutomationRuleResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := automationresource.NewResource()
	s := getResourceSchema(t, r)
	computedAttrs := []string{"id", "state", "trigger_config", "conditions"}
	for _, name := range computedAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("expected attribute %q to be computed", name)
		}
	}
}

// TestAutomationRuleResourceImplementsImportState verifies the resource implements ImportState.
func TestAutomationRuleResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := automationresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected automation rule resource to implement ResourceWithImportState")
	}
}

// TestAutomationRuleResourceInterfaceCompliance verifies resource interface compliance.
func TestAutomationRuleResourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = automationresource.NewResource()
	var _ resource.ResourceWithImportState = automationresource.NewResource().(resource.ResourceWithImportState)
}

// TestAutomationRuleResourceConfigureNil verifies nil provider data is handled.
func TestAutomationRuleResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := automationresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(interface {
		Configure(context.Context, resource.ConfigureRequest, *resource.ConfigureResponse)
	}).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("expected no error for nil provider data, got %v", resp.Diagnostics.Errors())
	}
}

// TestAutomationRuleResourceConfigureWrongType verifies wrong provider data type is handled.
func TestAutomationRuleResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := automationresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(interface {
		Configure(context.Context, resource.ConfigureRequest, *resource.ConfigureResponse)
	}).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

// ==================== RESOURCE CRUD TESTS ====================

// TestAutomationRuleResourceCRUDLifecycle tests the full create-read-update-delete lifecycle.
func TestAutomationRuleResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":           tftypes.NewValue(tftypes.String, "Auto Close Resolved"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, `{"key":"value"}`),
		"conditions":     tftypes.NewValue(tftypes.String, `{"type":"jql","value":"status = Done"}`),
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"transition","value":"Close"}]`),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty id after create")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Auto Close Resolved" {
		t.Errorf("expected name 'Auto Close Resolved', got %q", name)
	}
	if state := getStringAttr(t, createResp.State, "state"); state != "enabled" {
		t.Errorf("expected state 'enabled', got %q", state)
	}

	// Read
	readResp := &resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if readID := getStringAttr(t, readResp.State, "id"); readID != id {
		t.Errorf("expected id %q, got %q", id, readID)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, id),
		"name":           tftypes.NewValue(tftypes.String, "Updated Rule"),
		"state":          tftypes.NewValue(tftypes.String, "disabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "field_value_changed"),
		"trigger_config": tftypes.NewValue(tftypes.String, `{"field":"priority"}`),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"comment","value":"Updated"}]`),
	})}
	updateResp := &resource.UpdateResponse{State: readResp.State}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Rule" {
		t.Errorf("expected name 'Updated Rule', got %q", name)
	}
	if state := getStringAttr(t, updateResp.State, "state"); state != "disabled" {
		t.Errorf("expected state 'disabled', got %q", state)
	}

	// Delete
	deleteResp := &resource.DeleteResponse{State: updateResp.State}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Verify deleted - read should remove resource
	readAfterDelete := &resource.ReadResponse{State: updateResp.State}
	r.Read(ctx, resource.ReadRequest{State: updateResp.State}, readAfterDelete)
	if readAfterDelete.Diagnostics.HasError() {
		t.Fatalf("Read after delete: %v", readAfterDelete.Diagnostics.Errors())
	}
}

// TestAutomationRuleResourceCreateInvalidTrigger verifies error on invalid trigger type.
func TestAutomationRuleResourceCreateInvalidTrigger(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
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
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"comment"}]`),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for invalid trigger type")
	}
}

// TestAutomationRuleResourceCreateInvalidActions verifies error on invalid action config.
func TestAutomationRuleResourceCreateInvalidActions(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":           tftypes.NewValue(tftypes.String, "Bad Actions Rule"),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, "invalid"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for invalid actions")
	}
}

// TestAutomationRuleResourceReadNotFound verifies 404 removes resource from state.
func TestAutomationRuleResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":           tftypes.NewValue(tftypes.String, "Gone"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Errorf("expected no error for not-found read, got %v", readResp.Diagnostics.Errors())
	}
}

// TestAutomationRuleResourceUpdateNotFound verifies 404 on update.
func TestAutomationRuleResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":           tftypes.NewValue(tftypes.String, "Gone"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for update on nonexistent rule")
	}
}

// TestAutomationRuleResourceDeleteNotFound verifies deleting a nonexistent rule does not error.
func TestAutomationRuleResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":           tftypes.NewValue(tftypes.String, "Gone"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Errorf("expected no error for delete on nonexistent rule, got %v", deleteResp.Diagnostics.Errors())
	}
}

// TestAutomationRuleResourceCreateWithDefaults verifies defaults are applied when optionals omitted.
func TestAutomationRuleResourceCreateWithDefaults(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":           tftypes.NewValue(tftypes.String, "Minimal Rule"),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"comment","value":"Created"}]`),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	if state := getStringAttr(t, createResp.State, "state"); state != "enabled" {
		t.Errorf("expected default state 'enabled', got %q", state)
	}
}

// TestAutomationRuleResourceCreateMissingName verifies error when name is missing.
func TestAutomationRuleResourceCreateMissingName(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":           tftypes.NewValue(tftypes.String, ""),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"comment"}]`),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for empty name")
	}
}

// ==================== DATA SOURCE TESTS ====================

// TestAutomationRuleDataSourceMetadata verifies the data source type name.
func TestAutomationRuleDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := automationdatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_automation_rule" {
		t.Errorf("expected data source type name 'atlassian_jira_automation_rule', got %q", resp.TypeName)
	}
}

// TestAutomationRuleDataSourceSchema verifies the data source schema.
func TestAutomationRuleDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := automationdatasource.NewDataSource()
	s := getDatasourceSchema(t, ds)
	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "state", "trigger_type", "trigger_config", "conditions", "actions"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestAutomationRuleDataSourceSchemaAttributeCount verifies attribute count.
func TestAutomationRuleDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := automationdatasource.NewDataSource()
	s := getDatasourceSchema(t, ds)
	expected := 7
	actual := len(s.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestAutomationRuleDataSourceInterfaceCompliance verifies data source interface compliance.
func TestAutomationRuleDataSourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = automationdatasource.NewDataSource()
}

// TestAutomationRuleDataSourceConfigureNil verifies nil provider data is handled.
func TestAutomationRuleDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := automationdatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(interface {
		Configure(context.Context, datasource.ConfigureRequest, *datasource.ConfigureResponse)
	}).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("expected no error for nil provider data, got %v", resp.Diagnostics.Errors())
	}
}

// TestAutomationRuleDataSourceConfigureWrongType verifies wrong provider data type is handled.
func TestAutomationRuleDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := automationdatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(interface {
		Configure(context.Context, datasource.ConfigureRequest, *datasource.ConfigureResponse)
	}).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

// TestAutomationRuleDataSourceRead tests the read data source.
func TestAutomationRuleDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()

	// First create a rule using the resource
	r := automationresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTFType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTFType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":           tftypes.NewValue(tftypes.String, "DS Test Rule"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"comment"}]`),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	ruleID := getStringAttr(t, createResp.State, "id")

	// Now read using data source
	ds := automationdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, ruleID),
		"name":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DataSource Read: %v", dsResp.Diagnostics.Errors())
	}
}

// TestAutomationRuleDataSourceReadNotFound tests reading a nonexistent rule.
func TestAutomationRuleDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()

	ds := automationdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for nonexistent automation rule")
	}
}

// TestAutomationRuleResourceCreateForbidden verifies 403 error on create.
func TestAutomationRuleResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/automation/rule", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

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
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"comment"}]`),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden create")
	}
}

// TestAutomationRuleResourceCreateServerError verifies generic server error on create.
func TestAutomationRuleResourceCreateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/automation/rule", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":           tftypes.NewValue(tftypes.String, "Error Rule"),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, `[{"type":"comment"}]`),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for server error create")
	}
}

// TestAutomationRuleResourceReadServerError verifies generic read error.
func TestAutomationRuleResourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, "Test"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Error("expected error for server error read")
	}
}

// TestAutomationRuleResourceUpdateBadRequest verifies 400 error on update.
func TestAutomationRuleResourceUpdateBadRequest(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 400, "Invalid configuration")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, "Test"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for bad request update")
	}
}

// TestAutomationRuleResourceUpdateForbidden verifies 403 error on update.
func TestAutomationRuleResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, "Test"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden update")
	}
}

// TestAutomationRuleResourceUpdateServerError verifies generic server error on update.
func TestAutomationRuleResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, "Test"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for server error update")
	}
}

// TestAutomationRuleResourceDeleteForbidden verifies 403 error on delete.
func TestAutomationRuleResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, "Test"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden delete")
	}
}

// TestAutomationRuleResourceDeleteServerError verifies generic server error on delete.
func TestAutomationRuleResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, "Test"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for server error delete")
	}
}

// TestAutomationRuleDataSourceReadServerError verifies generic server error on DS read.
func TestAutomationRuleDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := automationdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for server error DS read")
	}
}

// TestAutomationRuleResourceCreateInvalidPlan verifies error when plan cannot be deserialized.
func TestAutomationRuleResourceCreateInvalidPlan(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for nil plan")
	}
}

// TestAutomationRuleResourceReadInvalidState verifies error when state cannot be deserialized.
func TestAutomationRuleResourceReadInvalidState(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Error("expected error for nil state")
	}
}

// TestAutomationRuleResourceUpdateInvalidPlan verifies error when plan cannot be deserialized.
func TestAutomationRuleResourceUpdateInvalidPlan(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, "Test"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for nil plan on update")
	}
}

// TestAutomationRuleResourceUpdateInvalidState verifies error when state cannot be deserialized.
func TestAutomationRuleResourceUpdateInvalidState(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "test-id"),
		"name":           tftypes.NewValue(tftypes.String, "Test"),
		"state":          tftypes.NewValue(tftypes.String, "enabled"),
		"trigger_type":   tftypes.NewValue(tftypes.String, "issue_created"),
		"trigger_config": tftypes.NewValue(tftypes.String, ""),
		"conditions":     tftypes.NewValue(tftypes.String, ""),
		"actions":        tftypes.NewValue(tftypes.String, "[]"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for nil state on update")
	}
}

// TestAutomationRuleResourceDeleteInvalidState verifies error when state cannot be deserialized.
func TestAutomationRuleResourceDeleteInvalidState(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for nil state on delete")
	}
}

// TestAutomationRuleDataSourceReadInvalidConfig verifies error when config cannot be deserialized.
func TestAutomationRuleDataSourceReadInvalidConfig(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	ds := automationdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dss.Type().TerraformType(ctx), nil)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for nil config on DS read")
	}
}

// TestAutomationRuleDataSourceReadMissingID verifies error when ID not provided.
func TestAutomationRuleDataSourceReadMissingID(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()

	ds := automationdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"name":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"state":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"trigger_config": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"conditions":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"actions":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for missing ID on data source read")
	}
}

// TestAutomationRuleResourceImportState verifies import state passthrough.
func TestAutomationRuleResourceImportState(t *testing.T) {
	t.Parallel()
	_, client := automationMockServer(t)
	ctx := context.Background()
	r := automationresource.NewResource()
	configureResource(t, r, client)

	importable := r.(resource.ResourceWithImportState)
	s := getResourceSchema(t, r)
	importResp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	importable.ImportState(ctx, resource.ImportStateRequest{ID: "test-rule-id"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	importedID := getStringAttr(t, importResp.State, "id")
	if importedID != "test-rule-id" {
		t.Errorf("expected imported id 'test-rule-id', got %q", importedID)
	}
}
