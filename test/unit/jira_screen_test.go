// Package unit contains unit tests for the atlassian_jira_screen, screen_scheme,
// and screen_tab_field resources and data sources.
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

// screenIDCounter provides unique IDs for screen mock server tests.
var screenIDCounter uint64

func screenNextID() int {
	return int(atomic.AddUint64(&screenIDCounter, 1))
}

// testScreenMockServer creates a mock HTTP server for Jira screen, screen scheme,
// and screen tab field endpoints.
func testScreenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	screens := make(map[int]map[string]interface{})
	schemes := make(map[int]map[string]interface{})
	// tabFields: screenID -> tabID -> []fieldID
	tabFields := make(map[string]map[string][]string)

	mux := http.NewServeMux()

	// Create screen
	mux.HandleFunc("POST /rest/api/3/screens", func(w http.ResponseWriter, r *http.Request) {
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
		id := screenNextID()
		description, _ := req["description"].(string)
		screen := map[string]interface{}{
			"id":          float64(id),
			"name":        name,
			"description": description,
		}
		screens[id] = screen
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(screen)
	})

	// Read screen by ID
	mux.HandleFunc("GET /rest/api/3/screens/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		mu.Lock()
		defer mu.Unlock()
		screen, ok := screens[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(screen)
	})

	// Update screen
	mux.HandleFunc("PUT /rest/api/3/screens/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		mu.Lock()
		defer mu.Unlock()
		screen, ok := screens[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen not found"},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				screen[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(screen)
	})

	// Delete screen
	mux.HandleFunc("DELETE /rest/api/3/screens/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		mu.Lock()
		defer mu.Unlock()
		if _, ok := screens[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen not found"},
			})
			return
		}
		delete(screens, id)
		w.WriteHeader(204)
	})

	// Create screen scheme
	mux.HandleFunc("POST /rest/api/3/screenscheme", func(w http.ResponseWriter, r *http.Request) {
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
		id := screenNextID()
		description, _ := req["description"].(string)
		scheme := map[string]interface{}{
			"id":          float64(id),
			"name":        name,
			"description": description,
		}
		schemes[id] = scheme
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(scheme)
	})

	// Read screen scheme by ID
	mux.HandleFunc("GET /rest/api/3/screenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		mu.Lock()
		defer mu.Unlock()
		scheme, ok := schemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen scheme not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(scheme)
	})

	// Update screen scheme
	mux.HandleFunc("PUT /rest/api/3/screenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		mu.Lock()
		defer mu.Unlock()
		scheme, ok := schemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen scheme not found"},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				scheme[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(scheme)
	})

	// Delete screen scheme
	mux.HandleFunc("DELETE /rest/api/3/screenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)
		mu.Lock()
		defer mu.Unlock()
		if _, ok := schemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Screen scheme not found"},
			})
			return
		}
		delete(schemes, id)
		w.WriteHeader(204)
	})

	// Add field to screen tab
	mux.HandleFunc("POST /rest/api/3/screens/{screenId}/tabs/{tabId}/fields", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		tabID := r.PathValue("tabId")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		fieldID, _ := req["fieldId"].(string)
		if fieldID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"fieldId is required"},
			})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if tabFields[screenID] == nil {
			tabFields[screenID] = make(map[string][]string)
		}
		tabFields[screenID][tabID] = append(tabFields[screenID][tabID], fieldID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   fieldID,
			"name": "Field " + fieldID,
		})
	})

	// List fields on screen tab
	mux.HandleFunc("GET /rest/api/3/screens/{screenId}/tabs/{tabId}/fields", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		tabID := r.PathValue("tabId")
		mu.Lock()
		defer mu.Unlock()
		fields, ok := tabFields[screenID]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		fieldList, ok := fields[tabID]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{})
			return
		}
		var result []map[string]interface{}
		for _, fid := range fieldList {
			result = append(result, map[string]interface{}{
				"id":   fid,
				"name": "Field " + fid,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// Delete field from screen tab
	mux.HandleFunc("DELETE /rest/api/3/screens/{screenId}/tabs/{tabId}/fields/{fieldId}", func(w http.ResponseWriter, r *http.Request) {
		screenID := r.PathValue("screenId")
		tabID := r.PathValue("tabId")
		fieldID := r.PathValue("fieldId")
		mu.Lock()
		defer mu.Unlock()
		if screenFields, ok := tabFields[screenID]; ok {
			if fieldList, ok := screenFields[tabID]; ok {
				for i, fid := range fieldList {
					if fid == fieldID {
						screenFields[tabID] = append(fieldList[:i], fieldList[i+1:]...)
						w.WriteHeader(204)
						return
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Field not found on screen tab"},
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

// testScreenForbiddenMockServer creates a mock that returns 403 for all screen endpoints.
func testScreenForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testScreenServerErrorMockServer creates a mock that returns 500 for all screen endpoints.
func testScreenServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// ==================== SCREEN RESOURCE SCHEMA TESTS ====================

// TestJiraScreenResourceMetadata verifies the resource type name.
func TestJiraScreenResourceMetadata(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_screen" {
		t.Errorf("expected resource type name 'atlassian_jira_screen', got %q", resp.TypeName)
	}
}

// TestJiraScreenResourceSchema verifies the resource schema has all expected attributes.
func TestJiraScreenResourceSchema(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
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

// TestJiraScreenResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraScreenResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraScreenResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraScreenResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
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

// TestJiraScreenResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraScreenResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
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

// TestJiraScreenResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraScreenResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
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

// TestJiraScreenResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraScreenResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraScreenResourceImplementsResource verifies the Resource interface.
func TestJiraScreenResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected screen resource to implement resource.Resource")
	}
}

// TestJiraScreenResourceImplementsImportState verifies the ImportState interface.
func TestJiraScreenResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected screen resource to implement ResourceWithImportState")
	}
}

// ==================== SCREEN RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraScreenResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraScreenResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Test Screen"),
		"description": tftypes.NewValue(tftypes.String, "A test screen"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	screenID := getStringAttr(t, createResp.State, "id")
	if screenID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Screen" {
		t.Errorf("expected name 'Test Screen', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "A test screen" {
		t.Errorf("expected description 'A test screen', got %q", desc)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, screenID),
		"name":        tftypes.NewValue(tftypes.String, "Test Screen"),
		"description": tftypes.NewValue(tftypes.String, "A test screen"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test Screen" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, screenID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Screen"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Screen" {
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
		t.Error("expected state to be removed after 404")
	}
}

// TestJiraScreenResourceCreateNoDescription tests creating a screen without description.
func TestJiraScreenResourceCreateNoDescription(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "No Desc Screen"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "No Desc Screen" {
		t.Errorf("expected name 'No Desc Screen', got %q", name)
	}
}

// TestJiraScreenResourceUpdateNotFound tests updating a nonexistent screen.
func TestJiraScreenResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "99999"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent screen")
	}
}

// TestJiraScreenResourceDeleteNotFound tests deleting an already-deleted screen.
func TestJiraScreenResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "99999"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent screen should not error (idempotent)")
	}
}

// TestJiraScreenResourceReadNotFound tests reading a nonexistent screen removes resource.
func TestJiraScreenResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "99999"),
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

// TestJiraScreenResourceCreateForbidden tests 403 on create.
func TestJiraScreenResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
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

// TestJiraScreenResourceUpdateForbidden tests 403 on update.
func TestJiraScreenResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
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

// TestJiraScreenResourceDeleteForbidden tests 403 on delete.
func TestJiraScreenResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraScreenResourceConfigureNil verifies nil provider data does not error.
func TestJiraScreenResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraScreenResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraScreenResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraScreenResourceImportState verifies import state passthrough.
func TestJiraScreenResourceImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraScreenResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraScreenResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraScreenResourceReadBadState tests Read with invalid state data.
func TestJiraScreenResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraScreenResourceUpdateBadPlan tests Update with invalid plan data.
func TestJiraScreenResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraScreenResourceUpdateBadState tests Update with invalid state data.
func TestJiraScreenResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
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

// TestJiraScreenResourceDeleteBadState tests Delete with invalid state data.
func TestJiraScreenResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// TestJiraScreenResourceCreateServerError tests generic error on create.
func TestJiraScreenResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
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

// TestJiraScreenResourceReadServerError tests generic error on read.
func TestJiraScreenResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraScreenResourceUpdateServerError tests generic error on update.
func TestJiraScreenResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
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

// TestJiraScreenResourceDeleteServerError tests generic error on delete.
func TestJiraScreenResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// ==================== SCREEN SCHEME RESOURCE SCHEMA TESTS ====================

// TestJiraScreenSchemeResourceMetadata verifies the resource type name.
func TestJiraScreenSchemeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := screenresource.NewSchemeResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_screen_scheme" {
		t.Errorf("expected resource type name 'atlassian_jira_screen_scheme', got %q", resp.TypeName)
	}
}

// TestJiraScreenSchemeResourceSchema verifies the resource schema has all expected attributes.
func TestJiraScreenSchemeResourceSchema(t *testing.T) {
	t.Parallel()
	r := screenresource.NewSchemeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
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

// TestJiraScreenSchemeResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraScreenSchemeResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := screenresource.NewSchemeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraScreenSchemeResourceImplementsResource verifies the Resource interface.
func TestJiraScreenSchemeResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := screenresource.NewSchemeResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected screen scheme resource to implement resource.Resource")
	}
}

// TestJiraScreenSchemeResourceImplementsImportState verifies the ImportState interface.
func TestJiraScreenSchemeResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewSchemeResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected screen scheme resource to implement ResourceWithImportState")
	}
}

// ==================== SCREEN SCHEME RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraScreenSchemeResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraScreenSchemeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Test Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A test scheme"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	schemeID := getStringAttr(t, createResp.State, "id")
	if schemeID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Scheme" {
		t.Errorf("expected name 'Test Scheme', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "A test scheme" {
		t.Errorf("expected description 'A test scheme', got %q", desc)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, schemeID),
		"name":        tftypes.NewValue(tftypes.String, "Test Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A test scheme"),
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
		"id":          tftypes.NewValue(tftypes.String, schemeID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Scheme"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
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
		t.Error("expected state to be removed after 404")
	}
}

// TestJiraScreenSchemeResourceCreateNoDescription tests creating a scheme without description.
func TestJiraScreenSchemeResourceCreateNoDescription(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "No Desc"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
}

// TestJiraScreenSchemeResourceUpdateNotFound tests updating a nonexistent scheme.
func TestJiraScreenSchemeResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "99999"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent scheme")
	}
}

// TestJiraScreenSchemeResourceDeleteNotFound tests deleting an already-deleted scheme.
func TestJiraScreenSchemeResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "99999"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent scheme should not error (idempotent)")
	}
}

// TestJiraScreenSchemeResourceReadNotFound tests reading a nonexistent scheme removes resource.
func TestJiraScreenSchemeResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "99999"),
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

// TestJiraScreenSchemeResourceCreateForbidden tests 403 on create.
func TestJiraScreenSchemeResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
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

// TestJiraScreenSchemeResourceUpdateForbidden tests 403 on update.
func TestJiraScreenSchemeResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
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

// TestJiraScreenSchemeResourceDeleteForbidden tests 403 on delete.
func TestJiraScreenSchemeResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraScreenSchemeResourceConfigureNil verifies nil provider data does not error.
func TestJiraScreenSchemeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := screenresource.NewSchemeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraScreenSchemeResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraScreenSchemeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := screenresource.NewSchemeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraScreenSchemeResourceImportState verifies import state passthrough.
func TestJiraScreenSchemeResourceImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewSchemeResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "456"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraScreenSchemeResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraScreenSchemeResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraScreenSchemeResourceReadBadState tests Read with invalid state data.
func TestJiraScreenSchemeResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraScreenSchemeResourceUpdateBadPlan tests Update with invalid plan data.
func TestJiraScreenSchemeResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraScreenSchemeResourceUpdateBadState tests Update with invalid state data.
func TestJiraScreenSchemeResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
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

// TestJiraScreenSchemeResourceDeleteBadState tests Delete with invalid state data.
func TestJiraScreenSchemeResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// TestJiraScreenSchemeResourceCreateServerError tests generic error on create.
func TestJiraScreenSchemeResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
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

// TestJiraScreenSchemeResourceReadServerError tests generic error on read.
func TestJiraScreenSchemeResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraScreenSchemeResourceUpdateServerError tests generic error on update.
func TestJiraScreenSchemeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
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

// TestJiraScreenSchemeResourceDeleteServerError tests generic error on delete.
func TestJiraScreenSchemeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// ==================== TAB FIELD RESOURCE SCHEMA TESTS ====================

// TestJiraScreenTabFieldResourceMetadata verifies the resource type name.
func TestJiraScreenTabFieldResourceMetadata(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_screen_tab_field" {
		t.Errorf("expected resource type name 'atlassian_jira_screen_tab_field', got %q", resp.TypeName)
	}
}

// TestJiraScreenTabFieldResourceSchema verifies the resource schema has all expected attributes.
func TestJiraScreenTabFieldResourceSchema(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "screen_id", "tab_id", "field_id"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraScreenTabFieldResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraScreenTabFieldResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraScreenTabFieldResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraScreenTabFieldResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	requiredAttrs := []string{"screen_id", "tab_id", "field_id"}
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

// TestJiraScreenTabFieldResourceImplementsResource verifies the Resource interface.
func TestJiraScreenTabFieldResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected tab field resource to implement resource.Resource")
	}
}

// TestJiraScreenTabFieldResourceImplementsImportState verifies the ImportState interface.
func TestJiraScreenTabFieldResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected tab field resource to implement ResourceWithImportState")
	}
}

// ==================== TAB FIELD RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraScreenTabFieldResourceCRUDLifecycle tests the full create-read-delete cycle.
func TestJiraScreenTabFieldResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"tab_id":    tftypes.NewValue(tftypes.String, "200"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_10001"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	compositeID := getStringAttr(t, createResp.State, "id")
	if compositeID != "100/200/customfield_10001" {
		t.Errorf("expected composite id '100/200/customfield_10001', got %q", compositeID)
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
	if fieldID := getStringAttr(t, readResp.State, "field_id"); fieldID != "customfield_10001" {
		t.Errorf("Read field_id: got %q", fieldID)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: readResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete should remove resource (field not found in list)
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp2)
	if !readResp2.State.Raw.IsNull() {
		t.Error("expected state to be removed after field deleted")
	}
}

// TestJiraScreenTabFieldResourceDeleteNotFound tests deleting an already-deleted field.
func TestJiraScreenTabFieldResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/200/nonexistent"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"tab_id":    tftypes.NewValue(tftypes.String, "200"),
		"field_id":  tftypes.NewValue(tftypes.String, "nonexistent"),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent tab field should not error (idempotent)")
	}
}

// TestJiraScreenTabFieldResourceReadFieldNotInList tests reading when field is not in the list.
func TestJiraScreenTabFieldResourceReadFieldNotInList(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/200/missing_field"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"tab_id":    tftypes.NewValue(tftypes.String, "200"),
		"field_id":  tftypes.NewValue(tftypes.String, "missing_field"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read should not error: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed when field not in list")
	}
}

// TestJiraScreenTabFieldResourceCreateForbidden tests 403 on create.
func TestJiraScreenTabFieldResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"tab_id":    tftypes.NewValue(tftypes.String, "200"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_10001"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraScreenTabFieldResourceDeleteForbidden tests 403 on delete.
func TestJiraScreenTabFieldResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testScreenForbiddenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/200/customfield_10001"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"tab_id":    tftypes.NewValue(tftypes.String, "200"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_10001"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraScreenTabFieldResourceConfigureNil verifies nil provider data does not error.
func TestJiraScreenTabFieldResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraScreenTabFieldResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraScreenTabFieldResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraScreenTabFieldResourceImportState verifies import state with composite ID.
func TestJiraScreenTabFieldResourceImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "100/200/customfield_10001"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
	if screenID := getStringAttr(t, resp.State, "screen_id"); screenID != "100" {
		t.Errorf("expected screen_id '100', got %q", screenID)
	}
	if tabID := getStringAttr(t, resp.State, "tab_id"); tabID != "200" {
		t.Errorf("expected tab_id '200', got %q", tabID)
	}
	if fieldID := getStringAttr(t, resp.State, "field_id"); fieldID != "customfield_10001" {
		t.Errorf("expected field_id 'customfield_10001', got %q", fieldID)
	}
}

// TestJiraScreenTabFieldResourceImportStateBadID verifies import state with invalid composite ID.
func TestJiraScreenTabFieldResourceImportStateBadID(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()

	badIDs := []string{"", "onlyOne", "two/parts", "///empty"}
	for _, badID := range badIDs {
		resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
		r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: badID}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("Expected error for bad import ID %q", badID)
		}
	}
}

// TestJiraScreenTabFieldResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraScreenTabFieldResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraScreenTabFieldResourceReadBadState tests Read with invalid state data.
func TestJiraScreenTabFieldResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraScreenTabFieldResourceDeleteBadState tests Delete with invalid state data.
func TestJiraScreenTabFieldResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// TestJiraScreenTabFieldResourceCreateServerError tests generic error on create.
func TestJiraScreenTabFieldResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"tab_id":    tftypes.NewValue(tftypes.String, "200"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_10001"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// TestJiraScreenTabFieldResourceReadServerError tests generic error on read.
func TestJiraScreenTabFieldResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/200/customfield_10001"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"tab_id":    tftypes.NewValue(tftypes.String, "200"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_10001"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraScreenTabFieldResourceDeleteServerError tests generic error on delete.
func TestJiraScreenTabFieldResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "100/200/customfield_10001"),
		"screen_id": tftypes.NewValue(tftypes.String, "100"),
		"tab_id":    tftypes.NewValue(tftypes.String, "200"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_10001"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraScreenTabFieldResourceUpdateAlwaysErrors tests that Update always errors.
func TestJiraScreenTabFieldResourceUpdateAlwaysErrors(t *testing.T) {
	t.Parallel()
	r := screenresource.NewTabFieldResource()
	resp := &resource.UpdateResponse{}
	r.Update(context.Background(), resource.UpdateRequest{}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from Update (all attrs require replace)")
	}
}

// ==================== SCREEN DATA SOURCE TESTS ====================

// TestJiraScreenDataSourceMetadata verifies the data source type name.
func TestJiraScreenDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_screen" {
		t.Errorf("expected data source type name 'atlassian_jira_screen', got %q", resp.TypeName)
	}
}

// TestJiraScreenDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraScreenDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
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

// TestJiraScreenDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraScreenDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraScreenDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraScreenDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected screen data source to implement datasource.DataSource")
	}
}

// TestJiraScreenDataSourceByID tests reading a screen by ID.
func TestJiraScreenDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()

	// Create a screen first via resource
	r := screenresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS Screen"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	screenID := getStringAttr(t, cResp.State, "id")

	// Read via data source
	ds := screendatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, screenID),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Screen" {
		t.Errorf("expected name 'DS Screen', got %q", name)
	}
}

// TestJiraScreenDataSourceNotFound tests 404 on data source read.
func TestJiraScreenDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "99999"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent screen")
	}
}

// TestJiraScreenDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraScreenDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraScreenDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraScreenDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraScreenDataSourceReadBadConfig tests data source Read with invalid config data.
func TestJiraScreenDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// TestJiraScreenDataSourceReadServerError tests generic error on data source read.
func TestJiraScreenDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// ==================== SCREEN SCHEME DATA SOURCE TESTS ====================

// TestJiraScreenSchemeDataSourceMetadata verifies the data source type name.
func TestJiraScreenSchemeDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewSchemeDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_screen_scheme" {
		t.Errorf("expected data source type name 'atlassian_jira_screen_scheme', got %q", resp.TypeName)
	}
}

// TestJiraScreenSchemeDataSourceSchema verifies the data source schema.
func TestJiraScreenSchemeDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewSchemeDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
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

// TestJiraScreenSchemeDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraScreenSchemeDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewSchemeDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraScreenSchemeDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraScreenSchemeDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewSchemeDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected screen scheme data source to implement datasource.DataSource")
	}
}

// TestJiraScreenSchemeDataSourceByID tests reading a screen scheme by ID.
func TestJiraScreenSchemeDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()

	// Create a scheme first via resource
	r := screenresource.NewSchemeResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS Scheme"),
		"description": tftypes.NewValue(tftypes.String, "ds scheme desc"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	schemeID := getStringAttr(t, cResp.State, "id")

	// Read via data source
	ds := screendatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, schemeID),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Scheme" {
		t.Errorf("expected name 'DS Scheme', got %q", name)
	}
}

// TestJiraScreenSchemeDataSourceNotFound tests 404 on data source read.
func TestJiraScreenSchemeDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "99999"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent scheme")
	}
}

// TestJiraScreenSchemeDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraScreenSchemeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewSchemeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraScreenSchemeDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraScreenSchemeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewSchemeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraScreenSchemeDataSourceReadBadConfig tests data source Read with invalid config data.
func TestJiraScreenSchemeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testScreenMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// TestJiraScreenSchemeDataSourceReadServerError tests generic error on data source read.
func TestJiraScreenSchemeDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testScreenServerErrorMockServer(t)
	ctx := context.Background()
	ds := screendatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "1"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}
