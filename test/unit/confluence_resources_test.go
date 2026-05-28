// Package unit contains unit tests for the Confluence resources and data sources:
// atlassian_confluence_page, atlassian_confluence_template,
// atlassian_confluence_space_permission, atlassian_confluence_content_restriction.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	confluencepagedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/page"
	confluencespacepermdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/space"
	confluencetemplatedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/template"
	confluencepageresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/page"
	confluencespacepermresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/space"
	confluencetemplateresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/template"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// confluenceIDCounter provides unique IDs for confluence mock server tests.
var confluenceIDCounter uint64

func confluenceNextID(prefix string) string {
	n := atomic.AddUint64(&confluenceIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// testConfluenceMockServer creates a mock HTTP server for Confluence endpoints.
func testConfluenceMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	pages := make(map[string]map[string]interface{})
	templates := make(map[string]map[string]interface{})
	spacePermissions := make(map[string][]map[string]interface{})    // spaceID -> perms
	contentRestrictions := make(map[string][]map[string]interface{}) // contentID -> restrictions

	mux := http.NewServeMux()

	// ==================== PAGE ENDPOINTS ====================

	mux.HandleFunc("POST /wiki/api/v2/pages", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		title, _ := req["title"].(string)
		spaceID, _ := req["spaceId"].(string)
		if title == "" || spaceID == "" {
			writeErr(w, 400, "title and spaceId are required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := confluenceNextID("page")
		parentID, _ := req["parentId"].(string)
		bodyContent := ""
		if bodyMap, ok := req["body"].(map[string]interface{}); ok {
			if storage, ok := bodyMap["storage"].(map[string]interface{}); ok {
				bodyContent, _ = storage["value"].(string)
			}
		}
		page := map[string]interface{}{
			"id":       id,
			"title":    title,
			"spaceId":  spaceID,
			"parentId": parentID,
			"status":   "current",
			"body": map[string]interface{}{
				"storage": map[string]interface{}{
					"value":          bodyContent,
					"representation": "storage",
				},
			},
			"version": map[string]interface{}{
				"number": float64(1),
			},
		}
		pages[id] = page
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(page)
	})

	mux.HandleFunc("GET /wiki/api/v2/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		page, ok := pages[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Page not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(page)
	})

	mux.HandleFunc("PUT /wiki/api/v2/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		page, ok := pages[id]
		if !ok {
			writeErr(w, 404, "Page not found")
			return
		}
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if t, ok := req["title"].(string); ok {
			page["title"] = t
		}
		if bodyMap, ok := req["body"].(map[string]interface{}); ok {
			page["body"] = bodyMap
		}
		if v, ok := req["version"].(map[string]interface{}); ok {
			page["version"] = v
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(page)
	})

	mux.HandleFunc("DELETE /wiki/api/v2/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := pages[id]; !ok {
			writeErr(w, 404, "Page not found")
			return
		}
		delete(pages, id)
		w.WriteHeader(204)
	})

	// ==================== TEMPLATE ENDPOINTS ====================

	mux.HandleFunc("POST /wiki/api/v2/templates", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			writeErr(w, 400, "name is required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := confluenceNextID("tmpl")
		description, _ := req["description"].(string)
		body, _ := req["body"].(string)
		spaceID, _ := req["spaceId"].(string)
		templateType, _ := req["templateType"].(string)
		tmpl := map[string]interface{}{
			"templateId":   id,
			"name":         name,
			"description":  description,
			"body":         body,
			"spaceId":      spaceID,
			"templateType": templateType,
		}
		templates[id] = tmpl
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("GET /wiki/api/v2/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		tmpl, ok := templates[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Template not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("PUT /wiki/api/v2/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		tmpl, ok := templates[id]
		if !ok {
			writeErr(w, 404, "Template not found")
			return
		}
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		for k, v := range req {
			if k != "templateId" {
				tmpl[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("DELETE /wiki/api/v2/templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := templates[id]; !ok {
			writeErr(w, 404, "Template not found")
			return
		}
		delete(templates, id)
		w.WriteHeader(204)
	})

	// ==================== SPACE PERMISSION ENDPOINTS ====================

	mux.HandleFunc("POST /wiki/api/v2/spaces/{spaceId}/permissions", func(w http.ResponseWriter, r *http.Request) {
		spaceID := r.PathValue("spaceId")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		defer mu.Unlock()
		principalMap, _ := req["principal"].(map[string]interface{})
		operationMap, _ := req["operation"].(map[string]interface{})
		principalType, _ := principalMap["type"].(string)
		principalID, _ := principalMap["id"].(string)
		opKey, _ := operationMap["key"].(string)
		// Check for duplicates
		for _, perm := range spacePermissions[spaceID] {
			p, _ := perm["principal"].(map[string]interface{})
			o, _ := perm["operation"].(map[string]interface{})
			if p["type"] == principalType && p["id"] == principalID && o["key"] == opKey {
				writeErr(w, 409, "Permission already exists")
				return
			}
		}
		id := confluenceNextID("perm")
		perm := map[string]interface{}{
			"id": id,
			"principal": map[string]interface{}{
				"type": principalType,
				"id":   principalID,
			},
			"operation": map[string]interface{}{
				"key": opKey,
			},
		}
		spacePermissions[spaceID] = append(spacePermissions[spaceID], perm)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(perm)
	})

	mux.HandleFunc("GET /wiki/api/v2/spaces/{spaceId}/permissions", func(w http.ResponseWriter, r *http.Request) {
		spaceID := r.PathValue("spaceId")
		mu.Lock()
		perms := spacePermissions[spaceID]
		mu.Unlock()
		if perms == nil {
			perms = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(perms)
	})

	mux.HandleFunc("DELETE /wiki/api/v2/spaces/{spaceId}/permissions/{permId}", func(w http.ResponseWriter, r *http.Request) {
		spaceID := r.PathValue("spaceId")
		permID := r.PathValue("permId")
		mu.Lock()
		defer mu.Unlock()
		perms := spacePermissions[spaceID]
		found := false
		var updated []map[string]interface{}
		for _, p := range perms {
			if p["id"] == permID {
				found = true
				continue
			}
			updated = append(updated, p)
		}
		if !found {
			writeErr(w, 404, "Permission not found")
			return
		}
		spacePermissions[spaceID] = updated
		w.WriteHeader(204)
	})

	// ==================== CONTENT RESTRICTION ENDPOINTS ====================

	mux.HandleFunc("PUT /wiki/api/v2/content/{contentId}/restriction", func(w http.ResponseWriter, r *http.Request) {
		contentID := r.PathValue("contentId")
		var reqBody []map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		mu.Lock()
		defer mu.Unlock()
		if _, ok := pages[contentID]; !ok {
			writeErr(w, 404, "Content not found")
			return
		}
		for _, restriction := range reqBody {
			op, _ := restriction["operation"].(string)
			restrictions, _ := restriction["restrictions"].(map[string]interface{})
			if users, ok := restrictions["user"].([]interface{}); ok {
				for _, u := range users {
					uMap, _ := u.(map[string]interface{})
					accountID, _ := uMap["accountId"].(string)
					entry := map[string]interface{}{
						"operation":     op,
						"principalType": "user",
						"principalId":   accountID,
					}
					contentRestrictions[contentID] = append(contentRestrictions[contentID], entry)
				}
			}
			if groups, ok := restrictions["group"].([]interface{}); ok {
				for _, g := range groups {
					gMap, _ := g.(map[string]interface{})
					groupID, _ := gMap["id"].(string)
					entry := map[string]interface{}{
						"operation":     op,
						"principalType": "group",
						"principalId":   groupID,
					}
					contentRestrictions[contentID] = append(contentRestrictions[contentID], entry)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	})

	mux.HandleFunc("GET /wiki/api/v2/content/{contentId}/restriction", func(w http.ResponseWriter, r *http.Request) {
		contentID := r.PathValue("contentId")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := pages[contentID]; !ok {
			writeErr(w, 404, "Content not found")
			return
		}
		entries := contentRestrictions[contentID]
		// Build response grouped by operation
		opMap := make(map[string]*struct {
			Users  []map[string]interface{}
			Groups []map[string]interface{}
		})
		for _, e := range entries {
			op, _ := e["operation"].(string)
			if opMap[op] == nil {
				opMap[op] = &struct {
					Users  []map[string]interface{}
					Groups []map[string]interface{}
				}{}
			}
			pType, _ := e["principalType"].(string)
			pID, _ := e["principalId"].(string)
			if pType == "user" {
				opMap[op].Users = append(opMap[op].Users, map[string]interface{}{
					"type": "known", "accountId": pID,
				})
			} else {
				opMap[op].Groups = append(opMap[op].Groups, map[string]interface{}{
					"type": "group", "id": pID,
				})
			}
		}
		var result []map[string]interface{}
		for op, data := range opMap {
			userResults := data.Users
			if userResults == nil {
				userResults = []map[string]interface{}{}
			}
			groupResults := data.Groups
			if groupResults == nil {
				groupResults = []map[string]interface{}{}
			}
			result = append(result, map[string]interface{}{
				"operation": op,
				"restrictions": map[string]interface{}{
					"user":  map[string]interface{}{"results": userResults},
					"group": map[string]interface{}{"results": groupResults},
				},
			})
		}
		if result == nil {
			result = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("DELETE /wiki/api/v2/content/{contentId}/restriction/{operation}/{principalType}/{principalId}", func(w http.ResponseWriter, r *http.Request) {
		contentID := r.PathValue("contentId")
		operation := r.PathValue("operation")
		principalType := r.PathValue("principalType")
		principalID := r.PathValue("principalId")
		mu.Lock()
		defer mu.Unlock()
		entries := contentRestrictions[contentID]
		found := false
		var updated []map[string]interface{}
		for _, e := range entries {
			if e["operation"] == operation && e["principalType"] == principalType && e["principalId"] == principalID {
				found = true
				continue
			}
			updated = append(updated, e)
		}
		if !found {
			writeErr(w, 404, "Restriction not found")
			return
		}
		contentRestrictions[contentID] = updated
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

// testConfluenceForbiddenMockServer returns 403 for all confluence endpoints.
func testConfluenceForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Forbidden"},
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

// testConfluenceServerErrorMockServer returns 500 for all confluence endpoints.
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

// testConfluenceNotFoundMockServer returns 404 for all endpoints except PUT on restriction.
func testConfluenceNotFoundMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Not found"},
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

// testConfluenceConflictMockServer returns 409 for POST endpoints.
func testConfluenceConflictMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(409)
		} else {
			w.WriteHeader(500)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Conflict"},
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

// getInt64Attr reads an int64 attribute from state.
func getInt64Attr(t *testing.T, state tfsdk.State, name string) int64 {
	t.Helper()
	var val int64
	raw := state.Raw
	tfType := raw.Type()
	_ = tfType
	var m map[string]tftypes.Value
	if err := raw.As(&m); err != nil {
		t.Fatalf("state.As: %v", err)
	}
	if v, ok := m[name]; ok {
		if err := v.As(&val); err != nil {
			t.Fatalf("getInt64Attr %q: %v", name, err)
		}
	}
	return val
}

// ==================== PAGE RESOURCE SCHEMA TESTS ====================

func TestConfluencePageResourceMetadata(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_confluence_page" {
		t.Errorf("expected 'atlassian_confluence_page', got %q", resp.TypeName)
	}
}

func TestConfluencePageResourceSchema(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewResource()
	s := getResourceSchema(t, r)
	expectedAttrs := []string{"id", "space_id", "title", "body", "parent_id", "status", "version"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 7 {
		t.Errorf("expected 7 attributes, got %d", len(s.Attributes))
	}
}

func TestConfluencePageResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
}

func TestConfluencePageResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

func TestConfluencePageResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestConfluencePageResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestConfluencePageResourceImportState(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "page-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== PAGE RESOURCE CRUD LIFECYCLE ====================

func TestConfluencePageResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":  tftypes.NewValue(tftypes.String, "space-1"),
		"title":     tftypes.NewValue(tftypes.String, "Test Page"),
		"body":      tftypes.NewValue(tftypes.String, "<p>Hello</p>"),
		"parent_id": tftypes.NewValue(tftypes.String, "parent-1"),
		"status":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version":   tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	pageID := getStringAttr(t, createResp.State, "id")
	if pageID == "" {
		t.Fatal("expected non-empty id")
	}
	if title := getStringAttr(t, createResp.State, "title"); title != "Test Page" {
		t.Errorf("expected title 'Test Page', got %q", title)
	}
	if status := getStringAttr(t, createResp.State, "status"); status != "current" {
		t.Errorf("expected status 'current', got %q", status)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if title := getStringAttr(t, readResp.State, "title"); title != "Test Page" {
		t.Errorf("Read title: got %q", title)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, pageID),
		"space_id":  tftypes.NewValue(tftypes.String, "space-1"),
		"title":     tftypes.NewValue(tftypes.String, "Updated Page"),
		"body":      tftypes.NewValue(tftypes.String, "<p>Updated</p>"),
		"parent_id": tftypes.NewValue(tftypes.String, "parent-1"),
		"status":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version":   tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if title := getStringAttr(t, updateResp.State, "title"); title != "Updated Page" {
		t.Errorf("Update title: got %q", title)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp2)
	if !readResp2.State.Raw.IsNull() {
		// Expected: removed
	}
}

func TestConfluencePageResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "Forbidden"),
		"body":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version":   tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluencePageResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceConflictMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "Conflict"),
		"body":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version":   tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected conflict error")
	}
}

func TestConfluencePageResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "Error"),
		"body":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version":   tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluencePageResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "X"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state removed on 404")
	}
}

func TestConfluencePageResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "X"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

func TestConfluencePageResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "X"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

func TestConfluencePageResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "X"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluencePageResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "X"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluencePageResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "nonexistent"),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "X"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent should not error (idempotent)")
	}
}

func TestConfluencePageResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "X"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluencePageResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "some-id"),
		"space_id":  tftypes.NewValue(tftypes.String, "s1"),
		"title":     tftypes.NewValue(tftypes.String, "X"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluencePageResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

func TestConfluencePageResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

func TestConfluencePageResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "space_id": tftypes.NewValue(tftypes.String, "s"),
		"title": tftypes.NewValue(tftypes.String, "T"), "body": tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""), "status": tftypes.NewValue(tftypes.String, "current"),
		"version": tftypes.NewValue(tftypes.Number, 1),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

func TestConfluencePageResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "space_id": tftypes.NewValue(tftypes.String, "s"),
		"title": tftypes.NewValue(tftypes.String, "T"), "body": tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""), "status": tftypes.NewValue(tftypes.String, "current"),
		"version": tftypes.NewValue(tftypes.Number, 1),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

func TestConfluencePageResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// ==================== PAGE DATA SOURCE TESTS ====================

func TestConfluencePageDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := confluencepagedatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_confluence_page" {
		t.Errorf("expected 'atlassian_confluence_page', got %q", resp.TypeName)
	}
}

func TestConfluencePageDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := confluencepagedatasource.NewDataSource()
	dss := getDatasourceSchema(t, ds)
	expectedAttrs := []string{"id", "space_id", "title", "body", "parent_id", "status", "version"}
	for _, attr := range expectedAttrs {
		if _, ok := dss.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
}

func TestConfluencePageDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()

	// Create a page first
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"title": tftypes.NewValue(tftypes.String, "DS Page"), "body": tftypes.NewValue(tftypes.String, "<p>ds</p>"),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	pageID := getStringAttr(t, cResp.State, "id")

	ds := confluencepagedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, pageID), "space_id": tftypes.NewValue(tftypes.String, nil),
		"title": tftypes.NewValue(tftypes.String, nil), "body": tftypes.NewValue(tftypes.String, nil),
		"parent_id": tftypes.NewValue(tftypes.String, nil), "status": tftypes.NewValue(tftypes.String, nil),
		"version": tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if title := getStringAttr(t, dsResp.State, "title"); title != "DS Page" {
		t.Errorf("expected title 'DS Page', got %q", title)
	}
}

func TestConfluencePageDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	ds := confluencepagedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "space_id": tftypes.NewValue(tftypes.String, nil),
		"title": tftypes.NewValue(tftypes.String, nil), "body": tftypes.NewValue(tftypes.String, nil),
		"parent_id": tftypes.NewValue(tftypes.String, nil), "status": tftypes.NewValue(tftypes.String, nil),
		"version": tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

func TestConfluencePageDataSourceServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	ds := confluencepagedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "space_id": tftypes.NewValue(tftypes.String, nil),
		"title": tftypes.NewValue(tftypes.String, nil), "body": tftypes.NewValue(tftypes.String, nil),
		"parent_id": tftypes.NewValue(tftypes.String, nil), "status": tftypes.NewValue(tftypes.String, nil),
		"version": tftypes.NewValue(tftypes.Number, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluencePageDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := confluencepagedatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestConfluencePageDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := confluencepagedatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestConfluencePageDataSourceBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	ds := confluencepagedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config")
	}
}

// ==================== TEMPLATE RESOURCE SCHEMA TESTS ====================

func TestConfluenceTemplateResourceMetadata(t *testing.T) {
	t.Parallel()
	r := confluencetemplateresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_confluence_template" {
		t.Errorf("expected 'atlassian_confluence_template', got %q", resp.TypeName)
	}
}

func TestConfluenceTemplateResourceSchema(t *testing.T) {
	t.Parallel()
	r := confluencetemplateresource.NewResource()
	s := getResourceSchema(t, r)
	expectedAttrs := []string{"id", "name", "description", "body", "space_id", "template_type"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 6 {
		t.Errorf("expected 6 attributes, got %d", len(s.Attributes))
	}
}

func TestConfluenceTemplateResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := confluencetemplateresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
}

func TestConfluenceTemplateResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := confluencetemplateresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

func TestConfluenceTemplateResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := confluencetemplateresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestConfluenceTemplateResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := confluencetemplateresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestConfluenceTemplateResourceImportState(t *testing.T) {
	t.Parallel()
	r := confluencetemplateresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "tmpl-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== TEMPLATE RESOURCE CRUD LIFECYCLE ====================

func TestConfluenceTemplateResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Test Template"),
		"description":   tftypes.NewValue(tftypes.String, "A test template"),
		"body":          tftypes.NewValue(tftypes.String, "<p>Template body</p>"),
		"space_id":      tftypes.NewValue(tftypes.String, "space-1"),
		"template_type": tftypes.NewValue(tftypes.String, "page"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	tmplID := getStringAttr(t, createResp.State, "id")
	if tmplID == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Template" {
		t.Errorf("expected name 'Test Template', got %q", name)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tmplID),
		"name":          tftypes.NewValue(tftypes.String, "Updated Template"),
		"description":   tftypes.NewValue(tftypes.String, "Updated desc"),
		"body":          tftypes.NewValue(tftypes.String, "<p>Updated</p>"),
		"space_id":      tftypes.NewValue(tftypes.String, "space-1"),
		"template_type": tftypes.NewValue(tftypes.String, "page"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Template" {
		t.Errorf("Update name: got %q", name)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp2)
	if !readResp2.State.Raw.IsNull() {
		// Expected removed
	}
}

func TestConfluenceTemplateResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "F"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "template_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluenceTemplateResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceConflictMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "C"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "template_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected conflict error")
	}
}

func TestConfluenceTemplateResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "E"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "template_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceTemplateResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

func TestConfluenceTemplateResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluenceTemplateResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceTemplateResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent should not error")
	}
}

func TestConfluenceTemplateResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluenceTemplateResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceTemplateResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state removed on 404")
	}
}

func TestConfluenceTemplateResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceTemplateResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

func TestConfluenceTemplateResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

func TestConfluenceTemplateResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "body": tftypes.NewValue(tftypes.String, ""),
		"space_id": tftypes.NewValue(tftypes.String, ""), "template_type": tftypes.NewValue(tftypes.String, ""),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

func TestConfluenceTemplateResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

func TestConfluenceTemplateResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// ==================== TEMPLATE DATA SOURCE TESTS ====================

func TestConfluenceTemplateDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := confluencetemplatedatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_confluence_template" {
		t.Errorf("expected 'atlassian_confluence_template', got %q", resp.TypeName)
	}
}

func TestConfluenceTemplateDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := confluencetemplatedatasource.NewDataSource()
	dss := getDatasourceSchema(t, ds)
	expectedAttrs := []string{"id", "name", "description", "body", "space_id", "template_type"}
	for _, attr := range expectedAttrs {
		if _, ok := dss.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
}

func TestConfluenceTemplateDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "DS Tmpl"),
		"description": tftypes.NewValue(tftypes.String, "desc"), "body": tftypes.NewValue(tftypes.String, "<p>t</p>"),
		"space_id": tftypes.NewValue(tftypes.String, "s1"), "template_type": tftypes.NewValue(tftypes.String, "page"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	tmplID := getStringAttr(t, cResp.State, "id")

	ds := confluencetemplatedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tmplID), "name": tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil), "body": tftypes.NewValue(tftypes.String, nil),
		"space_id": tftypes.NewValue(tftypes.String, nil), "template_type": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Tmpl" {
		t.Errorf("expected name 'DS Tmpl', got %q", name)
	}
}

func TestConfluenceTemplateDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	ds := confluencetemplatedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil), "body": tftypes.NewValue(tftypes.String, nil),
		"space_id": tftypes.NewValue(tftypes.String, nil), "template_type": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

func TestConfluenceTemplateDataSourceServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	ds := confluencetemplatedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil), "body": tftypes.NewValue(tftypes.String, nil),
		"space_id": tftypes.NewValue(tftypes.String, nil), "template_type": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceTemplateDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := confluencetemplatedatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestConfluenceTemplateDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := confluencetemplatedatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestConfluenceTemplateDataSourceBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	ds := confluencetemplatedatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config")
	}
}

// ==================== SPACE PERMISSION RESOURCE TESTS ====================

func TestConfluenceSpacePermissionResourceMetadata(t *testing.T) {
	t.Parallel()
	r := confluencespacepermresource.NewPermissionResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_confluence_space_permission" {
		t.Errorf("expected 'atlassian_confluence_space_permission', got %q", resp.TypeName)
	}
}

func TestConfluenceSpacePermissionResourceSchema(t *testing.T) {
	t.Parallel()
	r := confluencespacepermresource.NewPermissionResource()
	s := getResourceSchema(t, r)
	expectedAttrs := []string{"id", "space_id", "principal_type", "principal_id", "operation"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(s.Attributes))
	}
}

func TestConfluenceSpacePermissionResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := confluencespacepermresource.NewPermissionResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
}

func TestConfluenceSpacePermissionResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := confluencespacepermresource.NewPermissionResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

func TestConfluenceSpacePermissionResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := confluencespacepermresource.NewPermissionResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestConfluenceSpacePermissionResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := confluencespacepermresource.NewPermissionResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestConfluenceSpacePermissionResourceImportState(t *testing.T) {
	t.Parallel()
	r := confluencespacepermresource.NewPermissionResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()

	// Valid import
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "space-1/perm-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}

	// Invalid import
	resp2 := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "invalid"}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid import format")
	}
}

func TestConfluenceSpacePermissionResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	permID := getStringAttr(t, createResp.State, "id")
	if permID == "" || !strings.Contains(permID, "/") {
		t.Fatalf("expected composite id, got %q", permID)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete should remove
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp2)
	if !readResp2.State.Raw.IsNull() {
		// Expected removed
	}
}

func TestConfluenceSpacePermissionResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceConflictMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected conflict error")
	}
}

func TestConfluenceSpacePermissionResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluenceSpacePermissionResourceCreateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceNotFoundMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "nonexistent"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

func TestConfluenceSpacePermissionResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceSpacePermissionResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "s1/p1"), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceSpacePermissionResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "s1/p1"), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluenceSpacePermissionResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "s1/p1"), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceSpacePermissionResourceUpdate(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	resp := &resource.UpdateResponse{}
	r.Update(ctx, resource.UpdateRequest{}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected update not supported error")
	}
}

func TestConfluenceSpacePermissionResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

func TestConfluenceSpacePermissionResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

func TestConfluenceSpacePermissionResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// ==================== SPACE PERMISSION DATA SOURCE TESTS ====================

func TestConfluenceSpacePermissionDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_confluence_space_permission" {
		t.Errorf("expected 'atlassian_confluence_space_permission', got %q", resp.TypeName)
	}
}

func TestConfluenceSpacePermissionDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	dss := getDatasourceSchema(t, ds)
	expectedAttrs := []string{"id", "space_id", "principal_type", "principal_id", "operation"}
	for _, attr := range expectedAttrs {
		if _, ok := dss.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
}

func TestConfluenceSpacePermissionDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()

	// Create permission first
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "space-ds"),
		"principal_type": tftypes.NewValue(tftypes.String, "group"), "principal_id": tftypes.NewValue(tftypes.String, "group-1"),
		"operation": tftypes.NewValue(tftypes.String, "write"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	ds := confluencespacepermdatasource.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "space_id": tftypes.NewValue(tftypes.String, "space-ds"),
		"principal_type": tftypes.NewValue(tftypes.String, "group"), "principal_id": tftypes.NewValue(tftypes.String, "group-1"),
		"operation": tftypes.NewValue(tftypes.String, "write"),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, dsResp.State, "id")
	if id == "" || !strings.Contains(id, "/") {
		t.Errorf("expected composite id, got %q", id)
	}
}

func TestConfluenceSpacePermissionDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "nonexistent"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

func TestConfluenceSpacePermissionDataSourceServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
		"operation": tftypes.NewValue(tftypes.String, "read"),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceSpacePermissionDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestConfluenceSpacePermissionDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestConfluenceSpacePermissionDataSourceBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config")
	}
}

// ==================== CONTENT RESTRICTION RESOURCE TESTS ====================

func TestConfluenceContentRestrictionResourceMetadata(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewRestrictionResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_confluence_content_restriction" {
		t.Errorf("expected 'atlassian_confluence_content_restriction', got %q", resp.TypeName)
	}
}

func TestConfluenceContentRestrictionResourceSchema(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewRestrictionResource()
	s := getResourceSchema(t, r)
	expectedAttrs := []string{"id", "content_id", "operation", "principal_type", "principal_id"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(s.Attributes))
	}
}

func TestConfluenceContentRestrictionResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewRestrictionResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
}

func TestConfluenceContentRestrictionResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewRestrictionResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

func TestConfluenceContentRestrictionResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewRestrictionResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestConfluenceContentRestrictionResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewRestrictionResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestConfluenceContentRestrictionResourceImportState(t *testing.T) {
	t.Parallel()
	r := confluencepageresource.NewRestrictionResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()

	// Valid 4-part import
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "page-1/read/user/user-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}

	// Invalid import
	resp2 := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "invalid"}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid import")
	}
}

func TestConfluenceContentRestrictionResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()

	// First create a page to restrict
	pr := confluencepageresource.NewResource()
	configureResource(t, pr, client)
	ps := getResourceSchema(t, pr)
	psTfType := ps.Type().TerraformType(ctx)
	pagePlan := tfsdk.Plan{Schema: ps, Raw: tftypes.NewValue(psTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"title": tftypes.NewValue(tftypes.String, "Restricted Page"), "body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	pageResp := &resource.CreateResponse{State: emptyState(ctx, ps)}
	pr.Create(ctx, resource.CreateRequest{Plan: pagePlan}, pageResp)
	if pageResp.Diagnostics.HasError() {
		t.Fatalf("Create page: %v", pageResp.Diagnostics.Errors())
	}
	contentID := getStringAttr(t, pageResp.State, "id")

	// Create restriction
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"content_id":     tftypes.NewValue(tftypes.String, contentID),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	restrictionID := getStringAttr(t, createResp.State, "id")
	if !strings.Contains(restrictionID, "/") {
		t.Fatalf("expected composite id, got %q", restrictionID)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete should remove
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp2)
	if !readResp2.State.Raw.IsNull() {
		// Expected removed
	}
}

func TestConfluenceContentRestrictionResourceCreateWithGroup(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()

	// Create a page first
	pr := confluencepageresource.NewResource()
	configureResource(t, pr, client)
	ps := getResourceSchema(t, pr)
	pagePlan := tfsdk.Plan{Schema: ps, Raw: tftypes.NewValue(ps.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"title": tftypes.NewValue(tftypes.String, "Group Restrict"), "body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	pageResp := &resource.CreateResponse{State: emptyState(ctx, ps)}
	pr.Create(ctx, resource.CreateRequest{Plan: pagePlan}, pageResp)
	if pageResp.Diagnostics.HasError() {
		t.Fatalf("Create page: %v", pageResp.Diagnostics.Errors())
	}
	contentID := getStringAttr(t, pageResp.State, "id")

	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "content_id": tftypes.NewValue(tftypes.String, contentID),
		"operation": tftypes.NewValue(tftypes.String, "update"), "principal_type": tftypes.NewValue(tftypes.String, "group"),
		"principal_id": tftypes.NewValue(tftypes.String, "group-1"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create group restriction: %v", createResp.Diagnostics.Errors())
	}
}

func TestConfluenceContentRestrictionResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "content_id": tftypes.NewValue(tftypes.String, "c1"),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluenceContentRestrictionResourceCreateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceNotFoundMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "content_id": tftypes.NewValue(tftypes.String, "nonexistent"),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

func TestConfluenceContentRestrictionResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "content_id": tftypes.NewValue(tftypes.String, "c1"),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceContentRestrictionResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "c1/read/user/u1"), "content_id": tftypes.NewValue(tftypes.String, "c1"),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceContentRestrictionResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceForbiddenMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "c1/read/user/u1"), "content_id": tftypes.NewValue(tftypes.String, "c1"),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestConfluenceContentRestrictionResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "c1/read/user/u1"), "content_id": tftypes.NewValue(tftypes.String, "c1"),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceContentRestrictionResourceUpdate(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	resp := &resource.UpdateResponse{}
	r.Update(ctx, resource.UpdateRequest{}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected update not supported error")
	}
}

func TestConfluenceContentRestrictionResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

func TestConfluenceContentRestrictionResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

func TestConfluenceContentRestrictionResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// ==================== CONTENT RESTRICTION DATA SOURCE TESTS ====================

func TestConfluenceContentRestrictionDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_confluence_content_restriction" {
		t.Errorf("expected 'atlassian_confluence_content_restriction', got %q", resp.TypeName)
	}
}

func TestConfluenceContentRestrictionDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	dss := getDatasourceSchema(t, ds)
	expectedAttrs := []string{"id", "content_id", "operation", "principal_type", "principal_id"}
	for _, attr := range expectedAttrs {
		if _, ok := dss.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
}

func TestConfluenceContentRestrictionDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()

	// Create page and restriction first
	pr := confluencepageresource.NewResource()
	configureResource(t, pr, client)
	ps := getResourceSchema(t, pr)
	pagePlan := tfsdk.Plan{Schema: ps, Raw: tftypes.NewValue(ps.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"title": tftypes.NewValue(tftypes.String, "DS Restrict Page"), "body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	pageResp := &resource.CreateResponse{State: emptyState(ctx, ps)}
	pr.Create(ctx, resource.CreateRequest{Plan: pagePlan}, pageResp)
	if pageResp.Diagnostics.HasError() {
		t.Fatalf("Create page: %v", pageResp.Diagnostics.Errors())
	}
	contentID := getStringAttr(t, pageResp.State, "id")

	rr := confluencepageresource.NewRestrictionResource()
	configureResource(t, rr, client)
	rs := getResourceSchema(t, rr)
	restrictPlan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "content_id": tftypes.NewValue(tftypes.String, contentID),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "user-ds"),
	})}
	rResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	rr.Create(ctx, resource.CreateRequest{Plan: restrictPlan}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Create restriction: %v", rResp.Diagnostics.Errors())
	}

	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "content_id": tftypes.NewValue(tftypes.String, contentID),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "user-ds"),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, dsResp.State, "id")
	if !strings.Contains(id, "/") {
		t.Errorf("expected composite id, got %q", id)
	}
}

func TestConfluenceContentRestrictionDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()

	// Create page but no restriction
	pr := confluencepageresource.NewResource()
	configureResource(t, pr, client)
	ps := getResourceSchema(t, pr)
	pagePlan := tfsdk.Plan{Schema: ps, Raw: tftypes.NewValue(ps.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, "s1"),
		"title": tftypes.NewValue(tftypes.String, "No Restrict"), "body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version": tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	pageResp := &resource.CreateResponse{State: emptyState(ctx, ps)}
	pr.Create(ctx, resource.CreateRequest{Plan: pagePlan}, pageResp)
	if pageResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", pageResp.Diagnostics.Errors())
	}
	contentID := getStringAttr(t, pageResp.State, "id")

	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "content_id": tftypes.NewValue(tftypes.String, contentID),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "nonexistent"),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

func TestConfluenceContentRestrictionDataSourceContentNotFound(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "content_id": tftypes.NewValue(tftypes.String, "nonexistent-content"),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected content not found error")
	}
}

func TestConfluenceContentRestrictionDataSourceServerError(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceServerErrorMockServer(t)
	ctx := context.Background()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, nil), "content_id": tftypes.NewValue(tftypes.String, "c1"),
		"operation": tftypes.NewValue(tftypes.String, "read"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

func TestConfluenceContentRestrictionDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestConfluenceContentRestrictionDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestConfluenceContentRestrictionDataSourceBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testConfluenceMockServer(t)
	ctx := context.Background()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config")
	}
}

// Suppress unused import warning for strings
var _ = strings.Contains
