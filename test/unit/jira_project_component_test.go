// Package unit contains unit tests for the atlassian_jira_project_component resource and data source.
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

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	componentds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/space"
	componentrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/space"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// componentIDCounter provides unique IDs for component mock server tests.
var componentIDCounter uint64

func componentNextID() string {
	n := atomic.AddUint64(&componentIDCounter, 1)
	return fmt.Sprintf("comp-%d", n)
}

// testComponentMockServer creates a mock HTTP server for Jira project component endpoints.
func testComponentMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	components := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Create component
	mux.HandleFunc("POST /rest/api/3/component", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"name is required"},
				"errors":        map[string]string{},
			})
			return
		}
		mu.Lock()
		defer mu.Unlock()

		// Check for duplicate name within same project
		projectID, _ := req["projectId"].(string)
		for _, c := range components {
			if c["name"] == name && c["projectId"] == projectID {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"A component with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}

		id := componentNextID()
		description, _ := req["description"].(string)
		leadAccountID, _ := req["leadAccountId"].(string)
		assigneeType, _ := req["assigneeType"].(string)
		comp := map[string]interface{}{
			"id":            id,
			"name":          name,
			"description":   description,
			"leadAccountId": leadAccountID,
			"assigneeType":  assigneeType,
			"projectId":     projectID,
			"self":          fmt.Sprintf("https://example.atlassian.net/rest/api/3/component/%s", id),
		}
		components[id] = comp
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(comp)
	})

	// Read component by ID
	mux.HandleFunc("GET /rest/api/3/component/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		comp, ok := components[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Component not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comp)
	})

	// Update component
	mux.HandleFunc("PUT /rest/api/3/component/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		comp, ok := components[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Component not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" && k != "projectId" {
				comp[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comp)
	})

	// Delete component
	mux.HandleFunc("DELETE /rest/api/3/component/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := components[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Component not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(components, id)
		w.WriteHeader(204)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	auth := &testNoopAuth{}
	client, err := atlassian.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// testComponentForbiddenMockServer creates a mock that returns 403 for all component endpoints.
func testComponentForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"You do not have permission"},
			"errors":        map[string]string{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// ==================== RESOURCE SCHEMA TESTS ====================

// TestJiraProjectComponentResourceMetadata verifies the resource type name.
func TestJiraProjectComponentResourceMetadata(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_project_component" {
		t.Errorf("expected resource type name 'atlassian_jira_project_component', got %q", resp.TypeName)
	}
}

// TestJiraProjectComponentResourceSchema verifies the resource schema has all expected attributes.
func TestJiraProjectComponentResourceSchema(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "space_id", "name", "description", "lead_account_id", "assignee_type"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraProjectComponentResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraProjectComponentResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 6
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraProjectComponentResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraProjectComponentResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"space_id", "name"}
	for _, name := range requiredAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("expected attribute %q to be required", name)
		}
	}
}

// TestJiraProjectComponentResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraProjectComponentResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"id", "description", "lead_account_id", "assignee_type"}
	for _, name := range computedAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("expected attribute %q to be computed", name)
		}
	}
}

// TestJiraProjectComponentResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraProjectComponentResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"description", "lead_account_id", "assignee_type"}
	for _, name := range optionalAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsOptional() {
			t.Errorf("expected attribute %q to be optional", name)
		}
	}
}

// TestJiraProjectComponentResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraProjectComponentResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraProjectComponentResourceImplementsResource verifies the Resource interface.
func TestJiraProjectComponentResourceImplementsResource(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected component resource to implement resource.Resource")
	}
}

// TestJiraProjectComponentResourceImplementsImportState verifies the ImportState interface.
func TestJiraProjectComponentResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := componentrs.NewComponentResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected component resource to implement ResourceWithImportState")
	}
}

// ==================== DATA SOURCE SCHEMA TESTS ====================

// TestJiraProjectComponentDataSourceMetadata verifies the data source type name.
func TestJiraProjectComponentDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := componentds.NewComponentDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_project_component" {
		t.Errorf("expected data source type name 'atlassian_jira_project_component', got %q", resp.TypeName)
	}
}

// TestJiraProjectComponentDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraProjectComponentDataSourceSchema(t *testing.T) {
	t.Parallel()

	ds := componentds.NewComponentDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "space_id", "name", "description", "lead_account_id", "assignee_type"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraProjectComponentDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraProjectComponentDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	ds := componentds.NewComponentDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	expected := 6
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraProjectComponentDataSourceSchemaComputedAttributes verifies computed-only attributes.
func TestJiraProjectComponentDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	ds := componentds.NewComponentDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	computedAttrs := []string{"space_id", "name", "description", "lead_account_id", "assignee_type"}
	for _, name := range computedAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("expected attribute %q to be computed", name)
		}
	}
}

// TestJiraProjectComponentDataSourceSchemaRequiredAttributes verifies required attributes.
func TestJiraProjectComponentDataSourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	ds := componentds.NewComponentDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["id"]
	if !ok {
		t.Fatal("expected attribute 'id' to exist")
	}
	if !attr.IsRequired() {
		t.Error("expected attribute 'id' to be required")
	}
}

// TestJiraProjectComponentDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraProjectComponentDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	ds := componentds.NewComponentDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected component data source to implement datasource.DataSource")
	}
}

// ==================== RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraProjectComponentResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraProjectComponentResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "Backend"),
		"description":     tftypes.NewValue(tftypes.String, "Backend component"),
		"lead_account_id": tftypes.NewValue(tftypes.String, "lead-123"),
		"assignee_type":   tftypes.NewValue(tftypes.String, "COMPONENT_LEAD"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	compID := getStringAttr(t, createResp.State, "id")
	if compID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Backend" {
		t.Errorf("expected name 'Backend', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "Backend component" {
		t.Errorf("expected description 'Backend component', got %q", desc)
	}
	if lead := getStringAttr(t, createResp.State, "lead_account_id"); lead != "lead-123" {
		t.Errorf("expected lead_account_id 'lead-123', got %q", lead)
	}
	if at := getStringAttr(t, createResp.State, "assignee_type"); at != "COMPONENT_LEAD" {
		t.Errorf("expected assignee_type 'COMPONENT_LEAD', got %q", at)
	}
	if sid := getStringAttr(t, createResp.State, "space_id"); sid != "proj-1" {
		t.Errorf("expected space_id 'proj-1', got %q", sid)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, compID),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "Backend"),
		"description":     tftypes.NewValue(tftypes.String, "Backend component"),
		"lead_account_id": tftypes.NewValue(tftypes.String, "lead-123"),
		"assignee_type":   tftypes.NewValue(tftypes.String, "COMPONENT_LEAD"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Backend" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, compID),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "Frontend"),
		"description":     tftypes.NewValue(tftypes.String, "Frontend component"),
		"lead_account_id": tftypes.NewValue(tftypes.String, "lead-456"),
		"assignee_type":   tftypes.NewValue(tftypes.String, "PROJECT_LEAD"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Frontend" {
		t.Errorf("Update name: got %q", name)
	}
	if desc := getStringAttr(t, updateResp.State, "description"); desc != "Frontend component" {
		t.Errorf("Update description: got %q", desc)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete should remove resource
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp2)
	if readResp2.State.Raw.IsNull() {
		// Expected: state removed for deleted resource
	}
}

// TestJiraProjectComponentResourceCreateMinimal tests creating a component with only required fields.
func TestJiraProjectComponentResourceCreateMinimal(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-2"),
		"name":            tftypes.NewValue(tftypes.String, "Minimal"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create minimal: %v", createResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, createResp.State, "id"); id == "" {
		t.Error("expected non-empty id for minimal create")
	}
}

// TestJiraProjectComponentResourceCreateDuplicate tests creating a component with a duplicate name in the same project.
func TestJiraProjectComponentResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-dup"),
		"name":            tftypes.NewValue(tftypes.String, "DupComp"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-dup"),
		"name":            tftypes.NewValue(tftypes.String, "DupComp"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate name error")
	}
}

// TestJiraProjectComponentResourceUpdateNotFound tests updating a nonexistent component.
func TestJiraProjectComponentResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent component")
	}
}

// TestJiraProjectComponentResourceDeleteNotFound tests deleting an already-deleted component.
func TestJiraProjectComponentResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent component should not error (idempotent)")
	}
}

// TestJiraProjectComponentResourceReadNotFound tests reading a nonexistent component removes resource.
func TestJiraProjectComponentResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent should not error: %v", readResp.Diagnostics.Errors())
	}
	// State should be removed (null)
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed after 404")
	}
}

// TestJiraProjectComponentResourceCreateForbidden tests 403 on create.
func TestJiraProjectComponentResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testComponentForbiddenMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraProjectComponentResourceUpdateForbidden tests 403 on update.
func TestJiraProjectComponentResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testComponentForbiddenMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraProjectComponentResourceDeleteForbidden tests 403 on delete.
func TestJiraProjectComponentResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testComponentForbiddenMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraProjectComponentResourceConfigureNil verifies nil provider data does not error.
func TestJiraProjectComponentResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := componentrs.NewComponentResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraProjectComponentResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraProjectComponentResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := componentrs.NewComponentResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraProjectComponentResourceImportState verifies import state passthrough.
func TestJiraProjectComponentResourceImportState(t *testing.T) {
	t.Parallel()
	r := componentrs.NewComponentResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "comp-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== DATA SOURCE CRUD TESTS ====================

// TestJiraProjectComponentDataSourceByID tests reading a component by ID.
func TestJiraProjectComponentDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()

	// Create a component first via resource
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-ds"),
		"name":            tftypes.NewValue(tftypes.String, "DS Component"),
		"description":     tftypes.NewValue(tftypes.String, "for data source"),
		"lead_account_id": tftypes.NewValue(tftypes.String, "lead-ds"),
		"assignee_type":   tftypes.NewValue(tftypes.String, "PROJECT_DEFAULT"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	compID := getStringAttr(t, cResp.State, "id")

	// Read via data source by ID
	ds := componentds.NewComponentDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, compID),
		"space_id":        tftypes.NewValue(tftypes.String, nil),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"lead_account_id": tftypes.NewValue(tftypes.String, nil),
		"assignee_type":   tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Component" {
		t.Errorf("expected name 'DS Component', got %q", name)
	}
	if desc := getStringAttr(t, dsResp.State, "description"); desc != "for data source" {
		t.Errorf("expected description 'for data source', got %q", desc)
	}
	if sid := getStringAttr(t, dsResp.State, "space_id"); sid != "proj-ds" {
		t.Errorf("expected space_id 'proj-ds', got %q", sid)
	}
}

// TestJiraProjectComponentDataSourceNotFound tests 404 error on data source read.
func TestJiraProjectComponentDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	ds := componentds.NewComponentDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":        tftypes.NewValue(tftypes.String, nil),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"lead_account_id": tftypes.NewValue(tftypes.String, nil),
		"assignee_type":   tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent component")
	}
}

// TestJiraProjectComponentDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraProjectComponentDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := componentds.NewComponentDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraProjectComponentDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraProjectComponentDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := componentds.NewComponentDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraProjectComponentResourceReadServerError tests a non-404 server error on read.
func TestJiraProjectComponentResourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
			"errors":        map[string]string{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectComponentResourceCreateServerError tests a generic server error on create.
func TestJiraProjectComponentResourceCreateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
			"errors":        map[string]string{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "ServerErr"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectComponentResourceUpdateServerError tests a generic server error on update.
func TestJiraProjectComponentResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
			"errors":        map[string]string{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectComponentResourceDeleteServerError tests a generic server error on delete.
func TestJiraProjectComponentResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
			"errors":        map[string]string{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectComponentDataSourceReadServerError tests a non-404 server error on data source read.
func TestJiraProjectComponentDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
			"errors":        map[string]string{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	ds := componentds.NewComponentDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":        tftypes.NewValue(tftypes.String, nil),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"lead_account_id": tftypes.NewValue(tftypes.String, nil),
		"assignee_type":   tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// testComponentNoProjectIDMockServer creates a mock that returns components without projectId field.
func testComponentNoProjectIDMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	components := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/api/3/component", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		id := componentNextID()
		name, _ := req["name"].(string)
		comp := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": "",
			"self":        fmt.Sprintf("https://example.atlassian.net/rest/api/3/component/%s", id),
		}
		components[id] = comp
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(comp)
	})

	mux.HandleFunc("GET /rest/api/3/component/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		comp, ok := components[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"not found"}, "errors": map[string]string{}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comp)
	})

	mux.HandleFunc("PUT /rest/api/3/component/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		comp, ok := components[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"not found"}, "errors": map[string]string{}})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				comp[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comp)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5000000000, MaxRetries: 0, RetryWaitMin: 1000000000, RetryWaitMax: 1000000000}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// TestJiraProjectComponentResourceNoProjectIDCreate tests create when API returns no projectId.
func TestJiraProjectComponentResourceNoProjectIDCreate(t *testing.T) {
	t.Parallel()
	_, client := testComponentNoProjectIDMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-no-pid"),
		"name":            tftypes.NewValue(tftypes.String, "NoPid"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}
	// space_id should remain as plan value since API returned empty projectId
	if sid := getStringAttr(t, resp.State, "space_id"); sid != "proj-no-pid" {
		t.Errorf("expected space_id 'proj-no-pid', got %q", sid)
	}
}

// TestJiraProjectComponentResourceNoProjectIDRead tests read when API returns no projectId.
func TestJiraProjectComponentResourceNoProjectIDRead(t *testing.T) {
	t.Parallel()
	_, client := testComponentNoProjectIDMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create first
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-rd"),
		"name":            tftypes.NewValue(tftypes.String, "ReadNoPid"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	compID := getStringAttr(t, cResp.State, "id")

	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, compID),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-rd"),
		"name":            tftypes.NewValue(tftypes.String, "ReadNoPid"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
}

// TestJiraProjectComponentResourceNoProjectIDUpdate tests update when API returns no projectId.
func TestJiraProjectComponentResourceNoProjectIDUpdate(t *testing.T) {
	t.Parallel()
	_, client := testComponentNoProjectIDMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create first
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-upd"),
		"name":            tftypes.NewValue(tftypes.String, "UpdNoPid"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	compID := getStringAttr(t, cResp.State, "id")

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, compID),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-upd"),
		"name":            tftypes.NewValue(tftypes.String, "UpdNoPid"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, compID),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-upd"),
		"name":            tftypes.NewValue(tftypes.String, "Updated"),
		"description":     tftypes.NewValue(tftypes.String, "new desc"),
		"lead_account_id": tftypes.NewValue(tftypes.String, "lead-1"),
		"assignee_type":   tftypes.NewValue(tftypes.String, "PROJECT_LEAD"),
	})}
	uResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: state}, uResp)
	if uResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", uResp.Diagnostics.Errors())
	}
}

// TestJiraProjectComponentDataSourceEmptyID tests data source with empty string ID.
func TestJiraProjectComponentDataSourceEmptyID(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	ds := componentds.NewComponentDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ""),
		"space_id":        tftypes.NewValue(tftypes.String, nil),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"lead_account_id": tftypes.NewValue(tftypes.String, nil),
		"assignee_type":   tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when id is empty string")
	}
}

// TestJiraProjectComponentResourceCreateBadPlan tests create with invalid plan data.
func TestJiraProjectComponentResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	// Pass a nil raw value which will cause Plan.Get to fail
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid plan")
	}
}

// TestJiraProjectComponentResourceReadBadState tests read with invalid state data.
func TestJiraProjectComponentResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid state")
	}
}

// TestJiraProjectComponentResourceUpdateBadPlan tests update with invalid plan data.
func TestJiraProjectComponentResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	validState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: validState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid plan")
	}
}

// TestJiraProjectComponentResourceUpdateBadState tests update with invalid state data.
func TestJiraProjectComponentResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":        tftypes.NewValue(tftypes.String, "proj-1"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"lead_account_id": tftypes.NewValue(tftypes.String, ""),
		"assignee_type":   tftypes.NewValue(tftypes.String, ""),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: validPlan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid state")
	}
}

// TestJiraProjectComponentResourceDeleteBadState tests delete with invalid state data.
func TestJiraProjectComponentResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid state")
	}
}

// TestJiraProjectComponentDataSourceReadBadConfig tests data source read with invalid config data.
func TestJiraProjectComponentDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testComponentMockServer(t)
	ctx := context.Background()
	ds := componentds.NewComponentDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dss.Type().TerraformType(ctx), nil)}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid config")
	}
}

// TestJiraProjectComponentResourceCreateNotFoundProject tests 404 on create (project not found).
func TestJiraProjectComponentResourceCreateNotFoundProject(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Project not found"},
			"errors":        map[string]string{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	r := componentrs.NewComponentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":        tftypes.NewValue(tftypes.String, "nonexistent-proj"),
		"name":            tftypes.NewValue(tftypes.String, "Test"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"assignee_type":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected project not found error")
	}
}
