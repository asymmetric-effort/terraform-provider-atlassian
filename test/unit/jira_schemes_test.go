// Package unit contains unit tests for the atlassian_jira_permission_scheme,
// atlassian_jira_security_scheme, and atlassian_jira_notification_scheme
// resources and data sources.
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
	notificationschemeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/notification_scheme"
	permissionschemeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/permission_scheme"
	securityschemeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/security_scheme"
	notificationschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/notification_scheme"
	permissionschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/permission_scheme"
	securityschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/security_scheme"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// schemeIDCounter provides unique IDs for scheme mock server tests.
var schemeIDCounter uint64

func schemeNextID(prefix string) string {
	n := atomic.AddUint64(&schemeIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// testSchemeMockServer creates a mock HTTP server for permission scheme, security scheme,
// and notification scheme endpoints.
func testSchemeMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	permSchemes := make(map[string]map[string]interface{})
	secSchemes := make(map[string]map[string]interface{})
	notifSchemes := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Permission scheme endpoints
	mux.HandleFunc("POST /rest/api/3/permissionscheme", func(w http.ResponseWriter, r *http.Request) {
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
		for _, ps := range permSchemes {
			if ps["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"A permission scheme with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}
		id := schemeNextID("ps")
		description, _ := req["description"].(string)
		ps := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"self":        fmt.Sprintf("https://example.atlassian.net/rest/api/3/permissionscheme/%s", id),
		}
		permSchemes[id] = ps
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(ps)
	})

	mux.HandleFunc("GET /rest/api/3/permissionscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		ps, ok := permSchemes[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Permission scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ps)
	})

	mux.HandleFunc("PUT /rest/api/3/permissionscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		ps, ok := permSchemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Permission scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				ps[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ps)
	})

	mux.HandleFunc("DELETE /rest/api/3/permissionscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := permSchemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Permission scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(permSchemes, id)
		w.WriteHeader(204)
	})

	// Security scheme endpoints
	mux.HandleFunc("POST /rest/api/3/issuesecurityschemes", func(w http.ResponseWriter, r *http.Request) {
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
		for _, ss := range secSchemes {
			if ss["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"A security scheme with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}
		id := schemeNextID("ss")
		description, _ := req["description"].(string)
		ss := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"self":        fmt.Sprintf("https://example.atlassian.net/rest/api/3/issuesecurityschemes/%s", id),
		}
		if levels, ok := req["security_levels"]; ok && levels != nil {
			ss["security_levels"] = levels
		}
		secSchemes[id] = ss
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(ss)
	})

	mux.HandleFunc("GET /rest/api/3/issuesecurityschemes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		ss, ok := secSchemes[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Security scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ss)
	})

	mux.HandleFunc("PUT /rest/api/3/issuesecurityschemes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		ss, ok := secSchemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Security scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				ss[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ss)
	})

	mux.HandleFunc("DELETE /rest/api/3/issuesecurityschemes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := secSchemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Security scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(secSchemes, id)
		w.WriteHeader(204)
	})

	// Notification scheme endpoints
	mux.HandleFunc("POST /rest/api/3/notificationscheme", func(w http.ResponseWriter, r *http.Request) {
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
		for _, ns := range notifSchemes {
			if ns["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"A notification scheme with this name already exists"},
					"errors":        map[string]string{},
				})
				return
			}
		}
		id := schemeNextID("ns")
		description, _ := req["description"].(string)
		ns := map[string]interface{}{
			"id":          id,
			"name":        name,
			"description": description,
			"self":        fmt.Sprintf("https://example.atlassian.net/rest/api/3/notificationscheme/%s", id),
		}
		notifSchemes[id] = ns
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(ns)
	})

	mux.HandleFunc("GET /rest/api/3/notificationscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		ns, ok := notifSchemes[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Notification scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ns)
	})

	mux.HandleFunc("PUT /rest/api/3/notificationscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		ns, ok := notifSchemes[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Notification scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				ns[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ns)
	})

	mux.HandleFunc("DELETE /rest/api/3/notificationscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := notifSchemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Notification scheme not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(notifSchemes, id)
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

// testSchemeForbiddenMockServer creates a mock that returns 403 for all scheme endpoints.
func testSchemeForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testSchemeServerErrorMockServer creates a mock that returns 500 for all scheme endpoints.
func testSchemeServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// ==================== PERMISSION SCHEME RESOURCE SCHEMA TESTS ====================

// TestJiraPermissionSchemeResourceMetadata verifies the resource type name.
func TestJiraPermissionSchemeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_permission_scheme" {
		t.Errorf("expected resource type name 'atlassian_jira_permission_scheme', got %q", resp.TypeName)
	}
}

// TestJiraPermissionSchemeResourceSchema verifies the resource schema has all expected attributes.
func TestJiraPermissionSchemeResourceSchema(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "description", "grants"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraPermissionSchemeResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraPermissionSchemeResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraPermissionSchemeResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraPermissionSchemeResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
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

// TestJiraPermissionSchemeResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraPermissionSchemeResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
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

// TestJiraPermissionSchemeResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraPermissionSchemeResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
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

// TestJiraPermissionSchemeResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraPermissionSchemeResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraPermissionSchemeResourceImplementsResource verifies the Resource interface.
func TestJiraPermissionSchemeResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected permission scheme resource to implement resource.Resource")
	}
}

// TestJiraPermissionSchemeResourceImplementsImportState verifies the ImportState interface.
func TestJiraPermissionSchemeResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected permission scheme resource to implement ResourceWithImportState")
	}
}

// ==================== PERMISSION SCHEME RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraPermissionSchemeResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraPermissionSchemeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Test Permission Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A test permission scheme"),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	psID := getStringAttr(t, createResp.State, "id")
	if psID == "" {
		t.Fatal("expected non-empty ID after create")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Permission Scheme" {
		t.Errorf("expected name 'Test Permission Scheme', got %q", name)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, psID),
		"name":        tftypes.NewValue(tftypes.String, "Test Permission Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A test permission scheme"),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test Permission Scheme" {
		t.Errorf("expected name 'Test Permission Scheme', got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, psID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Permission Scheme"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, psID),
		"name":        tftypes.NewValue(tftypes.String, "Test Permission Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A test permission scheme"),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Permission Scheme" {
		t.Errorf("expected name 'Updated Permission Scheme', got %q", name)
	}

	// Delete
	delState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, psID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Permission Scheme"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	delResp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: delState}, delResp)
	if delResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", delResp.Diagnostics.Errors())
	}

	// Read after delete should remove resource
	readState2 := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, psID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Permission Scheme"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState2.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState2}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("Read after delete should not error: %v", readResp2.Diagnostics.Errors())
	}
}

// TestJiraPermissionSchemeResourceCreateConflict tests duplicate name 409 error.
func TestJiraPermissionSchemeResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Dup PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("First create: %v", cResp.Diagnostics.Errors())
	}

	cResp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp2)
	if !cResp2.Diagnostics.HasError() {
		t.Fatal("Expected conflict error on duplicate name")
	}
}

// TestJiraPermissionSchemeResourceCreateForbidden tests 403 error on create.
func TestJiraPermissionSchemeResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if !cResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

// TestJiraPermissionSchemeResourceCreateServerError tests 500 error on create.
func TestJiraPermissionSchemeResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Error PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if !cResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestJiraPermissionSchemeResourceReadServerError tests 500 on read.
func TestJiraPermissionSchemeResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraPermissionSchemeResourceUpdateNotFound tests 404 on update.
func TestJiraPermissionSchemeResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "Updated"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "Old"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error on update")
	}
}

// TestJiraPermissionSchemeResourceUpdateForbidden tests 403 on update.
func TestJiraPermissionSchemeResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Updated"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Old"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error on update")
	}
}

// TestJiraPermissionSchemeResourceUpdateServerError tests 500 on update.
func TestJiraPermissionSchemeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Updated"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Old"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on update")
	}
}

// TestJiraPermissionSchemeResourceDeleteForbidden tests 403 on delete.
func TestJiraPermissionSchemeResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error on delete")
	}
}

// TestJiraPermissionSchemeResourceDeleteServerError tests 500 on delete.
func TestJiraPermissionSchemeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on delete")
	}
}

// TestJiraPermissionSchemeResourceDeleteNotFound tests 404 on delete (idempotent).
func TestJiraPermissionSchemeResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete of nonexistent should not error: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraPermissionSchemeResourceConfigureNil verifies nil provider data does not error.
func TestJiraPermissionSchemeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraPermissionSchemeResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraPermissionSchemeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraPermissionSchemeResourceCreatePlanGetError tests create with invalid plan.
func TestJiraPermissionSchemeResourceCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraPermissionSchemeResourceReadStateGetError tests read with invalid state.
func TestJiraPermissionSchemeResourceReadStateGetError(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraPermissionSchemeResourceUpdatePlanGetError tests update with invalid plan.
func TestJiraPermissionSchemeResourceUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.UpdateResponse{State: emptyState(context.Background(), s)}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraPermissionSchemeResourceUpdateStateGetError tests update with invalid state.
func TestJiraPermissionSchemeResourceUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "id"),
		"name":        tftypes.NewValue(tftypes.String, "n"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraPermissionSchemeResourceDeleteStateGetError tests delete with invalid state.
func TestJiraPermissionSchemeResourceDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== PERMISSION SCHEME DATA SOURCE TESTS ====================

// TestJiraPermissionSchemeDataSourceMetadata verifies the data source type name.
func TestJiraPermissionSchemeDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := permissionschemeds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_permission_scheme" {
		t.Errorf("expected data source type name 'atlassian_jira_permission_scheme', got %q", resp.TypeName)
	}
}

// TestJiraPermissionSchemeDataSourceSchema verifies the data source schema.
func TestJiraPermissionSchemeDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := permissionschemeds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "description", "grants"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraPermissionSchemeDataSourceSchemaAttributeCount verifies attribute count.
func TestJiraPermissionSchemeDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := permissionschemeds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)
	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraPermissionSchemeDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraPermissionSchemeDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()
	ds := permissionschemeds.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected permission scheme data source to implement datasource.DataSource")
	}
}

// TestJiraPermissionSchemeDataSourceByID tests reading by ID.
func TestJiraPermissionSchemeDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()

	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS Perm Scheme"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	psID := getStringAttr(t, cResp.State, "id")

	ds := permissionschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, psID),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Perm Scheme" {
		t.Errorf("expected name 'DS Perm Scheme', got %q", name)
	}
}

// TestJiraPermissionSchemeDataSourceNotFound tests 404 error on data source read.
func TestJiraPermissionSchemeDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	ds := permissionschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent permission scheme")
	}
}

// TestJiraPermissionSchemeDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraPermissionSchemeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := permissionschemeds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraPermissionSchemeDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraPermissionSchemeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := permissionschemeds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraPermissionSchemeDataSourceReadServerError tests generic error on data source read.
func TestJiraPermissionSchemeDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	ds := permissionschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestJiraPermissionSchemeDataSourceReadBadConfig tests data source Read with invalid config.
func TestJiraPermissionSchemeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	ds := permissionschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// ==================== SECURITY SCHEME RESOURCE SCHEMA TESTS ====================

// TestJiraSecuritySchemeResourceMetadata verifies the resource type name.
func TestJiraSecuritySchemeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_security_scheme" {
		t.Errorf("expected resource type name 'atlassian_jira_security_scheme', got %q", resp.TypeName)
	}
}

// TestJiraSecuritySchemeResourceSchema verifies the resource schema has all expected attributes.
func TestJiraSecuritySchemeResourceSchema(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "description", "security_levels"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraSecuritySchemeResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraSecuritySchemeResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraSecuritySchemeResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraSecuritySchemeResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
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

// TestJiraSecuritySchemeResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraSecuritySchemeResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
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

// TestJiraSecuritySchemeResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraSecuritySchemeResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
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

// TestJiraSecuritySchemeResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraSecuritySchemeResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraSecuritySchemeResourceImplementsResource verifies the Resource interface.
func TestJiraSecuritySchemeResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected security scheme resource to implement resource.Resource")
	}
}

// TestJiraSecuritySchemeResourceImplementsImportState verifies the ImportState interface.
func TestJiraSecuritySchemeResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected security scheme resource to implement ResourceWithImportState")
	}
}

// ==================== SECURITY SCHEME RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraSecuritySchemeResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraSecuritySchemeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Test Security Scheme"),
		"description":     tftypes.NewValue(tftypes.String, "A test security scheme"),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	ssID := getStringAttr(t, createResp.State, "id")
	if ssID == "" {
		t.Fatal("expected non-empty ID after create")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Security Scheme" {
		t.Errorf("expected name 'Test Security Scheme', got %q", name)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ssID),
		"name":            tftypes.NewValue(tftypes.String, "Test Security Scheme"),
		"description":     tftypes.NewValue(tftypes.String, "A test security scheme"),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test Security Scheme" {
		t.Errorf("expected name 'Test Security Scheme', got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ssID),
		"name":            tftypes.NewValue(tftypes.String, "Updated Security Scheme"),
		"description":     tftypes.NewValue(tftypes.String, "Updated desc"),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ssID),
		"name":            tftypes.NewValue(tftypes.String, "Test Security Scheme"),
		"description":     tftypes.NewValue(tftypes.String, "A test security scheme"),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Security Scheme" {
		t.Errorf("expected name 'Updated Security Scheme', got %q", name)
	}

	// Delete
	delState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ssID),
		"name":            tftypes.NewValue(tftypes.String, "Updated Security Scheme"),
		"description":     tftypes.NewValue(tftypes.String, "Updated desc"),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	delResp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: delState}, delResp)
	if delResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", delResp.Diagnostics.Errors())
	}

	// Read after delete should remove resource
	readState2 := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ssID),
		"name":            tftypes.NewValue(tftypes.String, "Updated Security Scheme"),
		"description":     tftypes.NewValue(tftypes.String, "Updated desc"),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState2.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState2}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("Read after delete should not error: %v", readResp2.Diagnostics.Errors())
	}
}

// TestJiraSecuritySchemeResourceCreateConflict tests duplicate name 409 error.
func TestJiraSecuritySchemeResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Dup SS"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("First create: %v", cResp.Diagnostics.Errors())
	}

	cResp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp2)
	if !cResp2.Diagnostics.HasError() {
		t.Fatal("Expected conflict error on duplicate name")
	}
}

// TestJiraSecuritySchemeResourceCreateForbidden tests 403 error on create.
func TestJiraSecuritySchemeResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Forbidden SS"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if !cResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

// TestJiraSecuritySchemeResourceCreateServerError tests 500 error on create.
func TestJiraSecuritySchemeResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Error SS"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if !cResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestJiraSecuritySchemeResourceReadServerError tests 500 on read.
func TestJiraSecuritySchemeResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "SS"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraSecuritySchemeResourceUpdateNotFound tests 404 on update.
func TestJiraSecuritySchemeResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":            tftypes.NewValue(tftypes.String, "Updated"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":            tftypes.NewValue(tftypes.String, "Old"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error on update")
	}
}

// TestJiraSecuritySchemeResourceUpdateForbidden tests 403 on update.
func TestJiraSecuritySchemeResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "Updated"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "Old"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error on update")
	}
}

// TestJiraSecuritySchemeResourceUpdateServerError tests 500 on update.
func TestJiraSecuritySchemeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "Updated"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "Old"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on update")
	}
}

// TestJiraSecuritySchemeResourceDeleteForbidden tests 403 on delete.
func TestJiraSecuritySchemeResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "SS"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error on delete")
	}
}

// TestJiraSecuritySchemeResourceDeleteServerError tests 500 on delete.
func TestJiraSecuritySchemeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, "SS"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on delete")
	}
}

// TestJiraSecuritySchemeResourceDeleteNotFound tests 404 on delete (idempotent).
func TestJiraSecuritySchemeResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":            tftypes.NewValue(tftypes.String, "SS"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete of nonexistent should not error: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraSecuritySchemeResourceConfigureNil verifies nil provider data does not error.
func TestJiraSecuritySchemeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraSecuritySchemeResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraSecuritySchemeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraSecuritySchemeResourceCreatePlanGetError tests create with invalid plan.
func TestJiraSecuritySchemeResourceCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraSecuritySchemeResourceReadStateGetError tests read with invalid state.
func TestJiraSecuritySchemeResourceReadStateGetError(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraSecuritySchemeResourceUpdatePlanGetError tests update with invalid plan.
func TestJiraSecuritySchemeResourceUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.UpdateResponse{State: emptyState(context.Background(), s)}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraSecuritySchemeResourceUpdateStateGetError tests update with invalid state.
func TestJiraSecuritySchemeResourceUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "id"),
		"name":            tftypes.NewValue(tftypes.String, "n"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraSecuritySchemeResourceDeleteStateGetError tests delete with invalid state.
func TestJiraSecuritySchemeResourceDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== SECURITY SCHEME DATA SOURCE TESTS ====================

// TestJiraSecuritySchemeDataSourceMetadata verifies the data source type name.
func TestJiraSecuritySchemeDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := securityschemeds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_security_scheme" {
		t.Errorf("expected data source type name 'atlassian_jira_security_scheme', got %q", resp.TypeName)
	}
}

// TestJiraSecuritySchemeDataSourceSchema verifies the data source schema.
func TestJiraSecuritySchemeDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := securityschemeds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)
	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "description", "security_levels"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestJiraSecuritySchemeDataSourceSchemaAttributeCount verifies attribute count.
func TestJiraSecuritySchemeDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := securityschemeds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)
	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraSecuritySchemeDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraSecuritySchemeDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()
	ds := securityschemeds.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected security scheme data source to implement datasource.DataSource")
	}
}

// TestJiraSecuritySchemeDataSourceByID tests reading by ID.
func TestJiraSecuritySchemeDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()

	r := securityschemers.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "DS Sec Scheme"),
		"description":     tftypes.NewValue(tftypes.String, "ds desc"),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	ssID := getStringAttr(t, cResp.State, "id")

	ds := securityschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ssID),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Sec Scheme" {
		t.Errorf("expected name 'DS Sec Scheme', got %q", name)
	}
}

// TestJiraSecuritySchemeDataSourceNotFound tests 404 error on data source read.
func TestJiraSecuritySchemeDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	ds := securityschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent security scheme")
	}
}

// TestJiraSecuritySchemeDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraSecuritySchemeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := securityschemeds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraSecuritySchemeDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraSecuritySchemeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := securityschemeds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraSecuritySchemeDataSourceReadServerError tests generic error on data source read.
func TestJiraSecuritySchemeDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	ds := securityschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "some-id"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"security_levels": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestJiraSecuritySchemeDataSourceReadBadConfig tests data source Read with invalid config.
func TestJiraSecuritySchemeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	ds := securityschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// ==================== SECURITY SCHEME SECURITY LEVELS TESTS ====================

// securityLevelListType is the tftypes type for the security_levels attribute.
var securityLevelListType = tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}}

// TestJiraSecuritySchemeResourceCreateWithLevels tests creating a scheme with non-empty security levels.
func TestJiraSecuritySchemeResourceCreateWithLevels(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Scheme With Levels"),
		"description": tftypes.NewValue(tftypes.String, "Has levels"),
		"security_levels": tftypes.NewValue(securityLevelListType, []tftypes.Value{
			tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}, map[string]tftypes.Value{
				"name":        tftypes.NewValue(tftypes.String, "Level One"),
				"description": tftypes.NewValue(tftypes.String, "First level"),
			}),
			tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{"name": tftypes.String, "description": tftypes.String}}, map[string]tftypes.Value{
				"name":        tftypes.NewValue(tftypes.String, "Level Two"),
				"description": tftypes.NewValue(tftypes.String, ""),
			}),
		}),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create with levels: %v", createResp.Diagnostics.Errors())
	}
	ssID := getStringAttr(t, createResp.State, "id")
	if ssID == "" {
		t.Fatal("expected non-empty ID after create")
	}

	// Read back and verify levels are present
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ssID),
		"name":            tftypes.NewValue(tftypes.String, "Scheme With Levels"),
		"description":     tftypes.NewValue(tftypes.String, "Has levels"),
		"security_levels": tftypes.NewValue(securityLevelListType, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read with levels: %v", readResp.Diagnostics.Errors())
	}
}

// TestJiraSecuritySchemeDataSourceReadWithLevels tests reading a scheme with non-empty security levels.
func TestJiraSecuritySchemeDataSourceReadWithLevels(t *testing.T) {
	t.Parallel()

	// Custom mock server that returns security_levels in the response.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/issuesecurityschemes/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          r.PathValue("id"),
			"name":        "Sec Scheme With Levels",
			"description": "Has security levels",
			"self":        "https://example.atlassian.net/rest/api/3/issuesecurityschemes/sl-1",
			"security_levels": []map[string]interface{}{
				{"name": "Confidential", "description": "Restricted access"},
				{"name": "Internal", "description": ""},
			},
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
	ds := securityschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "sl-1"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"security_levels": tftypes.NewValue(securityLevelListType, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read with levels: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "Sec Scheme With Levels" {
		t.Errorf("expected name 'Sec Scheme With Levels', got %q", name)
	}
}

// ==================== NOTIFICATION SCHEME RESOURCE SCHEMA TESTS ====================

// TestJiraNotificationSchemeResourceMetadata verifies the resource type name.
func TestJiraNotificationSchemeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_notification_scheme" {
		t.Errorf("expected resource type name 'atlassian_jira_notification_scheme', got %q", resp.TypeName)
	}
}

// TestJiraNotificationSchemeResourceSchema verifies the resource schema has all expected attributes.
func TestJiraNotificationSchemeResourceSchema(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
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

// TestJiraNotificationSchemeResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestJiraNotificationSchemeResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraNotificationSchemeResourceSchemaRequiredAttributes verifies required attributes.
func TestJiraNotificationSchemeResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
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

// TestJiraNotificationSchemeResourceSchemaComputedAttributes verifies computed attributes.
func TestJiraNotificationSchemeResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
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

// TestJiraNotificationSchemeResourceSchemaOptionalAttributes verifies optional attributes.
func TestJiraNotificationSchemeResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
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

// TestJiraNotificationSchemeResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestJiraNotificationSchemeResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)
	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraNotificationSchemeResourceImplementsResource verifies the Resource interface.
func TestJiraNotificationSchemeResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected notification scheme resource to implement resource.Resource")
	}
}

// TestJiraNotificationSchemeResourceImplementsImportState verifies the ImportState interface.
func TestJiraNotificationSchemeResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected notification scheme resource to implement ResourceWithImportState")
	}
}

// ==================== NOTIFICATION SCHEME RESOURCE CRUD LIFECYCLE TESTS ====================

// TestJiraNotificationSchemeResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestJiraNotificationSchemeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Test Notification Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A test notification scheme"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	nsID := getStringAttr(t, createResp.State, "id")
	if nsID == "" {
		t.Fatal("expected non-empty ID after create")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Notification Scheme" {
		t.Errorf("expected name 'Test Notification Scheme', got %q", name)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nsID),
		"name":        tftypes.NewValue(tftypes.String, "Test Notification Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A test notification scheme"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test Notification Scheme" {
		t.Errorf("expected name 'Test Notification Scheme', got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nsID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Notification Scheme"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
	})}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nsID),
		"name":        tftypes.NewValue(tftypes.String, "Test Notification Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A test notification scheme"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Notification Scheme" {
		t.Errorf("expected name 'Updated Notification Scheme', got %q", name)
	}

	// Delete
	delState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nsID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Notification Scheme"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
	})}
	delResp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: delState}, delResp)
	if delResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", delResp.Diagnostics.Errors())
	}

	// Read after delete should remove resource
	readState2 := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nsID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Notification Scheme"),
		"description": tftypes.NewValue(tftypes.String, "Updated desc"),
	})}
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState2.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState2}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("Read after delete should not error: %v", readResp2.Diagnostics.Errors())
	}
}

// TestJiraNotificationSchemeResourceCreateConflict tests duplicate name 409 error.
func TestJiraNotificationSchemeResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Dup NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("First create: %v", cResp.Diagnostics.Errors())
	}

	cResp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp2)
	if !cResp2.Diagnostics.HasError() {
		t.Fatal("Expected conflict error on duplicate name")
	}
}

// TestJiraNotificationSchemeResourceCreateForbidden tests 403 error on create.
func TestJiraNotificationSchemeResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Forbidden NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if !cResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

// TestJiraNotificationSchemeResourceCreateServerError tests 500 error on create.
func TestJiraNotificationSchemeResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Error NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if !cResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestJiraNotificationSchemeResourceReadServerError tests 500 on read.
func TestJiraNotificationSchemeResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestJiraNotificationSchemeResourceUpdateNotFound tests 404 on update.
func TestJiraNotificationSchemeResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "Updated"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "Old"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error on update")
	}
}

// TestJiraNotificationSchemeResourceUpdateForbidden tests 403 on update.
func TestJiraNotificationSchemeResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Updated"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Old"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error on update")
	}
}

// TestJiraNotificationSchemeResourceUpdateServerError tests 500 on update.
func TestJiraNotificationSchemeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Updated"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "Old"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on update")
	}
}

// TestJiraNotificationSchemeResourceDeleteForbidden tests 403 on delete.
func TestJiraNotificationSchemeResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testSchemeForbiddenMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error on delete")
	}
}

// TestJiraNotificationSchemeResourceDeleteServerError tests 500 on delete.
func TestJiraNotificationSchemeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "some-id"),
		"name":        tftypes.NewValue(tftypes.String, "NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error on delete")
	}
}

// TestJiraNotificationSchemeResourceDeleteNotFound tests 404 on delete (idempotent).
func TestJiraNotificationSchemeResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":        tftypes.NewValue(tftypes.String, "NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete of nonexistent should not error: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraNotificationSchemeResourceConfigureNil verifies nil provider data does not error.
func TestJiraNotificationSchemeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraNotificationSchemeResourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraNotificationSchemeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraNotificationSchemeResourceCreatePlanGetError tests create with invalid plan.
func TestJiraNotificationSchemeResourceCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraNotificationSchemeResourceReadStateGetError tests read with invalid state.
func TestJiraNotificationSchemeResourceReadStateGetError(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraNotificationSchemeResourceUpdatePlanGetError tests update with invalid plan.
func TestJiraNotificationSchemeResourceUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.UpdateResponse{State: emptyState(context.Background(), s)}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraNotificationSchemeResourceUpdateStateGetError tests update with invalid state.
func TestJiraNotificationSchemeResourceUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "id"),
		"name":        tftypes.NewValue(tftypes.String, "n"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestJiraNotificationSchemeResourceDeleteStateGetError tests delete with invalid state.
func TestJiraNotificationSchemeResourceDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== NOTIFICATION SCHEME DATA SOURCE TESTS ====================

// TestJiraNotificationSchemeDataSourceMetadata verifies the data source type name.
func TestJiraNotificationSchemeDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := notificationschemeds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_notification_scheme" {
		t.Errorf("expected data source type name 'atlassian_jira_notification_scheme', got %q", resp.TypeName)
	}
}

// TestJiraNotificationSchemeDataSourceSchema verifies the data source schema.
func TestJiraNotificationSchemeDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := notificationschemeds.NewDataSource()
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

// TestJiraNotificationSchemeDataSourceSchemaAttributeCount verifies attribute count.
func TestJiraNotificationSchemeDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := notificationschemeds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)
	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestJiraNotificationSchemeDataSourceImplementsDataSource verifies the DataSource interface.
func TestJiraNotificationSchemeDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()
	ds := notificationschemeds.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected notification scheme data source to implement datasource.DataSource")
	}
}

// TestJiraNotificationSchemeDataSourceByID tests reading by ID.
func TestJiraNotificationSchemeDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()

	r := notificationschemers.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS Notif Scheme"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	nsID := getStringAttr(t, cResp.State, "id")

	ds := notificationschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, nsID),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Notif Scheme" {
		t.Errorf("expected name 'DS Notif Scheme', got %q", name)
	}
}

// TestJiraNotificationSchemeDataSourceNotFound tests 404 error on data source read.
func TestJiraNotificationSchemeDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	ds := notificationschemeds.NewDataSource()
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
		t.Fatal("Expected error reading nonexistent notification scheme")
	}
}

// TestJiraNotificationSchemeDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraNotificationSchemeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := notificationschemeds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraNotificationSchemeDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestJiraNotificationSchemeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := notificationschemeds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraNotificationSchemeDataSourceReadServerError tests generic error on data source read.
func TestJiraNotificationSchemeDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testSchemeServerErrorMockServer(t)
	ctx := context.Background()
	ds := notificationschemeds.NewDataSource()
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

// TestJiraNotificationSchemeDataSourceReadBadConfig tests data source Read with invalid config.
func TestJiraNotificationSchemeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	ds := notificationschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}

// TestJiraPermissionSchemeWithGrants tests creating a scheme with grants.
func TestJiraPermissionSchemeWithGrants(t *testing.T) {
	t.Parallel()
	_, client := testSchemeMockServer(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	grantObjType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String,
	}}
	grantsListType := tftypes.List{ElementType: grantObjType}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Grants Scheme"),
		"description": tftypes.NewValue(tftypes.String, "With grants"),
		"grants": tftypes.NewValue(grantsListType, []tftypes.Value{
			tftypes.NewValue(grantObjType, map[string]tftypes.Value{
				"permission":  tftypes.NewValue(tftypes.String, "BROWSE_PROJECTS"),
				"holder_type": tftypes.NewValue(tftypes.String, "group"),
				"holder_id":   tftypes.NewValue(tftypes.String, "developers"),
			}),
		}),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create with grants: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
}

// TestJiraPermissionSchemeGrantsToStateNonEmpty exercises the non-empty
// path of grantsToState via a Read that returns permissions.
func TestJiraPermissionSchemeGrantsToStateNonEmpty(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/permissionscheme/ps-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "ps-1", "name": "Test", "description": "desc",
			"permissions": []map[string]interface{}{
				{"permission": "BROWSE_PROJECTS", "holder": map[string]string{"type": "group", "parameter": "devs"}},
				{"permission": "EDIT_ISSUES", "holder": map[string]string{"type": "user", "parameter": "uid-1"}},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()

	// Test resource Read
	r := permissionschemers.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	grantObjType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"permission": tftypes.String, "holder_type": tftypes.String, "holder_id": tftypes.String,
	}}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ps-1"),
		"name":        tftypes.NewValue(tftypes.String, "Test"),
		"description": tftypes.NewValue(tftypes.String, "desc"),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: grantObjType}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Test data source Read
	ds := permissionschemeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ps-1"),
		"name":        tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil),
		"grants":      tftypes.NewValue(tftypes.List{ElementType: grantObjType}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
}
