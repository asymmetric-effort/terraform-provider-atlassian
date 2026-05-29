// Package unit contains unit tests for the atlassian_jira_screen_tab resource and data source.
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
	screendatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/screen"
	screenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/screen"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// tabIDCounter provides unique IDs for screen tab mock server tests.
var tabIDCounter uint64

func tabNextID() int {
	return int(atomic.AddUint64(&tabIDCounter, 1))
}

// testScreenTabMockServer creates a mock HTTP server for Jira screen tab endpoints.
func testScreenTabMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	// tabs: screenId -> tabId -> tab data
	tabs := make(map[string]map[int]map[string]interface{})

	mux := http.NewServeMux()

	// Create tab on screen
	mux.HandleFunc("POST /rest/api/3/screens/{screenId}/tabs", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"name is required"},
			})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if tabs[screenID] == nil {
			tabs[screenID] = make(map[int]map[string]interface{})
		}
		id := tabNextID()
		pos := len(tabs[screenID])
		tab := map[string]interface{}{
			"id":       float64(id),
			"name":     name,
			"position": float64(pos),
		}
		tabs[screenID][id] = tab
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(tab)
	})

	// List tabs on screen
	mux.HandleFunc("GET /rest/api/3/screens/{screenId}/tabs", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		mu.Lock()
		defer mu.Unlock()
		screenTabs, ok := tabs[screenID]
		if !ok || len(screenTabs) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		var result []map[string]interface{}
		for _, tab := range screenTabs {
			result = append(result, tab)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// Update tab
	mux.HandleFunc("PUT /rest/api/3/screens/{screenId}/tabs/{tabId}", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		tabIDStr := r.PathValue("tabId")
		var tabID int
		fmt.Sscanf(tabIDStr, "%d", &tabID)
		mu.Lock()
		defer mu.Unlock()
		screenTabs, ok := tabs[screenID]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen tab not found"},
			})
			return
		}
		tab, ok := screenTabs[tabID]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen tab not found"},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				tab[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tab)
	})

	// Delete tab
	mux.HandleFunc("DELETE /rest/api/3/screens/{screenId}/tabs/{tabId}", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		tabIDStr := r.PathValue("tabId")
		var tabID int
		fmt.Sscanf(tabIDStr, "%d", &tabID)
		mu.Lock()
		defer mu.Unlock()
		screenTabs, ok := tabs[screenID]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen tab not found"},
			})
			return
		}
		if _, ok := screenTabs[tabID]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen tab not found"},
			})
			return
		}
		delete(screenTabs, tabID)
		w.WriteHeader(204)
	})

	// Move tab
	mux.HandleFunc("POST /rest/api/3/screens/{screenId}/tabs/{tabId}/move/{pos}", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		tabIDStr := r.PathValue("tabId")
		posStr := r.PathValue("pos")
		var tabID int
		fmt.Sscanf(tabIDStr, "%d", &tabID)
		var pos int
		fmt.Sscanf(posStr, "%d", &pos)
		mu.Lock()
		defer mu.Unlock()
		screenTabs, ok := tabs[screenID]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen tab not found"},
			})
			return
		}
		tab, ok := screenTabs[tabID]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen tab not found"},
			})
			return
		}
		tab["position"] = float64(pos)
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
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// testScreenTabForbiddenMockServer creates a mock that returns 403 for all screen tab endpoints.
func testScreenTabForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"You do not have permission"},
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

// testScreenTabServerErrorMockServer creates a mock that returns 500 for all screen tab endpoints.
func testScreenTabServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
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

// testScreenTabBadRequestMockServer creates a mock that returns 400 for all screen tab endpoints.
func testScreenTabBadRequestMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Bad request"},
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

// testScreenTabNotFoundMockServer creates a mock that returns 404 for all screen tab endpoints.
func testScreenTabNotFoundMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Not found"},
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

// ==================== SCREEN TAB RESOURCE SCHEMA TESTS ====================

// TestJiraScreenTabResourceMetadata verifies the resource type name.
func TestJiraScreenTabResourceMetadata(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_screen_tab" {
		t.Errorf("expected resource type name 'atlassian_jira_screen_tab', got %q", resp.TypeName)
	}
}

// TestJiraScreenTabResourceSchema verifies the resource schema has all expected attributes.
func TestJiraScreenTabResourceSchema(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "screen_id", "name", "position"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraScreenTabResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraScreenTabResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraScreenTabResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraScreenTabResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	requiredAttrs := []string{"screen_id", "name"}
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

// TestJiraScreenTabResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraScreenTabResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	computedAttrs := []string{"id", "position"}
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

// TestJiraScreenTabResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraScreenTabResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	optionalAttrs := []string{"position"}
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

// TestJiraScreenTabResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraScreenTabResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraScreenTabResourceImplementsResource verifies the Resource interface.
func TestJiraScreenTabResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected screen tab resource to implement resource.Resource")
	}
}

// TestJiraScreenTabResourceImplementsImportState verifies the ImportState interface.
func TestJiraScreenTabResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected screen tab resource to implement ResourceWithImportState")
	}
}

// ==================== SCREEN TAB RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraScreenTabResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraScreenTabResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "Test Tab"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	compositeID := getStringAttr(t, createResp.State, "id")
	if compositeID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Tab" {
		t.Errorf("expected name 'Test Tab', got %q", name)
	}
	if screenID := getStringAttr(t, createResp.State, "screen_id"); screenID != "100" {
		t.Errorf("expected screen_id '100', got %q", screenID)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test Tab" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, compositeID),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "Updated Tab"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Tab" {
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
		t.Error("expected state to be removed after delete")
	}
}

// TestJiraScreenTabResourceCreateWithPosition tests creating a tab with an explicit position.
func TestJiraScreenTabResourceCreateWithPosition(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create first tab
	plan1 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "200"),
		"name":      tftypes.NewValue(tftypes.String, "First Tab"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan1}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("Create first tab: %v", resp1.Diagnostics.Errors())
	}

	// Create second tab with explicit position 0
	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "200"),
		"name":      tftypes.NewValue(tftypes.String, "Second Tab"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if resp2.Diagnostics.HasError() {
		t.Fatalf("Create with position: %v", resp2.Diagnostics.Errors())
	}
	if name := getStringAttr(t, resp2.State, "name"); name != "Second Tab" {
		t.Errorf("expected name 'Second Tab', got %q", name)
	}
}

// TestJiraScreenTabResourceUpdateNotFound tests updating a nonexistent tab.
func TestJiraScreenTabResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/99999"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent screen tab")
	}
}

// TestJiraScreenTabResourceDeleteNotFound tests deleting an already-deleted tab.
func TestJiraScreenTabResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/99999"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent tab should not error (idempotent)")
	}
}

// TestJiraScreenTabResourceReadNotFound tests reading a nonexistent tab removes resource.
func TestJiraScreenTabResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/99999"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
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

// TestJiraScreenTabResourceCreateForbidden tests 403 on create.
func TestJiraScreenTabResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "Forbidden"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraScreenTabResourceCreateNotFound tests 404 on create (screen not found).
func TestJiraScreenTabResourceCreateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabNotFoundMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "99999"),
		"name":      tftypes.NewValue(tftypes.String, "NotFound"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

// TestJiraScreenTabResourceCreateBadRequest tests 400 on create.
func TestJiraScreenTabResourceCreateBadRequest(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabBadRequestMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "BadRequest"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error")
	}
}

// TestJiraScreenTabResourceUpdateForbidden tests 403 on update.
func TestJiraScreenTabResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "Forbidden"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraScreenTabResourceUpdateBadRequest tests 400 on update.
func TestJiraScreenTabResourceUpdateBadRequest(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabBadRequestMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "BadRequest"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on update")
	}
}

// TestJiraScreenTabResourceDeleteForbidden tests 403 on delete.
func TestJiraScreenTabResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "Forbidden"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraScreenTabResourceConfigureNil verifies nil provider data does not error.
func TestJiraScreenTabResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraScreenTabResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraScreenTabResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraScreenTabResourceImportState verifies import state parsing.
func TestJiraScreenTabResourceImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "100/42"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, resp.State, "id"); id != "100/42" {
		t.Errorf("expected id '100/42', got %q", id)
	}
	if screenID := getStringAttr(t, resp.State, "screen_id"); screenID != "100" {
		t.Errorf("expected screen_id '100', got %q", screenID)
	}
}

// TestJiraScreenTabResourceImportStateInvalid verifies invalid import ID errors.
func TestJiraScreenTabResourceImportStateInvalid(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()

	testCases := []string{"", "noslash", "/", "a/", "/b"}
	for _, tc := range testCases {
		resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
		r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: tc}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("Expected error for import ID %q", tc)
		}
	}
}

// TestJiraScreenTabResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraScreenTabResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraScreenTabResourceReadBadState tests Read with invalid state data.
func TestJiraScreenTabResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraScreenTabResourceUpdateBadPlan tests Update with invalid plan data.
func TestJiraScreenTabResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraScreenTabResourceUpdateBadState tests Update with invalid state data.
func TestJiraScreenTabResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraScreenTabResourceDeleteBadState tests Delete with invalid state data.
func TestJiraScreenTabResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// TestJiraScreenTabResourceCreateServerError tests generic server error on create.
func TestJiraScreenTabResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "ServerError"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestJiraScreenTabResourceReadServerError tests generic server error on read.
func TestJiraScreenTabResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected server error on read")
	}
}

// TestJiraScreenTabResourceUpdateServerError tests generic server error on update.
func TestJiraScreenTabResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on update")
	}
}

// TestJiraScreenTabResourceDeleteServerError tests generic server error on delete.
func TestJiraScreenTabResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on delete")
	}
}

// TestJiraScreenTabResourceReadNotFoundScreen tests Read when screen returns 404 removes resource.
func TestJiraScreenTabResourceReadNotFoundScreen(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabNotFoundMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "99999/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "99999"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent screen should not error: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed after 404")
	}
}

// ==================== SCREEN TAB DATA SOURCE TESTS ====================

// TestJiraScreenTabDataSourceMetadata verifies the data source type name.
func TestJiraScreenTabDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewTabDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_screen_tab" {
		t.Errorf("expected data source type name 'atlassian_jira_screen_tab', got %q", resp.TypeName)
	}
}

// TestJiraScreenTabDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraScreenTabDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewTabDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "screen_id", "name", "position"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraScreenTabDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraScreenTabDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewTabDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraScreenTabDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraScreenTabDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewTabDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected screen tab data source to implement datasource.DataSource")
	}
}

// TestJiraScreenTabDataSourceByID tests reading a screen tab by ID.
func TestJiraScreenTabDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()

	// Create a tab first via resource
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "300"),
		"name":      tftypes.NewValue(tftypes.String, "DS Tab"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	compositeID := getStringAttr(t, cResp.State, "id")
	// Extract tab ID from composite (screenId/tabId)
	parts := splitCompositeID(compositeID)
	tabID := parts[1]

	// Read via data source
	ds := screendatasource.NewTabDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tabID),
		"screen_id": tftypes.NewValue(tftypes.String, "300"),
		"name":      tftypes.NewValue(tftypes.String, nil),
		"position":  tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Tab" {
		t.Errorf("expected name 'DS Tab', got %q", name)
	}
}

// splitCompositeID splits a composite ID like "screenId/tabId" into parts.
func splitCompositeID(id string) []string {
	parts := make([]string, 0)
	current := ""
	for _, c := range id {
		if c == '/' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	parts = append(parts, current)
	return parts
}

// TestJiraScreenTabDataSourceNotFound tests 404 on data source read (tab not found).
func TestJiraScreenTabDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewTabDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "99999"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, nil),
		"position":  tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent screen tab")
	}
}

// TestJiraScreenTabDataSourceScreenNotFound tests 404 when screen not found on data source read.
func TestJiraScreenTabDataSourceScreenNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabNotFoundMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewTabDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "1"),
		"screen_id": tftypes.NewValue(tftypes.String, "99999"),
		"name":      tftypes.NewValue(tftypes.String, nil),
		"position":  tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for screen not found")
	}
}

// TestJiraScreenTabDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraScreenTabDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewTabDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraScreenTabDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraScreenTabDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewTabDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraScreenTabDataSourceReadBadConfig tests data source Read with invalid config data.
func TestJiraScreenTabDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewTabDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// TestJiraScreenTabDataSourceReadServerError tests generic error on data source read.
func TestJiraScreenTabDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabServerErrorMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewTabDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "1"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, nil),
		"position":  tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestJiraScreenTabResourceReadInvalidCompositeID tests Read with a bad composite ID.
func TestJiraScreenTabResourceReadInvalidCompositeID(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "noslash"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid composite ID in Read")
	}
}

// TestJiraScreenTabResourceUpdateInvalidCompositeID tests Update with a bad composite ID.
func TestJiraScreenTabResourceUpdateInvalidCompositeID(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "noslash"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid composite ID in Update")
	}
}

// TestJiraScreenTabResourceDeleteInvalidCompositeID tests Delete with a bad composite ID.
func TestJiraScreenTabResourceDeleteInvalidCompositeID(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "noslash"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"name":      tftypes.NewValue(tftypes.String, "X"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid composite ID in Delete")
	}
}

// TestJiraScreenTabResourceCreateWithMovePosition tests creating a tab and moving it.
func TestJiraScreenTabResourceCreateWithMovePosition(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create first tab on screen 400
	plan1 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "400"),
		"name":      tftypes.NewValue(tftypes.String, "Tab A"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan1}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("Create tab A: %v", resp1.Diagnostics.Errors())
	}

	// Create second tab with position 0 (different from default which would be 1)
	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "400"),
		"name":      tftypes.NewValue(tftypes.String, "Tab B"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if resp2.Diagnostics.HasError() {
		t.Fatalf("Create tab B with move: %v", resp2.Diagnostics.Errors())
	}
}

// TestJiraScreenTabResourceUpdateWithMovePosition tests updating a tab and moving it.
func TestJiraScreenTabResourceUpdateWithMovePosition(t *testing.T) {
	t.Parallel()
	_, client := testScreenTabMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create tab
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "500"),
		"name":      tftypes.NewValue(tftypes.String, "Move Tab"),
		"position":  tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Update with new position (different from the server-returned position)
	compositeID := getStringAttr(t, cResp.State, "id")
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, compositeID),
		"screen_id": tftypes.NewValue(tftypes.String, "500"),
		"name":      tftypes.NewValue(tftypes.String, "Moved Tab"),
		"position":  tftypes.NewValue(tftypes.Number, 5),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: cResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update with move: %v", updateResp.Diagnostics.Errors())
	}
}

// TestJiraScreenTabResourceCreateMoveError tests move failure after tab creation.
func TestJiraScreenTabResourceCreateMoveError(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	tabs := make(map[string]map[int]map[string]interface{})
	mux := http.NewServeMux()

	// Create succeeds
	mux.HandleFunc("POST /rest/api/3/screens/{screenId}/tabs", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		if tabs[screenID] == nil {
			tabs[screenID] = make(map[int]map[string]interface{})
		}
		id := tabNextID()
		tab := map[string]interface{}{
			"id":       float64(id),
			"name":     req["name"],
			"position": float64(1),
		}
		tabs[screenID][id] = tab
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(tab)
	})

	// Move fails with 500
	mux.HandleFunc("POST /rest/api/3/screens/{screenId}/tabs/{tabId}/move/{pos}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Move failed"},
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
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "600"),
		"name":      tftypes.NewValue(tftypes.String, "Move Fail"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from move failure")
	}
}

// TestJiraScreenTabResourceUpdateMoveError tests move failure after tab update.
func TestJiraScreenTabResourceUpdateMoveError(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	tabs := make(map[string]map[int]map[string]interface{})
	mux := http.NewServeMux()

	// Create succeeds
	mux.HandleFunc("POST /rest/api/3/screens/{screenId}/tabs", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		if tabs[screenID] == nil {
			tabs[screenID] = make(map[int]map[string]interface{})
		}
		id := tabNextID()
		tab := map[string]interface{}{
			"id":       float64(id),
			"name":     req["name"],
			"position": float64(0),
		}
		tabs[screenID][id] = tab
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(tab)
	})

	// Update succeeds but returns position 0
	mux.HandleFunc("PUT /rest/api/3/screens/{screenId}/tabs/{tabId}", func(w http.ResponseWriter, r *http.Request) {
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		tab := map[string]interface{}{
			"id":       float64(1),
			"name":     updates["name"],
			"position": float64(0),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tab)
	})

	// Move fails with 500
	mux.HandleFunc("POST /rest/api/3/screens/{screenId}/tabs/{tabId}/move/{pos}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Move failed"},
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
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := screenresource.NewTabResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "700/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "700"),
		"name":      tftypes.NewValue(tftypes.String, "Tab"),
		"position":  tftypes.NewValue(tftypes.Number, 0),
	})}
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "700/1"),
		"screen_id": tftypes.NewValue(tftypes.String, "700"),
		"name":      tftypes.NewValue(tftypes.String, "Updated"),
		"position":  tftypes.NewValue(tftypes.Number, 5),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from move failure on update")
	}
}

// TestJiraScreenTabDataSourceSchemaRequiredAttributes verifies required attributes.
func TestJiraScreenTabDataSourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewTabDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	requiredAttrs := []string{"id", "screen_id"}
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

// TestJiraScreenTabDataSourceSchemaComputedAttributes verifies computed attributes.
func TestJiraScreenTabDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewTabDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	computedAttrs := []string{"name", "position"}
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
