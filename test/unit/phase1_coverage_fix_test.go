// Package unit contains tests to close Phase 1 (Identity) coverage gaps to >= 98%.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	groupdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/group"
	roledatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/role"
	userdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/user"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// p1Client creates a client pointing at the given test server.
func p1Client(t *testing.T, handler http.HandlerFunc) *atlassian.Client {
	t.Helper()
	ts := httptest.NewServer(handler)
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

// ==================== Group DataSource: Read() 403 path ====================

func TestGroupDSRead403PermissionDenied(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Forbidden"},
		})
	})
	ctx := context.Background()
	ds := groupdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "gid-1"),
		"name":     tftypes.NewValue(tftypes.String, ""),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 permission denied error")
	}
}

// ==================== Role DataSource: Read() 403 path ====================

func TestRoleDSRead403PermissionDenied(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Forbidden"},
		})
	})
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, "999"),
		"name":        tftypes.NewValue(tftypes.String, ""),
		"description": tftypes.NewValue(tftypes.String, nil),
		"scope":       tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 permission denied error")
	}
}

// ==================== Role DataSource: findRoleByName() unmarshal error + not found ====================

func TestRoleDSFindRoleByNameUnmarshalError(t *testing.T) {
	t.Parallel()
	// Return a list with one invalid JSON item and no matching name
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return array of raw JSON: one bad item, one valid but non-matching
		w.Write([]byte(`[{"invalid json, "name":"other","id":1,"description":"d","scope":"org"}]`))
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
		t.Fatal("expected not-found error after unmarshal errors")
	}
}

// ==================== Role DataSource: findRoleByName() via Read with generic error on list ====================

func TestRoleDSFindRoleByNameGenericError(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Internal server error"]}`))
	})
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, ""),
		"name":        tftypes.NewValue(tftypes.String, "TestRole"),
		"description": tftypes.NewValue(tftypes.String, nil),
		"scope":       tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== User DataSource: Read() - isStatusCode nil check ====================

func TestUserDSIsStatusCodeNil(t *testing.T) {
	t.Parallel()
	// This test uses email lookup that returns 404 for the user,
	// which exercises isStatusCode with a real error.
	// But we also need to exercise isStatusCode(nil, ...) which returns false.
	// The nil path is covered when err == nil (successful read).
	// The uncovered isStatusCode path is the nil check at line 184.
	// We need to trigger a non-404 error so isStatusCode returns false.
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Server error"]}`))
	})
	ctx := context.Background()
	ds := userdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, "acc-1"),
		"email":        tftypes.NewValue(tftypes.String, nil),
		"display_name": tftypes.NewValue(tftypes.String, nil),
		"active":       tftypes.NewValue(tftypes.Bool, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error from 500 status")
	}
}

// TestUserDSReadByEmailNotFound exercises the email not-found branch
func TestUserDSReadByEmailNotFound(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return empty results for search - triggers 404 from findUserByEmail
		json.NewEncoder(w).Encode([]interface{}{})
	})
	ctx := context.Background()
	ds := userdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, nil),
		"email":        tftypes.NewValue(tftypes.String, "missing@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, nil),
		"active":       tftypes.NewValue(tftypes.Bool, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not found error")
	}
}

// ==================== Membership Resource: Read() with members that partially match state ====================

func TestMembershipResourceReadPartialMatch(t *testing.T) {
	t.Parallel()
	callCount := 0
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// Return members where only some match the state
		json.NewEncoder(w).Encode(map[string]interface{}{
			"isLast": true,
			"values": []map[string]interface{}{
				{"accountId": "user-1"},
				{"accountId": "user-3"},
			},
		})
	})
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// State has user-1 and user-2, but API only returns user-1 and user-3
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "grp-1"),
		"user_account_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "user-1"),
			tftypes.NewValue(tftypes.String, "user-2"),
		}),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Membership Resource: listGroupMembers pagination ====================

func TestMembershipResourceListGroupMembersPaginated(t *testing.T) {
	t.Parallel()
	callCount := 0
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// First page: not last
			json.NewEncoder(w).Encode(map[string]interface{}{
				"isLast": false,
				"values": []map[string]interface{}{
					{"accountId": "user-1"},
				},
			})
		} else {
			// Second page: last
			json.NewEncoder(w).Encode(map[string]interface{}{
				"isLast": true,
				"values": []map[string]interface{}{
					{"accountId": "user-2"},
				},
			})
		}
	})
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "grp-1"),
		"user_account_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "user-1"),
			tftypes.NewValue(tftypes.String, "user-2"),
		}),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
	if callCount < 2 {
		t.Errorf("expected at least 2 API calls for pagination, got %d", callCount)
	}
}

// ==================== Membership Resource: Read() generic error ====================

func TestMembershipResourceReadGenericError(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Internal error"]}`))
	})
	ctx := context.Background()
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "grp-1"),
		"user_account_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "user-1"),
		}),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error from 500")
	}
}

// ==================== Role Assignment Resource: Update() delete 404 + create 400 ====================

func TestRoleAssignmentUpdateDelete404ThenCreate400(t *testing.T) {
	t.Parallel()
	callCount := 0
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			// 404 on delete - should continue to create
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Not found"},
			})
			return
		}
		if r.Method == "POST" {
			// 400 on create
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Bad request"},
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
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 400 error on create")
	}
}

// ==================== Role Assignment Resource: Update() delete error (non-404) ====================

func TestRoleAssignmentUpdateDeleteErrorNon404(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Server error"},
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
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error from delete failure")
	}
}

// ==================== Role Assignment Resource: Update() product scope missing product_id ====================

func TestRoleAssignmentUpdateProductScopeMissingProductID(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
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
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "product"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected missing product_id error")
	}
	found := false
	for _, e := range resp.Diagnostics.Errors() {
		if e.Summary() == "Missing product_id" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'Missing product_id' error")
	}
}

// ==================== Role Resource: Read() generic error (non-404) ====================

func TestRoleResourceReadGenericError(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Internal error"]}`))
	})
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, "42"),
		"name":        tftypes.NewValue(tftypes.String, "MyRole"),
		"description": tftypes.NewValue(tftypes.String, "desc"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error from 500")
	}
}

// ==================== Role Resource: Read() with empty scope ====================

func TestRoleResourceReadEmptyScope(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": 42, "name": "R", "description": "D", "scope": "",
		})
	})
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, "42"),
		"name":        tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Role Resource: Read() 404 removes resource ====================

func TestRoleResourceRead404RemovesResource(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Not found"},
		})
	})
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, "42"),
		"name":        tftypes.NewValue(tftypes.String, "R"),
		"description": tftypes.NewValue(tftypes.String, "D"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected 404 to silently remove resource")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Expected state to be removed on 404")
	}
}

// ==================== Role Assignment Resource: Update() create 409 conflict ====================

func TestRoleAssignmentUpdateCreate409Conflict(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		if r.Method == "POST" {
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Conflict"},
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
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 409 conflict error")
	}
	found := false
	for _, e := range resp.Diagnostics.Errors() {
		if e.Summary() == "Duplicate role assignment" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'Duplicate role assignment' error")
	}
}

// ==================== Role Assignment Resource: Update() create 403 forbidden ====================

func TestRoleAssignmentUpdateCreate403(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		if r.Method == "POST" {
			w.WriteHeader(403)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Forbidden"},
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
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

// ==================== Role Assignment Resource: Update() create 404 ====================

func TestRoleAssignmentUpdateCreate404(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		if r.Method == "POST" {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Not found"},
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
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 404 error on create")
	}
}

// ==================== Role Assignment Resource: Update() create generic error ====================

func TestRoleAssignmentUpdateCreateGenericError(t *testing.T) {
	t.Parallel()
	client := p1Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		if r.Method == "POST" {
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Server error"},
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
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
	found := false
	for _, e := range resp.Diagnostics.Errors() {
		if e.Summary() == "Failed to update role assignment" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'Failed to update role assignment' error, got: %v", resp.Diagnostics.Errors())
	}
}

// Force use of fmt to avoid unused import
var _ = fmt.Sprintf
