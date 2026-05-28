// Package unit contains unit tests for the Bitbucket resources and data sources:
// atlassian_bitbucket_branch_restriction, atlassian_bitbucket_pipeline,
// atlassian_bitbucket_deployment, atlassian_bitbucket_repository_permission.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	bbbranchrestrictionds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/bitbucket/branch_restriction"
	bbdeploymentds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/bitbucket/deployment"
	bbpipelineds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/bitbucket/pipeline"
	bbrepopermds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/bitbucket/repository"
	bbbranchrestrictionrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/branch_restriction"
	bbdeploymentrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/deployment"
	bbpipeliners "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/pipeline"
	bbrepopermrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/repository"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// bitbucketIDCounter provides unique IDs for bitbucket mock server tests.
var bitbucketIDCounter uint64

func bitbucketNextID(prefix string) string {
	n := atomic.AddUint64(&bitbucketIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// testBitbucketMockServer creates a mock HTTP server for Bitbucket endpoints.
func testBitbucketMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	branchRestrictions := make(map[string]map[string]interface{})
	brIDCounter := int64(0)
	pipelines := make(map[string]map[string]interface{}) // "workspace/slug" -> config
	deployments := make(map[string]map[string]interface{})
	permissions := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// ==================== BRANCH RESTRICTION ENDPOINTS ====================

	mux.HandleFunc("POST /2.0/repositories/{workspace}/{slug}/branch-restrictions", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		kind, _ := req["kind"].(string)
		pattern, _ := req["pattern"].(string)
		if kind == "" || pattern == "" {
			writeErr(w, 400, "kind and pattern are required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := atomic.AddInt64(&brIDCounter, 1)
		idStr := strconv.FormatInt(id, 10)
		br := map[string]interface{}{
			"id":      float64(id),
			"kind":    kind,
			"pattern": pattern,
		}
		branchRestrictions[idStr] = br
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(br)
	})

	mux.HandleFunc("GET /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		br, ok := branchRestrictions[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Branch restriction not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(br)
	})

	mux.HandleFunc("PUT /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		br, ok := branchRestrictions[id]
		if !ok {
			writeErr(w, 404, "Branch restriction not found")
			return
		}
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if kind, ok := req["kind"].(string); ok {
			br["kind"] = kind
		}
		if pattern, ok := req["pattern"].(string); ok {
			br["pattern"] = pattern
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(br)
	})

	mux.HandleFunc("DELETE /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := branchRestrictions[id]; !ok {
			writeErr(w, 404, "Branch restriction not found")
			return
		}
		delete(branchRestrictions, id)
		w.WriteHeader(204)
	})

	// ==================== PIPELINE ENDPOINTS ====================

	mux.HandleFunc("PUT /2.0/repositories/{workspace}/{slug}/pipelines_config", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		enabled, _ := req["enabled"].(bool)
		mu.Lock()
		defer mu.Unlock()
		config := map[string]interface{}{
			"enabled": enabled,
		}
		pipelines[key] = config
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	})

	mux.HandleFunc("GET /2.0/repositories/{workspace}/{slug}/pipelines_config", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug
		mu.Lock()
		config, ok := pipelines[key]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Pipeline configuration not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(config)
	})

	// ==================== DEPLOYMENT ENDPOINTS ====================

	mux.HandleFunc("POST /2.0/repositories/{workspace}/{slug}/environments", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			writeErr(w, 400, "name is required")
			return
		}
		envType := ""
		if et, ok := req["environment_type"].(map[string]interface{}); ok {
			envType, _ = et["name"].(string)
		}
		mu.Lock()
		defer mu.Unlock()
		uuid := bitbucketNextID("env")
		dep := map[string]interface{}{
			"uuid": uuid,
			"name": name,
			"environment_type": map[string]interface{}{
				"name": envType,
			},
		}
		deployments[uuid] = dep
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(dep)
	})

	mux.HandleFunc("GET /2.0/repositories/{workspace}/{slug}/environments/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		uuid := r.PathValue("uuid")
		mu.Lock()
		dep, ok := deployments[uuid]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Deployment environment not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dep)
	})

	mux.HandleFunc("PUT /2.0/repositories/{workspace}/{slug}/environments/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		uuid := r.PathValue("uuid")
		mu.Lock()
		defer mu.Unlock()
		dep, ok := deployments[uuid]
		if !ok {
			writeErr(w, 404, "Deployment environment not found")
			return
		}
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if name, ok := req["name"].(string); ok {
			dep["name"] = name
		}
		if et, ok := req["environment_type"].(map[string]interface{}); ok {
			dep["environment_type"] = et
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dep)
	})

	mux.HandleFunc("DELETE /2.0/repositories/{workspace}/{slug}/environments/{uuid}", func(w http.ResponseWriter, r *http.Request) {
		uuid := r.PathValue("uuid")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := deployments[uuid]; !ok {
			writeErr(w, 404, "Deployment environment not found")
			return
		}
		delete(deployments, uuid)
		w.WriteHeader(204)
	})

	// ==================== PERMISSION ENDPOINTS ====================

	mux.HandleFunc("POST /2.0/repositories/{workspace}/{slug}/permissions-config", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		principalType, _ := req["principal_type"].(string)
		principalID, _ := req["principal_id"].(string)
		permission, _ := req["permission"].(string)
		if principalType == "" || principalID == "" || permission == "" {
			writeErr(w, 400, "principal_type, principal_id, and permission are required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := bitbucketNextID("perm")
		perm := map[string]interface{}{
			"id":             id,
			"principal_type": principalType,
			"principal_id":   principalID,
			"permission":     permission,
		}
		permissions[id] = perm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(perm)
	})

	mux.HandleFunc("GET /2.0/repositories/{workspace}/{slug}/permissions-config/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		perm, ok := permissions[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Permission not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(perm)
	})

	mux.HandleFunc("PUT /2.0/repositories/{workspace}/{slug}/permissions-config/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		perm, ok := permissions[id]
		if !ok {
			writeErr(w, 404, "Permission not found")
			return
		}
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if pt, ok := req["principal_type"].(string); ok {
			perm["principal_type"] = pt
		}
		if pid, ok := req["principal_id"].(string); ok {
			perm["principal_id"] = pid
		}
		if p, ok := req["permission"].(string); ok {
			perm["permission"] = p
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(perm)
	})

	mux.HandleFunc("DELETE /2.0/repositories/{workspace}/{slug}/permissions-config/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := permissions[id]; !ok {
			writeErr(w, 404, "Permission not found")
			return
		}
		delete(permissions, id)
		w.WriteHeader(204)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// testBitbucketErrorMockServer returns specific HTTP error codes for testing error paths.
func testBitbucketErrorMockServer(t *testing.T, statusCode int) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{http.StatusText(statusCode)},
			"errors":        map[string]string{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// ==================== BRANCH RESTRICTION RESOURCE TESTS ====================

func TestBranchRestrictionResourceMetadata(t *testing.T) {
	t.Parallel()
	r := bbbranchrestrictionrs.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_bitbucket_branch_restriction" {
		t.Errorf("expected 'atlassian_bitbucket_branch_restriction', got %q", resp.TypeName)
	}
}

func TestBranchRestrictionResourceSchema(t *testing.T) {
	t.Parallel()
	r := bbbranchrestrictionrs.NewResource()
	s := getResourceSchema(t, r)
	expectedAttrs := []string{"id", "repository", "pattern", "kind"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(s.Attributes))
	}
}

func TestBranchRestrictionResourceImplementsResource(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = bbbranchrestrictionrs.NewResource()
	var _ resource.ResourceWithImportState = bbbranchrestrictionrs.NewResource().(resource.ResourceWithImportState)
}

func TestBranchRestrictionResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := bbbranchrestrictionrs.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestBranchRestrictionResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := bbbranchrestrictionrs.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong type")
	}
}

func TestBranchRestrictionResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
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
	if got := getStringAttr(t, createResp.State, "pattern"); got != "main" {
		t.Errorf("expected pattern 'main', got %q", got)
	}
	if got := getStringAttr(t, createResp.State, "kind"); got != "push" {
		t.Errorf("expected kind 'push', got %q", got)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, id),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	readResp := &resource.ReadResponse{State: readState}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, readResp.State, "kind"); got != "push" {
		t.Errorf("expected kind 'push', got %q", got)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, id),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"pattern":    tftypes.NewValue(tftypes.String, "release/*"),
		"kind":       tftypes.NewValue(tftypes.String, "delete"),
	})}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, id),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	updateResp := &resource.UpdateResponse{State: updateState}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, updateResp.State, "pattern"); got != "release/*" {
		t.Errorf("expected pattern 'release/*', got %q", got)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, id),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"pattern":    tftypes.NewValue(tftypes.String, "release/*"),
		"kind":       tftypes.NewValue(tftypes.String, "delete"),
	})}
	deleteResp := &resource.DeleteResponse{State: deleteState}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete should remove resource
	readResp2 := &resource.ReadResponse{State: readState}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp2)
	if readResp2.Diagnostics.HasError() {
		t.Fatalf("Read after delete: %v", readResp2.Diagnostics.Errors())
	}
}

func TestBranchRestrictionResourceCreateInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "invalidrepo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo format")
	}
}

func TestBranchRestrictionResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
	if !strings.Contains(createResp.Diagnostics.Errors()[0].Detail(), "permission") {
		t.Errorf("expected permission error, got: %s", createResp.Diagnostics.Errors()[0].Detail())
	}
}

func TestBranchRestrictionResourceCreateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestBranchRestrictionResourceCreateGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestBranchRestrictionResourceReadInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestBranchRestrictionResourceReadError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestBranchRestrictionResourceUpdateInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestBranchRestrictionResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestBranchRestrictionResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestBranchRestrictionResourceUpdateGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestBranchRestrictionResourceDeleteInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestBranchRestrictionResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("not found on delete should not error")
	}
}

func TestBranchRestrictionResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestBranchRestrictionResourceDeleteGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== PIPELINE RESOURCE TESTS ====================

func TestPipelineResourceMetadata(t *testing.T) {
	t.Parallel()
	r := bbpipeliners.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_bitbucket_pipeline" {
		t.Errorf("expected 'atlassian_bitbucket_pipeline', got %q", resp.TypeName)
	}
}

func TestPipelineResourceSchema(t *testing.T) {
	t.Parallel()
	r := bbpipeliners.NewResource()
	s := getResourceSchema(t, r)
	expectedAttrs := []string{"id", "repository", "enabled"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 3 {
		t.Errorf("expected 3 attributes, got %d", len(s.Attributes))
	}
}

func TestPipelineResourceImplementsResource(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = bbpipeliners.NewResource()
	var _ resource.ResourceWithImportState = bbpipeliners.NewResource().(resource.ResourceWithImportState)
}

func TestPipelineResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := bbpipeliners.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestPipelineResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := bbpipeliners.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong type")
	}
}

func TestPipelineResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, createResp.State, "id"); got != "myworkspace/myrepo" {
		t.Errorf("expected id 'myworkspace/myrepo', got %q", got)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	readResp := &resource.ReadResponse{State: readState}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update (disable)
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, false),
	})}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	updateResp := &resource.UpdateResponse{State: updateState}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, false),
	})}
	deleteResp := &resource.DeleteResponse{State: deleteState}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

func TestPipelineResourceCreateInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo format")
	}
}

func TestPipelineResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestPipelineResourceCreateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestPipelineResourceCreateGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestPipelineResourceReadInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "noslash"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestPipelineResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	// Not found on read should not error, just remove resource
	if readResp.Diagnostics.HasError() {
		t.Fatalf("not found on read should not error: %v", readResp.Diagnostics.Errors())
	}
}

func TestPipelineResourceReadGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestPipelineResourceUpdateInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "noslash"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "noslash"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestPipelineResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestPipelineResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestPipelineResourceUpdateGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestPipelineResourceDeleteInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "noslash"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestPipelineResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("not found on delete should not error")
	}
}

func TestPipelineResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestPipelineResourceDeleteGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestPipelineResourceCreateWithDefaultEnabled(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create with null enabled (should default to true)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/myrepo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, nil),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
}

// ==================== DEPLOYMENT RESOURCE TESTS ====================

func TestDeploymentResourceMetadata(t *testing.T) {
	t.Parallel()
	r := bbdeploymentrs.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_bitbucket_deployment" {
		t.Errorf("expected 'atlassian_bitbucket_deployment', got %q", resp.TypeName)
	}
}

func TestDeploymentResourceSchema(t *testing.T) {
	t.Parallel()
	r := bbdeploymentrs.NewResource()
	s := getResourceSchema(t, r)
	expectedAttrs := []string{"id", "repository", "name", "environment_type"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(s.Attributes))
	}
}

func TestDeploymentResourceImplementsResource(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = bbdeploymentrs.NewResource()
	var _ resource.ResourceWithImportState = bbdeploymentrs.NewResource().(resource.ResourceWithImportState)
}

func TestDeploymentResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := bbdeploymentrs.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestDeploymentResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := bbdeploymentrs.NewResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong type")
	}
}

func TestDeploymentResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":       tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"name":             tftypes.NewValue(tftypes.String, "staging-env"),
		"environment_type": tftypes.NewValue(tftypes.String, "staging"),
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
	if got := getStringAttr(t, createResp.State, "name"); got != "staging-env" {
		t.Errorf("expected name 'staging-env', got %q", got)
	}
	if got := getStringAttr(t, createResp.State, "environment_type"); got != "staging" {
		t.Errorf("expected environment_type 'staging', got %q", got)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, id),
		"repository":       tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"name":             tftypes.NewValue(tftypes.String, "staging-env"),
		"environment_type": tftypes.NewValue(tftypes.String, "staging"),
	})}
	readResp := &resource.ReadResponse{State: readState}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, id),
		"repository":       tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"name":             tftypes.NewValue(tftypes.String, "production-env"),
		"environment_type": tftypes.NewValue(tftypes.String, "production"),
	})}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, id),
		"repository":       tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"name":             tftypes.NewValue(tftypes.String, "staging-env"),
		"environment_type": tftypes.NewValue(tftypes.String, "staging"),
	})}
	updateResp := &resource.UpdateResponse{State: updateState}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, updateResp.State, "name"); got != "production-env" {
		t.Errorf("expected name 'production-env', got %q", got)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, id),
		"repository":       tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"name":             tftypes.NewValue(tftypes.String, "production-env"),
		"environment_type": tftypes.NewValue(tftypes.String, "production"),
	})}
	deleteResp := &resource.DeleteResponse{State: deleteState}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

func TestDeploymentResourceCreateInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":       tftypes.NewValue(tftypes.String, "noslash"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo format")
	}
}

func TestDeploymentResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestDeploymentResourceCreateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestDeploymentResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusConflict)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for conflict")
	}
}

func TestDeploymentResourceCreateGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestDeploymentResourceReadInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "noslash"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestDeploymentResourceReadError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestDeploymentResourceUpdateInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "noslash"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "noslash"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestDeploymentResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestDeploymentResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestDeploymentResourceUpdateGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestDeploymentResourceDeleteInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "noslash"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestDeploymentResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("not found on delete should not error")
	}
}

func TestDeploymentResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestDeploymentResourceDeleteGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== REPOSITORY PERMISSION RESOURCE TESTS ====================

func TestRepositoryPermissionResourceMetadata(t *testing.T) {
	t.Parallel()
	r := bbrepopermrs.NewPermissionResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_bitbucket_repository_permission" {
		t.Errorf("expected 'atlassian_bitbucket_repository_permission', got %q", resp.TypeName)
	}
}

func TestRepositoryPermissionResourceSchema(t *testing.T) {
	t.Parallel()
	r := bbrepopermrs.NewPermissionResource()
	s := getResourceSchema(t, r)
	expectedAttrs := []string{"id", "repository", "principal_type", "principal_id", "permission"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(s.Attributes))
	}
}

func TestRepositoryPermissionResourceImplementsResource(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = bbrepopermrs.NewPermissionResource()
	var _ resource.ResourceWithImportState = bbrepopermrs.NewPermissionResource().(resource.ResourceWithImportState)
}

func TestRepositoryPermissionResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := bbrepopermrs.NewPermissionResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestRepositoryPermissionResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := bbrepopermrs.NewPermissionResource().(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong type")
	}
}

func TestRepositoryPermissionResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":     tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
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
	if got := getStringAttr(t, createResp.State, "permission"); got != "write" {
		t.Errorf("expected permission 'write', got %q", got)
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, id),
		"repository":     tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	readResp := &resource.ReadResponse{State: readState}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, id),
		"repository":     tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "admin"),
	})}
	updateState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, id),
		"repository":     tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	updateResp := &resource.UpdateResponse{State: updateState}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, updateResp.State, "permission"); got != "admin" {
		t.Errorf("expected permission 'admin', got %q", got)
	}

	// Delete
	deleteState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, id),
		"repository":     tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "admin"),
	})}
	deleteResp := &resource.DeleteResponse{State: deleteState}
	r.Delete(ctx, resource.DeleteRequest{State: deleteState}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

func TestRepositoryPermissionResourceCreateInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":     tftypes.NewValue(tftypes.String, "noslash"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo format")
	}
}

func TestRepositoryPermissionResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestRepositoryPermissionResourceCreateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestRepositoryPermissionResourceCreateConflict(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusConflict)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for conflict")
	}
}

func TestRepositoryPermissionResourceCreateGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRepositoryPermissionResourceReadInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "noslash"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestRepositoryPermissionResourceReadError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRepositoryPermissionResourceUpdateInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "noslash"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestRepositoryPermissionResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestRepositoryPermissionResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestRepositoryPermissionResourceUpdateGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRepositoryPermissionResourceDeleteInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "noslash"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestRepositoryPermissionResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("not found on delete should not error")
	}
}

func TestRepositoryPermissionResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusForbidden)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for forbidden")
	}
}

func TestRepositoryPermissionResourceDeleteGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== DATA SOURCE TESTS ====================

func TestBranchRestrictionDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := bbbranchrestrictionds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_bitbucket_branch_restriction" {
		t.Errorf("expected 'atlassian_bitbucket_branch_restriction', got %q", resp.TypeName)
	}
}

func TestBranchRestrictionDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := bbbranchrestrictionds.NewDataSource()
	s := getDatasourceSchema(t, ds)
	expectedAttrs := []string{"id", "repository", "pattern", "kind"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if len(s.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(s.Attributes))
	}
}

func TestBranchRestrictionDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := bbbranchrestrictionds.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestBranchRestrictionDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := bbbranchrestrictionds.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong type")
	}
}

func TestBranchRestrictionDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()

	// First create a restriction via the resource
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	// Now read via data source
	ds := bbbranchrestrictionds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, id),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/myrepo"),
		"pattern":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"kind":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, readResp.State, "kind"); got != "push" {
		t.Errorf("expected kind 'push', got %q", got)
	}
}

func TestBranchRestrictionDataSourceReadInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	ds := bbbranchrestrictionds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"pattern":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"kind":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestBranchRestrictionDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	ds := bbbranchrestrictionds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "999"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"kind":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestBranchRestrictionDataSourceReadGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	ds := bbbranchrestrictionds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"kind":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestPipelineDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := bbpipelineds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_bitbucket_pipeline" {
		t.Errorf("expected 'atlassian_bitbucket_pipeline', got %q", resp.TypeName)
	}
}

func TestPipelineDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := bbpipelineds.NewDataSource()
	s := getDatasourceSchema(t, ds)
	if len(s.Attributes) != 3 {
		t.Errorf("expected 3 attributes, got %d", len(s.Attributes))
	}
}

func TestPipelineDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := bbpipelineds.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestPipelineDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := bbpipelineds.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong type")
	}
}

func TestPipelineDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()

	// Create pipeline first
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/dsrepo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}

	// Read via data source
	ds := bbpipelineds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "myworkspace/dsrepo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
}

func TestPipelineDataSourceReadInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	ds := bbpipelineds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "noslash"),
		"enabled":    tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestPipelineDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	ds := bbpipelineds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestPipelineDataSourceReadGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	ds := bbpipelineds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestDeploymentDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := bbdeploymentds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_bitbucket_deployment" {
		t.Errorf("expected 'atlassian_bitbucket_deployment', got %q", resp.TypeName)
	}
}

func TestDeploymentDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := bbdeploymentds.NewDataSource()
	s := getDatasourceSchema(t, ds)
	if len(s.Attributes) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(s.Attributes))
	}
}

func TestDeploymentDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := bbdeploymentds.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestDeploymentDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := bbdeploymentds.NewDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong type")
	}
}

func TestDeploymentDataSourceReadInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	ds := bbdeploymentds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "noslash"),
		"name":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"environment_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestDeploymentDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	ds := bbdeploymentds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-999"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"environment_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestDeploymentDataSourceReadGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	ds := bbdeploymentds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"environment_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRepositoryPermissionDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := bbrepopermds.NewPermissionDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_bitbucket_repository_permission" {
		t.Errorf("expected 'atlassian_bitbucket_repository_permission', got %q", resp.TypeName)
	}
}

func TestRepositoryPermissionDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := bbrepopermds.NewPermissionDataSource()
	s := getDatasourceSchema(t, ds)
	if len(s.Attributes) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(s.Attributes))
	}
}

func TestRepositoryPermissionDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := bbrepopermds.NewPermissionDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestRepositoryPermissionDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := bbrepopermds.NewPermissionDataSource().(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error for wrong type")
	}
}

func TestRepositoryPermissionDataSourceReadInvalidRepo(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	ds := bbrepopermds.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "noslash"),
		"principal_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"principal_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"permission":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for invalid repo")
	}
}

func TestRepositoryPermissionDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	ds := bbrepopermds.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-999"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"principal_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"permission":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for not found")
	}
}

func TestRepositoryPermissionDataSourceReadGenericError(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusInternalServerError)
	ctx := context.Background()
	ds := bbrepopermds.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"principal_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"permission":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== IMPORT STATE TESTS ====================

func TestBranchRestrictionResourceImportState(t *testing.T) {
	t.Parallel()
	r := bbbranchrestrictionrs.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	importReq := resource.ImportStateRequest{ID: "42"}
	importResp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, importReq, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, importResp.State, "id"); got != "42" {
		t.Errorf("expected id '42', got %q", got)
	}
}

func TestPipelineResourceImportState(t *testing.T) {
	t.Parallel()
	r := bbpipeliners.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	importReq := resource.ImportStateRequest{ID: "ws/repo"}
	importResp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, importReq, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, importResp.State, "id"); got != "ws/repo" {
		t.Errorf("expected id 'ws/repo', got %q", got)
	}
}

func TestDeploymentResourceImportState(t *testing.T) {
	t.Parallel()
	r := bbdeploymentrs.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	importReq := resource.ImportStateRequest{ID: "env-uuid-123"}
	importResp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, importReq, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, importResp.State, "id"); got != "env-uuid-123" {
		t.Errorf("expected id 'env-uuid-123', got %q", got)
	}
}

func TestRepositoryPermissionResourceImportState(t *testing.T) {
	t.Parallel()
	r := bbrepopermrs.NewPermissionResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	importReq := resource.ImportStateRequest{ID: "perm-uuid-456"}
	importResp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, importReq, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, importResp.State, "id"); got != "perm-uuid-456" {
		t.Errorf("expected id 'perm-uuid-456', got %q", got)
	}
}

// ==================== DATA SOURCE SUCCESS READ (deployment, permission) ====================

func TestDeploymentDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()

	// Create a deployment first
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":       tftypes.NewValue(tftypes.String, "myworkspace/dsrepo2"),
		"name":             tftypes.NewValue(tftypes.String, "my-env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	// Now read via data source
	ds := bbdeploymentds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, id),
		"repository":       tftypes.NewValue(tftypes.String, "myworkspace/dsrepo2"),
		"name":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"environment_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, readResp.State, "name"); got != "my-env" {
		t.Errorf("expected name 'my-env', got %q", got)
	}
}

func TestRepositoryPermissionDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()

	// Create a permission first
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"repository":     tftypes.NewValue(tftypes.String, "myworkspace/dsrepo3"),
		"principal_type": tftypes.NewValue(tftypes.String, "group"),
		"principal_id":   tftypes.NewValue(tftypes.String, "devs"),
		"permission":     tftypes.NewValue(tftypes.String, "read"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")

	// Now read via data source
	ds := bbrepopermds.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	tfType := s.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, id),
		"repository":     tftypes.NewValue(tftypes.String, "myworkspace/dsrepo3"),
		"principal_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"principal_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"permission":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if got := getStringAttr(t, readResp.State, "permission"); got != "read" {
		t.Errorf("expected permission 'read', got %q", got)
	}
}

// ==================== RESOURCE READ NOT-FOUND (removes resource) ====================

func TestBranchRestrictionResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "999"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "main"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("not found on read should not error: %v", readResp.Diagnostics.Errors())
	}
}

func TestDeploymentResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "uuid-999"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "env"),
		"environment_type": tftypes.NewValue(tftypes.String, "test"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("not found on read should not error: %v", readResp.Diagnostics.Errors())
	}
}

func TestRepositoryPermissionResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketErrorMockServer(t, http.StatusNotFound)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-999"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-123"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("not found on read should not error: %v", readResp.Diagnostics.Errors())
	}
}

// ==================== DIAGNOSTIC ERROR PATH TESTS (malformed state/plan) ====================

func TestBranchRestrictionResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	// Empty/nil plan triggers HasError
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil plan")
	}
}

func TestBranchRestrictionResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil state")
	}
}

func TestBranchRestrictionResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil plan/state")
	}
}

func TestBranchRestrictionResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbbranchrestrictionrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil state")
	}
}

func TestPipelineResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil plan")
	}
}

func TestPipelineResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil state")
	}
}

func TestPipelineResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil plan/state")
	}
}

func TestPipelineResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbpipeliners.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil state")
	}
}

func TestDeploymentResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil plan")
	}
}

func TestDeploymentResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil state")
	}
}

func TestDeploymentResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil plan/state")
	}
}

func TestDeploymentResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbdeploymentrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil state")
	}
}

func TestRepositoryPermissionResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil plan")
	}
}

func TestRepositoryPermissionResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil state")
	}
}

func TestRepositoryPermissionResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil plan/state")
	}
}

func TestRepositoryPermissionResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	r := bbrepopermrs.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil state")
	}
}

func TestBranchRestrictionDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	ds := bbbranchrestrictionds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil config")
	}
}

func TestPipelineDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	ds := bbpipelineds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil config")
	}
}

func TestDeploymentDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	ds := bbdeploymentds.NewDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil config")
	}
}

func TestRepositoryPermissionDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testBitbucketMockServer(t)
	ctx := context.Background()
	ds := bbrepopermds.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	s := getDatasourceSchema(t, ds)
	config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &datasource.ReadResponse{State: emptyDSState(ctx, s)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("expected error for nil config")
	}
}

func TestBranchRestrictionResourceMetadataName(t *testing.T) {
	t.Parallel()
	r := bbbranchrestrictionrs.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if !strings.Contains(resp.TypeName, "bitbucket_branch_restriction") {
		t.Errorf("expected type name containing 'bitbucket_branch_restriction', got %q", resp.TypeName)
	}
}
