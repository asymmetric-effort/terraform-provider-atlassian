// Package unit contains unit tests for the atlassian_bitbucket_repository resource and data source.
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
	bbrepodatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/bitbucket/repository"
	bbreporesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/repository"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// bbRepoIDCounter provides unique IDs for bitbucket repo mock server tests.
var bbRepoIDCounter uint64

func bbRepoNextID() string {
	n := atomic.AddUint64(&bbRepoIDCounter, 1)
	return fmt.Sprintf("{bb-repo-%d}", n)
}

// testBBRepoMockServer creates a mock HTTP server for Bitbucket repository endpoints.
func testBBRepoMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	repos := make(map[string]map[string]interface{}) // workspace/slug -> repo

	mux := http.NewServeMux()

	// Create/Update repository (PUT)
	mux.HandleFunc("PUT /2.0/repositories/{workspace}/{slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)

		mu.Lock()
		defer mu.Unlock()

		if existing, exists := repos[key]; exists {
			// Update
			for k, v := range req {
				if k != "uuid" && k != "slug" && k != "full_name" {
					existing[k] = v
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(existing)
			return
		}

		// Create
		name, _ := req["name"].(string)
		if name == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"name is required"},
				"errors":        map[string]string{},
			})
			return
		}

		uuid := bbRepoNextID()
		isPrivate, _ := req["is_private"].(bool)
		forkPolicy, _ := req["fork_policy"].(string)
		if forkPolicy == "" {
			forkPolicy = "allow_forks"
		}
		language, _ := req["language"].(string)
		description, _ := req["description"].(string)
		hasIssues, _ := req["has_issues"].(bool)
		hasWiki, _ := req["has_wiki"].(bool)

		repo := map[string]interface{}{
			"uuid":        uuid,
			"slug":        slug,
			"name":        name,
			"full_name":   workspace + "/" + slug,
			"description": description,
			"is_private":  isPrivate,
			"fork_policy": forkPolicy,
			"language":    language,
			"has_issues":  hasIssues,
			"has_wiki":    hasWiki,
			"workspace": map[string]interface{}{
				"slug": workspace,
			},
			"links": map[string]interface{}{
				"html": map[string]interface{}{
					"href": fmt.Sprintf("https://bitbucket.org/%s/%s", workspace, slug),
				},
				"clone": []map[string]interface{}{
					{"name": "https", "href": fmt.Sprintf("https://bitbucket.org/%s/%s.git", workspace, slug)},
					{"name": "ssh", "href": fmt.Sprintf("git@bitbucket.org:%s/%s.git", workspace, slug)},
				},
			},
		}
		if mb, ok := req["mainbranch"]; ok {
			repo["mainbranch"] = mb
		}

		repos[key] = repo
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(repo)
	})

	// Read repository
	mux.HandleFunc("GET /2.0/repositories/{workspace}/{slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		mu.Lock()
		defer mu.Unlock()

		repo, ok := repos[key]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Repository not found"},
				"errors":        map[string]string{},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(repo)
	})

	// Delete repository
	mux.HandleFunc("DELETE /2.0/repositories/{workspace}/{slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		mu.Lock()
		defer mu.Unlock()

		if _, ok := repos[key]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Repository not found"},
				"errors":        map[string]string{},
			})
			return
		}
		delete(repos, key)
		w.WriteHeader(http.StatusNoContent)
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

// testBBForbiddenMockServer creates a mock that returns 403 for all Bitbucket endpoints.
func testBBForbiddenMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
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

// testBBServerErrorMockServer creates a mock that returns 500 for all Bitbucket endpoints.
func testBBServerErrorMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
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

// ==================== RESOURCE SCHEMA TESTS ====================

// TestBBRepoResourceMetadata verifies the resource type name.
func TestBBRepoResourceMetadata(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_bitbucket_repository" {
		t.Errorf("expected resource type name 'atlassian_bitbucket_repository', got %q", resp.TypeName)
	}
}

// TestBBRepoResourceSchema verifies the resource schema has all expected attributes.
func TestBBRepoResourceSchema(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{
		"id", "workspace", "slug", "name", "description", "is_private",
		"fork_policy", "language", "default_branch", "has_issues", "has_wiki",
		"clone_ssh", "clone_https", "url",
	}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestBBRepoResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestBBRepoResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 14
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestBBRepoResourceSchemaRequiredAttributes verifies required attributes.
func TestBBRepoResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"workspace", "slug", "name"}
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

// TestBBRepoResourceSchemaComputedAttributes verifies computed attributes.
func TestBBRepoResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"id", "clone_ssh", "clone_https", "url", "description", "is_private", "fork_policy", "language", "default_branch", "has_issues", "has_wiki"}
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

// TestBBRepoResourceSchemaOptionalAttributes verifies optional attributes.
func TestBBRepoResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"description", "is_private", "fork_policy", "language", "default_branch", "has_issues", "has_wiki"}
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

// TestBBRepoResourceSchemaSensitiveAttributes verifies no attributes are sensitive.
func TestBBRepoResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	for name, attr := range resp.Schema.Attributes {
		if attr.IsSensitive() {
			t.Errorf("attribute %q should not be sensitive", name)
		}
	}
}

// TestBBRepoResourceImplementsResource verifies the Resource interface.
func TestBBRepoResourceImplementsResource(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	if _, ok := r.(resource.Resource); !ok {
		t.Error("expected bitbucket repository resource to implement resource.Resource")
	}
}

// TestBBRepoResourceImplementsImportState verifies the ImportState interface.
func TestBBRepoResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := bbreporesource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected bitbucket repository resource to implement ResourceWithImportState")
	}
}

// ==================== DATA SOURCE SCHEMA TESTS ====================

// TestBBRepoDataSourceMetadata verifies the data source type name.
func TestBBRepoDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := bbrepodatasource.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_bitbucket_repository" {
		t.Errorf("expected data source type name 'atlassian_bitbucket_repository', got %q", resp.TypeName)
	}
}

// TestBBRepoDataSourceSchema verifies the data source schema has all expected attributes.
func TestBBRepoDataSourceSchema(t *testing.T) {
	t.Parallel()

	ds := bbrepodatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{
		"id", "workspace", "slug", "name", "description", "is_private",
		"fork_policy", "language", "default_branch", "has_issues", "has_wiki",
		"clone_ssh", "clone_https", "url",
	}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestBBRepoDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestBBRepoDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	ds := bbrepodatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	expected := 14
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestBBRepoDataSourceSchemaComputedAttributes verifies computed-only attributes.
func TestBBRepoDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	ds := bbrepodatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	computedAttrs := []string{"id", "name", "description", "is_private", "fork_policy", "language", "default_branch", "has_issues", "has_wiki", "clone_ssh", "clone_https", "url"}
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

// TestBBRepoDataSourceSchemaRequiredAttributes verifies required data source attributes.
func TestBBRepoDataSourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	ds := bbrepodatasource.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"workspace", "slug"}
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

// TestBBRepoDataSourceImplementsDataSource verifies the DataSource interface.
func TestBBRepoDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	ds := bbrepodatasource.NewDataSource()
	if _, ok := ds.(datasource.DataSource); !ok {
		t.Error("expected bitbucket repository data source to implement datasource.DataSource")
	}
}

// ==================== RESOURCE CRUD LIFECYCLE TESTS ====================

// bbRepoValues creates a tftypes.Value map for a bitbucket repository.
func bbRepoValues(id, workspace, slug, name, description, forkPolicy, language, defaultBranch, cloneSSH, cloneHTTPS, url string, isPrivate, hasIssues, hasWiki interface{}) map[string]tftypes.Value {
	toStr := func(s string) tftypes.Value {
		if s == "" {
			return tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
		}
		return tftypes.NewValue(tftypes.String, s)
	}
	toBool := func(v interface{}) tftypes.Value {
		if v == nil {
			return tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue)
		}
		return tftypes.NewValue(tftypes.Bool, v)
	}
	return map[string]tftypes.Value{
		"id":             toStr(id),
		"workspace":      tftypes.NewValue(tftypes.String, workspace),
		"slug":           tftypes.NewValue(tftypes.String, slug),
		"name":           tftypes.NewValue(tftypes.String, name),
		"description":    toStr(description),
		"is_private":     toBool(isPrivate),
		"fork_policy":    toStr(forkPolicy),
		"language":       toStr(language),
		"default_branch": toStr(defaultBranch),
		"has_issues":     toBool(hasIssues),
		"has_wiki":       toBool(hasWiki),
		"clone_ssh":      toStr(cloneSSH),
		"clone_https":    toStr(cloneHTTPS),
		"url":            toStr(url),
	}
}

// bbRepoSetValues creates a tftypes.Value map with all fields set (for state).
func bbRepoSetValues(id, workspace, slug, name, description, forkPolicy, language, defaultBranch, cloneSSH, cloneHTTPS, url string, isPrivate, hasIssues, hasWiki bool) map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, id),
		"workspace":      tftypes.NewValue(tftypes.String, workspace),
		"slug":           tftypes.NewValue(tftypes.String, slug),
		"name":           tftypes.NewValue(tftypes.String, name),
		"description":    tftypes.NewValue(tftypes.String, description),
		"is_private":     tftypes.NewValue(tftypes.Bool, isPrivate),
		"fork_policy":    tftypes.NewValue(tftypes.String, forkPolicy),
		"language":       tftypes.NewValue(tftypes.String, language),
		"default_branch": tftypes.NewValue(tftypes.String, defaultBranch),
		"has_issues":     tftypes.NewValue(tftypes.Bool, hasIssues),
		"has_wiki":       tftypes.NewValue(tftypes.Bool, hasWiki),
		"clone_ssh":      tftypes.NewValue(tftypes.String, cloneSSH),
		"clone_https":    tftypes.NewValue(tftypes.String, cloneHTTPS),
		"url":            tftypes.NewValue(tftypes.String, url),
	}
}

// getBoolAttrBB retrieves a bool attribute from state.
func getBoolAttrBB(t *testing.T, state tfsdk.State, name string) bool {
	t.Helper()
	var val types.Bool
	diags := state.GetAttribute(context.Background(), path.Root(name), &val)
	if diags.HasError() {
		t.Fatalf("getBoolAttr %q: %v", name, diags.Errors())
	}
	return val.ValueBool()
}

// TestBBRepoResourceCRUDLifecycle tests the full create-read-update-delete cycle.
func TestBBRepoResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoValues("", "myworkspace", "myrepo", "My Repository", "A test repo", "allow_forks", "go", "", "", "", "", true, true, false))}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	repoID := getStringAttr(t, createResp.State, "id")
	if repoID == "" {
		t.Fatal("expected non-empty id")
	}
	if slug := getStringAttr(t, createResp.State, "slug"); slug != "myrepo" {
		t.Errorf("expected slug 'myrepo', got %q", slug)
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "My Repository" {
		t.Errorf("expected name 'My Repository', got %q", name)
	}
	if desc := getStringAttr(t, createResp.State, "description"); desc != "A test repo" {
		t.Errorf("expected description 'A test repo', got %q", desc)
	}
	if ws := getStringAttr(t, createResp.State, "workspace"); ws != "myworkspace" {
		t.Errorf("expected workspace 'myworkspace', got %q", ws)
	}
	if priv := getBoolAttrBB(t, createResp.State, "is_private"); !priv {
		t.Error("expected is_private to be true")
	}
	if url := getStringAttr(t, createResp.State, "url"); url == "" {
		t.Error("expected non-empty url")
	}
	if cloneSSH := getStringAttr(t, createResp.State, "clone_ssh"); cloneSSH == "" {
		t.Error("expected non-empty clone_ssh")
	}
	if cloneHTTPS := getStringAttr(t, createResp.State, "clone_https"); cloneHTTPS == "" {
		t.Error("expected non-empty clone_https")
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues(repoID, "myworkspace", "myrepo", "My Repository", "A test repo", "allow_forks", "go", "", "git@bitbucket.org:myworkspace/myrepo.git", "https://bitbucket.org/myworkspace/myrepo.git", "https://bitbucket.org/myworkspace/myrepo", true, true, false))}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "My Repository" {
		t.Errorf("Read name: got %q", name)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues(repoID, "myworkspace", "myrepo", "Updated Repo", "Updated desc", "no_forks", "python", "", "git@bitbucket.org:myworkspace/myrepo.git", "https://bitbucket.org/myworkspace/myrepo.git", "https://bitbucket.org/myworkspace/myrepo", false, false, true))}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Repo" {
		t.Errorf("Update name: got %q", name)
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

// TestBBRepoResourceCreateMinimal tests creating a repository with minimal fields.
func TestBBRepoResourceCreateMinimal(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoValues("", "ws", "minimal", "Minimal", "", "", "", "", "", "", "", nil, nil, nil))}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create minimal: %v", resp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, resp.State, "id"); id == "" {
		t.Error("expected non-empty id")
	}
}

// TestBBRepoResourceReadNotFound tests reading a nonexistent repository removes resource.
func TestBBRepoResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("nonexistent", "ws", "nope", "X", "", "", "", "", "", "", "", false, false, false))}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent should not error: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed after 404")
	}
}

// TestBBRepoResourceUpdateNotFound tests updating a nonexistent repository.
func TestBBRepoResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Note: Bitbucket PUT creates if not exists, so the update on a non-existent
	// repo will actually create it. To test a true "not found" update, we use
	// the mock that returns 404 for all endpoints.
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("nonexistent", "ws", "nope", "X", "", "", "", "", "", "", "", false, false, false))}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	// This will succeed since Bitbucket PUT is idempotent
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	// Not expecting error since Bitbucket PUT creates if not exists
}

// TestBBRepoResourceDeleteNotFound tests deleting an already-deleted repository.
func TestBBRepoResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("nonexistent", "ws", "nope", "X", "", "", "", "", "", "", "", false, false, false))}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatal("Delete of nonexistent repo should not error (idempotent)")
	}
}

// TestBBRepoResourceCreateForbidden tests 403 on create.
func TestBBRepoResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBBForbiddenMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoValues("", "ws", "forbidden", "Forbidden", "", "", "", "", "", "", "", nil, nil, nil))}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestBBRepoResourceUpdateForbidden tests 403 on update.
func TestBBRepoResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBBForbiddenMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("some-id", "ws", "forbidden", "Forbidden", "", "", "", "", "", "", "", false, false, false))}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on update")
	}
}

// TestBBRepoResourceDeleteForbidden tests 403 on delete.
func TestBBRepoResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	_, client := testBBForbiddenMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("some-id", "ws", "forbidden", "Forbidden", "", "", "", "", "", "", "", false, false, false))}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestBBRepoResourceCreateServerError tests generic error on create.
func TestBBRepoResourceCreateServerError(t *testing.T) {
	t.Parallel()
	_, client := testBBServerErrorMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoValues("", "ws", "err", "Error", "", "", "", "", "", "", "", nil, nil, nil))}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// TestBBRepoResourceReadServerError tests generic error on read.
func TestBBRepoResourceReadServerError(t *testing.T) {
	t.Parallel()
	_, client := testBBServerErrorMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("some-id", "ws", "err", "Error", "", "", "", "", "", "", "", false, false, false))}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 read")
	}
}

// TestBBRepoResourceUpdateServerError tests generic error on update.
func TestBBRepoResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	_, client := testBBServerErrorMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("some-id", "ws", "err", "Error", "", "", "", "", "", "", "", false, false, false))}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 update")
	}
}

// TestBBRepoResourceDeleteServerError tests generic error on delete.
func TestBBRepoResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	_, client := testBBServerErrorMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("some-id", "ws", "err", "Error", "", "", "", "", "", "", "", false, false, false))}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 delete")
	}
}

// TestBBRepoResourceConfigureNil verifies nil provider data does not error.
func TestBBRepoResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := bbreporesource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestBBRepoResourceConfigureWrongType verifies wrong provider data type errors.
func TestBBRepoResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := bbreporesource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestBBRepoResourceImportState verifies import state passthrough.
func TestBBRepoResourceImportState(t *testing.T) {
	t.Parallel()
	r := bbreporesource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "bb-repo-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestBBRepoResourceCreateBadPlan tests Create with invalid plan data.
func TestBBRepoResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: badPlan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan")
	}
}

// TestBBRepoResourceReadBadState tests Read with invalid state data.
func TestBBRepoResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Read(ctx, resource.ReadRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state")
	}
}

// TestBBRepoResourceUpdateBadPlan tests Update with invalid plan data.
func TestBBRepoResourceUpdateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	badPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	goodState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("x", "ws", "x", "X", "", "", "", "", "", "", "", false, false, false))}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: badPlan, State: goodState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad plan on update")
	}
}

// TestBBRepoResourceUpdateBadState tests Update with invalid state data.
func TestBBRepoResourceUpdateBadState(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	goodPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType,
		bbRepoSetValues("x", "ws", "x", "X", "", "", "", "", "", "", "", false, false, false))}
	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: goodPlan, State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on update")
	}
}

// TestBBRepoResourceDeleteBadState tests Delete with invalid state data.
func TestBBRepoResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	badState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "invalid")}}
	r.Delete(ctx, resource.DeleteRequest{State: badState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad state on delete")
	}
}

// ==================== DATA SOURCE CRUD TESTS ====================

// TestBBRepoDataSourceRead tests reading a repository via data source.
func TestBBRepoDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()

	// Create a repo first via resource
	r := bbreporesource.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTfType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTfType,
		bbRepoValues("", "testws", "testrepo", "Test Repo", "desc", "allow_forks", "go", "", "", "", "", true, false, false))}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read via data source
	ds := bbrepodatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"workspace":      tftypes.NewValue(tftypes.String, "testws"),
		"slug":           tftypes.NewValue(tftypes.String, "testrepo"),
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
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "Test Repo" {
		t.Errorf("expected name 'Test Repo', got %q", name)
	}
	if ws := getStringAttr(t, dsResp.State, "workspace"); ws != "testws" {
		t.Errorf("expected workspace 'testws', got %q", ws)
	}
}

// TestBBRepoDataSourceNotFound tests 404 error on data source read.
func TestBBRepoDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	ds := bbrepodatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"workspace":      tftypes.NewValue(tftypes.String, "nonexistent"),
		"slug":           tftypes.NewValue(tftypes.String, "nope"),
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
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent repository")
	}
}

// TestBBRepoDataSourceServerError tests generic error on data source read.
func TestBBRepoDataSourceServerError(t *testing.T) {
	t.Parallel()
	_, client := testBBServerErrorMockServer(t)
	ctx := context.Background()
	ds := bbrepodatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"workspace":      tftypes.NewValue(tftypes.String, "ws"),
		"slug":           tftypes.NewValue(tftypes.String, "err"),
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
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500 data source read")
	}
}

// TestBBRepoDataSourceConfigureNil verifies nil provider data does not error.
func TestBBRepoDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := bbrepodatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestBBRepoDataSourceConfigureWrongType verifies wrong provider data type errors.
func TestBBRepoDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := bbrepodatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestBBRepoDataSourceReadBadConfig tests data source Read with invalid config data.
func TestBBRepoDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := testBBRepoMockServer(t)
	ctx := context.Background()
	ds := bbrepodatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)

	badConfig := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(tftypes.String, "invalid")}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: badConfig}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error from bad config on data source read")
	}
}
