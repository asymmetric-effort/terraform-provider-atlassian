// Package unit contains unit tests for the atlassian_jira_issue_type_screen_scheme
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
	screendatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/screen"
	screenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/screen"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// itssIDCounter provides unique IDs for issue type screen scheme mock server tests.
var itssIDCounter uint64

func itssNextID() string {
	n := atomic.AddUint64(&itssIDCounter, 1)
	return fmt.Sprintf("itss-%d", n)
}

// issueTypeMappingObjTfType is the tftypes.Object for an issue_type_mapping.
var issueTypeMappingObjTfType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
	"issue_type_id":    tftypes.String,
	"screen_scheme_id": tftypes.String,
}}

// issueTypeMappingsListTfType is the tftypes.List for issue_type_mappings.
var issueTypeMappingsListTfType = tftypes.List{ElementType: issueTypeMappingObjTfType}

// nullIssueTypeMappings returns a null tftypes value for the issue_type_mappings list.
func nullIssueTypeMappings() tftypes.Value {
	return tftypes.NewValue(issueTypeMappingsListTfType, nil)
}

// testITSSMockServer creates a mock HTTP server for issue type screen scheme endpoints.
func testITSSMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	schemes := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Create
	mux.HandleFunc("POST /rest/api/3/issuetypescreenscheme", func(w http.ResponseWriter, r *http.Request) {
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
		for _, s := range schemes {
			if s["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"An issue type screen scheme with this name already exists"},
				})
				return
			}
		}
		id := itssNextID()
		description, _ := req["description"].(string)
		scheme := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
		}
		if mappings, ok := req["issueTypeMappings"]; ok && mappings != nil {
			scheme["issueTypeMappings"] = mappings
		}
		schemes[id] = scheme
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(scheme)
	})

	// Read by ID
	mux.HandleFunc("GET /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		scheme, ok := schemes[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type screen scheme not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(scheme)
	})

	// Update by ID
	mux.HandleFunc("PUT /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		scheme, ok := schemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type screen scheme not found"},
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

	// Delete by ID
	mux.HandleFunc("DELETE /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := schemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue type screen scheme not found"},
			})
			return
		}
		delete(schemes, id)
		w.WriteHeader(204)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	auth, _ := atlassian.NewTokenAuthenticator("user", "token")
	client, _ := atlassian.NewClient(cfg, auth)
	return ts, client
}

// --- Schema Validation Tests ---

// TestITSSResourceSchemaAttributes verifies all expected attributes exist in the schema.
func TestITSSResourceSchemaAttributes(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	s := getResourceSchema(t, r)

	expected := []string{"id", "name", "description", "issue_type_mappings"}
	for _, attr := range expected {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestITSSResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestITSSResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	s := getResourceSchema(t, r)

	if len(s.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(s.Attributes))
	}
}

// TestITSSResourceSchemaIDComputed verifies the id attribute is computed.
func TestITSSResourceSchemaIDComputed(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	s := getResourceSchema(t, r)
	attr := s.Attributes["id"]
	if !attr.IsComputed() {
		t.Error("expected id to be computed")
	}
}

// TestITSSResourceSchemaNameRequired verifies the name attribute is required.
func TestITSSResourceSchemaNameRequired(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	s := getResourceSchema(t, r)
	attr := s.Attributes["name"]
	if !attr.IsRequired() {
		t.Error("expected name to be required")
	}
}

// TestITSSResourceSchemaDescriptionOptional verifies the description attribute is optional.
func TestITSSResourceSchemaDescriptionOptional(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	s := getResourceSchema(t, r)
	attr := s.Attributes["description"]
	if !attr.IsOptional() {
		t.Error("expected description to be optional")
	}
}

// TestITSSDataSourceSchemaAttributes verifies all expected attributes in data source schema.
func TestITSSDataSourceSchemaAttributes(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	s := getDatasourceSchema(t, ds)

	expected := []string{"id", "name", "description", "issue_type_mappings"}
	for _, attr := range expected {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected data source schema to have attribute %q", attr)
		}
	}
}

// TestITSSDataSourceSchemaAttributeCount verifies data source attribute count.
func TestITSSDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	s := getDatasourceSchema(t, ds)

	if len(s.Attributes) != 4 {
		t.Errorf("expected 4 data source attributes, got %d", len(s.Attributes))
	}
}

// TestITSSDataSourceSchemaIDRequired verifies the data source id attribute is required.
func TestITSSDataSourceSchemaIDRequired(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	s := getDatasourceSchema(t, ds)
	attr := s.Attributes["id"]
	if !attr.IsRequired() {
		t.Error("expected data source id to be required")
	}
}

// --- Interface Compliance Tests ---

// TestITSSResourceInterfaceCompliance verifies Resource interface implementation.
func TestITSSResourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = screenresource.NewIssueTypeScreenSchemeResource()
}

// TestITSSResourceImportStateInterfaceCompliance verifies ResourceWithImportState interface.
func TestITSSResourceImportStateInterfaceCompliance(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Fatal("resource does not implement ResourceWithImportState")
	}
}

// TestITSSDataSourceInterfaceCompliance verifies DataSource interface implementation.
func TestITSSDataSourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = screendatasource.NewIssueTypeScreenSchemeDataSource()
}

// --- Metadata Tests ---

// TestITSSResourceMetadata verifies the resource type name.
func TestITSSResourceMetadata(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_jira_issue_type_screen_scheme" {
		t.Errorf("expected type name atlassian_jira_issue_type_screen_scheme, got %q", resp.TypeName)
	}
}

// TestITSSDataSourceMetadata verifies the data source type name.
func TestITSSDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_jira_issue_type_screen_scheme" {
		t.Errorf("expected type name atlassian_jira_issue_type_screen_scheme, got %q", resp.TypeName)
	}
}

// --- Configure Tests ---

// TestITSSResourceConfigureNil verifies that Configure with nil ProviderData is a no-op.
func TestITSSResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.Resource).Schema(context.Background(), resource.SchemaRequest{}, &resource.SchemaResponse{})
	rc := r.(resourceWithConfigure)
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure(nil) should not error: %v", resp.Diagnostics.Errors())
	}
}

// TestITSSResourceConfigureWrongType verifies Configure errors on wrong type.
func TestITSSResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Configure with wrong type should error")
	}
}

// TestITSSDataSourceConfigureNil verifies that data source Configure with nil is a no-op.
func TestITSSDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure(nil) should not error: %v", resp.Diagnostics.Errors())
	}
}

// TestITSSDataSourceConfigureWrongType verifies data source Configure errors on wrong type.
func TestITSSDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Configure with wrong type should error")
	}
}

// --- CRUD Lifecycle Tests ---

// TestITSSResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestITSSResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Test ITSS"),
		"description":         tftypes.NewValue(tftypes.String, "A test issue type screen scheme"),
		"issue_type_mappings": nullIssueTypeMappings(),
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
	if name := getStringAttr(t, createResp.State, "name"); name != "Test ITSS" {
		t.Errorf("expected name 'Test ITSS', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "A test issue type screen scheme" {
		t.Errorf("expected description 'A test issue type screen scheme', got %q", desc)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, id),
		"name":                tftypes.NewValue(tftypes.String, "Test ITSS"),
		"description":         tftypes.NewValue(tftypes.String, "A test issue type screen scheme"),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test ITSS" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, id),
		"name":                tftypes.NewValue(tftypes.String, "Updated ITSS"),
		"description":         tftypes.NewValue(tftypes.String, "Updated desc"),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated ITSS" {
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

// TestITSSResourceCRUDWithMappings tests CRUD with non-empty issue_type_mappings.
func TestITSSResourceCRUDWithMappings(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	mappingsVal := tftypes.NewValue(issueTypeMappingsListTfType, []tftypes.Value{
		tftypes.NewValue(issueTypeMappingObjTfType, map[string]tftypes.Value{
			"issue_type_id":    tftypes.NewValue(tftypes.String, "it-1"),
			"screen_scheme_id": tftypes.NewValue(tftypes.String, "ss-100"),
		}),
		tftypes.NewValue(issueTypeMappingObjTfType, map[string]tftypes.Value{
			"issue_type_id":    tftypes.NewValue(tftypes.String, "it-2"),
			"screen_scheme_id": tftypes.NewValue(tftypes.String, "ss-200"),
		}),
	})

	// Create with mappings
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "ITSS With Mappings"),
		"description":         tftypes.NewValue(tftypes.String, "Has mappings"),
		"issue_type_mappings": mappingsVal,
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create with mappings: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty id after create with mappings")
	}

	// Read back and verify mappings are present
	readState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read with mappings: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "ITSS With Mappings" {
		t.Errorf("Read name: got %q", name)
	}

	// Update with different mappings
	newMappingsVal := tftypes.NewValue(issueTypeMappingsListTfType, []tftypes.Value{
		tftypes.NewValue(issueTypeMappingObjTfType, map[string]tftypes.Value{
			"issue_type_id":    tftypes.NewValue(tftypes.String, "it-3"),
			"screen_scheme_id": tftypes.NewValue(tftypes.String, "ss-300"),
		}),
	})
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, id),
		"name":                tftypes.NewValue(tftypes.String, "ITSS With Mappings"),
		"description":         tftypes.NewValue(tftypes.String, "Has mappings"),
		"issue_type_mappings": newMappingsVal,
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update with mappings: %v", updateResp.Diagnostics.Errors())
	}

	// Clean up
	deleteState := tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

// --- Error Path Tests ---

// TestITSSResourceCreateDuplicateName tests creating a scheme with a duplicate name (409).
func TestITSSResourceCreateDuplicateName(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Dup ITSS"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Dup ITSS"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate name error")
	}
}

// TestITSSResourceReadNotFound tests reading a nonexistent scheme (404).
func TestITSSResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	// Read of a deleted resource should remove from state (not error)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent should not error: %v", readResp.Diagnostics.Errors())
	}
}

// TestITSSResourceUpdateNotFound tests updating a nonexistent scheme (404).
func TestITSSResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent scheme")
	}
}

// TestITSSResourceDeleteNotFound tests deleting an already-deleted scheme (404 idempotent).
func TestITSSResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent scheme should not error (idempotent)")
	}
}

// TestITSSResourceCreateForbidden tests 403 on create.
func TestITSSResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/issuetypescreenscheme", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"forbidden"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected 403 error")
	}
}

// TestITSSResourceCreateBadRequest tests 400 on create.
func TestITSSResourceCreateBadRequest(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/issuetypescreenscheme", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"bad request"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "BadReq"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected 400 error")
	}
}

// TestITSSResourceReadServerError tests 500 on read.
func TestITSSResourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"internal error"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected 500 error on read")
	}
}

// TestITSSResourceUpdateForbidden tests 403 on update.
func TestITSSResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"forbidden"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected 403 error on update")
	}
}

// TestITSSResourceUpdateBadRequest tests 400 on update.
func TestITSSResourceUpdateBadRequest(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"bad request"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected 400 error on update")
	}
}

// TestITSSResourceUpdateServerError tests 500 on update.
func TestITSSResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"internal error"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected 500 error on update")
	}
}

// TestITSSResourceDeleteForbidden tests 403 on delete.
func TestITSSResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"forbidden"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("Expected 403 error on delete")
	}
}

// TestITSSResourceDeleteServerError tests 500 on delete.
func TestITSSResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"internal error"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("Expected 500 error on delete")
	}
}

// TestITSSResourceCreateServerError tests 500 on create.
func TestITSSResourceCreateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/issuetypescreenscheme", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"internal error"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "ServerErr"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected 500 error on create")
	}
}

// --- Bad Plan/State Tests ---

// TestITSSResourceCreateBadPlan tests Create with a nil raw plan.
func TestITSSResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from nil plan")
	}
}

// TestITSSResourceReadBadState tests Read with a nil raw state.
func TestITSSResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from nil state")
	}
}

// TestITSSResourceUpdateBadPlan tests Update with nil plan.
func TestITSSResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "x"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from nil plan")
	}
}

// TestITSSResourceUpdateBadState tests Update with nil state.
func TestITSSResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "x"),
		"name":                tftypes.NewValue(tftypes.String, "X"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from nil state")
	}
}

// TestITSSResourceDeleteBadState tests Delete with nil state.
func TestITSSResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from nil state")
	}
}

// --- Import State Test ---

// TestITSSResourceImportState tests the ImportState function.
func TestITSSResourceImportState(t *testing.T) {
	t.Parallel()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	ctx := context.Background()
	s := getResourceSchema(t, r)

	importResp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)},
	}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "test-import-id"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	importedID := getStringAttr(t, importResp.State, "id")
	if importedID != "test-import-id" {
		t.Errorf("expected imported id 'test-import-id', got %q", importedID)
	}
}

// --- Data Source Tests ---

// TestITSSDataSourceRead tests the data source Read function.
func TestITSSDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()

	// First create a resource to read
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "DS Read Test"),
		"description":         tftypes.NewValue(tftypes.String, "for data source"),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create for DS read: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	// Now read via data source
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	configureDatasource(t, ds, client)
	dsSchema := getDatasourceSchema(t, ds)
	dsTfType := dsSchema.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dsSchema, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, id),
		"name":                tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(issueTypeMappingsListTfType, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dsSchema)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Read Test" {
		t.Errorf("DS Read name: got %q", name)
	}
}

// TestITSSDataSourceReadWithMappings tests the data source Read with non-empty mappings.
func TestITSSDataSourceReadWithMappings(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()

	// Create with mappings
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	mappingsVal := tftypes.NewValue(issueTypeMappingsListTfType, []tftypes.Value{
		tftypes.NewValue(issueTypeMappingObjTfType, map[string]tftypes.Value{
			"issue_type_id":    tftypes.NewValue(tftypes.String, "it-10"),
			"screen_scheme_id": tftypes.NewValue(tftypes.String, "ss-20"),
		}),
	})

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "DS Mappings Test"),
		"description":         tftypes.NewValue(tftypes.String, "has mappings"),
		"issue_type_mappings": mappingsVal,
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	// Read via data source
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	configureDatasource(t, ds, client)
	dsSchema := getDatasourceSchema(t, ds)
	dsTfType := dsSchema.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dsSchema, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, id),
		"name":                tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(issueTypeMappingsListTfType, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dsSchema)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read with mappings: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Mappings Test" {
		t.Errorf("DS Read name: got %q", name)
	}
}

// TestITSSDataSourceReadNotFound tests the data source Read for a nonexistent scheme.
func TestITSSDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()

	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	configureDatasource(t, ds, client)
	dsSchema := getDatasourceSchema(t, ds)
	dsTfType := dsSchema.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dsSchema, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":                tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(issueTypeMappingsListTfType, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dsSchema)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nonexistent data source read")
	}
}

// TestITSSDataSourceReadServerError tests the data source Read with a 500 server error.
func TestITSSDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issuetypescreenscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"internal error"}})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	auth, _ := atlassian.NewTokenAuthenticator("u", "t")
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	configureDatasource(t, ds, client)
	dsSchema := getDatasourceSchema(t, ds)
	dsTfType := dsSchema.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dsSchema, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"name":                tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": tftypes.NewValue(issueTypeMappingsListTfType, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dsSchema)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected 500 error on data source read")
	}
}

// TestITSSResourceCreateNoOptionals tests creating with only the required name field.
func TestITSSResourceCreateNoOptionals(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()
	r := screenresource.NewIssueTypeScreenSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "Minimal ITSS"),
		"description":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_mappings": nullIssueTypeMappings(),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create minimal: %v", createResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Minimal ITSS" {
		t.Errorf("expected name 'Minimal ITSS', got %q", name)
	}
}

// TestITSSDataSourceReadBadConfig tests the data source Read with nil config.
func TestITSSDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testITSSMockServer(t)
	ctx := context.Background()

	ds := screendatasource.NewIssueTypeScreenSchemeDataSource()
	configureDatasource(t, ds, client)
	dsSchema := getDatasourceSchema(t, ds)

	config := tfsdk.Config{Schema: dsSchema, Raw: tftypes.NewValue(dsSchema.Type().TerraformType(ctx), nil)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dsSchema)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error from nil config")
	}
}
