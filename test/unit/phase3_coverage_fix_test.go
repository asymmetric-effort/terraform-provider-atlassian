// Package unit contains tests to close Phase 3 (Confluence) coverage gaps to >= 98%.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	confluencepagedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/page"
	confluencespacepermdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/space"
	confluencepageresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/page"
	confluencespacepermresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/space"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// p3Client creates a client pointing at the given test server.
func p3Client(t *testing.T, handler http.HandlerFunc) *atlassian.Client {
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

// ==================== Restriction Resource: Read() user found path ====================

func TestRestrictionResourceReadUserFound(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "read",
				"restrictions": map[string]interface{}{
					"user": map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{"type": "known", "accountId": "user-1"},
						},
					},
					"group": map[string]interface{}{"results": []interface{}{}},
				},
			},
		})
	})
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "c1/read/user/user-1"),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Restriction Resource: Read() user not found in list ====================

func TestRestrictionResourceReadUserNotFoundInList(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "read",
				"restrictions": map[string]interface{}{
					"user": map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{"type": "known", "accountId": "other-user"},
						},
					},
					"group": map[string]interface{}{"results": []interface{}{}},
				},
			},
		})
	})
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "c1/read/user/user-1"),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected silent removal")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Expected state to be removed when user not in list")
	}
}

// ==================== Restriction Resource: Read() generic error ====================

func TestRestrictionResourceReadGenericError(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "c1/read/user/user-1"),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== Restriction Resource: Delete() 403 forbidden ====================

func TestRestrictionResourceDelete403Forbidden(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Forbidden"},
		})
	})
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "c1/read/user/u1"),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "u1"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

// ==================== Restriction Resource: Delete() generic error ====================

func TestRestrictionResourceDeleteGenericError(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "c1/read/user/u1"),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "u1"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== Permission Resource: Read() permission found ====================

func TestPermissionResourceReadPermissionFound(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id": "perm-1",
				"principal": map[string]interface{}{
					"type": "user",
					"id":   "user-1",
				},
				"operation": map[string]interface{}{
					"key": "read",
				},
			},
		})
	})
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "space-1/perm-1"),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Permission Resource: Read() permission not found ====================

func TestPermissionResourceReadPermissionNotFound(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id": "perm-other",
				"principal": map[string]interface{}{
					"type": "group",
					"id":   "group-1",
				},
				"operation": map[string]interface{}{
					"key": "write",
				},
			},
		})
	})
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "space-1/perm-1"),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected silent removal")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Expected state to be removed when permission not found")
	}
}

// ==================== Permission Resource: Read() generic error ====================

func TestPermissionResourceReadGenericError(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "space-1/perm-1"),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== Permission Resource: Delete() 403 forbidden ====================

func TestPermissionResourceDelete403Forbidden(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Forbidden"},
		})
	})
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "space-1/perm-1"),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

// ==================== Permission Resource: Delete() generic error ====================

func TestPermissionResourceDeleteGenericError(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "space-1/perm-1"),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== Restriction DS: Read() user found path ====================

func TestRestrictionDSReadUserFound(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "read",
				"restrictions": map[string]interface{}{
					"user": map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{"type": "known", "accountId": "user-1"},
						},
					},
					"group": map[string]interface{}{"results": []interface{}{}},
				},
			},
		})
	})
	ctx := context.Background()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Restriction DS: Read() user not found ====================

func TestRestrictionDSReadUserNotFound(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "read",
				"restrictions": map[string]interface{}{
					"user": map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{"type": "known", "accountId": "other-user"},
						},
					},
					"group": map[string]interface{}{"results": []interface{}{}},
				},
			},
		})
	})
	ctx := context.Background()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error")
	}
}

// ==================== Restriction DS: Read() generic error ====================

func TestRestrictionDSReadGenericError(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== Restriction DS: Read() group not found ====================

func TestRestrictionDSReadGroupNotFound(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "read",
				"restrictions": map[string]interface{}{
					"user":  map[string]interface{}{"results": []interface{}{}},
					"group": map[string]interface{}{"results": []interface{}{}},
				},
			},
		})
	})
	ctx := context.Background()
	ds := confluencepagedatasource.NewRestrictionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "group"),
		"principal_id":   tftypes.NewValue(tftypes.String, "g-missing"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error")
	}
}

// ==================== Permission DS: Read() permission found ====================

func TestPermissionDSReadPermissionFound(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id": "perm-1",
				"principal": map[string]interface{}{
					"type": "user",
					"id":   "user-1",
				},
				"operation": map[string]interface{}{
					"key": "read",
				},
			},
		})
	})
	ctx := context.Background()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Permission DS: Read() permission not found ====================

func TestPermissionDSReadPermissionNotFound(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id": "perm-other",
				"principal": map[string]interface{}{
					"type": "group",
					"id":   "group-1",
				},
				"operation": map[string]interface{}{
					"key": "write",
				},
			},
		})
	})
	ctx := context.Background()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected not-found error")
	}
}

// ==================== Permission DS: Read() generic error ====================

func TestPermissionDSReadGenericError(t *testing.T) {
	t.Parallel()
	client := p3Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}
