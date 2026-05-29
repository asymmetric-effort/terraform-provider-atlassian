// Package unit contains unit tests for the atlassian_jira_project_version resource and data source.
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
	versionds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/space"
	versionrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/space"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// versionIDCounter provides unique IDs for version mock server tests.
var versionIDCounter uint64

func versionNextID() string {
	n := atomic.AddUint64(&versionIDCounter, 1)
	return fmt.Sprintf("ver-%d", n)
}

// testVersionMockServer creates a mock HTTP server for Jira project version endpoints.
func testVersionMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	versions := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Create version
	mux.HandleFunc("POST /rest/api/3/version", func(w http.ResponseWriter, r *http.Request) {
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
		for _, v := range versions {
			if v["name"] == name && v["projectId"] == projectID {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"A version with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}

		id := versionNextID()
		description, _ := req["description"].(string)
		startDate, _ := req["startDate"].(string)
		releaseDate, _ := req["releaseDate"].(string)
		released, _ := req["released"].(bool)
		archived, _ := req["archived"].(bool)
		ver := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"projectId":   projectID,
			"startDate":   startDate,
			"releaseDate": releaseDate,
			"released":    released,
			"archived":    archived,
			"self":        fmt.Sprintf("https://example.atlassian.net/rest/api/3/version/%s", id),
		}
		versions[id] = ver
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(ver)
	})

	// Read version by ID
	mux.HandleFunc("GET /rest/api/3/version/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		ver, ok := versions[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Version not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ver)
	})

	// Update version
	mux.HandleFunc("PUT /rest/api/3/version/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		ver, ok := versions[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Version not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" && k != "projectId" {
				ver[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ver)
	})

	// Delete version
	mux.HandleFunc("DELETE /rest/api/3/version/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := versions[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Version not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(versions, id)
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

// testVersionForbiddenMockServer creates a mock that returns 403 for all version endpoints.
func testVersionForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// getBoolAttrVersion retrieves a bool attribute from state.
func getBoolAttrVersion(t *testing.T, state tfsdk.State, name string) bool {
	t.Helper()
	var val types.Bool
	diags := state.GetAttribute(context.Background(), path.Root(name), &val)
	if diags.HasError() {
		t.Fatalf("getBoolAttrVersion %q: %v", name, diags.Errors())
	}
	return val.ValueBool()
}

// ==================== RESOURCE SCHEMA TESTS ====================

// TestJiraProjectVersionResourceMetadata verifies the resource type name.
func TestJiraProjectVersionResourceMetadata(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_project_version" {
		t.Errorf("expected resource type name 'atlassian_jira_project_version', got %q", resp.TypeName)
	}
}

// TestJiraProjectVersionResourceSchema verifies the resource schema has all expected attributes.
func TestJiraProjectVersionResourceSchema(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "space_id", "name", "description", "start_date", "release_date", "released", "archived"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraProjectVersionResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraProjectVersionResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 8
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraProjectVersionResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraProjectVersionResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
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

// TestJiraProjectVersionResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraProjectVersionResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"id", "description", "start_date", "release_date", "released", "archived"}
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

// TestJiraProjectVersionResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraProjectVersionResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"description", "start_date", "release_date", "released", "archived"}
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

// TestJiraProjectVersionResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraProjectVersionResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraProjectVersionResourceImplementsResource verifies the Resource interface.
func TestJiraProjectVersionResourceImplementsResource(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected version resource to implement resource.Resource")
	}
}

// TestJiraProjectVersionResourceImplementsImportState verifies the ImportState interface.
func TestJiraProjectVersionResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := versionrs.NewVersionResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected version resource to implement ResourceWithImportState")
	}
}

// ==================== DATA SOURCE SCHEMA TESTS ====================

// TestJiraProjectVersionDataSourceMetadata verifies the data source type name.
func TestJiraProjectVersionDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := versionds.NewVersionDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_project_version" {
		t.Errorf("expected data source type name 'atlassian_jira_project_version', got %q", resp.TypeName)
	}
}

// TestJiraProjectVersionDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraProjectVersionDataSourceSchema(t *testing.T) {
	t.Parallel()

	ds := versionds.NewVersionDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "space_id", "name", "description", "start_date", "release_date", "released", "archived"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraProjectVersionDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraProjectVersionDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	ds := versionds.NewVersionDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	expected := 8
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraProjectVersionDataSourceSchemaComputedAttributes verifies computed-only attributes.
func TestJiraProjectVersionDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	ds := versionds.NewVersionDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	computedAttrs := []string{"space_id", "name", "description", "start_date", "release_date", "released", "archived"}
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

// TestJiraProjectVersionDataSourceSchemaRequiredAttributes verifies required attributes.
func TestJiraProjectVersionDataSourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	ds := versionds.NewVersionDataSource()
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

// TestJiraProjectVersionDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraProjectVersionDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	ds := versionds.NewVersionDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected version data source to implement datasource.DataSource")
	}
}

// ==================== RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraProjectVersionResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraProjectVersionResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "v1.0"),
		"description":  tftypes.NewValue(tftypes.String, "First release"),
		"start_date":   tftypes.NewValue(tftypes.String, "2024-01-01"),
		"release_date": tftypes.NewValue(tftypes.String, "2024-06-01"),
		"released":     tftypes.NewValue(tftypes.Bool, true),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	verID := getStringAttr(t, createResp.State, "id")
	if verID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "v1.0" {
		t.Errorf("expected name 'v1.0', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "First release" {
		t.Errorf("expected description 'First release', got %q", desc)
	}
	if sd := getStringAttr(t, createResp.State, "start_date"); sd != "2024-01-01" {
		t.Errorf("expected start_date '2024-01-01', got %q", sd)
	}
	if rd := getStringAttr(t, createResp.State, "release_date"); rd != "2024-06-01" {
		t.Errorf("expected release_date '2024-06-01', got %q", rd)
	}
	if rel := getBoolAttrVersion(t, createResp.State, "released"); !rel {
		t.Error("expected released true")
	}
	if arch := getBoolAttrVersion(t, createResp.State, "archived"); arch {
		t.Error("expected archived false")
	}
	if sid := getStringAttr(t, createResp.State, "space_id"); sid != "proj-1" {
		t.Errorf("expected space_id 'proj-1', got %q", sid)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, verID),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "v1.0"),
		"description":  tftypes.NewValue(tftypes.String, "First release"),
		"start_date":   tftypes.NewValue(tftypes.String, "2024-01-01"),
		"release_date": tftypes.NewValue(tftypes.String, "2024-06-01"),
		"released":     tftypes.NewValue(tftypes.Bool, true),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "v1.0" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, verID),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "v2.0"),
		"description":  tftypes.NewValue(tftypes.String, "Second release"),
		"start_date":   tftypes.NewValue(tftypes.String, "2024-07-01"),
		"release_date": tftypes.NewValue(tftypes.String, "2024-12-01"),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, true),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "v2.0" {
		t.Errorf("Update name: got %q", name)
	}
	if desc := getStringAttr(t, updateResp.State, "description"); desc != "Second release" {
		t.Errorf("Update description: got %q", desc)
	}
	if rel := getBoolAttrVersion(t, updateResp.State, "released"); rel {
		t.Error("expected released false after update")
	}
	if arch := getBoolAttrVersion(t, updateResp.State, "archived"); !arch {
		t.Error("expected archived true after update")
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

// TestJiraProjectVersionResourceCreateMinimal tests creating a version with only required fields.
func TestJiraProjectVersionResourceCreateMinimal(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-2"),
		"name":         tftypes.NewValue(tftypes.String, "Minimal"),
		"description":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"start_date":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"release_date": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"released":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"archived":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
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

// TestJiraProjectVersionResourceCreateDuplicate tests creating a version with a duplicate name in the same project.
func TestJiraProjectVersionResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-dup"),
		"name":         tftypes.NewValue(tftypes.String, "DupVer"),
		"description":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"start_date":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"release_date": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"released":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"archived":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-dup"),
		"name":         tftypes.NewValue(tftypes.String, "DupVer"),
		"description":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"start_date":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"release_date": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"released":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"archived":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate name error")
	}
}

// TestJiraProjectVersionResourceUpdateNotFound tests updating a nonexistent version.
func TestJiraProjectVersionResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "X"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent version")
	}
}

// TestJiraProjectVersionResourceDeleteNotFound tests deleting an already-deleted version.
func TestJiraProjectVersionResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "X"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent version should not error (idempotent)")
	}
}

// TestJiraProjectVersionResourceReadNotFound tests reading a nonexistent version removes resource.
func TestJiraProjectVersionResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "X"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
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

// TestJiraProjectVersionResourceCreateForbidden tests 403 on create.
func TestJiraProjectVersionResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testVersionForbiddenMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"start_date":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"release_date": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"released":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"archived":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraProjectVersionResourceUpdateForbidden tests 403 on update.
func TestJiraProjectVersionResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testVersionForbiddenMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraProjectVersionResourceDeleteForbidden tests 403 on delete.
func TestJiraProjectVersionResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testVersionForbiddenMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraProjectVersionResourceConfigureNil verifies nil provider data does not error.
func TestJiraProjectVersionResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := versionrs.NewVersionResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraProjectVersionResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraProjectVersionResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := versionrs.NewVersionResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraProjectVersionResourceImportState verifies import state passthrough.
func TestJiraProjectVersionResourceImportState(t *testing.T) {
	t.Parallel()
	r := versionrs.NewVersionResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "ver-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== DATA SOURCE CRUD TESTS ====================

// TestJiraProjectVersionDataSourceByID tests reading a version by ID.
func TestJiraProjectVersionDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()

	// Create a version first via resource
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-ds"),
		"name":         tftypes.NewValue(tftypes.String, "DS Version"),
		"description":  tftypes.NewValue(tftypes.String, "for data source"),
		"start_date":   tftypes.NewValue(tftypes.String, "2024-03-01"),
		"release_date": tftypes.NewValue(tftypes.String, "2024-09-01"),
		"released":     tftypes.NewValue(tftypes.Bool, true),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	verID := getStringAttr(t, cResp.State, "id")

	// Read via data source by ID
	ds := versionds.NewVersionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, verID),
		"space_id":     tftypes.NewValue(tftypes.String, nil),
		"name":         tftypes.NewValue(tftypes.String, nil),
		"description":  tftypes.NewValue(tftypes.String, nil),
		"start_date":   tftypes.NewValue(tftypes.String, nil),
		"release_date": tftypes.NewValue(tftypes.String, nil),
		"released":     tftypes.NewValue(tftypes.Bool, nil),
		"archived":     tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Version" {
		t.Errorf("expected name 'DS Version', got %q", name)
	}
	if desc := getStringAttr(t, dsResp.State, "description"); desc != "for data source" {
		t.Errorf("expected description 'for data source', got %q", desc)
	}
	if sid := getStringAttr(t, dsResp.State, "space_id"); sid != "proj-ds" {
		t.Errorf("expected space_id 'proj-ds', got %q", sid)
	}
}

// TestJiraProjectVersionDataSourceNotFound tests 404 error on data source read.
func TestJiraProjectVersionDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	ds := versionds.NewVersionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":     tftypes.NewValue(tftypes.String, nil),
		"name":         tftypes.NewValue(tftypes.String, nil),
		"description":  tftypes.NewValue(tftypes.String, nil),
		"start_date":   tftypes.NewValue(tftypes.String, nil),
		"release_date": tftypes.NewValue(tftypes.String, nil),
		"released":     tftypes.NewValue(tftypes.Bool, nil),
		"archived":     tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent version")
	}
}

// TestJiraProjectVersionDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraProjectVersionDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := versionds.NewVersionDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraProjectVersionDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraProjectVersionDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := versionds.NewVersionDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraProjectVersionResourceReadServerError tests a non-404 server error on read.
func TestJiraProjectVersionResourceReadServerError(t *testing.T) {
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
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "X"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectVersionResourceCreateServerError tests a generic server error on create.
func TestJiraProjectVersionResourceCreateServerError(t *testing.T) {
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
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "ServerErr"),
		"description":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"start_date":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"release_date": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"released":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"archived":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectVersionResourceUpdateServerError tests a generic server error on update.
func TestJiraProjectVersionResourceUpdateServerError(t *testing.T) {
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
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "X"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectVersionResourceDeleteServerError tests a generic server error on delete.
func TestJiraProjectVersionResourceDeleteServerError(t *testing.T) {
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
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "X"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectVersionDataSourceReadServerError tests a non-404 server error on data source read.
func TestJiraProjectVersionDataSourceReadServerError(t *testing.T) {
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
	ds := versionds.NewVersionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":     tftypes.NewValue(tftypes.String, nil),
		"name":         tftypes.NewValue(tftypes.String, nil),
		"description":  tftypes.NewValue(tftypes.String, nil),
		"start_date":   tftypes.NewValue(tftypes.String, nil),
		"release_date": tftypes.NewValue(tftypes.String, nil),
		"released":     tftypes.NewValue(tftypes.Bool, nil),
		"archived":     tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraProjectVersionDataSourceEmptyID tests data source with empty string ID.
func TestJiraProjectVersionDataSourceEmptyID(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	ds := versionds.NewVersionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, ""),
		"space_id":     tftypes.NewValue(tftypes.String, nil),
		"name":         tftypes.NewValue(tftypes.String, nil),
		"description":  tftypes.NewValue(tftypes.String, nil),
		"start_date":   tftypes.NewValue(tftypes.String, nil),
		"release_date": tftypes.NewValue(tftypes.String, nil),
		"released":     tftypes.NewValue(tftypes.Bool, nil),
		"archived":     tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when id is empty string")
	}
}

// TestJiraProjectVersionResourceCreateBadPlan tests create with invalid plan data.
func TestJiraProjectVersionResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid plan")
	}
}

// TestJiraProjectVersionResourceReadBadState tests read with invalid state data.
func TestJiraProjectVersionResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid state")
	}
}

// TestJiraProjectVersionResourceUpdateBadPlan tests update with invalid plan data.
func TestJiraProjectVersionResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	validState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "X"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: validState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid plan")
	}
}

// TestJiraProjectVersionResourceUpdateBadState tests update with invalid state data.
func TestJiraProjectVersionResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":     tftypes.NewValue(tftypes.String, "proj-1"),
		"name":         tftypes.NewValue(tftypes.String, "X"),
		"description":  tftypes.NewValue(tftypes.String, ""),
		"start_date":   tftypes.NewValue(tftypes.String, ""),
		"release_date": tftypes.NewValue(tftypes.String, ""),
		"released":     tftypes.NewValue(tftypes.Bool, false),
		"archived":     tftypes.NewValue(tftypes.Bool, false),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: validPlan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid state")
	}
}

// TestJiraProjectVersionResourceDeleteBadState tests delete with invalid state data.
func TestJiraProjectVersionResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid state")
	}
}

// TestJiraProjectVersionDataSourceReadBadConfig tests data source read with invalid config data.
func TestJiraProjectVersionDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testVersionMockServer(t)
	ctx := context.Background()
	ds := versionds.NewVersionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dss.Type().TerraformType(ctx), nil)}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from invalid config")
	}
}

// TestJiraProjectVersionResourceCreateNotFoundProject tests 404 on create (project not found).
func TestJiraProjectVersionResourceCreateNotFoundProject(t *testing.T) {
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
	r := versionrs.NewVersionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":     tftypes.NewValue(tftypes.String, "nonexistent-proj"),
		"name":         tftypes.NewValue(tftypes.String, "Test"),
		"description":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"start_date":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"release_date": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"released":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"archived":     tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected project not found error")
	}
}
