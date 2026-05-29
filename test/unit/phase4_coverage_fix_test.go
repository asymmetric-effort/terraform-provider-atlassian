// Package unit contains tests to close Phase 4 (Bitbucket) coverage gaps to >= 98%.
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
	bbrestrictionres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/branch_restriction"
	bbdeploymentres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/deployment"
	bbpipelineres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/pipeline"
	bbreporesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/repository"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// p4Client creates a client pointing at the given test server.
func p4Client(t *testing.T, handler http.HandlerFunc) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	c, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)
	return c
}

// ==================== BB Repo DS: extractCloneURL and extractHTMLURL nil links ====================

func TestBBRepoDSReadNilLinks(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return repository with nil links - exercises extractCloneURL and extractHTMLURL nil cases
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":        "{repo-1}",
			"slug":        "test-repo",
			"name":        "Test Repo",
			"full_name":   "ws/test-repo",
			"description": "A test repo",
			"is_private":  true,
			"fork_policy": "allow_forks",
			"language":    "go",
			"has_issues":  true,
			"has_wiki":    false,
			// No "links" field - nil links
		})
	})
	ctx := context.Background()
	ds := bbrepodatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"workspace":      tftypes.NewValue(tftypes.String, "ws"),
		"slug":           tftypes.NewValue(tftypes.String, "test-repo"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, nil),
		"is_private":     tftypes.NewValue(tftypes.Bool, nil),
		"fork_policy":    tftypes.NewValue(tftypes.String, nil),
		"language":       tftypes.NewValue(tftypes.String, nil),
		"default_branch": tftypes.NewValue(tftypes.String, nil),
		"has_issues":     tftypes.NewValue(tftypes.Bool, nil),
		"has_wiki":       tftypes.NewValue(tftypes.Bool, nil),
		"clone_ssh":      tftypes.NewValue(tftypes.String, nil),
		"clone_https":    tftypes.NewValue(tftypes.String, nil),
		"url":            tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== BB Repo DS: extractCloneURL no matching name ====================

func TestBBRepoDSReadLinksNoMatch(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return repository with links but no ssh/https clone
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":        "{repo-1}",
			"slug":        "test-repo",
			"name":        "Test Repo",
			"full_name":   "ws/test-repo",
			"description": "",
			"is_private":  false,
			"fork_policy": "allow_forks",
			"language":    "",
			"has_issues":  false,
			"has_wiki":    false,
			"links": map[string]interface{}{
				"clone": []interface{}{
					map[string]interface{}{"name": "other", "href": "https://other.url"},
				},
				"html": map[string]interface{}{"href": "https://bb.org/ws/test-repo"},
			},
		})
	})
	ctx := context.Background()
	ds := bbrepodatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"workspace":      tftypes.NewValue(tftypes.String, "ws"),
		"slug":           tftypes.NewValue(tftypes.String, "test-repo"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, nil),
		"is_private":     tftypes.NewValue(tftypes.Bool, nil),
		"fork_policy":    tftypes.NewValue(tftypes.String, nil),
		"language":       tftypes.NewValue(tftypes.String, nil),
		"default_branch": tftypes.NewValue(tftypes.String, nil),
		"has_issues":     tftypes.NewValue(tftypes.Bool, nil),
		"has_wiki":       tftypes.NewValue(tftypes.Bool, nil),
		"clone_ssh":      tftypes.NewValue(tftypes.String, nil),
		"clone_https":    tftypes.NewValue(tftypes.String, nil),
		"url":            tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== BB Repo DS: Read() generic error ====================

func TestBBRepoDSReadGenericError(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	ds := bbrepodatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"workspace":      tftypes.NewValue(tftypes.String, "ws"),
		"slug":           tftypes.NewValue(tftypes.String, "test-repo"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"description":    tftypes.NewValue(tftypes.String, nil),
		"is_private":     tftypes.NewValue(tftypes.Bool, nil),
		"fork_policy":    tftypes.NewValue(tftypes.String, nil),
		"language":       tftypes.NewValue(tftypes.String, nil),
		"default_branch": tftypes.NewValue(tftypes.String, nil),
		"has_issues":     tftypes.NewValue(tftypes.Bool, nil),
		"has_wiki":       tftypes.NewValue(tftypes.Bool, nil),
		"clone_ssh":      tftypes.NewValue(tftypes.String, nil),
		"clone_https":    tftypes.NewValue(tftypes.String, nil),
		"url":            tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== BB Repo Resource: extractCloneURL/extractHTMLURL nil links ====================

func TestBBRepoResourceCreateNilLinks(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"uuid":        "{repo-new}",
				"slug":        "new-repo",
				"name":        "New Repo",
				"full_name":   "ws/new-repo",
				"description": "",
				"is_private":  true,
				"fork_policy": "allow_forks",
				"language":    "",
				"has_issues":  false,
				"has_wiki":    false,
				// No links - nil
			})
			return
		}
	})
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoValues("", "ws", "new-repo", "New Repo", "", "allow_forks", "", "", "", "", "", true, false, false))}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}
}

// ==================== BB Repo Resource: mapRepoToModel with nil MainBranch ====================

func TestBBRepoResourceCreateNoMainBranch(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"uuid":        "{repo-nb}",
				"slug":        "nb-repo",
				"name":        "NB Repo",
				"full_name":   "ws/nb-repo",
				"description": "",
				"is_private":  false,
				"fork_policy": "allow_forks",
				"language":    "",
				"has_issues":  false,
				"has_wiki":    false,
				// No mainbranch
				"links": map[string]interface{}{
					"clone": []interface{}{},
					"html":  nil,
				},
			})
			return
		}
	})
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoValues("", "ws", "nb-repo", "NB Repo", "", "", "", "", "", "", "", nil, nil, nil))}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}
}

// ==================== BB Repo Resource: Create() 403 forbidden ====================

func TestBBRepoResourceCreate403(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "Forbidden"},
		})
	})
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
		t.Fatal("expected 403 error")
	}
}

// ==================== BB Repo Resource: Create() generic error ====================

func TestBBRepoResourceCreateGenericError(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"Error"}}`))
	})
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
		t.Fatal("expected generic error")
	}
}

// ==================== BB Repo Resource: Update() 404 ====================

func TestBBRepoResourceUpdate404(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "Not found"},
		})
	})
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("{id}", "ws", "repo", "Repo", "", "allow_forks", "", "", "", "", "", true, false, false))}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("{id}", "ws", "repo", "Updated", "", "allow_forks", "", "", "", "", "", true, false, false))}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 404 error")
	}
}

// ==================== BB Repo Resource: Update() 403 ====================

func TestBBRepoResourceUpdate403(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "Forbidden"},
		})
	})
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("{id}", "ws", "repo", "Repo", "", "allow_forks", "", "", "", "", "", true, false, false))}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("{id}", "ws", "repo", "Updated", "", "allow_forks", "", "", "", "", "", true, false, false))}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

// ==================== BB Repo Resource: Update() generic error ====================

func TestBBRepoResourceUpdateGenericError(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"error":{"message":"Error"}}`))
	})
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("{id}", "ws", "repo", "Repo", "", "allow_forks", "", "", "", "", "", true, false, false))}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("{id}", "ws", "repo", "Updated", "", "allow_forks", "", "", "", "", "", true, false, false))}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== BB Branch Restriction: Update() 403 ====================

func TestBBBranchRestrictionUpdate403(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "Forbidden"},
		})
	})
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
		"users":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"groups":     tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "develop"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
		"users":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"groups":     tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

// ==================== BB Deployment: Update() 403 ====================

func TestBBDeploymentUpdate403(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "Forbidden"},
		})
	})
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
		"lock":             tftypes.NewValue(tftypes.Bool, false),
		"restrictions":     tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "pattern": tftypes.String}}}, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "env-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "Staging"),
		"environment_type": tftypes.NewValue(tftypes.String, "Staging"),
		"lock":             tftypes.NewValue(tftypes.Bool, false),
		"restrictions":     tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "pattern": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

// ==================== BB Pipeline: Update() 403 ====================

func TestBBPipelineUpdate403(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "Forbidden"},
		})
	})
	ctx := context.Background()
	r := bbpipelineres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
		"variables":  tftypes.NewValue(pipelineVariableListType, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, false),
		"variables":  tftypes.NewValue(pipelineVariableListType, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

// ==================== BB Repo Permission: Update() 403 ====================

func TestBBRepoPermissionUpdate403(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "Forbidden"},
		})
	})
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
		t.Fatal("expected 403 error")
	}
}

// ==================== BB Repo Resource: Create() with links having no match ====================

func TestBBRepoResourceCreateLinksNoMatchingClone(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"uuid":        "{repo-lnm}",
				"slug":        "lnm-repo",
				"name":        "LNM Repo",
				"full_name":   "ws/lnm-repo",
				"description": "",
				"is_private":  false,
				"fork_policy": "allow_forks",
				"language":    "",
				"has_issues":  false,
				"has_wiki":    false,
				"links": map[string]interface{}{
					"clone": []interface{}{
						map[string]interface{}{"name": "other", "href": "https://other.url"},
					},
					"html": nil,
				},
			})
			return
		}
	})
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoValues("", "ws", "lnm-repo", "LNM Repo", "", "", "", "", "", "", "", nil, nil, nil))}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}
}

// ==================== BB Repo Resource: Update() with all optional fields set ====================

func TestBBRepoResourceUpdateWithAllOptionalFields(t *testing.T) {
	t.Parallel()
	client := p4Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"uuid":        "{id}",
				"slug":        "repo",
				"name":        "Updated",
				"full_name":   "ws/repo",
				"description": "Updated desc",
				"is_private":  true,
				"fork_policy": "no_forks",
				"language":    "python",
				"has_issues":  true,
				"has_wiki":    true,
				"mainbranch":  map[string]interface{}{"name": "develop"},
				"links": map[string]interface{}{
					"clone": []interface{}{
						map[string]interface{}{"name": "ssh", "href": "git@bb.org:ws/repo.git"},
						map[string]interface{}{"name": "https", "href": "https://bb.org/ws/repo.git"},
					},
					"html": map[string]interface{}{"href": "https://bb.org/ws/repo"},
				},
			})
			return
		}
	})
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("{id}", "ws", "repo", "Repo", "desc", "allow_forks", "go", "main", "git@bb.org:ws/repo.git", "https://bb.org/ws/repo.git", "https://bb.org/ws/repo", true, false, false))}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("{id}", "ws", "repo", "Updated", "Updated desc", "no_forks", "python", "develop", "git@bb.org:ws/repo.git", "https://bb.org/ws/repo.git", "https://bb.org/ws/repo", true, true, true))}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics.Errors())
	}
}
