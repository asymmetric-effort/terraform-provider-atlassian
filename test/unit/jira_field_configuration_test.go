// Package unit contains unit tests for the atlassian_jira_field_configuration
// and atlassian_jira_field_configuration_scheme resources and data sources.
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
	customfielddatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/custom_field"
	customfieldresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/custom_field"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// fcIDCounter provides unique IDs for field configuration mock server tests.
var fcIDCounter uint64

func fcNextID(prefix string) string {
	n := atomic.AddUint64(&fcIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// testFCMockServer creates a mock HTTP server for field configuration and field configuration scheme endpoints.
func testFCMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	fieldConfigs := make(map[string]map[string]interface{})
	fieldConfigSchemes := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Field configuration endpoints
	mux.HandleFunc("POST /rest/api/3/fieldconfiguration", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"name is required"}})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, fc := range fieldConfigs {
			if fc["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}})
				return
			}
		}
		id := fcNextID("fc")
		description, _ := req["description"].(string)
		fc := map[string]interface{}{"id": id, "name": name, "description": description, "self": fmt.Sprintf("https://example.atlassian.net/rest/api/3/fieldconfiguration/%s", id)}
		fieldConfigs[id] = fc
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(fc)
	})
	mux.HandleFunc("GET /rest/api/3/fieldconfiguration", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		filterID := r.URL.Query().Get("id")
		var values []map[string]interface{}
		for id, fc := range fieldConfigs {
			if filterID == "" || id == filterID {
				values = append(values, fc)
			}
		}
		if values == nil {
			values = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values":     values,
			"maxResults": len(values),
			"startAt":    0,
			"total":      len(values),
			"isLast":     true,
		})
	})
	mux.HandleFunc("PUT /rest/api/3/fieldconfiguration/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		fc, ok := fieldConfigs[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				fc[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fc)
	})
	mux.HandleFunc("DELETE /rest/api/3/fieldconfiguration/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := fieldConfigs[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		delete(fieldConfigs, id)
		w.WriteHeader(204)
	})

	// Field configuration scheme endpoints
	mux.HandleFunc("POST /rest/api/3/fieldconfigurationscheme", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"name is required"}})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, fcs := range fieldConfigSchemes {
			if fcs["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}})
				return
			}
		}
		id := fcNextID("fcs")
		description, _ := req["description"].(string)
		fcs := map[string]interface{}{"id": id, "name": name, "description": description, "self": fmt.Sprintf("https://example.atlassian.net/rest/api/3/fieldconfigurationscheme/%s", id)}
		fieldConfigSchemes[id] = fcs
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(fcs)
	})
	mux.HandleFunc("GET /rest/api/3/fieldconfigurationscheme", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		filterID := r.URL.Query().Get("id")
		var values []map[string]interface{}
		for id, fcs := range fieldConfigSchemes {
			if filterID == "" || id == filterID {
				values = append(values, fcs)
			}
		}
		if values == nil {
			values = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values":     values,
			"maxResults": len(values),
			"startAt":    0,
			"total":      len(values),
			"isLast":     true,
		})
	})
	mux.HandleFunc("PUT /rest/api/3/fieldconfigurationscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		fcs, ok := fieldConfigSchemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				fcs[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fcs)
	})
	mux.HandleFunc("DELETE /rest/api/3/fieldconfigurationscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := fieldConfigSchemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		delete(fieldConfigSchemes, id)
		w.WriteHeader(204)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth := &testNoopAuth{}
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5000000000, MaxRetries: 0, RetryWaitMin: 1000000000, RetryWaitMax: 1000000000}
	client, err := atlassian.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// testFCForbiddenMockServer creates a mock that returns 403 for all endpoints.
func testFCForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Permission denied"}})
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

// testFCServerErrorMockServer creates a mock that returns 500 for all endpoints.
func testFCServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Internal server error"}})
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

// testFCBadRequestMockServer creates a mock that returns 400 for all endpoints.
func testFCBadRequestMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Bad request"}})
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

// ==================== FIELD CONFIGURATION RESOURCE TESTS ====================

// TestFieldConfigurationResourceMetadata verifies the resource type name.
func TestFieldConfigurationResourceMetadata(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_field_configuration" {
		t.Errorf("expected 'atlassian_jira_field_configuration', got %q", resp.TypeName)
	}
}

// TestFieldConfigurationResourceSchema verifies schema attributes.
func TestFieldConfigurationResourceSchema(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, attr := range []string{"id", "name", "description"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(resp.Schema.Attributes) != 3 {
		t.Errorf("expected 3 attributes, got %d", len(resp.Schema.Attributes))
	}
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Error("name should be required")
	}
	if !resp.Schema.Attributes["id"].IsComputed() {
		t.Error("id should be computed")
	}
	if resp.Schema.Attributes["description"].IsRequired() {
		t.Error("description should be optional")
	}
}

// TestFieldConfigurationResourceImplementsInterfaces verifies interfaces.
func TestFieldConfigurationResourceImplementsInterfaces(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

// TestFieldConfigurationResourceCRUDLifecycle tests the full CRUD cycle.
func TestFieldConfigurationResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Test FC"),
		"description": tftypes.NewValue(tftypes.String, "A field configuration"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test FC" {
		t.Errorf("expected name 'Test FC', got %q", name)
	}

	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, id),
		"name":        tftypes.NewValue(tftypes.String, "Test FC"),
		"description": tftypes.NewValue(tftypes.String, "A field configuration"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, id),
		"name":        tftypes.NewValue(tftypes.String, "Updated FC"),
		"description": tftypes.NewValue(tftypes.String, "Updated description"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}

	deleteState := tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

// TestFieldConfigurationResourceErrorPaths tests error scenarios.
func TestFieldConfigurationResourceErrorPaths(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Duplicate
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Dup FC"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate error")
	}

	// Not found on update/delete/read
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	uResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, uResp)
	if !uResp.Diagnostics.HasError() {
		t.Fatal("Expected not found on update")
	}
	dResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, dResp)
	if dResp.Diagnostics.HasError() {
		t.Fatal("Delete nonexistent should not error")
	}
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, rResp)
	if !rResp.State.Raw.IsNull() {
		t.Error("expected state removed")
	}

	// Forbidden
	_, fclient := testFCForbiddenMockServer(t)
	r2 := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r2, fclient)
	fResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r2.Create(ctx, resource.CreateRequest{Plan: plan}, fResp)
	if !fResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden")
	}
	fuResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r2.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, fuResp)
	if !fuResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden on update")
	}
	fdResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r2.Delete(ctx, resource.DeleteRequest{State: state}, fdResp)
	if !fdResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden on delete")
	}

	// Server error
	_, sclient := testFCServerErrorMockServer(t)
	r3 := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r3, sclient)
	seResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r3.Read(ctx, resource.ReadRequest{State: state}, seResp)
	if !seResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
	seCreateResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r3.Create(ctx, resource.CreateRequest{Plan: plan}, seCreateResp)
	if !seCreateResp.Diagnostics.HasError() {
		t.Fatal("Expected server error on create")
	}

	// Bad request
	_, bclient := testFCBadRequestMockServer(t)
	r4 := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r4, bclient)
	brResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r4.Create(ctx, resource.CreateRequest{Plan: plan}, brResp)
	if !brResp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on create")
	}
	brUpdateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r4.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, brUpdateResp)
	if !brUpdateResp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on update")
	}
}

// TestFieldConfigurationResourceUpdateServerError tests 500 on update.
func TestFieldConfigurationResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testFCServerErrorMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on update")
	}
}

// TestFieldConfigurationResourceDeleteServerError tests 500 on delete.
func TestFieldConfigurationResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testFCServerErrorMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on delete")
	}
}

// TestFieldConfigurationResourceImportState verifies import state passthrough.
func TestFieldConfigurationResourceImportState(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "fc-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestFieldConfigurationResourceConfigureNil tests nil provider data.
func TestFieldConfigurationResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestFieldConfigurationResourceConfigureWrongType tests wrong type.
func TestFieldConfigurationResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestFieldConfigurationResourceCreateBadPlan tests Create with invalid plan.
func TestFieldConfigurationResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestFieldConfigurationResourceReadBadState tests Read with invalid state.
func TestFieldConfigurationResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestFieldConfigurationResourceUpdateBadPlan tests Update with invalid plan.
func TestFieldConfigurationResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestFieldConfigurationResourceUpdateBadState tests Update with invalid state.
func TestFieldConfigurationResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestFieldConfigurationResourceDeleteBadState tests Delete with invalid state.
func TestFieldConfigurationResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// ==================== FIELD CONFIGURATION DATA SOURCE TESTS ====================

// TestFieldConfigurationDataSource tests the data source.
func TestFieldConfigurationDataSource(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	ctx := context.Background()

	r := customfieldresource.NewFieldConfigurationResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS FC"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	id := getStringAttr(t, createResp.State, "id")

	ds := customfielddatasource.NewFieldConfigurationDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	// Metadata
	metaReq := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	metaResp := &datasource.MetadataResponse{}
	ds.Metadata(ctx, metaReq, metaResp)
	if metaResp.TypeName != "atlassian_jira_field_configuration" {
		t.Errorf("expected 'atlassian_jira_field_configuration', got %q", metaResp.TypeName)
	}

	// Schema
	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if len(schemaResp.Schema.Attributes) != 3 {
		t.Errorf("expected 3 attributes, got %d", len(schemaResp.Schema.Attributes))
	}

	// Read
	dsConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, id),
		"name":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: dsConfig}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", dsResp.Diagnostics.Errors())
	}

	// Not found
	dsConfigNotFound := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsRespNotFound := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: dsConfigNotFound}, dsRespNotFound)
	if !dsRespNotFound.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}

	// Server error
	_, sclient := testFCServerErrorMockServer(t)
	ds2 := customfielddatasource.NewFieldConfigurationDataSource()
	configureDatasource(t, ds2, sclient)
	dsRespErr := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds2.Read(ctx, datasource.ReadRequest{Config: dsConfigNotFound}, dsRespErr)
	if !dsRespErr.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestFieldConfigurationDataSourceConfigureNil tests nil provider data.
func TestFieldConfigurationDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := customfielddatasource.NewFieldConfigurationDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestFieldConfigurationDataSourceConfigureWrongType tests wrong type.
func TestFieldConfigurationDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := customfielddatasource.NewFieldConfigurationDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestFieldConfigurationDataSourceReadBadConfig tests Read with invalid config.
func TestFieldConfigurationDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	ds := customfielddatasource.NewFieldConfigurationDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	ctx := context.Background()
	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config")
	}
}

// ==================== FIELD CONFIGURATION SCHEME RESOURCE TESTS ====================

// TestFieldConfigurationSchemeResourceMetadata verifies the resource type name.
func TestFieldConfigurationSchemeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_field_configuration_scheme" {
		t.Errorf("expected 'atlassian_jira_field_configuration_scheme', got %q", resp.TypeName)
	}
}

// TestFieldConfigurationSchemeResourceSchema verifies schema attributes.
func TestFieldConfigurationSchemeResourceSchema(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, attr := range []string{"id", "name", "description"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(resp.Schema.Attributes) != 3 {
		t.Errorf("expected 3 attributes, got %d", len(resp.Schema.Attributes))
	}
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Error("name should be required")
	}
	if !resp.Schema.Attributes["id"].IsComputed() {
		t.Error("id should be computed")
	}
	if resp.Schema.Attributes["description"].IsRequired() {
		t.Error("description should be optional")
	}
}

// TestFieldConfigurationSchemeResourceImplementsInterfaces verifies interfaces.
func TestFieldConfigurationSchemeResourceImplementsInterfaces(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

// TestFieldConfigurationSchemeResourceCRUDLifecycle tests the full CRUD cycle.
func TestFieldConfigurationSchemeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Test FCS"),
		"description": tftypes.NewValue(tftypes.String, "A field configuration scheme"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test FCS" {
		t.Errorf("expected name 'Test FCS', got %q", name)
	}

	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, id),
		"name":        tftypes.NewValue(tftypes.String, "Test FCS"),
		"description": tftypes.NewValue(tftypes.String, "A field configuration scheme"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, id),
		"name":        tftypes.NewValue(tftypes.String, "Updated FCS"),
		"description": tftypes.NewValue(tftypes.String, "Updated scheme description"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}

	deleteState := tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

// TestFieldConfigurationSchemeResourceErrorPaths tests error scenarios.
func TestFieldConfigurationSchemeResourceErrorPaths(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Duplicate
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Dup FCS"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate error")
	}

	// Not found on update/delete/read
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	uResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, uResp)
	if !uResp.Diagnostics.HasError() {
		t.Fatal("Expected not found on update")
	}
	dResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, dResp)
	if dResp.Diagnostics.HasError() {
		t.Fatal("Delete nonexistent should not error")
	}
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, rResp)
	if !rResp.State.Raw.IsNull() {
		t.Error("expected state removed")
	}

	// Forbidden
	_, fclient := testFCForbiddenMockServer(t)
	r2 := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r2, fclient)
	fResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r2.Create(ctx, resource.CreateRequest{Plan: plan}, fResp)
	if !fResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden")
	}
	fuResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r2.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, fuResp)
	if !fuResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden on update")
	}
	fdResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r2.Delete(ctx, resource.DeleteRequest{State: state}, fdResp)
	if !fdResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden on delete")
	}

	// Server error
	_, sclient := testFCServerErrorMockServer(t)
	r3 := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r3, sclient)
	seResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r3.Read(ctx, resource.ReadRequest{State: state}, seResp)
	if !seResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
	seCreateResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r3.Create(ctx, resource.CreateRequest{Plan: plan}, seCreateResp)
	if !seCreateResp.Diagnostics.HasError() {
		t.Fatal("Expected server error on create")
	}

	// Bad request
	_, bclient := testFCBadRequestMockServer(t)
	r4 := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r4, bclient)
	brResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r4.Create(ctx, resource.CreateRequest{Plan: plan}, brResp)
	if !brResp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on create")
	}
	brUpdateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r4.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, brUpdateResp)
	if !brUpdateResp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on update")
	}
}

// TestFieldConfigurationSchemeResourceUpdateServerError tests 500 on update.
func TestFieldConfigurationSchemeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testFCServerErrorMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on update")
	}
}

// TestFieldConfigurationSchemeResourceDeleteServerError tests 500 on delete.
func TestFieldConfigurationSchemeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testFCServerErrorMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on delete")
	}
}

// TestFieldConfigurationSchemeResourceImportState verifies import state passthrough.
func TestFieldConfigurationSchemeResourceImportState(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "fcs-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestFieldConfigurationSchemeResourceConfigureNil tests nil provider data.
func TestFieldConfigurationSchemeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestFieldConfigurationSchemeResourceConfigureWrongType tests wrong type.
func TestFieldConfigurationSchemeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestFieldConfigurationSchemeResourceCreateBadPlan tests Create with invalid plan.
func TestFieldConfigurationSchemeResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestFieldConfigurationSchemeResourceReadBadState tests Read with invalid state.
func TestFieldConfigurationSchemeResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestFieldConfigurationSchemeResourceUpdateBadPlan tests Update with invalid plan.
func TestFieldConfigurationSchemeResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestFieldConfigurationSchemeResourceUpdateBadState tests Update with invalid state.
func TestFieldConfigurationSchemeResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestFieldConfigurationSchemeResourceDeleteBadState tests Delete with invalid state.
func TestFieldConfigurationSchemeResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// ==================== FIELD CONFIGURATION SCHEME DATA SOURCE TESTS ====================

// TestFieldConfigurationSchemeDataSource tests the data source.
func TestFieldConfigurationSchemeDataSource(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	ctx := context.Background()

	r := customfieldresource.NewFieldConfigurationSchemeResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS FCS"),
		"description": tftypes.NewValue(tftypes.String, "ds scheme desc"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	id := getStringAttr(t, createResp.State, "id")

	ds := customfielddatasource.NewFieldConfigurationSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	// Metadata
	metaReq := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	metaResp := &datasource.MetadataResponse{}
	ds.Metadata(ctx, metaReq, metaResp)
	if metaResp.TypeName != "atlassian_jira_field_configuration_scheme" {
		t.Errorf("expected 'atlassian_jira_field_configuration_scheme', got %q", metaResp.TypeName)
	}

	// Schema
	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if len(schemaResp.Schema.Attributes) != 3 {
		t.Errorf("expected 3 attributes, got %d", len(schemaResp.Schema.Attributes))
	}

	// Read
	dsConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, id),
		"name":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: dsConfig}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", dsResp.Diagnostics.Errors())
	}

	// Not found
	dsConfigNotFound := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsRespNotFound := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: dsConfigNotFound}, dsRespNotFound)
	if !dsRespNotFound.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}

	// Server error
	_, sclient := testFCServerErrorMockServer(t)
	ds2 := customfielddatasource.NewFieldConfigurationSchemeDataSource()
	configureDatasource(t, ds2, sclient)
	dsRespErr := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds2.Read(ctx, datasource.ReadRequest{Config: dsConfigNotFound}, dsRespErr)
	if !dsRespErr.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestFieldConfigurationSchemeDataSourceConfigureNil tests nil provider data.
func TestFieldConfigurationSchemeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := customfielddatasource.NewFieldConfigurationSchemeDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestFieldConfigurationSchemeDataSourceConfigureWrongType tests wrong type.
func TestFieldConfigurationSchemeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := customfielddatasource.NewFieldConfigurationSchemeDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestFieldConfigurationSchemeDataSourceReadBadConfig tests Read with invalid config.
func TestFieldConfigurationSchemeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testFCMockServer(t)
	ds := customfielddatasource.NewFieldConfigurationSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	ctx := context.Background()
	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config")
	}
}
