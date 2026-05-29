// Package unit contains extended error path tests for complete coverage.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func statusMockClient(t *testing.T, code int) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"error"}, "errors": map[string]string{}})
	}))
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	c, _ := atlassian.NewClient(atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)
	return c
}

// --- Group Update with 409 (duplicate on recreate) ---
func TestGroupUpdateConflictOnRecreate(t *testing.T) {
	t.Parallel()
	// Mock: DELETE succeeds, POST returns 409
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"duplicate"}, "errors": map[string]string{}})
			return
		}
		w.WriteHeader(200)
	}))
	defer ts.Close()
	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	client, _ := atlassian.NewClient(atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)

	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, "old"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, "new"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected conflict error")
	}
}

// --- Group Update generic error on recreate ---
func TestGroupUpdateErrorOnRecreate(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"error"}, "errors": map[string]string{}})
	}))
	defer ts.Close()
	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	client, _ := atlassian.NewClient(atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)

	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, "old"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, "new"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Group Update delete error (non-403) ---
func TestGroupUpdateDeleteError(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 500)
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, "old"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, "new"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Group Create Conflict (duplicate name) ---
func TestGroupCreateConflict(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 409)
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":     tftypes.NewValue(tftypes.String, "dup"),
		"self_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected conflict error")
	}
}

// --- Role Create Conflict ---
func TestRoleCreateConflict(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 409)
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "dup"), "description": tftypes.NewValue(tftypes.String, "d"),
		"scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected conflict")
	}
}

// --- Role Update 404 ---
func TestRoleUpdateNotFound(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 404)
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "123"), "name": tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"), "scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Role Update 409 ---
func TestRoleUpdateConflict(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 409)
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "123"), "name": tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"), "scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Role Assignment Create 400, 404, 409 ---
func TestRoleAssignmentCreate400(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 400)
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleAssignmentCreate404(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 404)
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleAssignmentCreate409(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 409)
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Role Assignment Update errors ---
func TestRoleAssignmentUpdateProductScopeMissing(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "aid"),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "product"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for missing product_id on product scope update")
	}
}

func TestRoleAssignmentUpdateDeleteError(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 500)
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "aid"),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r2"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Role Assignment Update 404 on delete then create succeeds ---
func TestRoleAssignmentUpdateDeleteNotFoundThenCreate(t *testing.T) {
	t.Parallel()
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method == "DELETE" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"not found"}})
			return
		}
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "new-id", "roleId": "r", "principalType": "user", "principalId": "u", "scope": "org",
			})
			return
		}
		w.WriteHeader(200)
	}))
	defer ts.Close()
	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	client, _ := atlassian.NewClient(atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)

	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "old-id"),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Expected success (404 on delete ignored): %v", resp.Diagnostics.Errors())
	}
}

// --- Membership Delete with generic error ---
func TestGroupMembershipDeleteGenericError(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 500)
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Membership Update with error on add ---
func TestGroupMembershipUpdateAddError(t *testing.T) {
	t.Parallel()
	client := statusMockClient(t, 500)
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1"), tftypes.NewValue(tftypes.String, "u2")}),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}
