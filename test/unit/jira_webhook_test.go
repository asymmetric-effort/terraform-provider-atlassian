// Package unit contains unit tests for the atlassian_jira_webhook
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
	webhookdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/webhook"
	webhookresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/webhook"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// whIDCounter provides unique IDs for webhook mock server tests.
var whIDCounter uint64

func whNextID() string {
	return fmt.Sprintf("%d", atomic.AddUint64(&whIDCounter, 1))
}

// eventsListType is the tftypes.List type for the events attribute.
var eventsListType = tftypes.List{ElementType: tftypes.String}

// testWebhookMockServer creates a mock HTTP server for webhook endpoints.
func testWebhookMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	webhooks := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/api/3/webhook", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		url, _ := req["url"].(string)
		if name == "" || url == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"name and url are required"},
			})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := whNextID()
		// Ensure events is stored as a []interface{}
		events, _ := req["events"].([]interface{})
		if events == nil {
			events = []interface{}{}
		}
		enabled := true
		if e, ok := req["enabled"].(bool); ok {
			enabled = e
		}
		jqlFilter, _ := req["jqlFilter"].(string)
		wh := map[string]interface{}{
			"id":        id,
			"name":      name,
			"url":       url,
			"events":    events,
			"jqlFilter": jqlFilter,
			"enabled":   enabled,
			"self":      fmt.Sprintf("/rest/api/3/webhook/%s", id),
		}
		webhooks[id] = wh
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(wh)
	})

	mux.HandleFunc("GET /rest/api/3/webhook/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		wh, ok := webhooks[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Webhook not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wh)
	})

	mux.HandleFunc("PUT /rest/api/3/webhook/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		wh, ok := webhooks[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Webhook not found"},
			})
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" && k != "self" {
				wh[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(wh)
	})

	mux.HandleFunc("DELETE /rest/api/3/webhook/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := webhooks[id]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Webhook not found"},
			})
			return
		}
		delete(webhooks, id)
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

// testWebhookForbiddenMockServer creates a mock that returns 403 for all endpoints.
func testWebhookForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testWebhookServerErrorMockServer creates a mock that returns 500 for all endpoints.
func testWebhookServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// testWebhookBadRequestMockServer creates a mock that returns 400 for all endpoints.
func testWebhookBadRequestMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
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

// webhookPlanValues builds tftypes values for a webhook plan/state.
func webhookPlanValues(id, name, url string, events []string, jqlFilter string, enabled bool, isCreate bool) map[string]tftypes.Value {
	idVal := tftypes.NewValue(tftypes.String, id)
	if isCreate {
		idVal = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}

	var eventVals []tftypes.Value
	for _, e := range events {
		eventVals = append(eventVals, tftypes.NewValue(tftypes.String, e))
	}
	eventsVal := tftypes.NewValue(eventsListType, eventVals)

	jqlVal := tftypes.NewValue(tftypes.String, jqlFilter)
	if jqlFilter == "" && isCreate {
		jqlVal = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}

	return map[string]tftypes.Value{
		"id":         idVal,
		"name":       tftypes.NewValue(tftypes.String, name),
		"url":        tftypes.NewValue(tftypes.String, url),
		"events":     eventsVal,
		"jql_filter": jqlVal,
		"enabled":    tftypes.NewValue(tftypes.Bool, enabled),
	}
}

// ==================== WEBHOOK RESOURCE ====================

// TestJiraWebhookResourceMetadata verifies the resource type name.
func TestJiraWebhookResourceMetadata(t *testing.T) {
	t.Parallel()
	r := webhookresource.NewResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_jira_webhook" {
		t.Errorf("expected type name 'atlassian_jira_webhook', got %q", resp.TypeName)
	}
}

// TestJiraWebhookResourceSchema verifies the schema has required attributes.
func TestJiraWebhookResourceSchema(t *testing.T) {
	t.Parallel()
	r := webhookresource.NewResource()
	s := getResourceSchema(t, r)
	for _, attr := range []string{"id", "name", "url", "events", "jql_filter", "enabled"} {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestJiraWebhookResourceImportState verifies ImportState is implemented.
func TestJiraWebhookResourceImportState(t *testing.T) {
	t.Parallel()
	r := webhookresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Fatal("resource does not implement ImportState")
	}
}

// TestJiraWebhookResourceCRUDLifecycle tests full create-read-update-delete cycle.
func TestJiraWebhookResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues("", "My Webhook", "https://example.com/hook", []string{"jira:issue_created", "jira:issue_updated"}, "", true, true),
	)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "My Webhook" {
		t.Errorf("expected name 'My Webhook', got %q", name)
	}
	if url := getStringAttr(t, createResp.State, "url"); url != "https://example.com/hook" {
		t.Errorf("expected url 'https://example.com/hook', got %q", url)
	}
	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues(id, "My Webhook", "https://example.com/hook", []string{"jira:issue_created", "jira:issue_updated"}, "", true, false),
	)}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "My Webhook" {
		t.Errorf("Read: expected name 'My Webhook', got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues(id, "Updated Hook", "https://example.com/hook2", []string{"jira:issue_deleted"}, "project = TEST", false, false),
	)}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues(id, "My Webhook", "https://example.com/hook", []string{"jira:issue_created", "jira:issue_updated"}, "", true, false),
	)}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Hook" {
		t.Errorf("Update: expected name 'Updated Hook', got %q", name)
	}
	if jql := getStringAttr(t, updateResp.State, "jql_filter"); jql != "project = TEST" {
		t.Errorf("Update: expected jql_filter 'project = TEST', got %q", jql)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues(id, "Updated Hook", "https://example.com/hook2", []string{"jira:issue_deleted"}, "project = TEST", false, false),
	)}
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

// TestJiraWebhookResourceUpdateNotFound tests updating a nonexistent webhook.
func TestJiraWebhookResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("99999", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent webhook")
	}
}

// TestJiraWebhookResourceDeleteNotFound tests deleting an already-deleted webhook.
func TestJiraWebhookResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("99999", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent webhook should not error (idempotent)")
	}
}

// TestJiraWebhookResourceReadNotFound tests reading a nonexistent webhook removes state.
func TestJiraWebhookResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("99999", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent should not error: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed after 404")
	}
}

// TestJiraWebhookResourceCreateForbidden tests 403 on create.
func TestJiraWebhookResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWebhookForbiddenMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues("", "Forbidden", "https://x.com", []string{"e"}, "", true, true),
	)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestJiraWebhookResourceUpdateForbidden tests 403 on update.
func TestJiraWebhookResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWebhookForbiddenMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("1", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestJiraWebhookResourceDeleteForbidden tests 403 on delete.
func TestJiraWebhookResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testWebhookForbiddenMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("1", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestJiraWebhookResourceCreateServerError tests 500 on create.
func TestJiraWebhookResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testWebhookServerErrorMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues("", "ServerError", "https://x.com", []string{"e"}, "", true, true),
	)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraWebhookResourceReadServerError tests 500 on read.
func TestJiraWebhookResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testWebhookServerErrorMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("1", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error read")
	}
}

// TestJiraWebhookResourceDeleteServerError tests 500 on delete.
func TestJiraWebhookResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testWebhookServerErrorMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("1", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error delete")
	}
}

// TestJiraWebhookResourceUpdateServerError tests 500 on update.
func TestJiraWebhookResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testWebhookServerErrorMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("1", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error update")
	}
}

// TestJiraWebhookResourceCreateBadRequest tests 400 on create.
func TestJiraWebhookResourceCreateBadRequest(t *testing.T) {
	t.Parallel()
	_, client := testWebhookBadRequestMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues("", "Bad", "https://x.com", []string{"e"}, "", true, true),
	)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on create")
	}
}

// TestJiraWebhookResourceUpdateBadRequest tests 400 on update.
func TestJiraWebhookResourceUpdateBadRequest(t *testing.T) {
	t.Parallel()
	_, client := testWebhookBadRequestMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := webhookPlanValues("1", "X", "https://x.com", []string{"e"}, "", true, false)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error on update")
	}
}

// TestJiraWebhookResourceConfigureNil verifies nil provider data does not error.
func TestJiraWebhookResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := webhookresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraWebhookResourceConfigureBadType verifies wrong provider data type errors.
func TestJiraWebhookResourceConfigureBadType(t *testing.T) {
	t.Parallel()
	r := webhookresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "bad"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with wrong type")
	}
}

// TestJiraWebhookResourceCreateBadPlan tests Create with invalid plan.
func TestJiraWebhookResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil plan")
	}
}

// TestJiraWebhookResourceReadBadState tests Read with invalid state.
func TestJiraWebhookResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.ReadResponse{State: emptyState(ctx, s)}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil state")
	}
}

// TestJiraWebhookResourceUpdateBadPlan tests Update with invalid plan.
func TestJiraWebhookResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
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

// TestJiraWebhookResourceUpdateBadState tests Update with valid plan but nil state.
func TestJiraWebhookResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		webhookPlanValues("1", "Valid", "https://x.com", []string{"e"}, "", true, false),
	)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil state in update")
	}
}

// TestJiraWebhookResourceDeleteBadState tests Delete with invalid state.
func TestJiraWebhookResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil state")
	}
}

// TestJiraWebhookResourceImportStateExec tests actual ImportState execution.
func TestJiraWebhookResourceImportStateExec(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()
	r := webhookresource.NewResource()
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

// ==================== WEBHOOK DATA SOURCE ====================

// TestJiraWebhookDataSourceMetadata verifies the data source type name.
func TestJiraWebhookDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := webhookdatasource.NewDataSource()
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_jira_webhook" {
		t.Errorf("expected type name 'atlassian_jira_webhook', got %q", resp.TypeName)
	}
}

// TestJiraWebhookDataSourceSchema verifies the schema has required attributes.
func TestJiraWebhookDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := webhookdatasource.NewDataSource()
	s := getDatasourceSchema(t, ds)
	for _, attr := range []string{"id", "name", "url", "events", "jql_filter", "enabled"} {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestJiraWebhookDataSourceRead tests reading a webhook via data source.
func TestJiraWebhookDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()

	// Create via resource
	r := webhookresource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType,
		webhookPlanValues("", "DS Test Hook", "https://ds.example.com/hook", []string{"jira:issue_created"}, "project = DS", true, true),
	)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	// Read via data source
	ds := webhookdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, id),
		"name":       tftypes.NewValue(tftypes.String, nil),
		"url":        tftypes.NewValue(tftypes.String, nil),
		"events":     tftypes.NewValue(eventsListType, nil),
		"jql_filter": tftypes.NewValue(tftypes.String, nil),
		"enabled":    tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "DS Test Hook" {
		t.Errorf("expected name 'DS Test Hook', got %q", name)
	}
	if url := getStringAttr(t, dsResp.State, "url"); url != "https://ds.example.com/hook" {
		t.Errorf("expected url 'https://ds.example.com/hook', got %q", url)
	}
	if jql := getStringAttr(t, dsResp.State, "jql_filter"); jql != "project = DS" {
		t.Errorf("expected jql_filter 'project = DS', got %q", jql)
	}
}

// TestJiraWebhookDataSourceReadNotFound tests reading a nonexistent webhook.
func TestJiraWebhookDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()

	ds := webhookdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "99999"),
		"name":       tftypes.NewValue(tftypes.String, nil),
		"url":        tftypes.NewValue(tftypes.String, nil),
		"events":     tftypes.NewValue(eventsListType, nil),
		"jql_filter": tftypes.NewValue(tftypes.String, nil),
		"enabled":    tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent webhook")
	}
}

// TestJiraWebhookDataSourceReadServerError tests 500 on data source read.
func TestJiraWebhookDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testWebhookServerErrorMockServer(t)
	ctx := context.Background()

	ds := webhookdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"name":       tftypes.NewValue(tftypes.String, nil),
		"url":        tftypes.NewValue(tftypes.String, nil),
		"events":     tftypes.NewValue(eventsListType, nil),
		"jql_filter": tftypes.NewValue(tftypes.String, nil),
		"enabled":    tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestJiraWebhookDataSourceConfigureNil verifies nil provider data does not error.
func TestJiraWebhookDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := webhookdatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestJiraWebhookDataSourceConfigureBadType verifies wrong provider data type errors.
func TestJiraWebhookDataSourceConfigureBadType(t *testing.T) {
	t.Parallel()
	ds := webhookdatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "bad"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with wrong type")
	}
}

// TestJiraWebhookDataSourceReadBadConfig tests Read with invalid config.
func TestJiraWebhookDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testWebhookMockServer(t)
	ctx := context.Background()

	ds := webhookdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dss.Type().TerraformType(ctx), nil)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil config")
	}
}
