// Package unit contains unit tests for the atlassian_jira_dashboard,
// atlassian_jira_filter, atlassian_jira_custom_field, atlassian_jira_board,
// atlassian_jira_priority, and atlassian_jira_priority_scheme resources
// and data sources.
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
	boarddatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/board"
	customfielddatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/custom_field"
	dashboarddatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/dashboard"
	prioritydatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/priority"
	boardresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/board"
	customfieldresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/custom_field"
	dashboardresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/dashboard"
	priorityresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/priority"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// miscIDCounter provides unique IDs for misc mock server tests.
var miscIDCounter uint64

func miscNextID(prefix string) string {
	n := atomic.AddUint64(&miscIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// testMiscMockServer creates a mock HTTP server for dashboard, filter, custom_field, board, priority, and priority scheme endpoints.
func testMiscMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	dashboards := make(map[string]map[string]interface{})
	filters := make(map[string]map[string]interface{})
	fields := make(map[string]map[string]interface{})
	boards := make(map[string]map[string]interface{})
	priorities := make(map[string]map[string]interface{})
	prioritySchemes := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// Dashboard endpoints
	mux.HandleFunc("POST /rest/api/3/dashboard", func(w http.ResponseWriter, r *http.Request) {
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
		for _, d := range dashboards {
			if d["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}})
				return
			}
		}
		id := miscNextID("dash")
		description, _ := req["description"].(string)
		d := map[string]interface{}{"id": id, "name": name, "description": description, "self": fmt.Sprintf("https://example.atlassian.net/rest/api/3/dashboard/%s", id)}
		dashboards[id] = d
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("GET /rest/api/3/dashboard/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		d, ok := dashboards[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("PUT /rest/api/3/dashboard/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		d, ok := dashboards[id]
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
				d[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("DELETE /rest/api/3/dashboard/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := dashboards[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		delete(dashboards, id)
		w.WriteHeader(204)
	})

	// Filter endpoints
	mux.HandleFunc("POST /rest/api/3/filter", func(w http.ResponseWriter, r *http.Request) {
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
		for _, f := range filters {
			if f["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}})
				return
			}
		}
		id := miscNextID("flt")
		description, _ := req["description"].(string)
		jql, _ := req["jql"].(string)
		f := map[string]interface{}{"id": id, "name": name, "description": description, "jql": jql, "self": fmt.Sprintf("https://example.atlassian.net/rest/api/3/filter/%s", id)}
		filters[id] = f
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(f)
	})
	mux.HandleFunc("GET /rest/api/3/filter/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		f, ok := filters[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(f)
	})
	mux.HandleFunc("PUT /rest/api/3/filter/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		f, ok := filters[id]
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
				f[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(f)
	})
	mux.HandleFunc("DELETE /rest/api/3/filter/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := filters[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		delete(filters, id)
		w.WriteHeader(204)
	})

	// Custom Field endpoints
	mux.HandleFunc("POST /rest/api/3/field", func(w http.ResponseWriter, r *http.Request) {
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
		for _, cf := range fields {
			if cf["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}})
				return
			}
		}
		id := miscNextID("cf")
		description, _ := req["description"].(string)
		fieldType, _ := req["type"].(string)
		cf := map[string]interface{}{"id": id, "name": name, "description": description, "type": fieldType, "self": fmt.Sprintf("https://example.atlassian.net/rest/api/3/field/%s", id)}
		fields[id] = cf
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(cf)
	})
	mux.HandleFunc("GET /rest/api/3/field/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		cf, ok := fields[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cf)
	})
	mux.HandleFunc("PUT /rest/api/3/field/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		cf, ok := fields[id]
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
				cf[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cf)
	})
	mux.HandleFunc("DELETE /rest/api/3/field/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := fields[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		delete(fields, id)
		w.WriteHeader(204)
	})

	// Board endpoints
	mux.HandleFunc("POST /rest/agile/1.0/board", func(w http.ResponseWriter, r *http.Request) {
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
		for _, b := range boards {
			if b["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}})
				return
			}
		}
		id := miscNextID("board")
		boardType, _ := req["type"].(string)
		spaceID, _ := req["spaceId"].(string)
		b := map[string]interface{}{"id": id, "name": name, "type": boardType, "spaceId": spaceID, "self": fmt.Sprintf("https://example.atlassian.net/rest/agile/1.0/board/%s", id)}
		boards[id] = b
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(b)
	})
	mux.HandleFunc("GET /rest/agile/1.0/board/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		b, ok := boards[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(b)
	})
	mux.HandleFunc("PUT /rest/agile/1.0/board/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		b, ok := boards[id]
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
				b[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(b)
	})
	mux.HandleFunc("DELETE /rest/agile/1.0/board/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := boards[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		delete(boards, id)
		w.WriteHeader(204)
	})

	// Priority endpoints
	mux.HandleFunc("POST /rest/api/3/priority", func(w http.ResponseWriter, r *http.Request) {
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
		for _, p := range priorities {
			if p["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}})
				return
			}
		}
		id := miscNextID("pri")
		description, _ := req["description"].(string)
		iconURL, _ := req["iconUrl"].(string)
		p := map[string]interface{}{"id": id, "name": name, "description": description, "iconUrl": iconURL, "self": fmt.Sprintf("https://example.atlassian.net/rest/api/3/priority/%s", id)}
		priorities[id] = p
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("GET /rest/api/3/priority/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		p, ok := priorities[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("PUT /rest/api/3/priority/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		p, ok := priorities[id]
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
				p[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("DELETE /rest/api/3/priority/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := priorities[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		delete(priorities, id)
		w.WriteHeader(204)
	})

	// Priority Scheme endpoints
	mux.HandleFunc("POST /rest/api/3/priorityscheme", func(w http.ResponseWriter, r *http.Request) {
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
		for _, ps := range prioritySchemes {
			if ps["name"] == name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(409)
				json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}})
				return
			}
		}
		id := miscNextID("ps")
		description, _ := req["description"].(string)
		ps := map[string]interface{}{"id": id, "name": name, "description": description, "self": fmt.Sprintf("https://example.atlassian.net/rest/api/3/priorityscheme/%s", id)}
		prioritySchemes[id] = ps
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(ps)
	})
	mux.HandleFunc("GET /rest/api/3/priorityscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		ps, ok := prioritySchemes[id]
		mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ps)
	})
	mux.HandleFunc("PUT /rest/api/3/priorityscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		ps, ok := prioritySchemes[id]
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
				ps[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ps)
	})
	mux.HandleFunc("DELETE /rest/api/3/priorityscheme/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := prioritySchemes[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Not found"}})
			return
		}
		delete(prioritySchemes, id)
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

// testMiscForbiddenMockServer creates a mock that returns 403 for all endpoints.
func testMiscForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testMiscServerErrorMockServer creates a mock that returns 500 for all endpoints.
func testMiscServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// ==================== DASHBOARD RESOURCE TESTS ====================

// TestJiraDashboardResourceMetadata verifies the resource type name.
func TestJiraDashboardResourceMetadata(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_dashboard" {
		t.Errorf("expected type name 'atlassian_jira_dashboard', got %q", resp.TypeName)
	}
}

// TestJiraDashboardResourceSchema verifies all expected attributes exist.
func TestJiraDashboardResourceSchema(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, attr := range []string{"id", "name", "description"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(resp.Schema.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(resp.Schema.Attributes))
	}
}

// TestJiraDashboardResourceSchemaRequired verifies required attributes.
func TestJiraDashboardResourceSchemaRequired(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Error("name should be required")
	}
}

// TestJiraDashboardResourceSchemaComputed verifies computed attributes.
func TestJiraDashboardResourceSchemaComputed(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, name := range []string{"id", "description"} {
		if !resp.Schema.Attributes[name].IsComputed() {
			t.Errorf("%q should be computed", name)
		}
	}
}

// TestJiraDashboardResourceSchemaOptional verifies optional attributes.
func TestJiraDashboardResourceSchemaOptional(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if !resp.Schema.Attributes["description"].IsOptional() {
		t.Error("description should be optional")
	}
}

// TestJiraDashboardResourceSchemaSensitive verifies no attributes are sensitive.
func TestJiraDashboardResourceSchemaSensitive(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestJiraDashboardResourceImplementsResource verifies the Resource interface.
func TestJiraDashboardResourceImplementsResource(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource interface")
	}
}

// TestJiraDashboardResourceImplementsImportState verifies the ImportState interface.
func TestJiraDashboardResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState interface")
	}
}

// TestJiraDashboardResourceCRUDLifecycle tests the full CRUD cycle.
func TestJiraDashboardResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":              tftypes.NewValue(tftypes.String, "Test Dashboard"),
		"description":       tftypes.NewValue(tftypes.String, "A test dashboard"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
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
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Dashboard" {
		t.Errorf("expected name 'Test Dashboard', got %q", name)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Test Dashboard"), "description": tftypes.NewValue(tftypes.String, "A test dashboard"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Updated Dashboard"), "description": tftypes.NewValue(tftypes.String, "Updated desc"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Dashboard" {
		t.Errorf("expected name 'Updated Dashboard', got %q", name)
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
		// state removed
	}
}

// TestJiraDashboardResourceCreateNoDescription tests creating without description.
func TestJiraDashboardResourceCreateNoDescription(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "No Desc Dashboard"), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create no desc: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraDashboardResourceCreateDuplicate tests duplicate name error.
func TestJiraDashboardResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Dup Dashboard"), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First: %v", resp1.Diagnostics.Errors())
	}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate error")
	}
}

// TestJiraDashboardResourceUpdateNotFound tests updating nonexistent.
func TestJiraDashboardResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

// TestJiraDashboardResourceDeleteNotFound tests deleting already deleted.
func TestJiraDashboardResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent should not error")
	}
}

// TestJiraDashboardResourceReadNotFound tests reading nonexistent.
func TestJiraDashboardResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read nonexistent should not error: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state removed")
	}
}

// TestJiraDashboardResourceCreateForbidden tests 403 on create.
func TestJiraDashboardResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testMiscForbiddenMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Forbidden"), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

// TestJiraDashboardResourceUpdateForbidden tests 403 on update.
func TestJiraDashboardResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testMiscForbiddenMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

// TestJiraDashboardResourceDeleteForbidden tests 403 on delete.
func TestJiraDashboardResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testMiscForbiddenMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

// TestJiraDashboardResourceReadServerError tests 500 on read.
func TestJiraDashboardResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestJiraDashboardResourceCreateServerError tests 500 on create.
func TestJiraDashboardResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Error"), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestJiraDashboardResourceConfigureNilProviderData tests nil provider data.
func TestJiraDashboardResourceConfigureNilProviderData(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil provider data should not error")
	}
}

// TestJiraDashboardResourceConfigureWrongType tests wrong provider data type.
func TestJiraDashboardResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// ==================== DASHBOARD DATA SOURCE TESTS ====================

// TestJiraDashboardDataSourceMetadata verifies the data source type name.
func TestJiraDashboardDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := dashboarddatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_dashboard" {
		t.Errorf("expected 'atlassian_jira_dashboard', got %q", resp.TypeName)
	}
}

// TestJiraDashboardDataSourceSchema verifies schema attributes.
func TestJiraDashboardDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := dashboarddatasource.NewDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	for _, attr := range []string{"id", "name", "description", "share_permissions"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(resp.Schema.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(resp.Schema.Attributes))
	}
}

// TestJiraDashboardDataSourceRead tests reading a dashboard.
func TestJiraDashboardDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()

	// Create first via resource
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "DS Dashboard"), "description": tftypes.NewValue(tftypes.String, "ds desc"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	// Read via data source
	ds := dashboarddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", readResp.Diagnostics.Errors())
	}
}

// TestJiraDashboardDataSourceReadNotFound tests reading nonexistent.
func TestJiraDashboardDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	ds := dashboarddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

// TestJiraDashboardDataSourceReadServerError tests 500 on read.
func TestJiraDashboardDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	ds := dashboarddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// TestJiraDashboardDataSourceConfigureNil tests nil provider data.
func TestJiraDashboardDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := dashboarddatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraDashboardDataSourceConfigureWrongType tests wrong type.
func TestJiraDashboardDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := dashboarddatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// ==================== FILTER RESOURCE TESTS ====================

// TestJiraFilterResourceMetadata verifies the resource type name.
func TestJiraFilterResourceMetadata(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_filter" {
		t.Errorf("expected 'atlassian_jira_filter', got %q", resp.TypeName)
	}
}

// TestJiraFilterResourceSchema verifies schema attributes.
func TestJiraFilterResourceSchema(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, attr := range []string{"id", "name", "description", "jql", "share_permissions"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(resp.Schema.Attributes) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(resp.Schema.Attributes))
	}
}

// TestJiraFilterResourceSchemaRequired verifies required attributes.
func TestJiraFilterResourceSchemaRequired(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, name := range []string{"name", "jql"} {
		if !resp.Schema.Attributes[name].IsRequired() {
			t.Errorf("%q should be required", name)
		}
	}
}

// TestJiraFilterResourceImplementsInterfaces verifies interfaces.
func TestJiraFilterResourceImplementsInterfaces(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

// TestJiraFilterResourceCRUDLifecycle tests the full CRUD cycle.
func TestJiraFilterResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Test Filter"),
		"description": tftypes.NewValue(tftypes.String, "A filter"), "jql": tftypes.NewValue(tftypes.String, "project = TEST"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
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
	if jql := getStringAttr(t, createResp.State, "jql"); jql != "project = TEST" {
		t.Errorf("expected jql 'project = TEST', got %q", jql)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Test Filter"),
		"description": tftypes.NewValue(tftypes.String, "A filter"), "jql": tftypes.NewValue(tftypes.String, "project = TEST"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Updated Filter"),
		"description": tftypes.NewValue(tftypes.String, "Updated"), "jql": tftypes.NewValue(tftypes.String, "project = UPDATED"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: deleteState.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

// TestJiraFilterResourceCreateDuplicate tests duplicate name.
func TestJiraFilterResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Dup Filter"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "jql": tftypes.NewValue(tftypes.String, "x=y"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First: %v", resp1.Diagnostics.Errors())
	}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate error")
	}
}

// TestJiraFilterResourceErrorPaths tests update/delete/read not found and forbidden.
func TestJiraFilterResourceErrorPaths(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "jql": tftypes.NewValue(tftypes.String, "x"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}

	// Update not found
	uResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, uResp)
	if !uResp.Diagnostics.HasError() {
		t.Fatal("Expected update not found")
	}

	// Delete not found (idempotent)
	dResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, dResp)
	if dResp.Diagnostics.HasError() {
		t.Fatal("Delete nonexistent should not error")
	}

	// Read not found
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Read nonexistent should not error: %v", rResp.Diagnostics.Errors())
	}

	// Forbidden
	_, fclient := testMiscForbiddenMockServer(t)
	r2 := dashboardresource.NewFilterResource()
	configureResource(t, r2, fclient)
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "F"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "jql": tftypes.NewValue(tftypes.String, "x"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	r2.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if !cResp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden")
	}

	// Forbidden update
	uResp2 := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r2.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, uResp2)
	if !uResp2.Diagnostics.HasError() {
		t.Fatal("Expected forbidden on update")
	}

	// Forbidden delete
	dResp2 := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r2.Delete(ctx, resource.DeleteRequest{State: state}, dResp2)
	if !dResp2.Diagnostics.HasError() {
		t.Fatal("Expected forbidden on delete")
	}

	// Server error
	_, sclient := testMiscServerErrorMockServer(t)
	r3 := dashboardresource.NewFilterResource()
	configureResource(t, r3, sclient)
	seResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r3.Read(ctx, resource.ReadRequest{State: state}, seResp)
	if !seResp.Diagnostics.HasError() {
		t.Fatal("Expected server error on read")
	}
}

// TestJiraFilterDataSourceRead tests the filter data source.
func TestJiraFilterDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()

	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "DS Filter"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"), "jql": tftypes.NewValue(tftypes.String, "a=b"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	ds := dashboarddatasource.NewFilterDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "jql": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", readResp.Diagnostics.Errors())
	}
}

// TestJiraFilterDataSourceMetadata verifies data source type name.
func TestJiraFilterDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := dashboarddatasource.NewFilterDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_filter" {
		t.Errorf("expected 'atlassian_jira_filter', got %q", resp.TypeName)
	}
}

// TestJiraFilterDataSourceSchema verifies schema.
func TestJiraFilterDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := dashboarddatasource.NewFilterDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if len(resp.Schema.Attributes) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(resp.Schema.Attributes))
	}
}

// TestJiraFilterDataSourceReadNotFound tests 404 on read.
func TestJiraFilterDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	ds := dashboarddatasource.NewFilterDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "jql": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
}

// ==================== CUSTOM FIELD RESOURCE TESTS ====================

// TestJiraCustomFieldResourceMetadata verifies the resource type name.
func TestJiraCustomFieldResourceMetadata(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_custom_field" {
		t.Errorf("expected 'atlassian_jira_custom_field', got %q", resp.TypeName)
	}
}

// TestJiraCustomFieldResourceSchema verifies schema.
func TestJiraCustomFieldResourceSchema(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, attr := range []string{"id", "name", "description", "type"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(resp.Schema.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(resp.Schema.Attributes))
	}
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Error("name should be required")
	}
	if !resp.Schema.Attributes["type"].IsRequired() {
		t.Error("type should be required")
	}
}

// TestJiraCustomFieldResourceImplementsInterfaces verifies interfaces.
func TestJiraCustomFieldResourceImplementsInterfaces(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

// TestJiraCustomFieldResourceCRUDLifecycle tests the full CRUD cycle.
func TestJiraCustomFieldResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Test Field"),
		"description": tftypes.NewValue(tftypes.String, "A field"), "type": tftypes.NewValue(tftypes.String, "text"),
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
	if ft := getStringAttr(t, createResp.State, "type"); ft != "text" {
		t.Errorf("expected type 'text', got %q", ft)
	}

	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Test Field"),
		"description": tftypes.NewValue(tftypes.String, "A field"), "type": tftypes.NewValue(tftypes.String, "text"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Updated Field"),
		"description": tftypes.NewValue(tftypes.String, "Updated"), "type": tftypes.NewValue(tftypes.String, "number"),
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

// TestJiraCustomFieldResourceErrorPaths tests error scenarios.
func TestJiraCustomFieldResourceErrorPaths(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Duplicate
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Dup Field"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "type": tftypes.NewValue(tftypes.String, "text"),
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
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "type": tftypes.NewValue(tftypes.String, "text"),
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
	_, fclient := testMiscForbiddenMockServer(t)
	r2 := customfieldresource.NewResource()
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
	_, sclient := testMiscServerErrorMockServer(t)
	r3 := customfieldresource.NewResource()
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
}

// TestJiraCustomFieldDataSource tests the data source.
func TestJiraCustomFieldDataSource(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()

	r := customfieldresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "DS Field"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"), "type": tftypes.NewValue(tftypes.String, "select"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	id := getStringAttr(t, createResp.State, "id")

	ds := customfielddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	// Metadata
	metaReq := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	metaResp := &datasource.MetadataResponse{}
	ds.Metadata(ctx, metaReq, metaResp)
	if metaResp.TypeName != "atlassian_jira_custom_field" {
		t.Errorf("expected 'atlassian_jira_custom_field', got %q", metaResp.TypeName)
	}

	// Schema
	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if len(schemaResp.Schema.Attributes) != 4 {
		t.Errorf("expected 4 attrs, got %d", len(schemaResp.Schema.Attributes))
	}

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", readResp.Diagnostics.Errors())
	}

	// Not found
	config404 := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp404 := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config404}, resp404)
	if !resp404.Diagnostics.HasError() {
		t.Fatal("Expected not found")
	}

	// Server error
	_, sclient := testMiscServerErrorMockServer(t)
	ds2 := customfielddatasource.NewDataSource()
	configureDatasource(t, ds2, sclient)
	seResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds2.Read(ctx, datasource.ReadRequest{Config: config}, seResp)
	if !seResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// ==================== BOARD RESOURCE TESTS ====================

// TestJiraBoardResourceMetadata verifies the resource type name.
func TestJiraBoardResourceMetadata(t *testing.T) {
	t.Parallel()
	r := boardresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_board" {
		t.Errorf("expected 'atlassian_jira_board', got %q", resp.TypeName)
	}
}

// TestJiraBoardResourceSchema verifies schema.
func TestJiraBoardResourceSchema(t *testing.T) {
	t.Parallel()
	r := boardresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, attr := range []string{"id", "name", "type", "space_id"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(resp.Schema.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(resp.Schema.Attributes))
	}
	for _, name := range []string{"name", "type", "space_id"} {
		if !resp.Schema.Attributes[name].IsRequired() {
			t.Errorf("%q should be required", name)
		}
	}
}

// TestJiraBoardResourceImplementsInterfaces verifies interfaces.
func TestJiraBoardResourceImplementsInterfaces(t *testing.T) {
	t.Parallel()
	r := boardresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

// TestJiraBoardResourceCRUDLifecycle tests the full CRUD cycle.
func TestJiraBoardResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := boardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Test Board"),
		"type": tftypes.NewValue(tftypes.String, "scrum"), "space_id": tftypes.NewValue(tftypes.String, "PROJ1"),
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
	if bt := getStringAttr(t, createResp.State, "type"); bt != "scrum" {
		t.Errorf("expected type 'scrum', got %q", bt)
	}

	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Test Board"),
		"type": tftypes.NewValue(tftypes.String, "scrum"), "space_id": tftypes.NewValue(tftypes.String, "PROJ1"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Updated Board"),
		"type": tftypes.NewValue(tftypes.String, "kanban"), "space_id": tftypes.NewValue(tftypes.String, "PROJ2"),
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

// TestJiraBoardResourceErrorPaths tests error scenarios.
func TestJiraBoardResourceErrorPaths(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := boardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Duplicate
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Dup Board"),
		"type": tftypes.NewValue(tftypes.String, "scrum"), "space_id": tftypes.NewValue(tftypes.String, "P"),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate error")
	}

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
		"type": tftypes.NewValue(tftypes.String, "scrum"), "space_id": tftypes.NewValue(tftypes.String, "P"),
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
	_, fclient := testMiscForbiddenMockServer(t)
	r2 := boardresource.NewResource()
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
	_, sclient := testMiscServerErrorMockServer(t)
	r3 := boardresource.NewResource()
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
}

// TestJiraBoardDataSource tests the data source.
func TestJiraBoardDataSource(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()

	r := boardresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "DS Board"),
		"type": tftypes.NewValue(tftypes.String, "kanban"), "space_id": tftypes.NewValue(tftypes.String, "SP1"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	id := getStringAttr(t, createResp.State, "id")

	ds := boarddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	metaReq := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	metaResp := &datasource.MetadataResponse{}
	ds.Metadata(ctx, metaReq, metaResp)
	if metaResp.TypeName != "atlassian_jira_board" {
		t.Errorf("expected 'atlassian_jira_board', got %q", metaResp.TypeName)
	}
	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if len(schemaResp.Schema.Attributes) != 4 {
		t.Errorf("expected 4 attrs, got %d", len(schemaResp.Schema.Attributes))
	}

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", readResp.Diagnostics.Errors())
	}

	// Not found
	config404 := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "space_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp404 := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config404}, resp404)
	if !resp404.Diagnostics.HasError() {
		t.Fatal("Expected not found")
	}

	// Server error
	_, sclient := testMiscServerErrorMockServer(t)
	ds2 := boarddatasource.NewDataSource()
	configureDatasource(t, ds2, sclient)
	seResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds2.Read(ctx, datasource.ReadRequest{Config: config}, seResp)
	if !seResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// ==================== PRIORITY RESOURCE TESTS ====================

// TestJiraPriorityResourceMetadata verifies the resource type name.
func TestJiraPriorityResourceMetadata(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_priority" {
		t.Errorf("expected 'atlassian_jira_priority', got %q", resp.TypeName)
	}
}

// TestJiraPriorityResourceSchema verifies schema.
func TestJiraPriorityResourceSchema(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, attr := range []string{"id", "name", "description", "icon_url"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(resp.Schema.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(resp.Schema.Attributes))
	}
	if !resp.Schema.Attributes["name"].IsRequired() {
		t.Error("name should be required")
	}
	for _, name := range []string{"description", "icon_url"} {
		if !resp.Schema.Attributes[name].IsOptional() {
			t.Errorf("%q should be optional", name)
		}
	}
}

// TestJiraPriorityResourceImplementsInterfaces verifies interfaces.
func TestJiraPriorityResourceImplementsInterfaces(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

// TestJiraPriorityResourceCRUDLifecycle tests the full CRUD cycle.
func TestJiraPriorityResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "High"),
		"description": tftypes.NewValue(tftypes.String, "High priority"), "icon_url": tftypes.NewValue(tftypes.String, "https://example.com/high.png"),
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
	if iu := getStringAttr(t, createResp.State, "icon_url"); iu != "https://example.com/high.png" {
		t.Errorf("expected icon_url, got %q", iu)
	}

	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "High"),
		"description": tftypes.NewValue(tftypes.String, "High priority"), "icon_url": tftypes.NewValue(tftypes.String, "https://example.com/high.png"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Critical"),
		"description": tftypes.NewValue(tftypes.String, "Critical"), "icon_url": tftypes.NewValue(tftypes.String, "https://example.com/critical.png"),
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

// TestJiraPriorityResourceCreateNoOptional tests creating without optional fields.
func TestJiraPriorityResourceCreateNoOptional(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Low No Opt"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "icon_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraPriorityResourceErrorPaths tests error scenarios.
func TestJiraPriorityResourceErrorPaths(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Dup Priority"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "icon_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate error")
	}

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "icon_url": tftypes.NewValue(tftypes.String, ""),
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

	_, fclient := testMiscForbiddenMockServer(t)
	r2 := priorityresource.NewResource()
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

	_, sclient := testMiscServerErrorMockServer(t)
	r3 := priorityresource.NewResource()
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
}

// TestJiraPriorityDataSource tests the data source.
func TestJiraPriorityDataSource(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()

	r := priorityresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "DS Priority"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"), "icon_url": tftypes.NewValue(tftypes.String, "https://x.com/i.png"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	id := getStringAttr(t, createResp.State, "id")

	ds := prioritydatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	metaReq := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	metaResp := &datasource.MetadataResponse{}
	ds.Metadata(ctx, metaReq, metaResp)
	if metaResp.TypeName != "atlassian_jira_priority" {
		t.Errorf("expected 'atlassian_jira_priority', got %q", metaResp.TypeName)
	}
	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if len(schemaResp.Schema.Attributes) != 4 {
		t.Errorf("expected 4 attrs, got %d", len(schemaResp.Schema.Attributes))
	}

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "icon_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", readResp.Diagnostics.Errors())
	}

	config404 := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "icon_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp404 := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config404}, resp404)
	if !resp404.Diagnostics.HasError() {
		t.Fatal("Expected not found")
	}

	_, sclient := testMiscServerErrorMockServer(t)
	ds2 := prioritydatasource.NewDataSource()
	configureDatasource(t, ds2, sclient)
	seResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds2.Read(ctx, datasource.ReadRequest{Config: config}, seResp)
	if !seResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// ==================== PRIORITY SCHEME RESOURCE TESTS ====================

// TestJiraPrioritySchemeResourceMetadata verifies the resource type name.
func TestJiraPrioritySchemeResourceMetadata(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewSchemeResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_priority_scheme" {
		t.Errorf("expected 'atlassian_jira_priority_scheme', got %q", resp.TypeName)
	}
}

// TestJiraPrioritySchemeResourceSchema verifies schema.
func TestJiraPrioritySchemeResourceSchema(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewSchemeResource()
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
}

// TestJiraPrioritySchemeResourceImplementsInterfaces verifies interfaces.
func TestJiraPrioritySchemeResourceImplementsInterfaces(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewSchemeResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected resource.Resource")
	}
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected ResourceWithImportState")
	}
}

// TestJiraPrioritySchemeResourceCRUDLifecycle tests the full CRUD cycle.
func TestJiraPrioritySchemeResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Test Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A scheme"),
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

	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Test Scheme"),
		"description": tftypes.NewValue(tftypes.String, "A scheme"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, "Updated Scheme"),
		"description": tftypes.NewValue(tftypes.String, "Updated"),
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

// TestJiraPrioritySchemeResourceErrorPaths tests error scenarios.
func TestJiraPrioritySchemeResourceErrorPaths(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Dup Scheme"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate error")
	}

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
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

	_, fclient := testMiscForbiddenMockServer(t)
	r2 := priorityresource.NewSchemeResource()
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

	_, sclient := testMiscServerErrorMockServer(t)
	r3 := priorityresource.NewSchemeResource()
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
}

// TestJiraPrioritySchemeDataSource tests the data source.
func TestJiraPrioritySchemeDataSource(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()

	r := priorityresource.NewSchemeResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "DS Scheme"),
		"description": tftypes.NewValue(tftypes.String, "ds desc"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	id := getStringAttr(t, createResp.State, "id")

	ds := prioritydatasource.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	metaReq := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	metaResp := &datasource.MetadataResponse{}
	ds.Metadata(ctx, metaReq, metaResp)
	if metaResp.TypeName != "atlassian_jira_priority_scheme" {
		t.Errorf("expected 'atlassian_jira_priority_scheme', got %q", metaResp.TypeName)
	}
	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if len(schemaResp.Schema.Attributes) != 3 {
		t.Errorf("expected 3 attrs, got %d", len(schemaResp.Schema.Attributes))
	}

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, id), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", readResp.Diagnostics.Errors())
	}

	config404 := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp404 := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config404}, resp404)
	if !resp404.Diagnostics.HasError() {
		t.Fatal("Expected not found")
	}

	_, sclient := testMiscServerErrorMockServer(t)
	ds2 := prioritydatasource.NewSchemeDataSource()
	configureDatasource(t, ds2, sclient)
	seResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds2.Read(ctx, datasource.ReadRequest{Config: config}, seResp)
	if !seResp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// ==================== CONFIGURE NIL/WRONG TYPE TESTS ====================

// TestJiraFilterResourceConfigureNil tests nil provider data.
func TestJiraFilterResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraFilterResourceConfigureWrongType tests wrong type.
func TestJiraFilterResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraCustomFieldResourceConfigureNil tests nil provider data.
func TestJiraCustomFieldResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraCustomFieldResourceConfigureWrongType tests wrong type.
func TestJiraCustomFieldResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraBoardResourceConfigureNil tests nil provider data.
func TestJiraBoardResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := boardresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraBoardResourceConfigureWrongType tests wrong type.
func TestJiraBoardResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := boardresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraPriorityResourceConfigureNil tests nil provider data.
func TestJiraPriorityResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraPriorityResourceConfigureWrongType tests wrong type.
func TestJiraPriorityResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraPrioritySchemeResourceConfigureNil tests nil provider data.
func TestJiraPrioritySchemeResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewSchemeResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraPrioritySchemeResourceConfigureWrongType tests wrong type.
func TestJiraPrioritySchemeResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewSchemeResource()
	resp := &resource.ConfigureResponse{}
	r.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraCustomFieldDataSourceConfigureNil tests nil provider data.
func TestJiraCustomFieldDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := customfielddatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraCustomFieldDataSourceConfigureWrongType tests wrong type.
func TestJiraCustomFieldDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := customfielddatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraBoardDataSourceConfigureNil tests nil provider data.
func TestJiraBoardDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := boarddatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraBoardDataSourceConfigureWrongType tests wrong type.
func TestJiraBoardDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := boarddatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraPriorityDataSourceConfigureNil tests nil provider data.
func TestJiraPriorityDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := prioritydatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraPriorityDataSourceConfigureWrongType tests wrong type.
func TestJiraPriorityDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := prioritydatasource.NewDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraPrioritySchemeDataSourceConfigureNil tests nil provider data.
func TestJiraPrioritySchemeDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := prioritydatasource.NewSchemeDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraPrioritySchemeDataSourceConfigureWrongType tests wrong type.
func TestJiraPrioritySchemeDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := prioritydatasource.NewSchemeDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestJiraFilterDataSourceConfigureNil tests nil provider data.
func TestJiraFilterDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := dashboarddatasource.NewFilterDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraFilterDataSourceConfigureWrongType tests wrong type.
func TestJiraFilterDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := dashboarddatasource.NewFilterDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(datasource.DataSourceWithConfigure).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// ==================== IMPORT STATE TESTS ====================

// TestJiraDashboardResourceImportState verifies import state passthrough.
func TestJiraDashboardResourceImportState(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "dash-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraFilterResourceImportState verifies import state passthrough.
func TestJiraFilterResourceImportState(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "flt-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraCustomFieldResourceImportState verifies import state passthrough.
func TestJiraCustomFieldResourceImportState(t *testing.T) {
	t.Parallel()
	r := customfieldresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "cf-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraBoardResourceImportState verifies import state passthrough.
func TestJiraBoardResourceImportState(t *testing.T) {
	t.Parallel()
	r := boardresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "board-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraPriorityResourceImportState verifies import state passthrough.
func TestJiraPriorityResourceImportState(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "pri-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestJiraPrioritySchemeResourceImportState verifies import state passthrough.
func TestJiraPrioritySchemeResourceImportState(t *testing.T) {
	t.Parallel()
	r := priorityresource.NewSchemeResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "ps-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== UPDATE/DELETE SERVER ERROR TESTS ====================

// TestJiraDashboardResourceUpdateServerError tests 500 on update.
func TestJiraDashboardResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraDashboardResourceDeleteServerError tests 500 on delete.
func TestJiraDashboardResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraFilterResourceUpdateServerError tests 500 on update.
func TestJiraFilterResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "jql": tftypes.NewValue(tftypes.String, "x"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraFilterResourceDeleteServerError tests 500 on delete.
func TestJiraFilterResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "jql": tftypes.NewValue(tftypes.String, "x"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraFilterResourceCreateServerError tests 500 on create.
func TestJiraFilterResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "Error"),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "jql": tftypes.NewValue(tftypes.String, "x"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 create")
	}
}

// TestJiraCustomFieldResourceUpdateServerError tests 500 on update.
func TestJiraCustomFieldResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "type": tftypes.NewValue(tftypes.String, "text"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraCustomFieldResourceDeleteServerError tests 500 on delete.
func TestJiraCustomFieldResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := customfieldresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "type": tftypes.NewValue(tftypes.String, "text"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraBoardResourceUpdateServerError tests 500 on update.
func TestJiraBoardResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := boardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"type": tftypes.NewValue(tftypes.String, "scrum"), "space_id": tftypes.NewValue(tftypes.String, "P"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraBoardResourceDeleteServerError tests 500 on delete.
func TestJiraBoardResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := boardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"type": tftypes.NewValue(tftypes.String, "scrum"), "space_id": tftypes.NewValue(tftypes.String, "P"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraPriorityResourceUpdateServerError tests 500 on update.
func TestJiraPriorityResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "icon_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraPriorityResourceDeleteServerError tests 500 on delete.
func TestJiraPriorityResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "icon_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraPrioritySchemeResourceUpdateServerError tests 500 on update.
func TestJiraPrioritySchemeResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestJiraPrioritySchemeResourceDeleteServerError tests 500 on delete.
func TestJiraPrioritySchemeResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "some-id"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestJiraFilterDataSourceServerError tests 500 on filter data source read.
func TestJiraFilterDataSourceServerError(t *testing.T) {
	t.Parallel()
	_, client := testMiscServerErrorMockServer(t)
	ctx := context.Background()
	ds := dashboarddatasource.NewFilterDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "jql": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected server error")
	}
}

// ==================== BAD PLAN/STATE TESTS ====================

// TestJiraDashboardResourceCreateBadPlan tests Create with invalid plan data.
func TestJiraDashboardResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewResource()
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

// TestJiraDashboardResourceReadBadState tests Read with invalid state.
func TestJiraDashboardResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewResource()
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

// TestJiraDashboardResourceUpdateBadPlan tests Update with invalid plan.
func TestJiraDashboardResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraDashboardResourceUpdateBadState tests Update with invalid state.
func TestJiraDashboardResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"), "description": tftypes.NewValue(tftypes.String, ""),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraDashboardResourceDeleteBadState tests Delete with invalid state.
func TestJiraDashboardResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewResource()
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

// TestJiraFilterResourceCreateBadPlan tests Create with invalid plan.
func TestJiraFilterResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewFilterResource()
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

// TestJiraFilterResourceReadBadState tests Read with invalid state.
func TestJiraFilterResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewFilterResource()
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

// TestJiraFilterResourceUpdateBadPlan tests Update with invalid plan.
func TestJiraFilterResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	tfType := s.Type().TerraformType(ctx)
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "jql": tftypes.NewValue(tftypes.String, "x"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraFilterResourceUpdateBadState tests Update with invalid state.
func TestJiraFilterResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "jql": tftypes.NewValue(tftypes.String, "x"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "parameter": tftypes.String}}}, nil),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraFilterResourceDeleteBadState tests Delete with invalid state.
func TestJiraFilterResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := dashboardresource.NewFilterResource()
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

// TestJiraCustomFieldResourceCreateBadPlan tests Create with invalid plan.
func TestJiraCustomFieldResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := customfieldresource.NewResource()
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

// TestJiraCustomFieldResourceReadBadState tests Read with invalid state.
func TestJiraCustomFieldResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := customfieldresource.NewResource()
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

// TestJiraCustomFieldResourceUpdateBadPlan tests Update with invalid plan.
func TestJiraCustomFieldResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := customfieldresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "type": tftypes.NewValue(tftypes.String, "text"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraCustomFieldResourceUpdateBadState tests Update with invalid state.
func TestJiraCustomFieldResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := customfieldresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "type": tftypes.NewValue(tftypes.String, "text"),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraCustomFieldResourceDeleteBadState tests Delete with invalid state.
func TestJiraCustomFieldResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := customfieldresource.NewResource()
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

// TestJiraBoardResourceCreateBadPlan tests Create with invalid plan.
func TestJiraBoardResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := boardresource.NewResource()
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

// TestJiraBoardResourceReadBadState tests Read with invalid state.
func TestJiraBoardResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := boardresource.NewResource()
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

// TestJiraBoardResourceUpdateBadPlan tests Update with invalid plan.
func TestJiraBoardResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := boardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"type": tftypes.NewValue(tftypes.String, "scrum"), "space_id": tftypes.NewValue(tftypes.String, "P"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraBoardResourceUpdateBadState tests Update with invalid state.
func TestJiraBoardResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := boardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"type": tftypes.NewValue(tftypes.String, "scrum"), "space_id": tftypes.NewValue(tftypes.String, "P"),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraBoardResourceDeleteBadState tests Delete with invalid state.
func TestJiraBoardResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := boardresource.NewResource()
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

// TestJiraPriorityResourceCreateBadPlan tests Create with invalid plan.
func TestJiraPriorityResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewResource()
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

// TestJiraPriorityResourceReadBadState tests Read with invalid state.
func TestJiraPriorityResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewResource()
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

// TestJiraPriorityResourceUpdateBadPlan tests Update with invalid plan.
func TestJiraPriorityResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "icon_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestJiraPriorityResourceUpdateBadState tests Update with invalid state.
func TestJiraPriorityResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "icon_url": tftypes.NewValue(tftypes.String, ""),
	})}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestJiraPriorityResourceDeleteBadState tests Delete with invalid state.
func TestJiraPriorityResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewResource()
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

// TestJiraPrioritySchemeResourceCreateBadPlan tests Create with invalid plan.
func TestJiraPrioritySchemeResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewSchemeResource()
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

// TestJiraPrioritySchemeResourceReadBadState tests Read with invalid state.
func TestJiraPrioritySchemeResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewSchemeResource()
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

// TestJiraPrioritySchemeResourceUpdateBadPlan tests Update with invalid plan.
func TestJiraPrioritySchemeResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewSchemeResource()
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

// TestJiraPrioritySchemeResourceUpdateBadState tests Update with invalid state.
func TestJiraPrioritySchemeResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewSchemeResource()
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

// TestJiraPrioritySchemeResourceDeleteBadState tests Delete with invalid state.
func TestJiraPrioritySchemeResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	r := priorityresource.NewSchemeResource()
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

// ==================== DATA SOURCE BAD CONFIG TESTS ====================

// TestJiraDashboardDataSourceReadBadConfig tests Read with invalid config.
func TestJiraDashboardDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ds := dashboarddatasource.NewDataSource()
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

// TestJiraFilterDataSourceReadBadConfig tests Read with invalid config.
func TestJiraFilterDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ds := dashboarddatasource.NewFilterDataSource()
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

// TestJiraCustomFieldDataSourceReadBadConfig tests Read with invalid config.
func TestJiraCustomFieldDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ds := customfielddatasource.NewDataSource()
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

// TestJiraBoardDataSourceReadBadConfig tests Read with invalid config.
func TestJiraBoardDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ds := boarddatasource.NewDataSource()
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

// TestJiraPriorityDataSourceReadBadConfig tests Read with invalid config.
func TestJiraPriorityDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ds := prioritydatasource.NewDataSource()
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

// TestJiraPrioritySchemeDataSourceReadBadConfig tests Read with invalid config.
func TestJiraPrioritySchemeDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ds := prioritydatasource.NewSchemeDataSource()
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

// TestJiraPriorityResourceReadNotFound tests reading nonexistent priority removes state.
func TestJiraPriorityResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := priorityresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, "X"),
		"description": tftypes.NewValue(tftypes.String, ""), "icon_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read nonexistent should not error: %v", resp.Diagnostics.Errors())
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state removed")
	}
}

// TestJiraDashboardWithSharePermissions tests creating a dashboard with non-empty share_permissions.
func TestJiraDashboardWithSharePermissions(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	spObjType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"type": tftypes.String, "parameter": tftypes.String,
	}}
	spListType := tftypes.List{ElementType: spObjType}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Shared Dashboard"),
		"description": tftypes.NewValue(tftypes.String, "With permissions"),
		"share_permissions": tftypes.NewValue(spListType, []tftypes.Value{
			tftypes.NewValue(spObjType, map[string]tftypes.Value{
				"type":      tftypes.NewValue(tftypes.String, "global"),
				"parameter": tftypes.NewValue(tftypes.String, ""),
			}),
		}),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create with share_permissions: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
}

// TestJiraDashboardSharePermissionsToStateNonEmpty exercises the non-empty
// path of sharePermissionsToState via a Read that returns sharePermissions.
func TestJiraDashboardSharePermissionsToStateNonEmpty(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/dashboard/d-sp-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "d-sp-1", "name": "Test", "description": "desc",
			"sharePermissions": []map[string]interface{}{
				{"type": "global", "parameter": ""},
				{"type": "group", "parameter": "developers"},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()

	r := dashboardresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	spObjType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"type": tftypes.String, "parameter": tftypes.String,
	}}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, "d-sp-1"),
		"name":              tftypes.NewValue(tftypes.String, "Test"),
		"description":       tftypes.NewValue(tftypes.String, "desc"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: spObjType}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	ds := dashboarddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)
	dsConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, "d-sp-1"),
		"name":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: spObjType}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: dsConfig}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DataSource Read: %v", dsResp.Diagnostics.Errors())
	}
}

// TestJiraFilterWithSharePermissions tests creating a filter with non-empty share_permissions.
func TestJiraFilterWithSharePermissions(t *testing.T) {
	t.Parallel()
	_, client := testMiscMockServer(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	spObjType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"type": tftypes.String, "parameter": tftypes.String,
	}}
	spListType := tftypes.List{ElementType: spObjType}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Shared Filter"),
		"description": tftypes.NewValue(tftypes.String, "With permissions"),
		"jql":         tftypes.NewValue(tftypes.String, "project = TEST"),
		"share_permissions": tftypes.NewValue(spListType, []tftypes.Value{
			tftypes.NewValue(spObjType, map[string]tftypes.Value{
				"type":      tftypes.NewValue(tftypes.String, "group"),
				"parameter": tftypes.NewValue(tftypes.String, "developers"),
			}),
		}),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create with share_permissions: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
}

// TestJiraFilterSharePermissionsToStateNonEmpty exercises the non-empty
// path of filterSharePermissionsToState via a Read that returns sharePermissions.
func TestJiraFilterSharePermissionsToStateNonEmpty(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/filter/f-sp-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "f-sp-1", "name": "Test Filter", "description": "desc", "jql": "project = TEST",
			"sharePermissions": []map[string]interface{}{
				{"type": "group", "parameter": "testers"},
			},
		})
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5e9, MaxRetries: 0, RetryWaitMin: 1e9, RetryWaitMax: 1e9}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})
	ctx := context.Background()

	r := dashboardresource.NewFilterResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	spObjType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"type": tftypes.String, "parameter": tftypes.String,
	}}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, "f-sp-1"),
		"name":              tftypes.NewValue(tftypes.String, "Test Filter"),
		"description":       tftypes.NewValue(tftypes.String, "desc"),
		"jql":               tftypes.NewValue(tftypes.String, "project = TEST"),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: spObjType}, nil),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	ds := dashboarddatasource.NewFilterDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)
	dsConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":                tftypes.NewValue(tftypes.String, "f-sp-1"),
		"name":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"jql":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"share_permissions": tftypes.NewValue(tftypes.List{ElementType: spObjType}, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: dsConfig}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DataSource Read: %v", dsResp.Diagnostics.Errors())
	}
}

// TestJiraFilterJQLRequired verifies that the jql attribute is required in the filter resource schema.
func TestJiraFilterJQLRequired(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	jqlAttr, ok := resp.Schema.Attributes["jql"]
	if !ok {
		t.Fatal("expected jql attribute in schema")
	}
	if !jqlAttr.IsRequired() {
		t.Error("jql should be required")
	}
}

// TestJiraFilterJQLDescriptionMentionsValidation verifies the JQL description mentions validation.
func TestJiraFilterJQLDescriptionMentionsValidation(t *testing.T) {
	t.Parallel()
	r := dashboardresource.NewFilterResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	jqlAttr := resp.Schema.Attributes["jql"]
	desc := jqlAttr.GetDescription()
	if desc == "" {
		t.Fatal("expected non-empty jql description")
	}
	if !jqlContainsValidation(desc) {
		t.Errorf("expected jql description to mention validation, got %q", desc)
	}
}

// jqlContainsValidation checks if the JQL description mentions validation.
func jqlContainsValidation(desc string) bool {
	for i := 0; i <= len(desc)-5; i++ {
		if desc[i:i+5] == "valid" {
			return true
		}
	}
	return false
}
