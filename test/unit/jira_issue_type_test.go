// Package unit contains unit tests for the atlassian_jira_issue_type and
// atlassian_jira_issue_type_scheme resources and data sources.
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
	issuetypedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/issue_type"
	issuetyperesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/issue_type"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// issueTypeIDCounter provides unique IDs for issue type mock server tests.
var issueTypeIDCounter uint64

func issueTypeNextID() string {
	n := atomic.AddUint64(&issueTypeIDCounter, 1)
	return fmt.Sprintf("it-%d", n)
}

// issueTypeSchemeIDCounter provides unique IDs for issue type scheme mock server tests.
var issueTypeSchemeIDCounter uint64

func issueTypeSchemeNextID() string {
	n := atomic.AddUint64(&issueTypeSchemeIDCounter, 1)
	return fmt.Sprintf("its-%d", n)
}

// testIssueTypeMockServer creates a mock HTTP server for Jira issue type and scheme endpoints.
func testIssueTypeMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	issueTypes := make(map[string]map[string]interface{})
	schemes := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Issue Type endpoints
	mux.HandleFunc("POST /rest/api/3/issuetype", func(w http.ResponseWriter, r *http.Request) {
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
		for _, it := range issueTypes {
			if it["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"An issue type with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}
		id := issueTypeNextID()
		description, _ := req["description"].(string)
		var hierarchyLevel float64
		if hl, ok := req["hierarchyLevel"].(float64); ok {
			hierarchyLevel = hl
		}
		it := map[string]interface{}{
			"id":             id,
			"name":           name,
			"description":    description,
			"iconUrl":        "",
			"subtask":        false,
			"hierarchyLevel": hierarchyLevel,
			"self":           fmt.Sprintf("/rest/api/3/issuetype/%s", id),
		}
		issueTypes[id] = it
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(it)
	})

	mux.HandleFunc("GET /rest/api/3/issuetype/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		it, ok := issueTypes[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(it)
	})

	mux.HandleFunc("PUT /rest/api/3/issuetype/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		it, ok := issueTypes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				it[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(it)
	})

	mux.HandleFunc("DELETE /rest/api/3/issuetype/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := issueTypes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(issueTypes, id)
		w.WriteHeader(204)
	})

	// Issue Type Scheme endpoints
	mux.HandleFunc("POST /rest/api/3/issuetypescheme", func(w http.ResponseWriter, r *http.Request) {
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
		for _, s := range schemes {
			if s["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"An issue type scheme with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}
		id := issueTypeSchemeNextID()
		description, _ := req["description"].(string)
		defaultIssueTypeID, _ := req["defaultIssueTypeId"].(string)
		scheme := map[string]interface{}{
			"id":                 id,
			"name":               name,
			"description":        description,
			"defaultIssueTypeId": defaultIssueTypeID,
			"self":               fmt.Sprintf("/rest/api/3/issuetypescheme/%s", id),
		}
		schemes[id] = scheme
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(scheme)
	})

	mux.HandleFunc("GET /rest/api/3/issuetypescheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		s, ok := schemes[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
	})

	mux.HandleFunc("PUT /rest/api/3/issuetypescheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		s, ok := schemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				s[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s)
	})

	mux.HandleFunc("DELETE /rest/api/3/issuetypescheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := schemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type scheme not found"},
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
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// testIssueTypeForbiddenServer creates a mock that returns 403 for all endpoints.
func testIssueTypeForbiddenServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testIssueTypeServerErrorServer creates a mock that returns 500 for all endpoints.
func testIssueTypeServerErrorServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// ==================== ISSUE TYPE RESOURCE SCHEMA TESTS ====================

// TestJiraIssueTypeResourceMetadata verifies the resource type name.
func TestJiraIssueTypeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_issue_type" {
		t.Errorf("expected resource type name 'atlassian_jira_issue_type', got %q", resp.TypeName)
	}
}

// TestJiraIssueTypeResourceSchema verifies the resource schema has all expected attributes.
func TestJiraIssueTypeResourceSchema(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "description", "icon_url", "subtask", "hierarchy_level"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraIssueTypeResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraIssueTypeResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	expected := 6
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraIssueTypeResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraIssueTypeResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
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

// TestJiraIssueTypeResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraIssueTypeResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	computedAttrs := []string{"id", "description", "icon_url", "subtask", "hierarchy_level"}
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

// TestJiraIssueTypeResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraIssueTypeResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	optionalAttrs := []string{"description", "icon_url", "subtask", "hierarchy_level"}
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

// TestJiraIssueTypeResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraIssueTypeResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraIssueTypeResourceImplementsResource verifies the Resource interface.
func TestJiraIssueTypeResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected issue type resource to implement resource.Resource")
	}
}

// TestJiraIssueTypeResourceImplementsImportState verifies the ImportState interface.
func TestJiraIssueTypeResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected issue type resource to implement ResourceWithImportState")
	}
}

// ==================== ISSUE TYPE RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraIssueTypeResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraIssueTypeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Bug"),
		"description":     tftypes.NewValue(tftypes.String, "A bug report"),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	itID := getStringAttr(t, createResp.State, "id")
	if itID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Bug" {
		t.Errorf("expected name 'Bug', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "A bug report" {
		t.Errorf("expected description 'A bug report', got %q", desc)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, itID),
		"name":            tftypes.NewValue(tftypes.String, "Bug"),
		"description":     tftypes.NewValue(tftypes.String, "A bug report"),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Bug" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, itID),
		"name":            tftypes.NewValue(tftypes.String, "Defect"),
		"description":     tftypes.NewValue(tftypes.String, "A defect report"),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Defect" {
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

// TestJiraIssueTypeResourceCreateDuplicate tests creating an issue type with a duplicate name.
func TestJiraIssueTypeResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "DupType"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate name error")
	}
}

// TestJiraIssueTypeResourceUpdateNotFound tests updating a nonexistent issue type.
func TestJiraIssueTypeResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent issue type")
	}
}

// TestJiraIssueTypeResourceDeleteNotFound tests deleting an already-deleted issue type.
func TestJiraIssueTypeResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent issue type should not error (idempotent)")
	}
}

// TestJiraIssueTypeResourceReadNotFound tests reading a nonexistent issue type removes resource.
func TestJiraIssueTypeResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
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

// TestJiraIssueTypeResourceCreateForbidden tests 403 on create.
func TestJiraIssueTypeResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeForbiddenServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraIssueTypeResourceUpdateForbidden tests 403 on update.
func TestJiraIssueTypeResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeForbiddenServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraIssueTypeResourceDeleteForbidden tests 403 on delete.
func TestJiraIssueTypeResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeForbiddenServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraIssueTypeResourceCreateServerError tests generic error on create.
func TestJiraIssueTypeResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Error"),
		"description":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// TestJiraIssueTypeResourceReadServerError tests generic error on read.
func TestJiraIssueTypeResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraIssueTypeResourceUpdateServerError tests generic error on update.
func TestJiraIssueTypeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraIssueTypeResourceDeleteServerError tests generic error on delete.
func TestJiraIssueTypeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraIssueTypeResourceConfigureNil verifies nil provider data does not error.
func TestJiraIssueTypeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraIssueTypeResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraIssueTypeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraIssueTypeResourceImportState verifies import state passthrough.
func TestJiraIssueTypeResourceImportState(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "it-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraIssueTypeResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraIssueTypeResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraIssueTypeResourceReadBadState tests Read with invalid state data.
func TestJiraIssueTypeResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraIssueTypeResourceUpdateBadPlan tests Update with invalid plan data.
func TestJiraIssueTypeResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "x"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraIssueTypeResourceUpdateBadState tests Update with invalid state data.
func TestJiraIssueTypeResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "x"),
		"name":            tftypes.NewValue(tftypes.String, "X"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, int64(0)),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraIssueTypeResourceDeleteBadState tests Delete with invalid state data.
func TestJiraIssueTypeResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// ==================== ISSUE TYPE DATA SOURCE TESTS ====================

// TestJiraIssueTypeDataSourceMetadata verifies the data source type name.
func TestJiraIssueTypeDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_issue_type" {
		t.Errorf("expected data source type name 'atlassian_jira_issue_type', got %q", resp.TypeName)
	}
}

// TestJiraIssueTypeDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraIssueTypeDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "description", "icon_url", "subtask", "hierarchy_level"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraIssueTypeDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraIssueTypeDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	expected := 6
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraIssueTypeDataSourceSchemaComputedAttributes verifies computed attributes.
func TestJiraIssueTypeDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	computedAttrs := []string{"name", "description", "icon_url", "subtask", "hierarchy_level"}
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

// TestJiraIssueTypeDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraIssueTypeDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected issue type data source to implement datasource.DataSource")
	}
}

// TestJiraIssueTypeDataSourceByID tests reading an issue type by ID.
func TestJiraIssueTypeDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()

	// Create an issue type via resource
	r := issuetyperesource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "DS Bug"),
		"description":     tftypes.NewValue(tftypes.String, "DS desc"),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	itID := getStringAttr(t, cResp.State, "id")

	// Read via data source
	ds := issuetypedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, itID),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, nil),
		"subtask":         tftypes.NewValue(tftypes.Bool, nil),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Bug" {
		t.Errorf("expected name 'DS Bug', got %q", name)
	}
}

// TestJiraIssueTypeDataSourceNotFound tests 404 error on data source read.
func TestJiraIssueTypeDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	ds := issuetypedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, nil),
		"subtask":         tftypes.NewValue(tftypes.Bool, nil),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent issue type")
	}
}

// TestJiraIssueTypeDataSourceServerError tests generic error on data source read.
func TestJiraIssueTypeDataSourceServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	ds := issuetypedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, nil),
		"subtask":         tftypes.NewValue(tftypes.Bool, nil),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestJiraIssueTypeDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraIssueTypeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraIssueTypeDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraIssueTypeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraIssueTypeDataSourceReadBadConfig tests data source Read with invalid config data.
func TestJiraIssueTypeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	ds := issuetypedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// ==================== ISSUE TYPE SCHEME RESOURCE SCHEMA TESTS ====================

// TestJiraIssueTypeSchemeResourceMetadata verifies the resource type name.
func TestJiraIssueTypeSchemeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_issue_type_scheme" {
		t.Errorf("expected resource type name 'atlassian_jira_issue_type_scheme', got %q", resp.TypeName)
	}
}

// TestJiraIssueTypeSchemeResourceSchema verifies the resource schema has all expected attributes.
func TestJiraIssueTypeSchemeResourceSchema(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "description", "issue_type_ids", "default_issue_type_id"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraIssueTypeSchemeResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraIssueTypeSchemeResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	expected := 5
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraIssueTypeSchemeResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraIssueTypeSchemeResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	requiredAttrs := []string{"name", "issue_type_ids"}
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

// TestJiraIssueTypeSchemeResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraIssueTypeSchemeResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	computedAttrs := []string{"id", "description", "default_issue_type_id"}
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

// TestJiraIssueTypeSchemeResourceImplementsResource verifies the Resource interface.
func TestJiraIssueTypeSchemeResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected issue type scheme resource to implement resource.Resource")
	}
}

// TestJiraIssueTypeSchemeResourceImplementsImportState verifies the ImportState interface.
func TestJiraIssueTypeSchemeResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected issue type scheme resource to implement ResourceWithImportState")
	}
}

// ==================== ISSUE TYPE SCHEME RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraIssueTypeSchemeResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraIssueTypeSchemeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                  tftypes.NewValue(tftypes.String, "Default Scheme"),
		"description":           tftypes.NewValue(tftypes.String, "A test scheme"),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1"), tftypes.NewValue(tftypes.String, "it-2")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, "it-1"),
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
	if name := getStringAttr(t, createResp.State, "name"); name != "Default Scheme" {
		t.Errorf("expected name 'Default Scheme', got %q", name)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, schemeID),
		"name":                  tftypes.NewValue(tftypes.String, "Default Scheme"),
		"description":           tftypes.NewValue(tftypes.String, "A test scheme"),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1"), tftypes.NewValue(tftypes.String, "it-2")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, "it-1"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Default Scheme" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, schemeID),
		"name":                  tftypes.NewValue(tftypes.String, "Updated Scheme"),
		"description":           tftypes.NewValue(tftypes.String, "Updated desc"),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-3")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, "it-3"),
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
		t.Error("expected state to be removed after delete")
	}
}

// TestJiraIssueTypeSchemeResourceCreateDuplicate tests creating a scheme with a duplicate name.
func TestJiraIssueTypeSchemeResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                  tftypes.NewValue(tftypes.String, "DupScheme"),
		"description":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate name error")
	}
}

// TestJiraIssueTypeSchemeResourceUpdateNotFound tests updating a nonexistent scheme.
func TestJiraIssueTypeSchemeResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent scheme")
	}
}

// TestJiraIssueTypeSchemeResourceDeleteNotFound tests deleting an already-deleted scheme.
func TestJiraIssueTypeSchemeResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent scheme should not error (idempotent)")
	}
}

// TestJiraIssueTypeSchemeResourceReadNotFound tests reading a nonexistent scheme removes resource.
func TestJiraIssueTypeSchemeResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
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

// TestJiraIssueTypeSchemeResourceCreateForbidden tests 403 on create.
func TestJiraIssueTypeSchemeResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeForbiddenServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                  tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraIssueTypeSchemeResourceUpdateForbidden tests 403 on update.
func TestJiraIssueTypeSchemeResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeForbiddenServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "some-id"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraIssueTypeSchemeResourceDeleteForbidden tests 403 on delete.
func TestJiraIssueTypeSchemeResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeForbiddenServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "some-id"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraIssueTypeSchemeResourceCreateServerError tests generic error on create.
func TestJiraIssueTypeSchemeResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                  tftypes.NewValue(tftypes.String, "Error"),
		"description":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// TestJiraIssueTypeSchemeResourceReadServerError tests generic error on read.
func TestJiraIssueTypeSchemeResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "some-id"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraIssueTypeSchemeResourceUpdateServerError tests generic error on update.
func TestJiraIssueTypeSchemeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "some-id"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraIssueTypeSchemeResourceDeleteServerError tests generic error on delete.
func TestJiraIssueTypeSchemeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "some-id"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraIssueTypeSchemeResourceConfigureNil verifies nil provider data does not error.
func TestJiraIssueTypeSchemeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraIssueTypeSchemeResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraIssueTypeSchemeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraIssueTypeSchemeResourceImportState verifies import state passthrough.
func TestJiraIssueTypeSchemeResourceImportState(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "its-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraIssueTypeSchemeResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraIssueTypeSchemeResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraIssueTypeSchemeResourceReadBadState tests Read with invalid state data.
func TestJiraIssueTypeSchemeResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraIssueTypeSchemeResourceUpdateBadPlan tests Update with invalid plan data.
func TestJiraIssueTypeSchemeResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "x"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraIssueTypeSchemeResourceUpdateBadState tests Update with invalid state data.
func TestJiraIssueTypeSchemeResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "x"),
		"name":                  tftypes.NewValue(tftypes.String, "X"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraIssueTypeSchemeResourceDeleteBadState tests Delete with invalid state data.
func TestJiraIssueTypeSchemeResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// ==================== ISSUE TYPE SCHEME DATA SOURCE TESTS ====================

// TestJiraIssueTypeSchemeDataSourceMetadata verifies the data source type name.
func TestJiraIssueTypeSchemeDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewSchemeDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_issue_type_scheme" {
		t.Errorf("expected data source type name 'atlassian_jira_issue_type_scheme', got %q", resp.TypeName)
	}
}

// TestJiraIssueTypeSchemeDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraIssueTypeSchemeDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewSchemeDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "description", "default_issue_type_id"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraIssueTypeSchemeDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraIssueTypeSchemeDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewSchemeDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraIssueTypeSchemeDataSourceSchemaComputedAttributes verifies computed attributes.
func TestJiraIssueTypeSchemeDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewSchemeDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	computedAttrs := []string{"name", "description", "default_issue_type_id"}
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

// TestJiraIssueTypeSchemeDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraIssueTypeSchemeDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewSchemeDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected issue type scheme data source to implement datasource.DataSource")
	}
}

// TestJiraIssueTypeSchemeDataSourceByID tests reading a scheme by ID.
func TestJiraIssueTypeSchemeDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()

	// Create a scheme via resource
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                  tftypes.NewValue(tftypes.String, "DS Scheme"),
		"description":           tftypes.NewValue(tftypes.String, "DS desc"),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, "it-1"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	schemeID := getStringAttr(t, cResp.State, "id")

	// Read via data source
	ds := issuetypedatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, schemeID),
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Scheme" {
		t.Errorf("expected name 'DS Scheme', got %q", name)
	}
}

// TestJiraIssueTypeSchemeDataSourceNotFound tests 404 error on data source read.
func TestJiraIssueTypeSchemeDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	ds := issuetypedatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent scheme")
	}
}

// TestJiraIssueTypeSchemeDataSourceServerError tests generic error on data source read.
func TestJiraIssueTypeSchemeDataSourceServerError(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeServerErrorServer(t)
	ctx := context.Background()
	ds := issuetypedatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "some-id"),
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestJiraIssueTypeSchemeDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraIssueTypeSchemeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewSchemeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraIssueTypeSchemeDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraIssueTypeSchemeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := issuetypedatasource.NewSchemeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraIssueTypeSchemeDataSourceReadBadConfig tests data source Read with invalid config data.
func TestJiraIssueTypeSchemeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testIssueTypeMockServer(t)
	ctx := context.Background()
	ds := issuetypedatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// TestJiraIssueTypeSchemeResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraIssueTypeSchemeResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()
	r := issuetyperesource.NewSchemeResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}
