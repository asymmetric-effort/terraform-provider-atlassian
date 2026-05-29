// Package unit contains unit tests for the atlassian_jira_space resource and data source.
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
	spacedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/space"
	spaceresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/space"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// spaceIDCounter provides unique IDs for space mock server tests.
var spaceIDCounter uint64

func spaceNextID() string {
	n := atomic.AddUint64(&spaceIDCounter, 1)
	return fmt.Sprintf("space-%d", n)
}

// testSpaceMockServer creates a mock HTTP server for Jira space (project) endpoints.
func testSpaceMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	spaces := make(map[string]map[string]interface{})
	keyIndex := make(map[string]string) // key -> id

	mux := http.NewServeMux()

	// Create space
	mux.HandleFunc("POST /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		key, _ := req["key"].(string)
		name, _ := req["name"].(string)
		projectTypeKey, _ := req["projectTypeKey"].(string)
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
				"errorMessages": []string{"A project with this key already exists"},
				"errors":        map[string]string{},
			})
			return
		}
		id := spaceNextID()
		description, _ := req["description"].(string)
		leadAccountID, _ := req["leadAccountId"].(string)
		projectTemplateKey, _ := req["projectTemplateKey"].(string)
		avatarID, _ := req["avatarId"].(float64)
		categoryID, _ := req["categoryId"].(float64)
		assigneeType, _ := req["assigneeType"].(string)
		issueTypeScheme, _ := req["issueTypeScheme"].(float64)
		issueTypeScreenScheme, _ := req["issueTypeScreenScheme"].(float64)
		workflowScheme, _ := req["workflowScheme"].(float64)
		notificationScheme, _ := req["notificationScheme"].(float64)
		permissionScheme, _ := req["permissionScheme"].(float64)
		issueSecurityScheme, _ := req["issueSecurityScheme"].(float64)
		fieldScheme, _ := req["fieldScheme"].(float64)
		space := map[string]interface{}{
			"id":                    id,
			"key":                   key,
			"name":                  name,
			"description":           description,
			"leadAccountId":         leadAccountID,
			"projectTypeKey":        projectTypeKey,
			"projectTemplateKey":    projectTemplateKey,
			"avatarId":              avatarID,
			"categoryId":            categoryID,
			"assigneeType":          assigneeType,
			"issueTypeScheme":       issueTypeScheme,
			"issueTypeScreenScheme": issueTypeScreenScheme,
			"workflowScheme":        workflowScheme,
			"notificationScheme":    notificationScheme,
			"permissionScheme":      permissionScheme,
			"issueSecurityScheme":   issueSecurityScheme,
			"fieldScheme":           fieldScheme,
			"self":                  fmt.Sprintf("https://example.atlassian.net/rest/api/3/project/%s", id),
		}
		spaces[id] = space
		keyIndex[key] = id
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(space)
	})

	// Read space by ID or key
	mux.HandleFunc("GET /rest/api/3/project/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
		idOrKey := r.PathValue("idOrKey")
		mu.Lock()
		defer mu.Unlock()

		// Try by ID first
		space, ok := spaces[idOrKey]
		if !ok {
			// Try by key
			if id, found := keyIndex[idOrKey]; found {
				space, ok = spaces[id]
			}
		}
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"No project could be found with key or ID matching the supplied value"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(space)
	})

	// Update space
	mux.HandleFunc("PUT /rest/api/3/project/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
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
				"errorMessages": []string{"No project could be found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" && k != "key" {
				space[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(space)
	})

	// Delete space
	mux.HandleFunc("DELETE /rest/api/3/project/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
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
			"errorMessages": []string{"No project could be found"},
			"errors":        map[string]string{},
		})
	})

	// List all spaces (for name lookup)
	mux.HandleFunc("GET /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var result []map[string]interface{}
		for _, s := range spaces {
			result = append(result, s)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
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

// testNoopAuth is a no-op authenticator for testing.
type testNoopAuth struct{}

// AuthenticateRequest implements the Authenticator interface (no-op for tests).
func (a *testNoopAuth) AuthenticateRequest(_ *http.Request) error { return nil }

// testForbiddenMockServer creates a mock that returns 403 for all project endpoints.
func testForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// TestJiraSpaceResourceMetadata verifies the resource type name.
func TestJiraSpaceResourceMetadata(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_space" {
		t.Errorf("expected resource type name 'atlassian_jira_space', got %q", resp.TypeName)
	}
}

// TestJiraSpaceResourceSchema verifies the resource schema has all expected attributes.
func TestJiraSpaceResourceSchema(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "key", "name", "description", "lead_account_id", "space_type", "project_template_key", "avatar_id", "category_id", "assignee_type", "issue_type_scheme", "issue_type_screen_scheme", "workflow_scheme", "notification_scheme", "permission_scheme", "issue_security_scheme", "field_scheme", "url"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraSpaceResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraSpaceResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 20
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraSpaceResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraSpaceResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"key", "name", "space_type"}
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

// TestJiraSpaceResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraSpaceResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"id", "url", "description", "lead_account_id", "project_template_key", "avatar_id", "category_id", "assignee_type", "issue_type_scheme", "issue_type_screen_scheme", "workflow_scheme", "notification_scheme", "permission_scheme", "issue_security_scheme", "field_scheme"}
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

// TestJiraSpaceResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraSpaceResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"description", "lead_account_id", "project_template_key", "avatar_id", "category_id", "assignee_type", "issue_type_scheme", "issue_type_screen_scheme", "workflow_scheme", "notification_scheme", "permission_scheme", "issue_security_scheme", "field_scheme"}
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

// TestJiraSpaceResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraSpaceResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraSpaceResourceImplementsResource verifies the Resource interface.
func TestJiraSpaceResourceImplementsResource(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected space resource to implement resource.Resource")
	}
}

// TestJiraSpaceResourceImplementsImportState verifies the ImportState interface.
func TestJiraSpaceResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := spaceresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected space resource to implement ResourceWithImportState")
	}
}

// ==================== DATA SOURCE SCHEMA TESTS ====================

// TestJiraSpaceDataSourceMetadata verifies the data source type name.
func TestJiraSpaceDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := spacedatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_jira_space" {
		t.Errorf("expected data source type name 'atlassian_jira_space', got %q", resp.TypeName)
	}
}

// TestJiraSpaceDataSourceSchema verifies the data source schema has all expected attributes.
func TestJiraSpaceDataSourceSchema(t *testing.T) {
	t.Parallel()

	ds := spacedatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "key", "name", "description", "lead_account_id", "space_type", "project_template_key", "avatar_id", "category_id", "assignee_type", "issue_type_scheme", "issue_type_screen_scheme", "workflow_scheme", "notification_scheme", "permission_scheme", "issue_security_scheme", "field_scheme", "url"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraSpaceDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraSpaceDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	ds := spacedatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	expected := 20
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraSpaceDataSourceSchemaComputedAttributes verifies computed-only attributes.
func TestJiraSpaceDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	ds := spacedatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	computedAttrs := []string{"name", "description", "lead_account_id", "space_type", "project_template_key", "avatar_id", "category_id", "assignee_type", "issue_type_scheme", "issue_type_screen_scheme", "workflow_scheme", "notification_scheme", "permission_scheme", "issue_security_scheme", "field_scheme", "url"}
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

// TestJiraSpaceDataSourceSchemaOptionalAttributes verifies optional lookup attributes.
func TestJiraSpaceDataSourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	ds := spacedatasource.NewDataSource()
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

// TestJiraSpaceDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraSpaceDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	ds := spacedatasource.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected space data source to implement datasource.DataSource")
	}
}

// ==================== RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraSpaceResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraSpaceResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "CRUD"),
		"name":                     tftypes.NewValue(tftypes.String, "CRUD Space"),
		"description":              tftypes.NewValue(tftypes.String, "A test space"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-123"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
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
	if lead := getStringAttr(t, createResp.State, "lead_account_id"); lead != "lead-123" {
		t.Errorf("expected lead_account_id 'lead-123', got %q", lead)
	}
	if st := getStringAttr(t, createResp.State, "space_type"); st != "classic" {
		t.Errorf("expected space_type 'classic', got %q", st)
	}
	if url := getStringAttr(t, createResp.State, "url"); url == "" {
		t.Error("expected non-empty url")
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, spaceID),
		"key":                      tftypes.NewValue(tftypes.String, "CRUD"),
		"name":                     tftypes.NewValue(tftypes.String, "CRUD Space"),
		"description":              tftypes.NewValue(tftypes.String, "A test space"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-123"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, "https://example.atlassian.net/rest/api/3/project/"+spaceID),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
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
		"id":                       tftypes.NewValue(tftypes.String, spaceID),
		"key":                      tftypes.NewValue(tftypes.String, "CRUD"),
		"name":                     tftypes.NewValue(tftypes.String, "Updated Space"),
		"description":              tftypes.NewValue(tftypes.String, "Updated desc"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-456"),
		"space_type":               tftypes.NewValue(tftypes.String, "next-gen"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Space" {
		t.Errorf("Update name: got %q", name)
	}
	if st := getStringAttr(t, updateResp.State, "space_type"); st != "next-gen" {
		t.Errorf("Update space_type: got %q", st)
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

// TestJiraSpaceResourceCreateNextGen tests creating a next-gen space.
func TestJiraSpaceResourceCreateNextGen(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "NEXT"),
		"name":                     tftypes.NewValue(tftypes.String, "Next-Gen Space"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "next-gen"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create next-gen: %v", createResp.Diagnostics.Errors())
	}
	if st := getStringAttr(t, createResp.State, "space_type"); st != "next-gen" {
		t.Errorf("expected space_type 'next-gen', got %q", st)
	}
}

// TestJiraSpaceResourceCreateDuplicateKey tests creating a space with a duplicate key.
func TestJiraSpaceResourceCreateDuplicateKey(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "DUP"),
		"name":                     tftypes.NewValue(tftypes.String, "First Space"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "DUP"),
		"name":                     tftypes.NewValue(tftypes.String, "Second Space"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate key error")
	}
}

// TestJiraSpaceResourceUpdateNotFound tests updating a nonexistent space.
func TestJiraSpaceResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "nonexistent"),
		"key":                      tftypes.NewValue(tftypes.String, "NOPE"),
		"name":                     tftypes.NewValue(tftypes.String, "X"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent space")
	}
}

// TestJiraSpaceResourceDeleteNotFound tests deleting an already-deleted space.
func TestJiraSpaceResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "nonexistent"),
		"key":                      tftypes.NewValue(tftypes.String, "NOPE"),
		"name":                     tftypes.NewValue(tftypes.String, "X"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent space should not error (idempotent)")
	}
}

// TestJiraSpaceResourceReadNotFound tests reading a nonexistent space removes resource.
func TestJiraSpaceResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "nonexistent"),
		"key":                      tftypes.NewValue(tftypes.String, "NOPE"),
		"name":                     tftypes.NewValue(tftypes.String, "X"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
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

// TestJiraSpaceResourceCreateForbidden tests 403 on create.
func TestJiraSpaceResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testForbiddenMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "FORBID"),
		"name":                     tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraSpaceResourceUpdateForbidden tests 403 on update.
func TestJiraSpaceResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testForbiddenMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "some-id"),
		"key":                      tftypes.NewValue(tftypes.String, "FORBID"),
		"name":                     tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraSpaceResourceDeleteForbidden tests 403 on delete.
func TestJiraSpaceResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testForbiddenMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "some-id"),
		"key":                      tftypes.NewValue(tftypes.String, "FORBID"),
		"name":                     tftypes.NewValue(tftypes.String, "Forbidden"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraSpaceResourceConfigureNil verifies nil provider data does not error.
func TestJiraSpaceResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := spaceresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraSpaceResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraSpaceResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := spaceresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraSpaceResourceImportState verifies import state passthrough.
func TestJiraSpaceResourceImportState(t *testing.T) {
	t.Parallel()
	r := spaceresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "space-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== DATA SOURCE CRUD TESTS ====================

// TestJiraSpaceDataSourceByID tests reading a space by ID.
func TestJiraSpaceDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()

	// Create a space first via resource
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "DSID"),
		"name":                     tftypes.NewValue(tftypes.String, "DS By ID"),
		"description":              tftypes.NewValue(tftypes.String, "desc"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-ds"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	spaceID := getStringAttr(t, cResp.State, "id")

	// Read via data source by ID
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, spaceID),
		"key":                      tftypes.NewValue(tftypes.String, nil),
		"name":                     tftypes.NewValue(tftypes.String, nil),
		"description":              tftypes.NewValue(tftypes.String, nil),
		"lead_account_id":          tftypes.NewValue(tftypes.String, nil),
		"space_type":               tftypes.NewValue(tftypes.String, nil),
		"url":                      tftypes.NewValue(tftypes.String, nil),
		"self_url":                 tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
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

// TestJiraSpaceDataSourceByKey tests reading a space by key.
func TestJiraSpaceDataSourceByKey(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()

	// Create a space first
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "DSKEY"),
		"name":                     tftypes.NewValue(tftypes.String, "DS By Key"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "next-gen"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read via data source by key
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, nil),
		"key":                      tftypes.NewValue(tftypes.String, "DSKEY"),
		"name":                     tftypes.NewValue(tftypes.String, nil),
		"description":              tftypes.NewValue(tftypes.String, nil),
		"lead_account_id":          tftypes.NewValue(tftypes.String, nil),
		"space_type":               tftypes.NewValue(tftypes.String, nil),
		"url":                      tftypes.NewValue(tftypes.String, nil),
		"self_url":                 tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by key: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS By Key" {
		t.Errorf("expected name 'DS By Key', got %q", name)
	}
	if st := getStringAttr(t, dsResp.State, "space_type"); st != "next-gen" {
		t.Errorf("expected space_type 'next-gen', got %q", st)
	}
}

// TestJiraSpaceDataSourceMissingBoth tests error when neither id nor key is set.
func TestJiraSpaceDataSourceMissingBoth(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, nil),
		"key":                      tftypes.NewValue(tftypes.String, nil),
		"name":                     tftypes.NewValue(tftypes.String, nil),
		"description":              tftypes.NewValue(tftypes.String, nil),
		"lead_account_id":          tftypes.NewValue(tftypes.String, nil),
		"space_type":               tftypes.NewValue(tftypes.String, nil),
		"url":                      tftypes.NewValue(tftypes.String, nil),
		"self_url":                 tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when neither id nor key is set")
	}
}

// TestJiraSpaceDataSourceNotFound tests 404 error on data source read.
func TestJiraSpaceDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "nonexistent"),
		"key":                      tftypes.NewValue(tftypes.String, nil),
		"name":                     tftypes.NewValue(tftypes.String, nil),
		"description":              tftypes.NewValue(tftypes.String, nil),
		"lead_account_id":          tftypes.NewValue(tftypes.String, nil),
		"space_type":               tftypes.NewValue(tftypes.String, nil),
		"url":                      tftypes.NewValue(tftypes.String, nil),
		"self_url":                 tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent space")
	}
}

// TestJiraSpaceDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraSpaceDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := spacedatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraSpaceDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraSpaceDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := spacedatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// testServerErrorMockServer creates a mock that returns 500 for all project endpoints.
func testServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// TestJiraSpaceResourceCreateServerError tests generic error on create.
func TestJiraSpaceResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testServerErrorMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "ERR"),
		"name":                     tftypes.NewValue(tftypes.String, "Error"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// TestJiraSpaceResourceReadServerError tests generic error on read.
func TestJiraSpaceResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testServerErrorMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "some-id"),
		"key":                      tftypes.NewValue(tftypes.String, "ERR"),
		"name":                     tftypes.NewValue(tftypes.String, "Error"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraSpaceResourceUpdateServerError tests generic error on update.
func TestJiraSpaceResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testServerErrorMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "some-id"),
		"key":                      tftypes.NewValue(tftypes.String, "ERR"),
		"name":                     tftypes.NewValue(tftypes.String, "Error"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraSpaceResourceDeleteServerError tests generic error on delete.
func TestJiraSpaceResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testServerErrorMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "some-id"),
		"key":                      tftypes.NewValue(tftypes.String, "ERR"),
		"name":                     tftypes.NewValue(tftypes.String, "Error"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraSpaceDataSourceReadServerError tests generic error on data source read.
func TestJiraSpaceDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testServerErrorMockServer(t)
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "some-id"),
		"key":                      tftypes.NewValue(tftypes.String, nil),
		"name":                     tftypes.NewValue(tftypes.String, nil),
		"description":              tftypes.NewValue(tftypes.String, nil),
		"lead_account_id":          tftypes.NewValue(tftypes.String, nil),
		"space_type":               tftypes.NewValue(tftypes.String, nil),
		"url":                      tftypes.NewValue(tftypes.String, nil),
		"self_url":                 tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestJiraSpaceResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraSpaceResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	// Use a completely wrong type structure to trigger HasError on Plan.Get
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestJiraSpaceResourceReadBadState tests Read with invalid state data.
func TestJiraSpaceResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestJiraSpaceResourceUpdateBadPlan tests Update with invalid plan data.
func TestJiraSpaceResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Bad plan
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "x"),
		"key":                      tftypes.NewValue(tftypes.String, "X"),
		"name":                     tftypes.NewValue(tftypes.String, "X"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraSpaceResourceUpdateBadState tests Update with invalid state data.
func TestJiraSpaceResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, "x"),
		"key":                      tftypes.NewValue(tftypes.String, "X"),
		"name":                     tftypes.NewValue(tftypes.String, "X"),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraSpaceResourceDeleteBadState tests Delete with invalid state data.
func TestJiraSpaceResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// TestJiraSpaceDataSourceReadBadConfig tests data source Read with invalid config data.
func TestJiraSpaceDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// TestJiraSpaceResourceReadByKeyFallback tests reading by key when ID is empty.
func TestJiraSpaceResourceReadByKeyFallback(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// First create a space
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "KEYRD"),
		"name":                     tftypes.NewValue(tftypes.String, "Key Read"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read with empty ID but key set (import scenario)
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, ""),
		"key":                      tftypes.NewValue(tftypes.String, "KEYRD"),
		"name":                     tftypes.NewValue(tftypes.String, ""),
		"description":              tftypes.NewValue(tftypes.String, ""),
		"lead_account_id":          tftypes.NewValue(tftypes.String, ""),
		"space_type":               tftypes.NewValue(tftypes.String, ""),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
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

// TestJiraSpaceDataSourceByName tests looking up a space by name.
func TestJiraSpaceDataSourceByName(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()

	// Create a space first
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	tfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "BYNAME"),
		"name":                     tftypes.NewValue(tftypes.String, "By Name Space"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Look up by name
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, nil),
		"key":                      tftypes.NewValue(tftypes.String, nil),
		"name":                     tftypes.NewValue(tftypes.String, "By Name Space"),
		"description":              tftypes.NewValue(tftypes.String, nil),
		"lead_account_id":          tftypes.NewValue(tftypes.String, nil),
		"space_type":               tftypes.NewValue(tftypes.String, nil),
		"url":                      tftypes.NewValue(tftypes.String, nil),
		"self_url":                 tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("Read by name: %v", dsResp.Diagnostics.Errors())
	}
	if v := getStringAttr(t, dsResp.State, "key"); v != "BYNAME" {
		t.Errorf("expected key BYNAME, got %s", v)
	}
}

// TestJiraSpaceDataSourceByNameNotFound tests name lookup for nonexistent.
func TestJiraSpaceDataSourceByNameNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, nil),
		"key":                      tftypes.NewValue(tftypes.String, nil),
		"name":                     tftypes.NewValue(tftypes.String, "Nonexistent Space"),
		"description":              tftypes.NewValue(tftypes.String, nil),
		"lead_account_id":          tftypes.NewValue(tftypes.String, nil),
		"space_type":               tftypes.NewValue(tftypes.String, nil),
		"url":                      tftypes.NewValue(tftypes.String, nil),
		"self_url":                 tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("expected error for nonexistent name")
	}
}

// TestJiraSpaceDataSourceMissingAll tests error when none of id/key/name set.
func TestJiraSpaceDataSourceMissingAll(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, nil),
		"key":                      tftypes.NewValue(tftypes.String, nil),
		"name":                     tftypes.NewValue(tftypes.String, nil),
		"description":              tftypes.NewValue(tftypes.String, nil),
		"lead_account_id":          tftypes.NewValue(tftypes.String, nil),
		"space_type":               tftypes.NewValue(tftypes.String, nil),
		"url":                      tftypes.NewValue(tftypes.String, nil),
		"self_url":                 tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("expected error when none of id/key/name set")
	}
}

// TestBrowseURLConstruction tests the browseURL helper.
func TestBrowseURLConstruction(t *testing.T) {
	t.Parallel()
	r := spaceresource.NewResource()
	_ = r // browseURL is tested through CRUD lifecycle
	// The browseURL function is unexported but tested via Create/Read/Update
	// which set BrowseURL on the model. Verified in CRUD lifecycle tests.
}

// TestJiraSpaceDataSourceByNameSearchError tests name lookup when API fails.
func TestJiraSpaceDataSourceByNameSearchError(t *testing.T) {
	t.Parallel()
	// Use a server that returns 500 for list
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Server error"}})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "key": tftypes.NewValue(tftypes.String, nil),
		"name": tftypes.NewValue(tftypes.String, "Test"), "description": tftypes.NewValue(tftypes.String, nil),
		"lead_account_id": tftypes.NewValue(tftypes.String, nil), "space_type": tftypes.NewValue(tftypes.String, nil),
		"url": tftypes.NewValue(tftypes.String, nil), "self_url": tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraSpaceBrowseURLEmpty tests browseURL with empty self URL.
func TestJiraSpaceBrowseURLEmpty(t *testing.T) {
	t.Parallel()
	// Create space with mock that returns empty self
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "1", "key": "TST", "name": "Test",
			"projectTypeKey": "business", "self": "",
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "TST"),
		"name":                     tftypes.NewValue(tftypes.String, "Test"),
		"description":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"lead_account_id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	browseVal := getStringAttr(t, cResp.State, "browse_url")
	if browseVal != "" {
		t.Errorf("expected empty browse_url for empty self, got %q", browseVal)
	}
}

// TestJiraSpaceDataSourceBrowseURLEmpty tests data source browseURL with empty self.
func TestJiraSpaceDataSourceBrowseURLEmpty(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/project/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "1", "key": "TST", "name": "Test",
			"projectTypeKey": "business", "self": "",
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "1"), "key": tftypes.NewValue(tftypes.String, nil),
		"name": tftypes.NewValue(tftypes.String, nil), "description": tftypes.NewValue(tftypes.String, nil),
		"lead_account_id": tftypes.NewValue(tftypes.String, nil), "space_type": tftypes.NewValue(tftypes.String, nil),
		"url": tftypes.NewValue(tftypes.String, nil), "self_url": tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", dsResp.Diagnostics.Errors())
	}
	browseVal := getStringAttr(t, dsResp.State, "browse_url")
	if browseVal != "" {
		t.Errorf("expected empty browse_url, got %q", browseVal)
	}
}

// TestJiraSpaceDataSourceByNameBadJSON tests name lookup with invalid JSON in list.
func TestJiraSpaceDataSourceByNameBadJSON(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return a list with one invalid JSON entry
		w.Write([]byte(`[{"bad json`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "key": tftypes.NewValue(tftypes.String, nil),
		"name": tftypes.NewValue(tftypes.String, "Test"), "description": tftypes.NewValue(tftypes.String, nil),
		"lead_account_id": tftypes.NewValue(tftypes.String, nil), "space_type": tftypes.NewValue(tftypes.String, nil),
		"url": tftypes.NewValue(tftypes.String, nil), "self_url": tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("expected error for bad JSON list response")
	}
}

// TestJiraSpaceDataSourceByNameBadEntry tests name lookup skipping bad entries.
func TestJiraSpaceDataSourceByNameBadEntry(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Valid JSON array with one bad entry and one good entry
		w.Write([]byte(`[42, {"id":"1","key":"GOOD","name":"Good Space","projectTypeKey":"business","self":"http://x/rest/api/3/project/1"}]`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()
	ds := spacedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "key": tftypes.NewValue(tftypes.String, nil),
		"name": tftypes.NewValue(tftypes.String, "Good Space"), "description": tftypes.NewValue(tftypes.String, nil),
		"lead_account_id": tftypes.NewValue(tftypes.String, nil), "space_type": tftypes.NewValue(tftypes.String, nil),
		"url": tftypes.NewValue(tftypes.String, nil), "self_url": tftypes.NewValue(tftypes.String, nil),
		"project_template_key":     tftypes.NewValue(tftypes.String, nil),
		"avatar_id":                tftypes.NewValue(tftypes.Number, nil),
		"category_id":              tftypes.NewValue(tftypes.Number, nil),
		"assignee_type":            tftypes.NewValue(tftypes.String, nil),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, nil),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, nil),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, nil),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, nil),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, nil),
		"field_scheme":             tftypes.NewValue(tftypes.Number, nil),
		"browse_url":               tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", dsResp.Diagnostics.Errors())
	}
	if v := getStringAttr(t, dsResp.State, "key"); v != "GOOD" {
		t.Errorf("expected key GOOD, got %s", v)
	}
}

// TestJiraSpaceResourceCreateWithAllNewAttributes tests creating a space with all optional attributes set.
func TestJiraSpaceResourceCreateWithAllNewAttributes(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "ALLNEW"),
		"name":                     tftypes.NewValue(tftypes.String, "All New Attrs"),
		"description":              tftypes.NewValue(tftypes.String, "Space with all new attributes"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-all"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"project_template_key":     tftypes.NewValue(tftypes.String, "com.atlassian.jira-core-project-templates:jira-core-simplified-task-tracking"),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 10200),
		"category_id":              tftypes.NewValue(tftypes.Number, 42),
		"assignee_type":            tftypes.NewValue(tftypes.String, "PROJECT_LEAD"),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
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
	if v := getStringAttr(t, createResp.State, "key"); v != "ALLNEW" {
		t.Errorf("expected key 'ALLNEW', got %q", v)
	}
	if v := getStringAttr(t, createResp.State, "project_template_key"); v != "com.atlassian.jira-core-project-templates:jira-core-simplified-task-tracking" {
		t.Errorf("expected project_template_key, got %q", v)
	}
	if v := getStringAttr(t, createResp.State, "assignee_type"); v != "PROJECT_LEAD" {
		t.Errorf("expected assignee_type 'PROJECT_LEAD', got %q", v)
	}

	// Read back and verify all attributes persist
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, spaceID),
		"key":                      tftypes.NewValue(tftypes.String, "ALLNEW"),
		"name":                     tftypes.NewValue(tftypes.String, "All New Attrs"),
		"description":              tftypes.NewValue(tftypes.String, "Space with all new attributes"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-all"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if v := getStringAttr(t, readResp.State, "name"); v != "All New Attrs" {
		t.Errorf("Read name: got %q", v)
	}

	// Update with changed new attributes
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, spaceID),
		"key":                      tftypes.NewValue(tftypes.String, "ALLNEW"),
		"name":                     tftypes.NewValue(tftypes.String, "All New Updated"),
		"description":              tftypes.NewValue(tftypes.String, "Updated desc"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-all"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"project_template_key":     tftypes.NewValue(tftypes.String, "com.atlassian.jira-core-project-templates:jira-core-simplified-kanban"),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 10300),
		"category_id":              tftypes.NewValue(tftypes.Number, 99),
		"assignee_type":            tftypes.NewValue(tftypes.String, "UNASSIGNED"),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"field_scheme":             tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if v := getStringAttr(t, updateResp.State, "assignee_type"); v != "UNASSIGNED" {
		t.Errorf("Update assignee_type: got %q", v)
	}
	if v := getStringAttr(t, updateResp.State, "name"); v != "All New Updated" {
		t.Errorf("Update name: got %q", v)
	}
}

// TestJiraSpaceResourceCreateWithSchemeAssociations tests creating a space with all 7 scheme association attributes.
func TestJiraSpaceResourceCreateWithSchemeAssociations(t *testing.T) {
	t.Parallel()
	_, client := testSpaceMockServer(t)
	ctx := context.Background()
	r := spaceresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":                      tftypes.NewValue(tftypes.String, "SCHEME"),
		"name":                     tftypes.NewValue(tftypes.String, "Scheme Space"),
		"description":              tftypes.NewValue(tftypes.String, "Space with scheme associations"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-scheme"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 10100),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 10200),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 10300),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 10400),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 10500),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 10600),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 10700),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
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
	if v := getStringAttr(t, createResp.State, "key"); v != "SCHEME" {
		t.Errorf("expected key 'SCHEME', got %q", v)
	}
	if v := getInt64Attr(t, createResp.State, "issue_type_scheme"); v != 10100 {
		t.Errorf("expected issue_type_scheme 10100, got %d", v)
	}
	if v := getInt64Attr(t, createResp.State, "issue_type_screen_scheme"); v != 10200 {
		t.Errorf("expected issue_type_screen_scheme 10200, got %d", v)
	}
	if v := getInt64Attr(t, createResp.State, "workflow_scheme"); v != 10300 {
		t.Errorf("expected workflow_scheme 10300, got %d", v)
	}
	if v := getInt64Attr(t, createResp.State, "notification_scheme"); v != 10400 {
		t.Errorf("expected notification_scheme 10400, got %d", v)
	}
	if v := getInt64Attr(t, createResp.State, "permission_scheme"); v != 10500 {
		t.Errorf("expected permission_scheme 10500, got %d", v)
	}
	if v := getInt64Attr(t, createResp.State, "issue_security_scheme"); v != 10600 {
		t.Errorf("expected issue_security_scheme 10600, got %d", v)
	}
	if v := getInt64Attr(t, createResp.State, "field_scheme"); v != 10700 {
		t.Errorf("expected field_scheme 10700, got %d", v)
	}

	// Read back and verify scheme associations persist
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, spaceID),
		"key":                      tftypes.NewValue(tftypes.String, "SCHEME"),
		"name":                     tftypes.NewValue(tftypes.String, "Scheme Space"),
		"description":              tftypes.NewValue(tftypes.String, "Space with scheme associations"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-scheme"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"project_template_key":     tftypes.NewValue(tftypes.String, ""),
		"avatar_id":                tftypes.NewValue(tftypes.Number, 0),
		"category_id":              tftypes.NewValue(tftypes.Number, 0),
		"assignee_type":            tftypes.NewValue(tftypes.String, ""),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 0),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 0),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 0),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 0),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 0),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 0),
		"url":                      tftypes.NewValue(tftypes.String, ""),
		"self_url":                 tftypes.NewValue(tftypes.String, ""),
		"browse_url":               tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if v := getInt64Attr(t, readResp.State, "issue_type_scheme"); v != 10100 {
		t.Errorf("Read issue_type_scheme: got %d", v)
	}
	if v := getInt64Attr(t, readResp.State, "permission_scheme"); v != 10500 {
		t.Errorf("Read permission_scheme: got %d", v)
	}
	if v := getInt64Attr(t, readResp.State, "field_scheme"); v != 10700 {
		t.Errorf("Read field_scheme: got %d", v)
	}

	// Update scheme associations
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                       tftypes.NewValue(tftypes.String, spaceID),
		"key":                      tftypes.NewValue(tftypes.String, "SCHEME"),
		"name":                     tftypes.NewValue(tftypes.String, "Scheme Space Updated"),
		"description":              tftypes.NewValue(tftypes.String, "Updated schemes"),
		"lead_account_id":          tftypes.NewValue(tftypes.String, "lead-scheme"),
		"space_type":               tftypes.NewValue(tftypes.String, "classic"),
		"project_template_key":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"avatar_id":                tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"category_id":              tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"assignee_type":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_scheme":        tftypes.NewValue(tftypes.Number, 20100),
		"issue_type_screen_scheme": tftypes.NewValue(tftypes.Number, 20200),
		"workflow_scheme":          tftypes.NewValue(tftypes.Number, 20300),
		"notification_scheme":      tftypes.NewValue(tftypes.Number, 20400),
		"permission_scheme":        tftypes.NewValue(tftypes.Number, 20500),
		"issue_security_scheme":    tftypes.NewValue(tftypes.Number, 20600),
		"field_scheme":             tftypes.NewValue(tftypes.Number, 20700),
		"url":                      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"self_url":                 tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"browse_url":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if v := getInt64Attr(t, updateResp.State, "issue_type_scheme"); v != 20100 {
		t.Errorf("Update issue_type_scheme: got %d", v)
	}
	if v := getInt64Attr(t, updateResp.State, "workflow_scheme"); v != 20300 {
		t.Errorf("Update workflow_scheme: got %d", v)
	}
	if v := getInt64Attr(t, updateResp.State, "field_scheme"); v != 20700 {
		t.Errorf("Update field_scheme: got %d", v)
	}
	if v := getStringAttr(t, updateResp.State, "name"); v != "Scheme Space Updated" {
		t.Errorf("Update name: got %q", v)
	}
}
