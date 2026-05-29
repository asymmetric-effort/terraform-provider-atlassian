// Package unit contains final coverage tests for remaining edge cases.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	tokenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/token"
	userrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/user"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// --- OAuth "unable to parse" path ---
func TestOAuthRefreshBadResponseJSON(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte("not json")) // Valid 200 but unparseable body
	})
	auth, _ := client.NewOAuthRefreshAuthenticator("cid", "csec", "rtok")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestOAuthClientCredentialsBadResponseJSON(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte("not json"))
	})
	auth, _ := client.NewOAuthClientCredentialsAuthenticator("cid", "csec")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

// --- Client Do with auth error ---
func TestClientDoAuthError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, _ := client.NewClient(cfg, &failingAuth{})
	var result map[string]string
	err := c.Get(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected auth error")
	}
}

type failingAuth struct{}

func (f *failingAuth) AuthenticateRequest(req *http.Request) error {
	return fmt.Errorf("auth failed")
}

// --- Role resource with scope in response ---
func TestRoleResourceCreateWithScope(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 42, "name": "R", "description": "D", "scope": "org",
			})
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 42, "name": "R", "description": "D", "scope": "product",
			})
			return
		}
		if r.Method == "PUT" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 42, "name": "R2", "description": "D2", "scope": "product",
			})
			return
		}
		w.WriteHeader(204)
	}))
	defer ts.Close()
	auth, _ := client.NewAPIKeyAuthenticator("test-api-key")
	cl, _ := client.NewClient(client.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)

	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":    tftypes.NewValue(tftypes.String, "R"), "description": tftypes.NewValue(tftypes.String, "D"),
		"scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: cResp.State}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", rResp.Diagnostics.Errors())
	}

	// Update
	uPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "42"),
		"name":    tftypes.NewValue(tftypes.String, "R2"), "description": tftypes.NewValue(tftypes.String, "D2"),
		"scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	uResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: uPlan, State: cResp.State}, uResp)
	if uResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", uResp.Diagnostics.Errors())
	}
}

// --- Role Assignment with productId in response ---
func TestRoleAssignmentWithProductIdInResponse(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "a1", "roleId": "r", "principalType": "user", "principalId": "u",
				"scope": "product", "productId": "jira",
			})
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "a1", "roleId": "r", "principalType": "user", "principalId": "u",
				"scope": "product", "productId": "jira",
			})
			return
		}
		w.WriteHeader(204)
	}))
	defer ts.Close()
	auth, _ := client.NewAPIKeyAuthenticator("test-api-key")
	cl, _ := client.NewClient(client.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)

	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "product"),
		"product_id": tftypes.NewValue(tftypes.String, "jira"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: cResp.State}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", rResp.Diagnostics.Errors())
	}
}

// --- Group Read 404 RemoveResource ---
func TestGroupReadNotFoundRemovesResource(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"),
		"name":     tftypes.NewValue(tftypes.String, "grp"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	// 404 should remove resource, no error
	if resp.Diagnostics.HasError() {
		t.Fatal("Read 404 should not error")
	}
}

// --- Token Delete 404 (already revoked) ---
func TestTokenResourceDeleteNotFoundAlreadyRevoked(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id": tftypes.NewValue(tftypes.String, "tid"), "label": tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, "val"), "created_at": tftypes.NewValue(tftypes.String, "2024"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Token delete 404 should not error")
	}
}

// --- Role Delete 404 (already deleted) ---
func TestRoleResourceDeleteNotFoundAlreadyDeleted(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "123"), "name": tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"), "scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Role delete 404 should not error")
	}
}

// --- Assignment Delete 404 ---
func TestRoleAssignmentDeleteNotFound(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "aid"),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Assignment delete 404 should not error")
	}
}

// --- Assignment Read 404 removes resource ---
func TestRoleAssignmentReadNotFound(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "aid"),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Read 404 should not error")
	}
}

// --- Role Read 404 removes resource ---
func TestRoleResourceReadNotFoundRemovesResource(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "123"), "name": tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"), "scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Read 404 should not error")
	}
}

// --- Membership Read 404 removes resource ---
func TestMembershipReadNotFound(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Read 404 should not error")
	}
}

// --- Membership Create with idempotent 409 ---
func TestMembershipCreateIdempotent409(t *testing.T) {
	t.Parallel()
	// Server returns 409 (already member) which the resource ignores
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"already member"}})
	}))
	defer ts.Close()
	auth, _ := client.NewAPIKeyAuthenticator("test-api-key")
	cl, _ := client.NewClient(client.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)

	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	// 409 on create is treated as idempotent (continue)
	if resp.Diagnostics.HasError() {
		t.Fatal("Create 409 should be treated as idempotent")
	}
}

// --- Membership Update with 409 on add (idempotent) ---
func TestMembershipUpdateAdd409(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"already member"}})
			return
		}
		w.WriteHeader(204) // DELETE succeeds
	}))
	defer ts.Close()
	auth, _ := client.NewAPIKeyAuthenticator("test-api-key")
	cl, _ := client.NewClient(client.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)

	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, cl)
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
	// 409 on add during update should be ignored (continue)
	if resp.Diagnostics.HasError() {
		t.Fatal("Update 409 on add should be idempotent")
	}
}

// --- Membership Update remove 404 (continue) ---
func TestMembershipUpdateRemove404(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"not found"}})
			return
		}
		w.WriteHeader(201) // POST succeeds
	}))
	defer ts.Close()
	auth, _ := client.NewAPIKeyAuthenticator("test-api-key")
	cl, _ := client.NewClient(client.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)

	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1"), tftypes.NewValue(tftypes.String, "u2")}),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u2")}),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Update remove 404 should be idempotent")
	}
}

// --- User Read 404 removes resource ---
func TestUserReadNotFoundRemoves(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id": tftypes.NewValue(tftypes.String, "uid"), "email": tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"), "active": tftypes.NewValue(tftypes.Bool, true),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Read 404 should not error")
	}
}

// --- Token Read 404 removes resource ---
func TestTokenReadNotFoundRemoves(t *testing.T) {
	t.Parallel()
	cl := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id": tftypes.NewValue(tftypes.String, "tid"), "label": tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, "val"), "created_at": tftypes.NewValue(tftypes.String, "2024"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Read 404 should not error")
	}
}
