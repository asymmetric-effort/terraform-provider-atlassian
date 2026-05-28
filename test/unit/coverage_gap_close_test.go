// Package unit contains targeted tests to close the coverage gap to >= 98%.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	automationds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/automation"
	issuetypeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/issue_type"
	boardresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/board"
	customfieldresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/custom_field"
	dashboardresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/dashboard"
	issuetyperesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/issue_type"
	notificationschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/notification_scheme"
	permissionschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/permission_scheme"
	priorityresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/priority"
	screenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/screen"
	securityschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/security_scheme"
	workflowresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/workflow"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// ==================== CLIENT POST/PUT/DELETE NIL RESULT ====================

// TestClientPostNilResultGap covers the Post method with nil result returning 200.
func TestClientPostNilResultGap(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	config := atlassian.DefaultConfig()
	config.BaseURL = server.URL
	config.MaxRetries = 0
	c, _ := atlassian.NewClient(config, &mockAuth{})

	err := c.Post(context.Background(), "/test", nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClientPutNilResultGap covers the Put method with nil result returning 200.
func TestClientPutNilResultGap(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	config := atlassian.DefaultConfig()
	config.BaseURL = server.URL
	config.MaxRetries = 0
	c, _ := atlassian.NewClient(config, &mockAuth{})

	err := c.Put(context.Background(), "/test", nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClientPostErrorPath covers the Post method error translation.
func TestClientPostErrorPath(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"bad request"},
		})
	}))
	defer server.Close()

	config := atlassian.DefaultConfig()
	config.BaseURL = server.URL
	config.MaxRetries = 0
	c, _ := atlassian.NewClient(config, &mockAuth{})

	err := c.Post(context.Background(), "/test", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestClientPutErrorPath covers the Put method error translation.
func TestClientPutErrorPath(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"bad request"},
		})
	}))
	defer server.Close()

	config := atlassian.DefaultConfig()
	config.BaseURL = server.URL
	config.MaxRetries = 0
	c, _ := atlassian.NewClient(config, &mockAuth{})

	err := c.Put(context.Background(), "/test", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestClientDeleteErrorPath covers the Delete method error translation.
func TestClientDeleteErrorPath(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"bad request"},
		})
	}))
	defer server.Close()

	config := atlassian.DefaultConfig()
	config.BaseURL = server.URL
	config.MaxRetries = 0
	c, _ := atlassian.NewClient(config, &mockAuth{})

	err := c.Delete(context.Background(), "/test")
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestClientDoRetryOn503 covers the 503 retry path in Do.
func TestClientDoRetryOn503(t *testing.T) {
	t.Parallel()
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	config := atlassian.DefaultConfig()
	config.BaseURL = server.URL
	config.MaxRetries = 3
	config.RetryWaitMin = 10 * time.Millisecond
	config.RetryWaitMax = 20 * time.Millisecond
	c, _ := atlassian.NewClient(config, &mockAuth{})

	var result map[string]string
	err := c.Get(context.Background(), "/test", &result)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

// ==================== DATASOURCE READ: JSON DECODE ERROR PATHS ====================

// invalidJSONClient creates a client whose server returns 200 OK with invalid JSON.
func invalidJSONClient(t *testing.T) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte("not valid json{{{"))
	}))
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	c, _ := atlassian.NewClient(atlassian.Config{
		BaseURL: ts.URL, RequestTimeout: 5 * time.Second,
		MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second,
	}, auth)
	return c
}

// TestAutomationDSReadInvalidJSON covers JSON decode error in automation DS Read.
func TestAutomationDSReadInvalidJSON(t *testing.T) {
	t.Parallel()
	cl := invalidJSONClient(t)
	ctx := context.Background()
	ds := automationds.NewDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "rule-1"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"state":          tftypes.NewValue(tftypes.String, nil),
		"trigger_type":   tftypes.NewValue(tftypes.String, nil),
		"trigger_config": tftypes.NewValue(tftypes.String, nil),
		"conditions":     tftypes.NewValue(tftypes.String, nil),
		"actions":        tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error from invalid JSON")
	}
}

// TestIssueTypeDSReadInvalidJSON covers JSON decode error in issue type DS Read.
func TestIssueTypeDSReadInvalidJSON(t *testing.T) {
	t.Parallel()
	cl := invalidJSONClient(t)
	ctx := context.Background()
	ds := issuetypeds.NewDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "it-1"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, nil),
		"subtask":         tftypes.NewValue(tftypes.Bool, nil),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error from invalid JSON")
	}
}

// TestIssueTypeSchemeDSReadInvalidJSON covers JSON decode error in issue type scheme DS Read.
func TestIssueTypeSchemeDSReadInvalidJSON(t *testing.T) {
	t.Parallel()
	cl := invalidJSONClient(t)
	ctx := context.Background()
	ds := issuetypeds.NewSchemeDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "its-1"),
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error from invalid JSON")
	}
}

// ==================== RESOURCE UPDATE: 400 BAD REQUEST ====================

// badRequestClient creates a client returning 400 with a proper API error body.
func badRequestClient(t *testing.T) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"bad request"},
			"errors":        map[string]string{},
		})
	}))
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	c, _ := atlassian.NewClient(atlassian.Config{
		BaseURL: ts.URL, RequestTimeout: 5 * time.Second,
		MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second,
	}, auth)
	return c
}

// --- Board Update 400 ---
func TestBoardUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := boardresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, "b-1"),
		"name":     tftypes.NewValue(tftypes.String, "Board"),
		"type":     tftypes.NewValue(tftypes.String, "scrum"),
		"space_id": tftypes.NewValue(tftypes.String, "sp-1"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Custom Field Create 400 ---
func TestCustomFieldCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := customfieldresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "CF"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "text"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Custom Field Update 400 ---
func TestCustomFieldUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := customfieldresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "cf-1"),
		"name":        tftypes.NewValue(tftypes.String, "CF"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "text"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Dashboard Create 400 ---
func TestDashboardCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "D"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Dashboard Update 400 ---
func TestDashboardUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := dashboardresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "d-1"),
		"name":        tftypes.NewValue(tftypes.String, "D"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Filter Create 400 ---
func TestFilterCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "F"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"jql":         tftypes.NewValue(tftypes.String, "project = TEST"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Filter Update 400 ---
func TestFilterUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := dashboardresource.NewFilterResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "f-1"),
		"name":        tftypes.NewValue(tftypes.String, "F"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"jql":         tftypes.NewValue(tftypes.String, "project = TEST"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Issue Type Create 400 ---
func TestIssueTypeCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "IT"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Issue Type Update 400 ---
func TestIssueTypeUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := issuetyperesource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "it-1"),
		"name":            tftypes.NewValue(tftypes.String, "IT"),
		"description":     tftypes.NewValue(tftypes.String, ""),
		"icon_url":        tftypes.NewValue(tftypes.String, ""),
		"subtask":         tftypes.NewValue(tftypes.Bool, false),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, 0),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Issue Type Scheme Create 400 ---
func TestIssueTypeSchemeCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                  tftypes.NewValue(tftypes.String, "ITS"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Issue Type Scheme Update 400 ---
func TestIssueTypeSchemeUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := issuetyperesource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "its-1"),
		"name":                  tftypes.NewValue(tftypes.String, "ITS"),
		"description":           tftypes.NewValue(tftypes.String, ""),
		"issue_type_ids":        tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "it-1")}),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, "it-1"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Notification Scheme Create 400 ---
func TestNotificationSchemeCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Notification Scheme Update 400 ---
func TestNotificationSchemeUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := notificationschemers.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ns-1"),
		"name":        tftypes.NewValue(tftypes.String, "NS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Permission Scheme Create 400 ---
func TestPermissionSchemeCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Permission Scheme Update 400 ---
func TestPermissionSchemeUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := permissionschemers.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ps-1"),
		"name":        tftypes.NewValue(tftypes.String, "PS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Security Scheme Create 400 ---
func TestSecuritySchemeCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "SS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Security Scheme Update 400 ---
func TestSecuritySchemeUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := securityschemers.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ss-1"),
		"name":        tftypes.NewValue(tftypes.String, "SS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Priority Create 400 ---
func TestPriorityCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := priorityresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "P"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"icon_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Priority Update 400 ---
func TestPriorityUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := priorityresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "p-1"),
		"name":        tftypes.NewValue(tftypes.String, "P"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"icon_url":    tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Priority Scheme Create 400 ---
func TestPrioritySchemeCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := priorityresource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "PSR"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Priority Scheme Update 400 ---
func TestPrioritySchemeUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := priorityresource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "psr-1"),
		"name":        tftypes.NewValue(tftypes.String, "PSR"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Screen Create 400 ---
func TestScreenCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "S"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Screen Create 403 ---
func TestScreenCreateForbidden(t *testing.T) {
	t.Parallel()
	cl := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "S"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected forbidden error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected permission denied message")
	}
}

// --- Screen Update 400 ---
func TestScreenUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := screenresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "scr-1"),
		"name":        tftypes.NewValue(tftypes.String, "S"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Screen Scheme Create 400 ---
func TestScreenSchemeCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "SS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Screen Scheme Create 403 ---
func TestScreenSchemeCreateForbidden(t *testing.T) {
	t.Parallel()
	cl := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "SS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected forbidden error")
	}
}

// --- Screen Scheme Update 400 ---
func TestScreenSchemeUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := screenresource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ss-1"),
		"name":        tftypes.NewValue(tftypes.String, "SS"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Tab Field Create 400 ---
func TestTabFieldCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "scr-1"),
		"tab_id":    tftypes.NewValue(tftypes.String, "tab-1"),
		"field_id":  tftypes.NewValue(tftypes.String, "fld-1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Tab Field Create 403 ---
func TestTabFieldCreateForbidden(t *testing.T) {
	t.Parallel()
	cl := errorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "scr-1"),
		"tab_id":    tftypes.NewValue(tftypes.String, "tab-1"),
		"field_id":  tftypes.NewValue(tftypes.String, "fld-1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected forbidden error")
	}
}

// --- Tab Field Create 404 ---
func TestTabFieldCreateNotFound(t *testing.T) {
	t.Parallel()
	cl := errorClient(t, 404, "Not Found")
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"screen_id": tftypes.NewValue(tftypes.String, "scr-bad"),
		"tab_id":    tftypes.NewValue(tftypes.String, "tab-bad"),
		"field_id":  tftypes.NewValue(tftypes.String, "fld-1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not found error")
	}
}

// --- Tab Field Read: JSON decode error (invalid JSON for fields list) ---
func TestTabFieldReadInvalidJSON(t *testing.T) {
	t.Parallel()
	cl := invalidJSONClient(t)
	ctx := context.Background()
	r := screenresource.NewTabFieldResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "scr-1/tab-1/fld-1"),
		"screen_id": tftypes.NewValue(tftypes.String, "scr-1"),
		"tab_id":    tftypes.NewValue(tftypes.String, "tab-1"),
		"field_id":  tftypes.NewValue(tftypes.String, "fld-1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error from invalid JSON in tab field Read")
	}
}

// --- Workflow Scheme Create 400 ---
func TestWorkflowSchemeCreateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                tftypes.NewValue(tftypes.String, "WS"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// --- Workflow Scheme Update 400 ---
func TestWorkflowSchemeUpdateBadRequest(t *testing.T) {
	t.Parallel()
	cl := badRequestClient(t)
	ctx := context.Background()
	r := workflowresource.NewSchemeResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "ws-1"),
		"name":                tftypes.NewValue(tftypes.String, "WS"),
		"description":         tftypes.NewValue(tftypes.String, ""),
		"default_workflow_id": tftypes.NewValue(tftypes.String, "wf-1"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected bad request error")
	}
}

// ==================== IMPORT STATE ====================

// TestNotificationSchemeImportState covers ImportState for notification scheme.
func TestNotificationSchemeImportState(t *testing.T) {
	t.Parallel()
	r := notificationschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()

	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "ns-import-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestPermissionSchemeImportState covers ImportState for permission scheme.
func TestPermissionSchemeImportState(t *testing.T) {
	t.Parallel()
	r := permissionschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()

	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "ps-import-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestSecuritySchemeImportState covers ImportState for security scheme.
func TestSecuritySchemeImportState(t *testing.T) {
	t.Parallel()
	r := securityschemers.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()

	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "ss-import-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== ROLE RESOURCE READ NON-404 ERROR ====================

// ==================== AUTOMATION DATASOURCE READ GENERIC ERROR ====================

// TestAutomationDSReadGenericError covers the default error path in automation datasource Read.
func TestAutomationDSReadGenericError(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 500, "Internal server error")
	ctx := context.Background()

	ds := automationds.NewDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "rule-x"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"state":          tftypes.NewValue(tftypes.String, nil),
		"trigger_type":   tftypes.NewValue(tftypes.String, nil),
		"trigger_config": tftypes.NewValue(tftypes.String, nil),
		"conditions":     tftypes.NewValue(tftypes.String, nil),
		"actions":        tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestAutomationDSReadNotFound covers 404 path in automation datasource Read.
func TestAutomationDSReadNotFound(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not found")
	ctx := context.Background()

	ds := automationds.NewDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "rule-missing"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"state":          tftypes.NewValue(tftypes.String, nil),
		"trigger_type":   tftypes.NewValue(tftypes.String, nil),
		"trigger_config": tftypes.NewValue(tftypes.String, nil),
		"conditions":     tftypes.NewValue(tftypes.String, nil),
		"actions":        tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not found error")
	}
}

// TestIssueTypeDSReadNotFound covers 404 path in issue type datasource Read.
func TestIssueTypeDSReadNotFound(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not found")
	ctx := context.Background()

	ds := issuetypeds.NewDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "it-missing"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, nil),
		"subtask":         tftypes.NewValue(tftypes.Bool, nil),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not found error")
	}
}

// TestIssueTypeSchemeDSReadNotFound covers 404 path in issue type scheme DS Read.
func TestIssueTypeSchemeDSReadNotFound(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not found")
	ctx := context.Background()

	ds := issuetypeds.NewSchemeDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "its-missing"),
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not found error")
	}
}

// TestIssueTypeSchemeDSReadGenericError covers non-404 error in issue type scheme DS Read.
func TestIssueTypeSchemeDSReadGenericError(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 500, "Internal Server Error")
	ctx := context.Background()

	ds := issuetypeds.NewSchemeDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "its-err"),
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// TestIssueTypeDSReadGenericError covers non-404 error in issue type DS Read.
func TestIssueTypeDSReadGenericError(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 500, "Internal Server Error")
	ctx := context.Background()

	ds := issuetypeds.NewDataSource()
	configureDatasource(t, ds, cl)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "it-err"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, nil),
		"subtask":         tftypes.NewValue(tftypes.Bool, nil),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}
