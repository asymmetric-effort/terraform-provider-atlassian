// Package unit contains targeted tests to close Confluence-specific coverage gaps to >= 98%.
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

// confluenceCovClient creates a client pointing at the given test server.
func confluenceCovClient(t *testing.T, ts *httptest.Server) *atlassian.Client {
	t.Helper()
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

// ==================== RESTRICTION RESOURCE: Read() 404 APIError path ====================

// TestRestrictionResourceRead404RemovesResource covers the 404 APIError branch in Read()
// that removes the resource from state (lines 247-250).
func TestRestrictionResourceRead404RemovesResource(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Content not found"},
			"errors":        map[string]string{},
		})
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
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
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected 404 to silently remove resource, got error")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Expected state to be removed on 404")
	}
}

// ==================== RESTRICTION RESOURCE: Read() group principal path ====================

// TestRestrictionResourceReadGroupPrincipalFound covers the group principal search branch
// in Read() (lines 270-274).
func TestRestrictionResourceReadGroupPrincipalFound(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return a restriction list with a group restriction
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "read",
				"restrictions": map[string]interface{}{
					"user": map[string]interface{}{"results": []interface{}{}},
					"group": map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{"type": "group", "id": "group-1"},
						},
					},
				},
			},
		})
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "c1/read/group/group-1"),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "group"),
		"principal_id":   tftypes.NewValue(tftypes.String, "group-1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Expected successful read, got: %v", resp.Diagnostics.Errors())
	}
}

// TestRestrictionResourceReadGroupNotFound covers the group principal not-found path
// in Read() that results in resource removal.
func TestRestrictionResourceReadGroupNotFound(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return restrictions but without the group we are looking for
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "read",
				"restrictions": map[string]interface{}{
					"user":  map[string]interface{}{"results": []interface{}{}},
					"group": map[string]interface{}{"results": []interface{}{}},
				},
			},
		})
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "c1/read/group/group-missing"),
		"content_id":     tftypes.NewValue(tftypes.String, "c1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "group"),
		"principal_id":   tftypes.NewValue(tftypes.String, "group-missing"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected silent removal, got error")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Expected state to be removed when group not found in list")
	}
}

// TestRestrictionResourceReadOperationMismatch covers the operation mismatch continue
// branch in Read() (line 260-261).
func TestRestrictionResourceReadOperationMismatch(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return restrictions but only for "update" operation, not "read"
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "update",
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
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Looking for "read" operation but only "update" is in the list
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
		t.Fatal("Expected silent removal, got error")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Expected state to be removed when operation not matched")
	}
}

// ==================== PERMISSION RESOURCE: Read() 404 path ====================

// TestPermissionResourceRead404RemovesResource covers the 404 APIError branch in
// space permission Read() (lines 231-234).
func TestPermissionResourceRead404RemovesResource(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Space not found"},
			"errors":        map[string]string{},
		})
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
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
		t.Fatal("Expected 404 to silently remove resource, got error")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Expected state to be removed on 404")
	}
}

// ==================== PERMISSION RESOURCE: Delete() parsePermissionID error ====================

// TestPermissionResourceDeleteBadCompositeID covers the parsePermissionID error branch
// in Delete() (lines 279-285).
func TestPermissionResourceDeleteBadCompositeID(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// ID without slash = invalid composite ID
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "no-slash-id"),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected parse error from invalid composite ID")
	}
}

// ==================== PERMISSION RESOURCE: Delete() 404 silent return ====================

// TestPermissionResourceDelete404Silent covers the 404 silent return path
// in Delete() (line 291-292).
func TestPermissionResourceDelete404Silent(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Permission not found"},
			"errors":        map[string]string{},
		})
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "space-1/perm-gone"),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected 404 on delete to succeed silently")
	}
}

// ==================== RESTRICTION DATA SOURCE: Read() group principal path ====================

// TestRestrictionDSReadGroupPrincipalFound covers the group principal search in
// the restriction data source Read() (lines 155-159).
func TestRestrictionDSReadGroupPrincipalFound(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "read",
				"restrictions": map[string]interface{}{
					"user": map[string]interface{}{"results": []interface{}{}},
					"group": map[string]interface{}{
						"results": []interface{}{
							map[string]interface{}{"type": "group", "id": "group-ds-1"},
						},
					},
				},
			},
		})
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
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
		"principal_id":   tftypes.NewValue(tftypes.String, "group-ds-1"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Expected successful read, got: %v", resp.Diagnostics.Errors())
	}
}

// TestRestrictionDSReadOperationMismatch covers the operation mismatch continue branch
// in the restriction data source Read() (line 145-146).
func TestRestrictionDSReadOperationMismatch(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Only "update" operation in list, searching for "read"
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"operation": "update",
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
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
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
		t.Fatal("Expected not-found error when operation does not match")
	}
}

// ==================== PERMISSION DATA SOURCE: Read() 404 path ====================

// TestPermissionDSRead404SpaceNotFound covers the 404 APIError path in the
// space permission data source Read() (lines 123-129).
func TestPermissionDSRead404SpaceNotFound(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Space not found"},
			"errors":        map[string]string{},
		})
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
	ctx := context.Background()
	ds := confluencespacepermdatasource.NewPermissionDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"space_id":       tftypes.NewValue(tftypes.String, "missing-space"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected space not found error")
	}
}

// ==================== RESTRICTION RESOURCE: Delete() 404 silent return ====================

// TestRestrictionResourceDelete404Silent covers the 404 silent return in restriction Delete().
func TestRestrictionResourceDelete404Silent(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Not found"},
			"errors":        map[string]string{},
		})
	}))
	defer ts.Close()

	client := confluenceCovClient(t, ts)
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
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected 404 on delete to succeed silently")
	}
}
