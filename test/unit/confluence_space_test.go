// Package unit contains unit tests for the atlassian_confluence_space resource and data source.
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
	confluencespaceds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/space"
	confluencespacers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/space"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// confluenceSpaceIDCounter provides unique IDs for confluence space mock server tests.
var confluenceSpaceIDCounter uint64

func confluenceSpaceNextID() string {
	n := atomic.AddUint64(&confluenceSpaceIDCounter, 1)
	return fmt.Sprintf("cs-%d", n)
}

// testConfluenceSpaceMockServer creates a mock HTTP server for Confluence space endpoints.
func testConfluenceSpaceMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	spaces := make(map[string]map[string]interface{})
	keyIndex := make(map[string]string) // key -> id

	mux := http.NewServeMux()

	// Create space
	mux.HandleFunc("POST /wiki/api/v2/spaces", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		key, _ := req["key"].(string)
		name, _ := req["name"].(string)
		if key == "" || name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"key and name are required"},
				"errors":        map[string]string{},
			})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if _, exists := keyIndex[key]; exists {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"A space with this key already exists"},
				"errors":        map[string]string{},
			})
			return
		}
		id := confluenceSpaceNextID()
		spaceType, _ := req["type"].(string)
		if spaceType == "" {
			spaceType = "global"
		}
		descValue := ""
		if descObj, ok := req["description"].(map[string]interface{}); ok {
			if plain, ok := descObj["plain"].(map[string]interface{}); ok {
				descValue, _ = plain["value"].(string)
			}
		}
		space := map[string]interface{}{
			"id":         id,
			"key":        key,
			"name":       name,
			"type":       spaceType,
			"status":     "current",
			"homepageId": fmt.Sprintf("page-%s", id),
			"description": map[string]interface{}{
				"plain": map[string]interface{}{
					"value": descValue,
				},
			},
			"_links": map[string]interface{}{
				"webui": fmt.Sprintf("https://example.atlassian.net/wiki/spaces/%s", key),
			},
		}
		spaces[id] = space
		keyIndex[key] = id
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(space)
	})

	// Read space by ID or key
	mux.HandleFunc("GET /wiki/api/v2/spaces/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
		idOrKey := r.PathValue("idOrKey")
		mu.Lock()
		defer mu.Unlock()

		space, ok := spaces[idOrKey]
		if !ok {
			if id, found := keyIndex[idOrKey]; found {
				space, ok = spaces[id]
			}
		}
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"No space could be found with key or ID matching the supplied value"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(space)
	})

	// Update space
	mux.HandleFunc("PUT /wiki/api/v2/spaces/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
		idOrKey := r.PathValue("idOrKey")
		mu.Lock()
		defer mu.Unlock()

		space, ok := spaces[idOrKey]
		if !ok {
			if id, found := keyIndex[idOrKey]; found {
				space, ok = spaces[id]
			}
		}
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"No space could be found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		if name, ok := updates["name"].(string); ok && name != "" {
			space["name"] = name
		}
		if spaceType, ok := updates["type"].(string); ok && spaceType != "" {
			space["type"] = spaceType
		}
		if descObj, ok := updates["description"].(map[string]interface{}); ok {
			space["description"] = descObj
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(space)
	})

	// Delete space
	mux.HandleFunc("DELETE /wiki/api/v2/spaces/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
		idOrKey := r.PathValue("idOrKey")
		mu.Lock()
		defer mu.Unlock()

		if space, ok := spaces[idOrKey]; ok {
			key, _ := space["key"].(string)
			delete(keyIndex, key)
			delete(spaces, idOrKey)
			w.WriteHeader(204)
			return
		}
		if id, found := keyIndex[idOrKey]; found {
			delete(spaces, id)
			delete(keyIndex, idOrKey)
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"No space could be found"},
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
	auth := &testNoopAuth{}
	client, err := atlassian.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// testConfluenceForbiddenMockServer creates a mock that returns 403 for all Confluence endpoints.
func testConfluenceForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testConfluenceServerErrorMockServer creates a mock that returns 500 for all Confluence endpoints.
func testConfluenceServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// ==================== RESOURCE SCHEMA TESTS ====================

// TestConfluenceSpaceResourceMetadata verifies the resource type name.
func TestConfluenceSpaceResourceMetadata(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_confluence_space" {
		t.Errorf("expected resource type name 'atlassian_confluence_space', got %q", resp.TypeName)
	}
}

// TestConfluenceSpaceResourceSchema verifies the resource schema has all expected attributes.
func TestConfluenceSpaceResourceSchema(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "key", "name", "description", "type", "homepage_id", "status", "url"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestConfluenceSpaceResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestConfluenceSpaceResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 8
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestConfluenceSpaceResourceSchemaRequiredAttributes verifies required attributes.
func TestConfluenceSpaceResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"key", "name"}
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

// TestConfluenceSpaceResourceSchemaComputedAttributes verifies computed attributes.
func TestConfluenceSpaceResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"id", "homepage_id", "status", "url", "description", "type"}
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

// TestConfluenceSpaceResourceSchemaOptionalAttributes verifies optional attributes.
func TestConfluenceSpaceResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"description", "type"}
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

// TestConfluenceSpaceResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestConfluenceSpaceResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestConfluenceSpaceResourceImplementsResource verifies the Resource interface.
func TestConfluenceSpaceResourceImplementsResource(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected confluence space resource to implement resource.Resource")
	}
}

// TestConfluenceSpaceResourceImplementsImportState verifies the ImportState interface.
func TestConfluenceSpaceResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := confluencespacers.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected confluence space resource to implement ResourceWithImportState")
	}
}

// ==================== DATA SOURCE SCHEMA TESTS ====================

// TestConfluenceSpaceDataSourceMetadata verifies the data source type name.
func TestConfluenceSpaceDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := confluencespaceds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_confluence_space" {
		t.Errorf("expected data source type name 'atlassian_confluence_space', got %q", resp.TypeName)
	}
}

// TestConfluenceSpaceDataSourceSchema verifies the data source schema has all expected attributes.
func TestConfluenceSpaceDataSourceSchema(t *testing.T) {
	t.Parallel()

	ds := confluencespaceds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "key", "name", "description", "type", "homepage_id", "status", "url"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestConfluenceSpaceDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestConfluenceSpaceDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	ds := confluencespaceds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	expected := 8
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestConfluenceSpaceDataSourceSchemaComputedAttributes verifies computed-only attributes.
func TestConfluenceSpaceDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	ds := confluencespaceds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	computedAttrs := []string{"name", "description", "type", "homepage_id", "status", "url"}
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

// TestConfluenceSpaceDataSourceSchemaOptionalAttributes verifies optional lookup attributes.
func TestConfluenceSpaceDataSourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	ds := confluencespaceds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"id", "key"}
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

// TestConfluenceSpaceDataSourceImplementsDataSource verifies the DataSource interface.
func TestConfluenceSpaceDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	ds := confluencespaceds.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected confluence space data source to implement datasource.DataSource")
	}
}

// ==================== RESOURCE CRUD LIFECYCLE TESTS ====================

// TestConfluenceSpaceResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestConfluenceSpaceResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "CRUD"),
		"name":        tftypes.NewValue(tftypes.String, "CRUD Space"),
		"description": tftypes.NewValue(tftypes.String, "A test space"),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	spaceID := getStringAttr(t, createResp.State, "id")
	if spaceID == "" {
		t.Fatal("expected non-empty id")
	}
	if key := getStringAttr(t, createResp.State, "key"); key != "CRUD" {
		t.Errorf("expected key 'CRUD', got %q", key)
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "CRUD Space" {
		t.Errorf("expected name 'CRUD Space', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "A test space" {
		t.Errorf("expected description 'A test space', got %q", desc)
	}
	if st := getStringAttr(t, createResp.State, "type"); st != "global" {
		t.Errorf("expected type 'global', got %q", st)
	}
	if hp := getStringAttr(t, createResp.State, "homepage_id"); hp == "" {
		t.Error("expected non-empty homepage_id")
	}
	if status := getStringAttr(t, createResp.State, "status"); status != "current" {
		t.Errorf("expected status 'current', got %q", status)
	}
	if url := getStringAttr(t, createResp.State, "url"); url == "" {
		t.Error("expected non-empty url")
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, spaceID),
		"key":         tftypes.NewValue(tftypes.String, "CRUD"),
		"name":        tftypes.NewValue(tftypes.String, "CRUD Space"),
		"description": tftypes.NewValue(tftypes.String, "A test space"),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, fmt.Sprintf("page-%s", spaceID)),
		"status":      tftypes.NewValue(tftypes.String, "current"),
		"url":         tftypes.NewValue(tftypes.String, "https://example.atlassian.net/wiki/spaces/CRUD"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "CRUD Space" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, spaceID),
		"key":         tftypes.NewValue(tftypes.String, "CRUD"),
		"name":        tftypes.NewValue(tftypes.String, "Updated Space"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
		"type":        tftypes.NewValue(tftypes.String, "personal"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Space" {
		t.Errorf("Update name: got %q", name)
	}
	if st := getStringAttr(t, updateResp.State, "type"); st != "personal" {
		t.Errorf("Update type: got %q", st)
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

// TestConfluenceSpaceResourceCreatePersonal tests creating a personal space.
func TestConfluenceSpaceResourceCreatePersonal(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "PERS"),
		"name":        tftypes.NewValue(tftypes.String, "Personal Space"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type":        tftypes.NewValue(tftypes.String, "personal"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create personal: %v", createResp.Diagnostics.Errors())
	}
	if st := getStringAttr(t, createResp.State, "type"); st != "personal" {
		t.Errorf("expected type 'personal', got %q", st)
	}
}

// TestConfluenceSpaceResourceCreateDuplicateKey tests creating a space with a duplicate key.
func TestConfluenceSpaceResourceCreateDuplicateKey(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "DUP"),
		"name":        tftypes.NewValue(tftypes.String, "First Space"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "DUP"),
		"name":        tftypes.NewValue(tftypes.String, "Second Space"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate key error")
	}
}

// TestConfluenceSpaceResourceUpdateNotFound tests updating a nonexistent space.
func TestConfluenceSpaceResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"key":         tftypes.NewValue(tftypes.String, "NOPE"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent space")
	}
}

// TestConfluenceSpaceResourceDeleteNotFound tests deleting an already-deleted space.
func TestConfluenceSpaceResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"key":         tftypes.NewValue(tftypes.String, "NOPE"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent space should not error (idempotent)")
	}
}

// TestConfluenceSpaceResourceReadNotFound tests reading a nonexistent space removes resource.
func TestConfluenceSpaceResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"key":         tftypes.NewValue(tftypes.String, "NOPE"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
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

// TestConfluenceSpaceResourceCreateForbidden tests 403 on create.
func TestConfluenceSpaceResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "FORBID"),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestConfluenceSpaceResourceUpdateForbidden tests 403 on update.
func TestConfluenceSpaceResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"key":         tftypes.NewValue(tftypes.String, "FORBID"),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestConfluenceSpaceResourceDeleteForbidden tests 403 on delete.
func TestConfluenceSpaceResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"key":         tftypes.NewValue(tftypes.String, "FORBID"),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestConfluenceSpaceResourceConfigureNil verifies nil provider data does not error.
func TestConfluenceSpaceResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := confluencespacers.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestConfluenceSpaceResourceConfigureWrongType verifies wrong provider data type errors.
func TestConfluenceSpaceResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := confluencespacers.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestConfluenceSpaceResourceImportState verifies import state passthrough.
func TestConfluenceSpaceResourceImportState(t *testing.T) {
	t.Parallel()
	r := confluencespacers.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "cs-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== DATA SOURCE CRUD TESTS ====================

// TestConfluenceSpaceDataSourceByID tests reading a space by ID.
func TestConfluenceSpaceDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()

	// Create a space first via resource
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "DSID"),
		"name":        tftypes.NewValue(tftypes.String, "DS By ID"),
		"description": tftypes.NewValue(tftypes.String, "desc"),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	spaceID := getStringAttr(t, cResp.State, "id")

	// Read via data source by ID
	ds := confluencespaceds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, spaceID),
		"key":         tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"type":        tftypes.NewValue(tftypes.String, nil),
		"homepage_id": tftypes.NewValue(tftypes.String, nil),
		"status":      tftypes.NewValue(tftypes.String, nil),
		"url":         tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS By ID" {
		t.Errorf("expected name 'DS By ID', got %q", name)
	}
	if key := getStringAttr(t, dsResp.State, "key"); key != "DSID" {
		t.Errorf("expected key 'DSID', got %q", key)
	}
}

// TestConfluenceSpaceDataSourceByKey tests reading a space by key.
func TestConfluenceSpaceDataSourceByKey(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()

	// Create a space first
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "DSKEY"),
		"name":        tftypes.NewValue(tftypes.String, "DS By Key"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type":        tftypes.NewValue(tftypes.String, "personal"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read via data source by key
	ds := confluencespaceds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nil),
		"key":         tftypes.NewValue(tftypes.String, "DSKEY"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"type":        tftypes.NewValue(tftypes.String, nil),
		"homepage_id": tftypes.NewValue(tftypes.String, nil),
		"status":      tftypes.NewValue(tftypes.String, nil),
		"url":         tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by key: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS By Key" {
		t.Errorf("expected name 'DS By Key', got %q", name)
	}
	if st := getStringAttr(t, dsResp.State, "type"); st != "personal" {
		t.Errorf("expected type 'personal', got %q", st)
	}
}

// TestConfluenceSpaceDataSourceMissingBoth tests error when neither id nor key is set.
func TestConfluenceSpaceDataSourceMissingBoth(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	ds := confluencespaceds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nil),
		"key":         tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"type":        tftypes.NewValue(tftypes.String, nil),
		"homepage_id": tftypes.NewValue(tftypes.String, nil),
		"status":      tftypes.NewValue(tftypes.String, nil),
		"url":         tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when neither id nor key is set")
	}
}

// TestConfluenceSpaceDataSourceNotFound tests 404 error on data source read.
func TestConfluenceSpaceDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	ds := confluencespaceds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"key":         tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"type":        tftypes.NewValue(tftypes.String, nil),
		"homepage_id": tftypes.NewValue(tftypes.String, nil),
		"status":      tftypes.NewValue(tftypes.String, nil),
		"url":         tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent space")
	}
}

// TestConfluenceSpaceDataSourceConfigureNil verifies nil provider data does not error.
func TestConfluenceSpaceDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := confluencespaceds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestConfluenceSpaceDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestConfluenceSpaceDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := confluencespaceds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// ==================== SERVER ERROR (500) TESTS ====================

// TestConfluenceSpaceResourceCreateServerError tests generic error on create.
func TestConfluenceSpaceResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "ERR"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// TestConfluenceSpaceResourceReadServerError tests generic error on read.
func TestConfluenceSpaceResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"key":         tftypes.NewValue(tftypes.String, "ERR"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestConfluenceSpaceResourceUpdateServerError tests generic error on update.
func TestConfluenceSpaceResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"key":         tftypes.NewValue(tftypes.String, "ERR"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestConfluenceSpaceResourceDeleteServerError tests generic error on delete.
func TestConfluenceSpaceResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"key":         tftypes.NewValue(tftypes.String, "ERR"),
		"name":        tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestConfluenceSpaceDataSourceReadServerError tests generic error on data source read.
func TestConfluenceSpaceDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	ds := confluencespaceds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"key":         tftypes.NewValue(tftypes.String, nil),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"type":        tftypes.NewValue(tftypes.String, nil),
		"homepage_id": tftypes.NewValue(tftypes.String, nil),
		"status":      tftypes.NewValue(tftypes.String, nil),
		"url":         tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// ==================== BAD PLAN/STATE TESTS ====================

// TestConfluenceSpaceResourceCreateBadPlan tests Create with invalid plan data.
func TestConfluenceSpaceResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestConfluenceSpaceResourceReadBadState tests Read with invalid state data.
func TestConfluenceSpaceResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestConfluenceSpaceResourceUpdateBadPlan tests Update with invalid plan data.
func TestConfluenceSpaceResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "x"),
		"key":         tftypes.NewValue(tftypes.String, "X"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestConfluenceSpaceResourceUpdateBadState tests Update with invalid state data.
func TestConfluenceSpaceResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "x"),
		"key":         tftypes.NewValue(tftypes.String, "X"),
		"name":        tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestConfluenceSpaceResourceDeleteBadState tests Delete with invalid state data.
func TestConfluenceSpaceResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// TestConfluenceSpaceDataSourceReadBadConfig tests data source Read with invalid config data.
func TestConfluenceSpaceDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	ds := confluencespaceds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// TestConfluenceSpaceResourceReadByKeyFallback tests reading by key when ID is empty.
func TestConfluenceSpaceResourceReadByKeyFallback(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceSpaceMockServer(t)
	ctx := context.Background()
	r := confluencespacers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// First create a space
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "KEYRD"),
		"name":        tftypes.NewValue(tftypes.String, "Key Read"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read with empty ID but key set (import scenario)
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, ""),
		"key":         tftypes.NewValue(tftypes.String, "KEYRD"),
		"name":        tftypes.NewValue(tftypes.String, ""),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, ""),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read by key: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Key Read" {
		t.Errorf("expected name 'Key Read', got %q", name)
	}
}
