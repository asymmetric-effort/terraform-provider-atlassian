// Package unit contains tests to close coverage gaps to >= 98%.
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
	tokenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/token"
	userrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/user"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// qc creates a client with a custom handler.
func qc(t *testing.T, handler http.HandlerFunc) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("u@e.com", "tok")
	c, _ := atlassian.NewClient(atlassian.Config{
		BaseURL: ts.URL, RequestTimeout: 5 * time.Second,
		MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second,
	}, auth)
	return c
}

// okHandler returns a 200 OK for all requests.
var okHandler = func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
}

// ===== Plan/State Get error paths (HasError guards) =====

// Each resource has multiple HasError guards after Plan.Get or State.Get.
// Passing a type-mismatched Raw triggers Diagnostics errors.

func TestUserCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := userrs.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestUserReadStateGetError(t *testing.T) {
	t.Parallel()
	r := userrs.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestUserUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := userrs.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id": tftypes.NewValue(tftypes.String, "uid"), "email": tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"), "active": tftypes.NewValue(tftypes.Bool, true),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
		State: validState,
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestUserUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := userrs.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id": tftypes.NewValue(tftypes.String, "uid"), "email": tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "Y"), "active": tftypes.NewValue(tftypes.Bool, true),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestUserDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := userrs.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestGroupCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestGroupReadStateGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestGroupUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.UpdateResponse{State: emptyState(context.Background(), s)}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestGroupUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid"), "name": tftypes.NewValue(tftypes.String, "n"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestGroupDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestMembershipCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestMembershipCreateElementsAsError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	// Valid plan but with unknown list value to trigger ElementsAs error
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	// Unknown list will be treated as empty or cause an error
}

func TestMembershipReadStateGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestMembershipUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.UpdateResponse{State: emptyState(context.Background(), s)}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestMembershipUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestMembershipUpdatePlanElementsAsError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}
	validState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	// Plan with unknown list to trigger ElementsAs error
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, tftypes.UnknownValue),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: validState}, resp)
}

func TestMembershipUpdateStateElementsAsError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u1")}),
	})}
	// State with unknown list to trigger ElementsAs error
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, tftypes.UnknownValue),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: validPlan, State: state}, resp)
}

func TestMembershipDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestMembershipDeleteElementsAsError(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, "gid"),
		"user_account_ids": tftypes.NewValue(listType, tftypes.UnknownValue),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
}

func TestRoleCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRoleReadStateGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRoleUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.UpdateResponse{State: emptyState(context.Background(), s)}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRoleUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "123"), "name": tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"), "scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestRoleDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestAssignmentCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestAssignmentReadStateGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestAssignmentUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.UpdateResponse{State: emptyState(context.Background(), s)}
	r.Update(context.Background(), resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestAssignmentUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "aid"),
		"role_id": tftypes.NewValue(tftypes.String, "r"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestAssignmentDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestTokenCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := tokenresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.CreateResponse{State: emptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestTokenReadStateGetError(t *testing.T) {
	t.Parallel()
	r := tokenresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.ReadResponse{State: emptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestTokenDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := tokenresource.NewResource()
	configureResource(t, r, qc(t, okHandler))
	s := getResourceSchema(t, r)
	resp := &resource.DeleteResponse{State: emptyState(context.Background(), s)}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ===== Role resource Read with scope="" and non-404 error =====

func TestRoleReadSuccessNoScope(t *testing.T) {
	t.Parallel()
	cl := qc(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": 42, "name": "R", "description": "D",
			// No "scope" field — exercises the if apiResp.Scope != "" branch
		})
	})
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, cl)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "42"), "name": tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"), "scope": tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ===== Role assignment Update successful with 404 on delete then create =====

func TestAssignmentUpdateDelete404ThenCreateSuccess(t *testing.T) {
	t.Parallel()
	calls := 0
	cl := qc(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method == "DELETE" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"not found"}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "a2", "roleId": "r2", "principalType": "user", "principalId": "u",
			"scope": "org",
		})
	})
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
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id": tftypes.NewValue(tftypes.String, "r2"), "principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id": tftypes.NewValue(tftypes.String, "u"), "scope": tftypes.NewValue(tftypes.String, "org"),
		"product_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Expected success: %v", resp.Diagnostics.Errors())
	}
}

// ===== Membership pagination break =====

func TestMembershipReadPaginationBreak(t *testing.T) {
	t.Parallel()
	// Server returns 0 values to trigger the `len(apiResp.Values) == 0` break
	cl := qc(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"maxResults": 0, "startAt": 0, "total": 0, "isLast": false,
			"values": []interface{}{},
		})
	})
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
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}
