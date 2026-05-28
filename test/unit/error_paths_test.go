// Package unit contains tests for error paths in resource CRUD methods.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	groupdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/group"
	roledatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/role"
	mocksrv "github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	tokenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/token"
	userrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/user"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// errorMockServer creates a mock server that returns a specific error code for all endpoints.
func errorMockServer(t *testing.T, statusCode int, message string) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{message},
			"errors":        map[string]string{},
		})
	}))
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	c, _ := atlassian.NewClient(atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}, auth)
	return c
}

// --- User Forbidden Errors ---

func TestUserCreateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"self_url":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestUserUpdateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, "uid"),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestUserDeleteForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, "uid"),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestUserResourceDeleteNotFoundHandled(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, "uid"),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	// User delete treats 404 as already deleted - should NOT error
	if resp.Diagnostics.HasError() {
		t.Fatal("User delete 404 should not error")
	}
}

func TestUserReadError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal Server Error")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, "uid"),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

func TestUserDeleteGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, "uid"),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

func TestUserUpdateGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, "uid"),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

func TestUserCreateGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"self_url":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on 500")
	}
}

// --- Group Error Paths ---

func TestGroupCreateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":     tftypes.NewValue(tftypes.String, "grp"),
		"self_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected forbidden error")
	}
}

func TestGroupCreateGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":     tftypes.NewValue(tftypes.String, "grp"),
		"self_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupReadError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"),
		"name":     tftypes.NewValue(tftypes.String, "grp"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupUpdateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"),
		"name":     tftypes.NewValue(tftypes.String, "old"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"),
		"name":     tftypes.NewValue(tftypes.String, "new"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupDeleteForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"),
		"name":     tftypes.NewValue(tftypes.String, "grp"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupDeleteGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"),
		"name":     tftypes.NewValue(tftypes.String, "grp"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupResourceDeleteNotFoundHandled(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"),
		"name":     tftypes.NewValue(tftypes.String, "grp"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	// 404 = already deleted, should not error
	if resp.Diagnostics.HasError() {
		t.Fatal("Group delete 404 should not error")
	}
}

// --- Role Error Paths ---

func TestRoleCreateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleCreateGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleUpdateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, "123"),
		"name":        tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleUpdateGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, "123"),
		"name":        tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleDeleteForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, "123"),
		"name":        tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleDeleteGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, "123"),
		"name":        tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Token Error Paths ---

func TestTokenCreateNotFound(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"label":           tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"created_at":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestTokenCreateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"label":           tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"created_at":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestTokenCreateConflict(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 409, "Limit")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"label":           tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"created_at":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestTokenCreateGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"label":           tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"created_at":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestTokenDeleteForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id":        tftypes.NewValue(tftypes.String, "tid"),
		"label":           tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, "val"),
		"created_at":      tftypes.NewValue(tftypes.String, "2024-01-01"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestTokenDeleteGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id":        tftypes.NewValue(tftypes.String, "tid"),
		"label":           tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, "val"),
		"created_at":      tftypes.NewValue(tftypes.String, "2024-01-01"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestTokenReadError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id":        tftypes.NewValue(tftypes.String, "tid"),
		"label":           tftypes.NewValue(tftypes.String, "T"),
		"user_account_id": tftypes.NewValue(tftypes.String, "uid"),
		"token_value":     tftypes.NewValue(tftypes.String, "val"),
		"created_at":      tftypes.NewValue(tftypes.String, "2024-01-01"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Role Assignment Error Paths ---

func TestRoleAssignmentCreateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r1"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleAssignmentCreateGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r1"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleAssignmentDeleteForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "aid"),
		"role_id": tftypes.NewValue(tftypes.String, "r1"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleAssignmentDeleteGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "aid"),
		"role_id": tftypes.NewValue(tftypes.String, "r1"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleAssignmentReadError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "aid"),
		"role_id": tftypes.NewValue(tftypes.String, "r1"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u1"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Membership Error Paths ---

func TestGroupMembershipReadError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
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
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupMembershipCreateNotFoundGroup(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 404, "Not Found")
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "nonexistent"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupMembershipCreateForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupMembershipCreateGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Datasource Error Paths ---

func TestGroupDataSourceForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	ds := groupdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, nil),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestGroupDataSourceGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	ds := groupdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, nil),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleDataSourceForbidden(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 403, "Forbidden")
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "rid"), "name": tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil), "scope": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleDataSourceGenericError(t *testing.T) {
	t.Parallel()
	client := errorMockServer(t, 500, "Internal")
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "rid"), "name": tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil), "scope": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

// --- Mock utility tests ---

func TestMockRequireAuth(t *testing.T) {
	t.Parallel()
	s := mocksrv.NewServer()
	validTokens := map[string]bool{"Bearer test": true}
	called := false
	handler := mocksrv.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}, validTokens)

	// Without auth header
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != 401 {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	// With wrong auth
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("Authorization", "Bearer wrong")
	rr2 := httptest.NewRecorder()
	handler(rr2, req2)
	if rr2.Code != 401 {
		t.Errorf("expected 401, got %d", rr2.Code)
	}

	// With valid auth
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.Header.Set("Authorization", "Bearer test")
	rr3 := httptest.NewRecorder()
	handler(rr3, req3)
	if rr3.Code != 200 {
		t.Errorf("expected 200, got %d", rr3.Code)
	}
	if !called {
		t.Error("handler was not called")
	}
	_ = s // ensure mock server is used
}

func TestMockErrorResponse(t *testing.T) {
	t.Parallel()
	resp := mocksrv.ErrorResponse("test error")
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	msgs, ok := resp["errorMessages"].([]string)
	if !ok || len(msgs) != 1 || msgs[0] != "test error" {
		t.Errorf("unexpected error response: %v", resp)
	}
}

func TestMockWriteJSON(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	mocksrv.WriteJSON(rr, 200, map[string]string{"status": "ok"})
	if rr.Code != 200 {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestMockWriteError(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	mocksrv.WriteError(rr, 400, "bad request")
	if rr.Code != 400 {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
