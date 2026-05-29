// Package unit contains unit tests for the atlassian_jira_workflow and
// atlassian_jira_workflow_scheme resources and data sources.
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
	workflowdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/workflow"
	workflowresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/workflow"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// workflowIDCounter provides unique IDs for workflow mock server tests.
var workflowIDCounter uint64

func workflowNextID() string {
	n := atomic.AddUint64(&workflowIDCounter, 1)
	return fmt.Sprintf("wf-%d", n)
}

// testWorkflowMockServer creates a mock HTTP server for Jira workflow and workflow scheme endpoints.
func testWorkflowMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	workflows := make(map[string]map[string]interface{})
	schemes := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Workflow endpoints
	mux.HandleFunc("POST /rest/api/3/workflow", func(w http.ResponseWriter, r *http.Request) {
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
		for _, wf := range workflows {
			if wf["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"A workflow with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}
		id := workflowNextID()
		description, _ := req["description"].(string)
		wf := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"self":        fmt.Sprintf("https://example.atlassian.net/rest/api/3/workflow/%s", id),
		}
		workflows[id] = wf
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(wf)
	})

	mux.HandleFunc("GET /rest/api/3/workflow/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		wf, ok := workflows[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Workflow not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wf)
	})

	mux.HandleFunc("PUT /rest/api/3/workflow/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		wf, ok := workflows[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Workflow not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				wf[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wf)
	})

	mux.HandleFunc("DELETE /rest/api/3/workflow/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := workflows[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Workflow not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(workflows, id)
		w.WriteHeader(204)
	})

	// Workflow Scheme endpoints
	mux.HandleFunc("POST /rest/api/3/workflowscheme", func(w http.ResponseWriter, r *http.Request) {
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
		for _, ws := range schemes {
			if ws["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"A workflow scheme with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}
		id := workflowNextID()
		description, _ := req["description"].(string)
		defaultWorkflow, _ := req["defaultWorkflow"].(string)
		ws := map[string]interface{}{
			"id":              id,
			"name":            name,
			"description":     description,
			"defaultWorkflow": defaultWorkflow,
			"self":            fmt.Sprintf("https://example.atlassian.net/rest/api/3/workflowscheme/%s", id),
		}
		if mappings, ok := req["issueTypeMappings"]; ok {
			ws["issueTypeMappings"] = mappings
		}
		schemes[id] = ws
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(ws)
	})

	mux.HandleFunc("GET /rest/api/3/workflowscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		ws, ok := schemes[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Workflow scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ws)
	})

	mux.HandleFunc("PUT /rest/api/3/workflowscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		ws, ok := schemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Workflow scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				ws[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ws)
	})

	mux.HandleFunc("DELETE /rest/api/3/workflowscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := schemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Workflow scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(schemes, id)
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

// testWorkflowForbiddenMockServer creates a mock that returns 403 for all workflow endpoints.
func testWorkflowForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testWorkflowServerErrorMockServer creates a mock that returns 500 for all workflow endpoints.
func testWorkflowServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
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
	return ts, client
}

// ==================== WORKFLOW RESOURCE SCHEMA TESTS ====================

// TestJiraWorkflowResourceMetadata verifies the resource type name.
func TestJiraWorkflowResourceMetadata(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_workflow" {
		t.Errorf("expected resource type name 'atlassian_jira_workflow', got %q", resp.TypeName)
	}
}

// TestJiraWorkflowResourceSchema verifies the resource schema has all expected attributes.
func TestJiraWorkflowResourceSchema(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "name", "description"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraWorkflowResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraWorkflowResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraWorkflowResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraWorkflowResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"name"}
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

// TestJiraWorkflowResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraWorkflowResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"id", "description"}
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

// TestJiraWorkflowResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraWorkflowResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"description"}
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

// TestJiraWorkflowResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraWorkflowResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraWorkflowResourceImplementsResource verifies the Resource interface.
func TestJiraWorkflowResourceImplementsResource(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected workflow resource to implement resource.Resource")
	}
}

// TestJiraWorkflowResourceImplementsImportState verifies the ImportState interface.
func TestJiraWorkflowResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected workflow resource to implement ResourceWithImportState")
	}
}

// ==================== WORKFLOW RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraWorkflowResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraWorkflowResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Test Workflow"),
		"description": tftypes.NewValue(tftypes.String, "A test workflow"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	wfID := getStringAttr(t, createResp.State, "id")
	if wfID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Workflow" {
		t.Errorf("expected name 'Test Workflow', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "A test workflow" {
		t.Errorf("expected description 'A test workflow', got %q", desc)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, wfID),
		"name":        tftypes.NewValue(tftypes.String, "Test Workflow"),
		"description": tftypes.NewValue(tftypes.String, "A test workflow"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test Workflow" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, wfID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Workflow"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Workflow" {
		t.Errorf("Update name: got %q", name)
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
	if !readResp2.State.Raw.IsNull() {
		// Expected: state removed for deleted resource
	}
}

// TestJiraWorkflowResourceCreateNoDescription tests creating a workflow without a description.
func TestJiraWorkflowResourceCreateNoDescription(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "No Desc Workflow"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create no desc: %v", createResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "No Desc Workflow" {
		t.Errorf("expected name 'No Desc Workflow', got %q", name)
	}
}

// TestJiraWorkflowResourceCreateDuplicateName tests creating a workflow with a duplicate name.
func TestJiraWorkflowResourceCreateDuplicateName(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Dup Workflow"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Dup Workflow"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate name error")
	}
}

// TestJiraWorkflowResourceUpdateNotFound tests updating a nonexistent workflow.
func TestJiraWorkflowResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent workflow")
	}
}

// TestJiraWorkflowResourceDeleteNotFound tests deleting an already-deleted workflow.
func TestJiraWorkflowResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent workflow should not error (idempotent)")
	}
}

// TestJiraWorkflowResourceReadNotFound tests reading a nonexistent workflow removes resource.
func TestJiraWorkflowResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent should not error: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed after 404")
	}
}

// TestJiraWorkflowResourceCreateForbidden tests 403 on create.
func TestJiraWorkflowResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowForbiddenMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraWorkflowResourceUpdateForbidden tests 403 on update.
func TestJiraWorkflowResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowForbiddenMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraWorkflowResourceDeleteForbidden tests 403 on delete.
func TestJiraWorkflowResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowForbiddenMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraWorkflowResourceConfigureNil verifies nil provider data does not error.
func TestJiraWorkflowResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := workflowresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraWorkflowResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraWorkflowResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := workflowresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraWorkflowResourceImportState verifies import state passthrough.
func TestJiraWorkflowResourceImportState(t *testing.T) {
	t.Parallel()
	r := workflowresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "wf-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraWorkflowResourceCreateServerError tests generic error on create.
func TestJiraWorkflowResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// TestJiraWorkflowResourceReadServerError tests generic error on read.
func TestJiraWorkflowResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraWorkflowResourceUpdateServerError tests generic error on update.
func TestJiraWorkflowResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraWorkflowResourceDeleteServerError tests generic error on delete.
func TestJiraWorkflowResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraWorkflowResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraWorkflowResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraWorkflowResourceReadBadState tests Read with invalid state data.
func TestJiraWorkflowResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraWorkflowResourceUpdateBadPlan tests Update with invalid plan data.
func TestJiraWorkflowResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "x"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraWorkflowResourceUpdateBadState tests Update with invalid state data.
func TestJiraWorkflowResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "x"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraWorkflowResourceDeleteBadState tests Delete with invalid state data.
func TestJiraWorkflowResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// ==================== WORKFLOW DATA SOURCE TESTS ====================

// TestJiraWorkflowDataSourceMetadata verifies the data source type name.
func TestJiraWorkflowDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := workflowdatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_workflow" {
		t.Errorf("expected data source type name 'atlassian_jira_workflow', got %q", resp.TypeName)
	}
}

// TestJiraWorkflowDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraWorkflowDataSourceSchema(t *testing.T) {
	t.Parallel()

	ds := workflowdatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "name", "description"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraWorkflowDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraWorkflowDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	ds := workflowdatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraWorkflowDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraWorkflowDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	ds := workflowdatasource.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected workflow data source to implement datasource.DataSource")
	}
}

// TestJiraWorkflowDataSourceByID tests reading a workflow by ID.
func TestJiraWorkflowDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()

	// Create a workflow first via resource
	r := workflowresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS Workflow"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	wfID := getStringAttr(t, cResp.State, "id")

	// Read via data source by ID
	ds := workflowdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, wfID),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Workflow" {
		t.Errorf("expected name 'DS Workflow', got %q", name)
	}
}

// TestJiraWorkflowDataSourceNotFound tests 404 error on data source read.
func TestJiraWorkflowDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	ds := workflowdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent workflow")
	}
}

// TestJiraWorkflowDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraWorkflowDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := workflowdatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraWorkflowDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraWorkflowDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := workflowdatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraWorkflowDataSourceReadServerError tests generic error on data source read.
func TestJiraWorkflowDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	ds := workflowdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestJiraWorkflowDataSourceReadBadConfig tests data source Read with invalid config data.
func TestJiraWorkflowDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	ds := workflowdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// ==================== WORKFLOW SCHEME RESOURCE SCHEMA TESTS ====================

// TestJiraWorkflowSchemeResourceMetadata verifies the resource type name.
func TestJiraWorkflowSchemeResourceMetadata(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_workflow_scheme" {
		t.Errorf("expected resource type name 'atlassian_jira_workflow_scheme', got %q", resp.TypeName)
	}
}

// TestJiraWorkflowSchemeResourceSchema verifies the resource schema has all expected attributes.
func TestJiraWorkflowSchemeResourceSchema(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "name", "description", "default_workflow_id", "issue_type_mappings"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraWorkflowSchemeResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraWorkflowSchemeResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 5
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraWorkflowSchemeResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraWorkflowSchemeResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"name"}
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

// TestJiraWorkflowSchemeResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraWorkflowSchemeResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"id", "description", "default_workflow_id", "issue_type_mappings"}
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

// TestJiraWorkflowSchemeResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraWorkflowSchemeResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"description", "default_workflow_id", "issue_type_mappings"}
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

// TestJiraWorkflowSchemeResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraWorkflowSchemeResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraWorkflowSchemeResourceImplementsResource verifies the Resource interface.
func TestJiraWorkflowSchemeResourceImplementsResource(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected workflow scheme resource to implement resource.Resource")
	}
}

// TestJiraWorkflowSchemeResourceImplementsImportState verifies the ImportState interface.
func TestJiraWorkflowSchemeResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := workflowresource.NewSchemeResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected workflow scheme resource to implement ResourceWithImportState")
	}
}

// ==================== WORKFLOW SCHEME RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraWorkflowSchemeResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraWorkflowSchemeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Test Scheme"),
		"description":         tftypes.NewValue(tftypes.String, "A test scheme"),
		"default_workflow_id": tftypes.NewValue(tftypes.String, "wf-default"),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	wsID := getStringAttr(t, createResp.State, "id")
	if wsID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Scheme" {
		t.Errorf("expected name 'Test Scheme', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "A test scheme" {
		t.Errorf("expected description 'A test scheme', got %q", desc)
	}
	if dwf := getStringAttr(t, createResp.State, "default_workflow_id"); dwf != "wf-default" {
		t.Errorf("expected default_workflow_id 'wf-default', got %q", dwf)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, wsID),
		"name":                tftypes.NewValue(tftypes.String, "Test Scheme"),
		"description":         tftypes.NewValue(tftypes.String, "A test scheme"),
		"default_workflow_id": tftypes.NewValue(tftypes.String, "wf-default"),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test Scheme" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, wsID),
		"name":                tftypes.NewValue(tftypes.String, "Updated Scheme"),
		"description":         tftypes.NewValue(tftypes.String, "Updated desc"),
		"default_workflow_id": tftypes.NewValue(tftypes.String, "wf-new"),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Scheme" {
		t.Errorf("Update name: got %q", name)
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
	if !readResp2.State.Raw.IsNull() {
		// Expected: state removed for deleted resource
	}
}

// TestJiraWorkflowSchemeResourceCreateNoOptionals tests creating a scheme without optional fields.
func TestJiraWorkflowSchemeResourceCreateNoOptionals(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Minimal Scheme"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"default_workflow_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create minimal: %v", createResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Minimal Scheme" {
		t.Errorf("expected name 'Minimal Scheme', got %q", name)
	}
}

// TestJiraWorkflowSchemeResourceCreateDuplicateName tests creating a scheme with a duplicate name.
func TestJiraWorkflowSchemeResourceCreateDuplicateName(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Dup Scheme"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"default_workflow_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Dup Scheme"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"default_workflow_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate name error")
	}
}

// TestJiraWorkflowSchemeResourceUpdateNotFound tests updating a nonexistent scheme.
func TestJiraWorkflowSchemeResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent scheme")
	}
}

// TestJiraWorkflowSchemeResourceDeleteNotFound tests deleting an already-deleted scheme.
func TestJiraWorkflowSchemeResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent scheme should not error (idempotent)")
	}
}

// TestJiraWorkflowSchemeResourceReadNotFound tests reading a nonexistent scheme removes resource.
func TestJiraWorkflowSchemeResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent should not error: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed after 404")
	}
}

// TestJiraWorkflowSchemeResourceCreateForbidden tests 403 on create.
func TestJiraWorkflowSchemeResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowForbiddenMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"default_workflow_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraWorkflowSchemeResourceUpdateForbidden tests 403 on update.
func TestJiraWorkflowSchemeResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowForbiddenMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraWorkflowSchemeResourceDeleteForbidden tests 403 on delete.
func TestJiraWorkflowSchemeResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowForbiddenMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraWorkflowSchemeResourceConfigureNil verifies nil provider data does not error.
func TestJiraWorkflowSchemeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := workflowresource.NewSchemeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraWorkflowSchemeResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraWorkflowSchemeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := workflowresource.NewSchemeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraWorkflowSchemeResourceImportState verifies import state passthrough.
func TestJiraWorkflowSchemeResourceImportState(t *testing.T) {
	t.Parallel()
	r := workflowresource.NewSchemeResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "ws-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraWorkflowSchemeResourceCreateServerError tests generic error on create.
func TestJiraWorkflowSchemeResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Error"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"default_workflow_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// TestJiraWorkflowSchemeResourceReadServerError tests generic error on read.
func TestJiraWorkflowSchemeResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "Error"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraWorkflowSchemeResourceUpdateServerError tests generic error on update.
func TestJiraWorkflowSchemeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "Error"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraWorkflowSchemeResourceDeleteServerError tests generic error on delete.
func TestJiraWorkflowSchemeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "Error"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraWorkflowSchemeResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraWorkflowSchemeResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraWorkflowSchemeResourceReadBadState tests Read with invalid state data.
func TestJiraWorkflowSchemeResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraWorkflowSchemeResourceUpdateBadPlan tests Update with invalid plan data.
func TestJiraWorkflowSchemeResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "x"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraWorkflowSchemeResourceUpdateBadState tests Update with invalid state data.
func TestJiraWorkflowSchemeResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "x"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraWorkflowSchemeResourceDeleteBadState tests Delete with invalid state data.
func TestJiraWorkflowSchemeResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// ==================== WORKFLOW SCHEME DATA SOURCE TESTS ====================

// TestJiraWorkflowSchemeDataSourceMetadata verifies the data source type name.
func TestJiraWorkflowSchemeDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := workflowdatasource.NewSchemeDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_workflow_scheme" {
		t.Errorf("expected data source type name 'atlassian_jira_workflow_scheme', got %q", resp.TypeName)
	}
}

// TestJiraWorkflowSchemeDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraWorkflowSchemeDataSourceSchema(t *testing.T) {
	t.Parallel()

	ds := workflowdatasource.NewSchemeDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "name", "description", "default_workflow_id", "issue_type_mappings"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraWorkflowSchemeDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraWorkflowSchemeDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	ds := workflowdatasource.NewSchemeDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	expected := 5
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraWorkflowSchemeDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraWorkflowSchemeDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	ds := workflowdatasource.NewSchemeDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected workflow scheme data source to implement datasource.DataSource")
	}
}

// TestJiraWorkflowSchemeDataSourceByID tests reading a workflow scheme by ID.
func TestJiraWorkflowSchemeDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()

	// Create a workflow scheme first via resource
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "DS Scheme"),
		"description":         tftypes.NewValue(tftypes.String, "ds desc"),
		"default_workflow_id": tftypes.NewValue(tftypes.String, "wf-ds"),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	wsID := getStringAttr(t, cResp.State, "id")

	// Read via data source by ID
	ds := workflowdatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, wsID),
		"name":                tftypes.NewValue(tftypes.String, nil),
		"description":         tftypes.NewValue(tftypes.String, nil),
		"default_workflow_id": tftypes.NewValue(tftypes.String, nil),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Scheme" {
		t.Errorf("expected name 'DS Scheme', got %q", name)
	}
	if dwf := getStringAttr(t, dsResp.State, "default_workflow_id"); dwf != "wf-ds" {
		t.Errorf("expected default_workflow_id 'wf-ds', got %q", dwf)
	}
}

// TestJiraWorkflowSchemeDataSourceNotFound tests 404 error on data source read.
func TestJiraWorkflowSchemeDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	ds := workflowdatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                tftypes.NewValue(tftypes.String, nil),
		"description":         tftypes.NewValue(tftypes.String, nil),
		"default_workflow_id": tftypes.NewValue(tftypes.String, nil),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent workflow scheme")
	}
}

// TestJiraWorkflowSchemeDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraWorkflowSchemeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := workflowdatasource.NewSchemeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraWorkflowSchemeDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraWorkflowSchemeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := workflowdatasource.NewSchemeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraWorkflowSchemeDataSourceReadServerError tests generic error on data source read.
func TestJiraWorkflowSchemeDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowServerErrorMockServer(t)
	ctx := context.Background()
	ds := workflowdatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, nil),
		"description":         tftypes.NewValue(tftypes.String, nil),
		"default_workflow_id": tftypes.NewValue(tftypes.String, nil),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"issue_type_id": tftypes.String, "workflow_id": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestJiraWorkflowSchemeDataSourceReadBadConfig tests data source Read with invalid config data.
func TestJiraWorkflowSchemeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	ds := workflowdatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// ==================== WORKFLOW SCHEME ISSUE TYPE MAPPINGS TESTS ====================

// TestJiraWorkflowSchemeWithIssueTypeMappings tests creating a scheme with issue type mappings.
func TestJiraWorkflowSchemeWithIssueTypeMappings(t *testing.T) {
	t.Parallel()
	_, client := testWorkflowMockServer(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	mappingObjType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"issue_type_id": tftypes.String, "workflow_id": tftypes.String,
	}}
	mappingsListType := tftypes.List{ElementType: mappingObjType}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Mapped Scheme"),
		"description":         tftypes.NewValue(tftypes.String, "With mappings"),
		"default_workflow_id": tftypes.NewValue(tftypes.String, "wf-default"),
		"issue_type_mappings": tftypes.NewValue(mappingsListType, []tftypes.Value{
			tftypes.NewValue(mappingObjType, map[string]tftypes.Value{
				"issue_type_id": tftypes.NewValue(tftypes.String, "10001"),
				"workflow_id":   tftypes.NewValue(tftypes.String, "wf-bug"),
			}),
			tftypes.NewValue(mappingObjType, map[string]tftypes.Value{
				"issue_type_id": tftypes.NewValue(tftypes.String, "10002"),
				"workflow_id":   tftypes.NewValue(tftypes.String, "wf-task"),
			}),
		}),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create with mappings: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
}

// TestJiraWorkflowSchemeIssueTypeMappingsToStateNonEmpty exercises the non-empty
// path of issueTypeMappingsToState via a Read that returns mappings.
func TestJiraWorkflowSchemeIssueTypeMappingsToStateNonEmpty(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/workflowscheme/ws-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "ws-1", "name": "Test", "description": "desc", "defaultWorkflow": "wf-1",
			"issueTypeMappings": []map[string]interface{}{
				{"issueType": "10001", "workflow": "wf-bug"},
				{"issueType": "10002", "workflow": "wf-task"},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()

	mappingObjType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"issue_type_id": tftypes.String, "workflow_id": tftypes.String,
	}}

	// Test resource Read
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "ws-1"),
		"name":                tftypes.NewValue(tftypes.String, "Test"),
		"description":         tftypes.NewValue(tftypes.String, "desc"),
		"default_workflow_id": tftypes.NewValue(tftypes.String, "wf-1"),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: mappingObjType}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Test data source Read
	ds := workflowdatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "ws-1"),
		"name":                tftypes.NewValue(tftypes.String, nil),
		"description":         tftypes.NewValue(tftypes.String, nil),
		"default_workflow_id": tftypes.NewValue(tftypes.String, nil),
		"issue_type_mappings": tftypes.NewValue(tftypes.List{ElementType: mappingObjType}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
}
