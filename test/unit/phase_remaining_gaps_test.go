// Package unit contains tests to close remaining per-function coverage gaps.
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
	roledatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/role"
	automationds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/automation"
	issuetypeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/issue_type"
	bbrestrictionres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/branch_restriction"
	bbdeploymentres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/deployment"
	bbpipelineres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/pipeline"
	bbreporesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/repository"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	issuetyperes "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/issue_type"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// rgClient creates a client pointing at the given test server.
func rgClient(t *testing.T, handler http.HandlerFunc) *atlassian.Client {
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

// ==================== BB Repo DS: Read with mainbranch set ====================

func TestBBRepoDSReadWithMainBranch(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"uuid":        "{repo-mb}",
			"slug":        "mb-repo",
			"name":        "MB Repo",
			"full_name":   "ws/mb-repo",
			"description": "",
			"is_private":  true,
			"fork_policy": "allow_forks",
			"language":    "go",
			"has_issues":  true,
			"has_wiki":    false,
			"mainbranch":  map[string]interface{}{"name": "develop"},
			"links": map[string]interface{}{
				"clone": []interface{}{
					map[string]interface{}{"name": "ssh", "href": "git@bb.org:ws/mb-repo.git"},
					map[string]interface{}{"name": "https", "href": "https://bb.org/ws/mb-repo.git"},
				},
				"html": map[string]interface{}{"href": "https://bb.org/ws/mb-repo"},
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
		"slug":           tftypes.NewValue(tftypes.String, "mb-repo"),
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

// ==================== findRoleByName: bad JSON item in list ====================

func TestRoleDSFindRoleByNameBadJSONItem(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return array with one badly formatted JSON item (note: raw JSON array)
		// The first element is invalid (missing closing brace), but since it's inside a valid array
		// we need to use json.RawMessage style
		w.Write([]byte(`[{"bad":}]`))
	})
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, ""),
		"name":        tftypes.NewValue(tftypes.String, "SomeRole"),
		"description": tftypes.NewValue(tftypes.String, nil),
		"scope":       tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	// This will fail at the JSON decode level since the whole array is invalid
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// findRoleByName with valid JSON but contains items that unmarshal fails for
func TestRoleDSFindRoleByNameMixedItems(t *testing.T) {
	t.Parallel()
	callCount := 0
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// Return a valid JSON array where one item is a number (unmarshal will fail for apiRoleResponse)
		// and one valid item that does not match the name
		w.Write([]byte(`[42, {"id":1,"name":"Other","description":"d","scope":"org"}]`))
	})
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, ""),
		"name":        tftypes.NewValue(tftypes.String, "NonExistent"),
		"description": tftypes.NewValue(tftypes.String, nil),
		"scope":       tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error after skipping bad items")
	}
}

// ==================== Automation DS: Read with real TriggerConfig and Conditions ====================

func TestAutomationDSReadWithTriggerConfigAndConditions(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return non-null, non-empty triggerConfig and conditions
		w.Write([]byte(`{"id":"rule-1","name":"Test","state":"ENABLED","triggerType":"t","triggerConfig":{"key":"val"},"conditions":[{"type":"c"}],"actions":[{"type":"a"}]}`))
	})
	ctx := context.Background()
	ds := automationds.NewDataSource()
	configureDatasource(t, ds, client)
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
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Issue Type DS: Read with empty ID ====================

func TestIssueTypeDSReadEmptyID(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	ctx := context.Background()
	ds := issuetypeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, ""),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, nil),
		"subtask":         tftypes.NewValue(tftypes.Bool, nil),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for empty ID")
	}
}

// ==================== Issue Type Scheme DS: Read with empty ID ====================

func TestIssueTypeSchemeDSReadEmptyID(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	ctx := context.Background()
	ds := issuetypeds.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, ""),
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for empty ID")
	}
}

// ==================== BB Update with invalid repo format (empty repoPath) ====================

func TestBBBranchRestrictionUpdateInvalidRepo(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	ctx := context.Background()
	r := bbrestrictionres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
		"users":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"groups":     tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"pattern":    tftypes.NewValue(tftypes.String, "develop"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
		"users":      tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"groups":     tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid repo format error")
	}
}

func TestBBDeploymentUpdateInvalidRepo(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	ctx := context.Background()
	r := bbdeploymentres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "env-1"),
		"repository":       tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"name":             tftypes.NewValue(tftypes.String, "Prod"),
		"environment_type": tftypes.NewValue(tftypes.String, "Production"),
		"lock":             tftypes.NewValue(tftypes.Bool, false),
		"restrictions":     tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "pattern": tftypes.String}}}, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "env-1"),
		"repository":       tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"name":             tftypes.NewValue(tftypes.String, "Staging"),
		"environment_type": tftypes.NewValue(tftypes.String, "Staging"),
		"lock":             tftypes.NewValue(tftypes.Bool, false),
		"restrictions":     tftypes.NewValue(tftypes.List{ElementType: tftypes.Object{AttributeTypes: map[string]tftypes.Type{"type": tftypes.String, "pattern": tftypes.String}}}, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid repo format error")
	}
}

func TestBBPipelineUpdateInvalidRepo(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	ctx := context.Background()
	r := bbpipelineres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"repository": tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
		"variables":  tftypes.NewValue(pipelineVariableListType, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"repository": tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"enabled":    tftypes.NewValue(tftypes.Bool, false),
		"variables":  tftypes.NewValue(pipelineVariableListType, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid repo format error")
	}
}

func TestBBRepoPermissionUpdateInvalidRepo(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	ctx := context.Background()
	r := bbreporesource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"permission":     tftypes.NewValue(tftypes.String, "read"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "invalid-no-slash"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid repo format error")
	}
}

// ==================== BB Repo Resource: Create with DefaultBranch set ====================

func TestBBRepoResourceCreateWithDefaultBranch(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "PUT" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"uuid":        "{repo-db}",
				"slug":        "db-repo",
				"name":        "DB Repo",
				"full_name":   "ws/db-repo",
				"description": "desc",
				"is_private":  true,
				"fork_policy": "allow_forks",
				"language":    "go",
				"has_issues":  true,
				"has_wiki":    true,
				"mainbranch":  map[string]interface{}{"name": "develop"},
				"links": map[string]interface{}{
					"clone": []interface{}{
						map[string]interface{}{"name": "ssh", "href": "git@bb.org:ws/db-repo.git"},
						map[string]interface{}{"name": "https", "href": "https://bb.org/ws/db-repo.git"},
					},
					"html": map[string]interface{}{"href": "https://bb.org/ws/db-repo"},
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
		bbRepoSetValues("", "ws", "db-repo", "DB Repo", "desc", "allow_forks", "go", "develop", "", "", "", true, true, true))}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Issue Type Resource: Create with HierarchyLevel set ====================

func TestIssueTypeResourceCreateWithHierarchyLevel(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "it-1", "name": "Task", "description": "A task",
			"iconUrl": "https://icon.url", "subtask": false, "hierarchyLevel": 0,
		})
	})
	ctx := context.Background()
	r := issuetyperes.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Task"),
		"description":     tftypes.NewValue(tftypes.String, "A task"),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, 0),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Role Assignment: Update success with ProductID ====================

func TestRoleAssignmentUpdateSuccessWithProductID(t *testing.T) {
	t.Parallel()
	client := rgClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		if r.Method == "POST" {
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "assign-new",
				"roleId":        "role-2",
				"principalType": "user",
				"principalId":   "user-1",
				"scope":         "product",
				"productId":     "prod-1",
			})
			return
		}
	})
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "assign-old"),
		"role_id":        tftypes.NewValue(tftypes.String, "role-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "product"),
		"product_id":     tftypes.NewValue(tftypes.String, "prod-1"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "product"),
		"product_id":     tftypes.NewValue(tftypes.String, "prod-1"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", resp.Diagnostics.Errors())
	}
}
