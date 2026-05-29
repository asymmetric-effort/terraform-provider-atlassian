// Package unit contains error path tests for Statuspage resources ensuring
// error messages are clear and cover all error branches.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	spcomponentds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/component"
	sppageds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/page"
	spsubscriberds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/subscriber"
	spcomponentrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/component"
	sppagers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/page"
	spsubscriberrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/subscriber"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// spErrorServer creates an httptest server that returns a specific error code for all requests.
func spErrorServer(t *testing.T, statusCode int, message string) *client.Client {
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
	auth, _ := client.NewAPIKeyAuthenticator("test-api-key")
	c, _ := client.NewClient(client.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)
	return c
}

// ============================================================================
// Page Resource Error Paths
// ============================================================================

// TestStatuspagePageCreateConflict tests 409 error path.
func TestStatuspagePageCreateConflict(t *testing.T) {
	t.Parallel()
	c := spErrorServer(t, 409, "Duplicate page")
	ctx := context.Background()
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":             tftypes.NewValue(tftypes.String, "Test"),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for 409")
	}
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Duplicate") {
			return
		}
	}
	t.Error("expected Duplicate error message")
}

// TestStatuspagePageCreateForbidden tests 403 error path.
func TestStatuspagePageCreateForbidden(t *testing.T) {
	t.Parallel()
	c := spErrorServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":             tftypes.NewValue(tftypes.String, "Test"),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for 403")
	}
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") {
			return
		}
	}
	t.Error("expected Permission denied message")
}

// TestStatuspagePageCreateGenericError tests generic error path.
func TestStatuspagePageCreateGenericError(t *testing.T) {
	t.Parallel()
	c := spErrorServer(t, 500, "Server error")
	ctx := context.Background()
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":             tftypes.NewValue(tftypes.String, "Test"),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for 500")
	}
}

// TestStatuspagePageReadGenericError tests generic error on read.
func TestStatuspagePageReadGenericError(t *testing.T) {
	t.Parallel()
	c := spErrorServer(t, 500, "Server error")
	ctx := context.Background()
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "page-1"),
		"name":             tftypes.NewValue(tftypes.String, ""),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for 500")
	}
}

// TestStatuspagePageUpdateForbidden tests 403 on update.
func TestStatuspagePageUpdateForbidden(t *testing.T) {
	t.Parallel()
	c := spErrorServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "page-1"),
		"name":             tftypes.NewValue(tftypes.String, ""),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":             tftypes.NewValue(tftypes.String, "Updated"),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for 403")
	}
}

// TestStatuspagePageUpdateGenericError tests generic error on update.
func TestStatuspagePageUpdateGenericError(t *testing.T) {
	t.Parallel()
	c := spErrorServer(t, 500, "Server error")
	ctx := context.Background()
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "page-1"),
		"name":             tftypes.NewValue(tftypes.String, ""),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":             tftypes.NewValue(tftypes.String, "Updated"),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for 500")
	}
}

// TestStatuspagePageDeleteForbidden tests 403 on delete.
func TestStatuspagePageDeleteForbidden(t *testing.T) {
	t.Parallel()
	c := spErrorServer(t, 403, "Forbidden")
	ctx := context.Background()
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "page-1"),
		"name":             tftypes.NewValue(tftypes.String, ""),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for 403")
	}
}

// TestStatuspagePageDeleteGenericError tests generic error on delete.
func TestStatuspagePageDeleteGenericError(t *testing.T) {
	t.Parallel()
	c := spErrorServer(t, 500, "Server error")
	ctx := context.Background()
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "page-1"),
		"name":             tftypes.NewValue(tftypes.String, ""),
		"page_description": tftypes.NewValue(tftypes.String, ""),
		"subdomain":        tftypes.NewValue(tftypes.String, ""),
		"url":              tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for 500")
	}
}

// ============================================================================
// Error tests for remaining resources: Component, ComponentGroup, Subscriber,
// IncidentTemplate, MaintenanceTemplate, Permission
// ============================================================================

// spResourceErrorTest is a generic error test for resources with page_id.
func spResourceErrorTest(t *testing.T, r resource.Resource, statusCode int, tfType tftypes.Object, planVals, stateVals map[string]tftypes.Value) {
	t.Helper()
	c := spErrorServer(t, statusCode, "Error")
	ctx := context.Background()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, planVals)}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Errorf("expected create error for %d", statusCode)
	}

	// Read
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, stateVals)}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	// 404 on read should remove resource, others should error
	if statusCode != 404 && !readResp.Diagnostics.HasError() {
		t.Errorf("expected read error for %d", statusCode)
	}

	// Update
	updateResp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Errorf("expected update error for %d", statusCode)
	}

	// Delete
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	// 404 on delete should NOT error (already gone)
	if statusCode != 404 && !deleteResp.Diagnostics.HasError() {
		t.Errorf("expected delete error for %d", statusCode)
	}
}

func componentTfType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String, "page_id": tftypes.String, "name": tftypes.String,
		"description": tftypes.String, "status": tftypes.String, "group_id": tftypes.String,
	}}
}

func componentGroupTfType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String, "page_id": tftypes.String, "name": tftypes.String,
		"description": tftypes.String,
	}}
}

func templateTfType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String, "page_id": tftypes.String, "name": tftypes.String,
		"title": tftypes.String, "body": tftypes.String,
	}}
}

func permissionTfType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String, "page_id": tftypes.String, "principal_type": tftypes.String,
		"principal_id": tftypes.String, "role": tftypes.String,
	}}
}

func subscriberTfType() tftypes.Object {
	return tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String, "page_id": tftypes.String, "email": tftypes.String,
		"endpoint": tftypes.String, "component_ids": tftypes.List{ElementType: tftypes.String},
	}}
}

// TestStatuspageComponentErrorPaths tests error paths for components.
func TestStatuspageComponentErrorPaths(t *testing.T) {
	t.Parallel()
	for _, code := range []int{403, 404, 500} {
		code := code
		t.Run(strings.Replace(string(rune(code+'0')), string(rune(code+'0')), fmt.Sprintf("%d", code), 1), func(t *testing.T) {
			t.Parallel()
			spResourceErrorTest(t, spcomponentrs.NewResource(), code, componentTfType(),
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"name": tftypes.NewValue(tftypes.String, "C"), "description": tftypes.NewValue(tftypes.String, ""),
					"status": tftypes.NewValue(tftypes.String, "operational"), "group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				},
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "comp-1"), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"name": tftypes.NewValue(tftypes.String, "C"), "description": tftypes.NewValue(tftypes.String, ""),
					"status": tftypes.NewValue(tftypes.String, "operational"), "group_id": tftypes.NewValue(tftypes.String, ""),
				},
			)
		})
	}
}

// TestStatuspageComponentGroupErrorPaths tests error paths.
func TestStatuspageComponentGroupErrorPaths(t *testing.T) {
	t.Parallel()
	for _, code := range []int{403, 404, 500} {
		code := code
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			t.Parallel()
			spResourceErrorTest(t, spcomponentrs.NewGroupResource(), code, componentGroupTfType(),
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"name": tftypes.NewValue(tftypes.String, "G"), "description": tftypes.NewValue(tftypes.String, ""),
				},
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "grp-1"), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"name": tftypes.NewValue(tftypes.String, "G"), "description": tftypes.NewValue(tftypes.String, ""),
				},
			)
		})
	}
}

// TestStatuspageIncidentTemplateErrorPaths tests error paths.
func TestStatuspageIncidentTemplateErrorPaths(t *testing.T) {
	t.Parallel()
	for _, code := range []int{403, 404, 500} {
		code := code
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			t.Parallel()
			spResourceErrorTest(t, sppagers.NewIncidentTemplateResource(), code, templateTfType(),
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"name": tftypes.NewValue(tftypes.String, "T"), "title": tftypes.NewValue(tftypes.String, ""),
					"body": tftypes.NewValue(tftypes.String, ""),
				},
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "it-1"), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"name": tftypes.NewValue(tftypes.String, "T"), "title": tftypes.NewValue(tftypes.String, ""),
					"body": tftypes.NewValue(tftypes.String, ""),
				},
			)
		})
	}
}

// TestStatuspageMaintenanceTemplateErrorPaths tests error paths.
func TestStatuspageMaintenanceTemplateErrorPaths(t *testing.T) {
	t.Parallel()
	for _, code := range []int{403, 404, 500} {
		code := code
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			t.Parallel()
			spResourceErrorTest(t, sppagers.NewMaintenanceTemplateResource(), code, templateTfType(),
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"name": tftypes.NewValue(tftypes.String, "T"), "title": tftypes.NewValue(tftypes.String, ""),
					"body": tftypes.NewValue(tftypes.String, ""),
				},
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "mt-1"), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"name": tftypes.NewValue(tftypes.String, "T"), "title": tftypes.NewValue(tftypes.String, ""),
					"body": tftypes.NewValue(tftypes.String, ""),
				},
			)
		})
	}
}

// TestStatuspagePermissionErrorPaths tests error paths.
func TestStatuspagePermissionErrorPaths(t *testing.T) {
	t.Parallel()
	for _, code := range []int{403, 404, 409, 500} {
		code := code
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			t.Parallel()
			spResourceErrorTest(t, sppagers.NewPermissionResource(), code, permissionTfType(),
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
					"role": tftypes.NewValue(tftypes.String, "admin"),
				},
				map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "perm-1"), "page_id": tftypes.NewValue(tftypes.String, "p1"),
					"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u1"),
					"role": tftypes.NewValue(tftypes.String, "admin"),
				},
			)
		})
	}
}

// TestStatuspageSubscriberErrorPaths tests error paths.
func TestStatuspageSubscriberErrorPaths(t *testing.T) {
	t.Parallel()
	for _, code := range []int{403, 404, 500} {
		code := code
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			t.Parallel()
			c := spErrorServer(t, code, "Error")
			ctx := context.Background()
			r := spsubscriberrs.NewResource()
			spConfigureResource(t, r, c)
			s := spGetResourceSchema(t, r)
			tfType := subscriberTfType()

			planVals := map[string]tftypes.Value{
				"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p1"),
				"email": tftypes.NewValue(tftypes.String, "e@e.com"), "endpoint": tftypes.NewValue(tftypes.String, ""),
				"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{}),
			}
			stateVals := map[string]tftypes.Value{
				"id": tftypes.NewValue(tftypes.String, "sub-1"), "page_id": tftypes.NewValue(tftypes.String, "p1"),
				"email": tftypes.NewValue(tftypes.String, "e@e.com"), "endpoint": tftypes.NewValue(tftypes.String, ""),
				"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{}),
			}

			plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, planVals)}
			createResp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
			if !createResp.Diagnostics.HasError() {
				t.Errorf("expected create error for %d", code)
			}

			state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, stateVals)}
			readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
			r.Read(ctx, resource.ReadRequest{State: state}, readResp)
			if code != 404 && !readResp.Diagnostics.HasError() {
				t.Errorf("expected read error for %d", code)
			}

			updateResp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
			r.Update(ctx, resource.UpdateRequest{State: state, Plan: plan}, updateResp)
			if !updateResp.Diagnostics.HasError() {
				t.Errorf("expected update error for %d", code)
			}

			deleteResp := &resource.DeleteResponse{}
			r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
			if code != 404 && !deleteResp.Diagnostics.HasError() {
				t.Errorf("expected delete error for %d", code)
			}
		})
	}
}

// TestStatuspageDataSourceReadErrors tests error paths for data sources.
func TestStatuspageDataSourceReadErrors(t *testing.T) {
	t.Parallel()
	for _, code := range []int{404, 500} {
		code := code
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			t.Parallel()
			c := spErrorServer(t, code, "Error")
			ctx := context.Background()

			// Page
			{
				d := sppageds.NewDataSource()
				spConfigureDS(t, d, c)
				ds := spGetDSSchema(t, d)
				dsTfType := ds.Type().TerraformType(ctx)
				config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
					"id":               tftypes.NewValue(tftypes.String, "x"),
					"name":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"page_description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"subdomain":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				})}
				resp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
				d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
				if !resp.Diagnostics.HasError() {
					t.Errorf("page ds: expected error for %d", code)
				}
			}

			// Component
			{
				d := spcomponentds.NewDataSource()
				spConfigureDS(t, d, c)
				ds := spGetDSSchema(t, d)
				dsTfType := ds.Type().TerraformType(ctx)
				config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
					"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				})}
				resp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
				d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
				if !resp.Diagnostics.HasError() {
					t.Errorf("component ds: expected error for %d", code)
				}
			}

			// Component Group
			{
				d := spcomponentds.NewGroupDataSource()
				spConfigureDS(t, d, c)
				ds := spGetDSSchema(t, d)
				dsTfType := ds.Type().TerraformType(ctx)
				config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
					"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				})}
				resp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
				d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
				if !resp.Diagnostics.HasError() {
					t.Errorf("component group ds: expected error for %d", code)
				}
			}

			// Subscriber
			{
				d := spsubscriberds.NewDataSource()
				spConfigureDS(t, d, c)
				ds := spGetDSSchema(t, d)
				dsTfType := ds.Type().TerraformType(ctx)
				config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
					"email": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "endpoint": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, tftypes.UnknownValue),
				})}
				resp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
				d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
				if !resp.Diagnostics.HasError() {
					t.Errorf("subscriber ds: expected error for %d", code)
				}
			}

			// Incident Template
			{
				d := sppageds.NewIncidentTemplateDataSource()
				spConfigureDS(t, d, c)
				ds := spGetDSSchema(t, d)
				dsTfType := ds.Type().TerraformType(ctx)
				config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
					"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "title": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				})}
				resp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
				d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
				if !resp.Diagnostics.HasError() {
					t.Errorf("incident template ds: expected error for %d", code)
				}
			}

			// Maintenance Template
			{
				d := sppageds.NewMaintenanceTemplateDataSource()
				spConfigureDS(t, d, c)
				ds := spGetDSSchema(t, d)
				dsTfType := ds.Type().TerraformType(ctx)
				config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
					"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "title": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"body": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				})}
				resp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
				d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
				if !resp.Diagnostics.HasError() {
					t.Errorf("maintenance template ds: expected error for %d", code)
				}
			}

			// Permission
			{
				d := sppageds.NewPermissionDataSource()
				spConfigureDS(t, d, c)
				ds := spGetDSSchema(t, d)
				dsTfType := ds.Type().TerraformType(ctx)
				config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
					"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
					"principal_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "principal_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
					"role": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				})}
				resp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
				d.Read(ctx, datasource.ReadRequest{Config: config}, resp)
				if !resp.Diagnostics.HasError() {
					t.Errorf("permission ds: expected error for %d", code)
				}
			}
		})
	}
}
