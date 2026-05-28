// Package unit contains tests to close Config.Get/State.Get HasError guard coverage gaps.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	bbrepodatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/bitbucket/repository"
	groupdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/group"
	roledatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/role"
	userdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/user"
	automationds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/automation"
	issuetypeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/issue_type"
	bbrestrictionres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/branch_restriction"
	bbdeploymentres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/deployment"
	bbpipelineres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/pipeline"
	bbreporesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/repository"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	issuetyperes "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/issue_type"
	screenres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/screen"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// cgClient creates a test client.
func cgClient(t *testing.T) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	c, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)
	return c
}

// ==================== Data Source Config.Get error paths ====================

func TestGroupDSConfigGetError(t *testing.T) {
	t.Parallel()
	ds := groupdatasource.NewDataSource()
	configureDatasource(t, ds, cgClient(t))
	dss := getDatasourceSchema(t, ds)
	resp := &datasource.ReadResponse{State: emptyDSState(context.Background(), dss)}
	ds.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRoleDSConfigGetError(t *testing.T) {
	t.Parallel()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, cgClient(t))
	dss := getDatasourceSchema(t, ds)
	resp := &datasource.ReadResponse{State: emptyDSState(context.Background(), dss)}
	ds.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestUserDSConfigGetError(t *testing.T) {
	t.Parallel()
	ds := userdatasource.NewDataSource()
	configureDatasource(t, ds, cgClient(t))
	dss := getDatasourceSchema(t, ds)
	resp := &datasource.ReadResponse{State: emptyDSState(context.Background(), dss)}
	ds.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestAutomationDSConfigGetError(t *testing.T) {
	t.Parallel()
	ds := automationds.NewDataSource()
	configureDatasource(t, ds, cgClient(t))
	dss := getDatasourceSchema(t, ds)
	resp := &datasource.ReadResponse{State: emptyDSState(context.Background(), dss)}
	ds.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestIssueTypeDSConfigGetError(t *testing.T) {
	t.Parallel()
	ds := issuetypeds.NewDataSource()
	configureDatasource(t, ds, cgClient(t))
	dss := getDatasourceSchema(t, ds)
	resp := &datasource.ReadResponse{State: emptyDSState(context.Background(), dss)}
	ds.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestIssueTypeSchemeDSConfigGetError(t *testing.T) {
	t.Parallel()
	ds := issuetypeds.NewSchemeDataSource()
	configureDatasource(t, ds, cgClient(t))
	dss := getDatasourceSchema(t, ds)
	resp := &datasource.ReadResponse{State: emptyDSState(context.Background(), dss)}
	ds.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestBBRepoDSConfigGetError(t *testing.T) {
	t.Parallel()
	ds := bbrepodatasource.NewDataSource()
	configureDatasource(t, ds, cgClient(t))
	dss := getDatasourceSchema(t, ds)
	resp := &datasource.ReadResponse{State: emptyDSState(context.Background(), dss)}
	ds.Read(context.Background(), datasource.ReadRequest{
		Config: tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== Resource State.Get error paths for Read ====================

func TestMembershipResourceReadStateGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, cgClient(t))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== BB Repo Resource: Create() 409 conflict ====================

func TestBBRepoResourceCreate409Conflict(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "Conflict"},
		})
	}))
	defer ts.Close()
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	client, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)

	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoValues("", "ws", "repo", "Repo", "", "", "", "", "", "", "", true, false, false))}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 409 conflict error")
	}
	found := false
	for _, e := range resp.Diagnostics.Errors() {
		if e.Summary() == "Duplicate repository" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'Duplicate repository' error")
	}
}

// ==================== Issue Type Resource: Create() 409 conflict ====================

func TestIssueTypeResourceCreate409Conflict(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Conflict"},
		})
	}))
	defer ts.Close()
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	client, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)

	ctx := context.Background()
	r := issuetyperes.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Test"),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 409 conflict error")
	}
}

// ==================== Screen Resource: Create() 409 conflict ====================

func TestScreenResourceCreate409Conflict(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Conflict"},
		})
	}))
	defer ts.Close()
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	client, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)

	ctx := context.Background()
	r := screenres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Screen"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 409 conflict error")
	}
}

// ==================== Screen Scheme Resource: Create() 409 conflict ====================

func TestScreenSchemeResourceCreate409Conflict(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Conflict"},
		})
	}))
	defer ts.Close()
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	client, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)

	ctx := context.Background()
	r := screenres.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Scheme"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 409 conflict error")
	}
}

// ==================== BB Update generic error paths ====================

func TestBBBranchRestrictionUpdateGenericError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"Error"}}`))
	}))
	defer ts.Close()
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	client, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)

	ctx := context.Background()
	r := bbrestrictionres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "develop"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestBBDeploymentUpdateGenericError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"Error"}}`))
	}))
	defer ts.Close()
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	client, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)

	ctx := context.Background()
	r := bbdeploymentres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "env-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "Production"),
		"environment_type": tftypes.NewValue(tftypes.String, "Production"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "env-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "Staging"),
		"environment_type": tftypes.NewValue(tftypes.String, "Staging"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestBBPipelineUpdateGenericError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"Error"}}`))
	}))
	defer ts.Close()
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	client, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)

	ctx := context.Background()
	r := bbpipelineres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, false),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestBBRepoPermissionUpdateGenericError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"Error"}}`))
	}))
	defer ts.Close()
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	client, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)

	ctx := context.Background()
	r := bbreporesource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"permission":     tftypes.NewValue(tftypes.String, "read"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}
