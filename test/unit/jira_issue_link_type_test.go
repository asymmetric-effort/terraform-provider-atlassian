// Package unit contains unit tests for the atlassian_jira_issue_link_type
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

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	iltdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/issue_link"
	iltresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/issue_link"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// iltIDCounter provides unique IDs for issue link type mock server tests.
var iltIDCounter uint64

func iltNextID() string {
	return fmt.Sprintf("%d", atomic.AddUint64(&iltIDCounter, 1))
}

// testIssueLinkTypeMockServer creates a mock HTTP server for issue link type endpoints.
func testIssueLinkTypeMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	linkTypes := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/api/3/issueLinkType", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		inward, _ := req["inward"].(string)
		outward, _ := req["outward"].(string)
		if name == "" || inward == "" || outward == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"name, inward, and outward are required"},
			})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, lt := range linkTypes {
			if lt["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"An issue link type with this name already exists"},
				})
				return
			}
		}
		id := iltNextID()
		lt := map[string]interface{}{
			"id":      id,
			"name":    name,
			"inward":  inward,
			"outward": outward,
			"self":    fmt.Sprintf("/rest/api/3/issueLinkType/%s", id),
		}
		linkTypes[id] = lt
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(lt)
	})

	mux.HandleFunc("GET /rest/api/3/issueLinkType/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		lt, ok := linkTypes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue link type not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lt)
	})

	mux.HandleFunc("PUT /rest/api/3/issueLinkType/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		lt, ok := linkTypes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue link type not found"},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" && k != "self" {
				lt[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(lt)
	})

	mux.HandleFunc("DELETE /rest/api/3/issueLinkType/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := linkTypes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue link type not found"},
			})
			return
		}
		delete(linkTypes, id)
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

// testIssueLinkTypeForbiddenMockServer creates a mock that returns 403 for all endpoints.
func testIssueLinkTypeForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testIssueLinkTypeServerErrorMockServer creates a mock that returns 500 for all endpoints.
func testIssueLinkTypeServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// ==================== ISSUE LINK TYPE RESOURCE ====================

// TestJiraIssueLinkTypeResourceMetadata verifies the resource type name.
func TestJiraIssueLinkTypeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := iltresource.NewTypeResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_jira_issue_link_type" {
		t.Errorf("expected type name 'atlassian_jira_issue_link_type', got %q", resp.TypeName)
	}
}

// TestJiraIssueLinkTypeResourceSchema verifies the schema has required attributes.
func TestJiraIssueLinkTypeResourceSchema(t *testing.T) {
	t.Parallel()
	r := iltresource.NewTypeResource()
	s := getResourceSchema(t, r)
	for _, attr := range []string{"id", "name", "inward", "outward"} {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestJiraIssueLinkTypeResourceImportState verifies ImportState is implemented.
func TestJiraIssueLinkTypeResourceImportState(t *testing.T) {
	t.Parallel()
	r := iltresource.NewTypeResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Fatal("resource does not implement ImportState")
	}
}

// TestJiraIssueLinkTypeResourceCRUDLifecycle tests full create-read-update-delete cycle.
func TestJiraIssueLinkTypeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "Blocks"),
		"inward":  tftypes.NewValue(tftypes.String, "is blocked by"),
		"outward": tftypes.NewValue(tftypes.String, "blocks"),
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
	if name := getStringAttr(t, createResp.State, "name"); name != "Blocks" {
		t.Errorf("expected name 'Blocks', got %q", name)
	}
	if inward := getStringAttr(t, createResp.State, "inward"); inward != "is blocked by" {
		t.Errorf("expected inward 'is blocked by', got %q", inward)
	}
	if outward := getStringAttr(t, createResp.State, "outward"); outward != "blocks" {
		t.Errorf("expected outward 'blocks', got %q", outward)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, id),
		"name":    tftypes.NewValue(tftypes.String, "Blocks"),
		"inward":  tftypes.NewValue(tftypes.String, "is blocked by"),
		"outward": tftypes.NewValue(tftypes.String, "blocks"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Blocks" {
		t.Errorf("Read: expected name 'Blocks', got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, id),
		"name":    tftypes.NewValue(tftypes.String, "Duplicates"),
		"inward":  tftypes.NewValue(tftypes.String, "is duplicated by"),
		"outward": tftypes.NewValue(tftypes.String, "duplicates"),
	})}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, id),
		"name":    tftypes.NewValue(tftypes.String, "Blocks"),
		"inward":  tftypes.NewValue(tftypes.String, "is blocked by"),
		"outward": tftypes.NewValue(tftypes.String, "blocks"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Duplicates" {
		t.Errorf("Update: expected name 'Duplicates', got %q", name)
	}
	if inward := getStringAttr(t, updateResp.State, "inward"); inward != "is duplicated by" {
		t.Errorf("Update: expected inward 'is duplicated by', got %q", inward)
	}
	if outward := getStringAttr(t, updateResp.State, "outward"); outward != "duplicates" {
		t.Errorf("Update: expected outward 'duplicates', got %q", outward)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, id),
		"name":    tftypes.NewValue(tftypes.String, "Duplicates"),
		"inward":  tftypes.NewValue(tftypes.String, "is duplicated by"),
		"outward": tftypes.NewValue(tftypes.String, "duplicates"),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Verify read after delete removes state
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: deleteState}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("Read after delete: %v", readResp2.Diagnostics.Errors())
	}
	if !readResp2.State.Raw.IsNull() {
		t.Error("expected state to be removed after delete")
	}
}

// TestJiraIssueLinkTypeResourceUpdateNotFound tests updating a nonexistent resource.
func TestJiraIssueLinkTypeResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "99999"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "x"),
		"outward": tftypes.NewValue(tftypes.String, "x"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent resource")
	}
}

// TestJiraIssueLinkTypeResourceDeleteNotFound tests deleting an already-deleted resource.
func TestJiraIssueLinkTypeResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "99999"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "x"),
		"outward": tftypes.NewValue(tftypes.String, "x"),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent resource should not error (idempotent)")
	}
}

// TestJiraIssueLinkTypeResourceReadNotFound tests reading a nonexistent resource removes state.
func TestJiraIssueLinkTypeResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "99999"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "x"),
		"outward": tftypes.NewValue(tftypes.String, "x"),
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

// TestJiraIssueLinkTypeResourceCreateForbidden tests 403 on create.
func TestJiraIssueLinkTypeResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeForbiddenMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "Forbidden"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraIssueLinkTypeResourceUpdateForbidden tests 403 on update.
func TestJiraIssueLinkTypeResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeForbiddenMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraIssueLinkTypeResourceDeleteForbidden tests 403 on delete.
func TestJiraIssueLinkTypeResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeForbiddenMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraIssueLinkTypeResourceCreateServerError tests 500 on create.
func TestJiraIssueLinkTypeResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeServerErrorMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "ServerError"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraIssueLinkTypeResourceReadServerError tests 500 on read.
func TestJiraIssueLinkTypeResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeServerErrorMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error read")
	}
}

// TestJiraIssueLinkTypeResourceConfigureNil verifies nil provider data does not error.
func TestJiraIssueLinkTypeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := iltresource.NewTypeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraIssueLinkTypeResourceConfigureBadType verifies wrong provider data type errors.
func TestJiraIssueLinkTypeResourceConfigureBadType(t *testing.T) {
	t.Parallel()
	r := iltresource.NewTypeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "bad"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with wrong type")
	}
}

// TestJiraIssueLinkTypeResourceCreateBadPlan tests Create with invalid plan.
func TestJiraIssueLinkTypeResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil plan")
	}
}

// TestJiraIssueLinkTypeResourceReadBadState tests Read with invalid state.
func TestJiraIssueLinkTypeResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.ReadResponse{State: emptyState(ctx, s)}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil state")
	}
}

// TestJiraIssueLinkTypeResourceUpdateBadPlan tests Update with invalid plan.
func TestJiraIssueLinkTypeResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil plan/state")
	}
}

// TestJiraIssueLinkTypeResourceDeleteBadState tests Delete with invalid state.
func TestJiraIssueLinkTypeResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil state")
	}
}

// ==================== ISSUE LINK TYPE DATA SOURCE ====================

// TestJiraIssueLinkTypeDataSourceMetadata verifies the data source type name.
func TestJiraIssueLinkTypeDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := iltdatasource.NewTypeDataSource()
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_jira_issue_link_type" {
		t.Errorf("expected type name 'atlassian_jira_issue_link_type', got %q", resp.TypeName)
	}
}

// TestJiraIssueLinkTypeDataSourceSchema verifies the schema has required attributes.
func TestJiraIssueLinkTypeDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := iltdatasource.NewTypeDataSource()
	s := getDatasourceSchema(t, ds)
	for _, attr := range []string{"id", "name", "inward", "outward"} {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestJiraIssueLinkTypeDataSourceRead tests reading a link type via data source.
func TestJiraIssueLinkTypeDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()

	// Create via resource
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "Relates"),
		"inward":  tftypes.NewValue(tftypes.String, "relates to"),
		"outward": tftypes.NewValue(tftypes.String, "relates to"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	// Read via data source
	ds := iltdatasource.NewTypeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, id),
		"name":    tftypes.NewValue(tftypes.String, nil),
		"inward":  tftypes.NewValue(tftypes.String, nil),
		"outward": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "Relates" {
		t.Errorf("expected name 'Relates', got %q", name)
	}
	if inward := getStringAttr(t, dsResp.State, "inward"); inward != "relates to" {
		t.Errorf("expected inward 'relates to', got %q", inward)
	}
	if outward := getStringAttr(t, dsResp.State, "outward"); outward != "relates to" {
		t.Errorf("expected outward 'relates to', got %q", outward)
	}
}

// TestJiraIssueLinkTypeDataSourceReadNotFound tests reading a nonexistent link type.
func TestJiraIssueLinkTypeDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()

	ds := iltdatasource.NewTypeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "99999"),
		"name":    tftypes.NewValue(tftypes.String, nil),
		"inward":  tftypes.NewValue(tftypes.String, nil),
		"outward": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent link type")
	}
}

// TestJiraIssueLinkTypeDataSourceReadServerError tests 500 on data source read.
func TestJiraIssueLinkTypeDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeServerErrorMockServer(t)
	ctx := context.Background()

	ds := iltdatasource.NewTypeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, nil),
		"inward":  tftypes.NewValue(tftypes.String, nil),
		"outward": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraIssueLinkTypeDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraIssueLinkTypeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := iltdatasource.NewTypeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraIssueLinkTypeDataSourceConfigureBadType verifies wrong provider data type errors.
func TestJiraIssueLinkTypeDataSourceConfigureBadType(t *testing.T) {
	t.Parallel()
	ds := iltdatasource.NewTypeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "bad"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with wrong type")
	}
}

// TestJiraIssueLinkTypeDataSourceReadBadConfig tests Read with invalid config.
func TestJiraIssueLinkTypeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()

	ds := iltdatasource.NewTypeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dss.Type().TerraformType(ctx), nil)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil config")
	}
}

// testIssueLinkTypeBadRequestMockServer creates a mock that returns 400 for all endpoints.
func testIssueLinkTypeBadRequestMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// TestJiraIssueLinkTypeResourceCreateBadRequest tests 400 on create.
func TestJiraIssueLinkTypeResourceCreateBadRequest(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeBadRequestMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "Bad"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on create")
	}
}

// TestJiraIssueLinkTypeResourceUpdateBadRequest tests 400 on update.
func TestJiraIssueLinkTypeResourceUpdateBadRequest(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeBadRequestMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on update")
	}
}

// TestJiraIssueLinkTypeResourceCreateConflict tests 409 on create (duplicate name).
func TestJiraIssueLinkTypeResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create first
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "Unique Conflict Test"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("First create: %v", createResp.Diagnostics.Errors())
	}

	// Create duplicate
	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "Unique Conflict Test"),
		"inward":  tftypes.NewValue(tftypes.String, "in2"),
		"outward": tftypes.NewValue(tftypes.String, "out2"),
	})}
	createResp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, createResp2)
	if !createResp2.Diagnostics.HasError() {
		t.Fatal("Expected conflict error on duplicate create")
	}
}

// TestJiraIssueLinkTypeResourceDeleteServerError tests generic error on delete.
func TestJiraIssueLinkTypeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeServerErrorMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error delete")
	}
}

// TestJiraIssueLinkTypeResourceImportStateExec tests actual ImportState execution.
func TestJiraIssueLinkTypeResourceImportStateExec(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	importResp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "42"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, importResp.State, "id"); id != "42" {
		t.Errorf("expected imported id '42', got %q", id)
	}
}

// TestJiraIssueLinkTypeResourceUpdateBadState tests Update with valid plan but nil state.
func TestJiraIssueLinkTypeResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, "Valid"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil state in update")
	}
}

// TestJiraIssueLinkTypeResourceUpdateServerError tests 500 on update.
func TestJiraIssueLinkTypeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueLinkTypeServerErrorMockServer(t)
	ctx := context.Background()
	r := iltresource.NewTypeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "1"),
		"name":    tftypes.NewValue(tftypes.String, "X"),
		"inward":  tftypes.NewValue(tftypes.String, "in"),
		"outward": tftypes.NewValue(tftypes.String, "out"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error update")
	}
}
