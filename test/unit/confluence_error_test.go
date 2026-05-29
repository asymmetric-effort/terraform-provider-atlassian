// Package unit contains tests that verify Confluence resource error messages are
// clear, user-friendly, and consistent across all Confluence resource types.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	confluencepageresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/page"
	confluencespacepermresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/space"
	confluencetemplateresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/template"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// confluenceErrorClient creates a mock server returning a fixed HTTP status and message.
func confluenceErrorClient(t *testing.T, statusCode int, message string) *atlassian.Client {
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
	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	c, _ := atlassian.NewClient(atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)
	return c
}

// --- Confluence Space: not found, duplicate key, permission denied ---

func TestConfluenceSpaceNotFoundRead(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 404, "Space not found")
	ctx := context.Background()
	r := confluencespacepermresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "nonexistent-id"),
		"key":         tftypes.NewValue(tftypes.String, "GONE"),
		"name":        tftypes.NewValue(tftypes.String, "Gone Space"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, "current"),
		"url":         tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: emptyState(ctx, s)}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	// 404 on Read should remove the resource from state (no error)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected 404 Read to silently remove resource, but got error")
	}
}

func TestConfluenceSpaceDuplicateKey(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 409, "Space key already exists")
	ctx := context.Background()
	r := confluencespacepermresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "DUP"),
		"name":        tftypes.NewValue(tftypes.String, "Duplicate"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected duplicate key error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Duplicate space key") &&
			strings.Contains(d.Detail(), "DUP") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the duplicate key. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluenceSpacePermissionDeniedCreate(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := confluencespacepermresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "NOPERM"),
		"name":        tftypes.NewValue(tftypes.String, "NoPerms"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "Confluence") &&
			strings.Contains(d.Detail(), "admin privileges") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention Confluence and admin privileges. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Confluence Page: not found, permission denied ---

func TestConfluencePageNotFoundUpdate(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 404, "Page not found")
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "page-404"),
		"space_id":  tftypes.NewValue(tftypes.String, "space-1"),
		"title":     tftypes.NewValue(tftypes.String, "Missing Page"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Confluence page not found") &&
			strings.Contains(d.Detail(), "page-404") &&
			strings.Contains(d.Detail(), "deleted outside of Terraform") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the page and suggest it may be deleted. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluencePagePermissionDeniedCreate(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":  tftypes.NewValue(tftypes.String, "space-1"),
		"title":     tftypes.NewValue(tftypes.String, "Forbidden Page"),
		"body":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version":   tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "Confluence") &&
			strings.Contains(d.Detail(), "admin privileges") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention Confluence and admin privileges. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluencePagePermissionDeniedDelete(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := confluencepageresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "page-del-403"),
		"space_id":  tftypes.NewValue(tftypes.String, "space-1"),
		"title":     tftypes.NewValue(tftypes.String, "Protected Page"),
		"body":      tftypes.NewValue(tftypes.String, ""),
		"parent_id": tftypes.NewValue(tftypes.String, ""),
		"status":    tftypes.NewValue(tftypes.String, "current"),
		"version":   tftypes.NewValue(tftypes.Number, 1),
	})}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "page-del-403") &&
			strings.Contains(d.Detail(), "admin privileges") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the page and suggest admin privileges. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Confluence Template: not found, permission denied ---

func TestConfluenceTemplateNotFoundUpdate(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 404, "Template not found")
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "tmpl-404"),
		"name":          tftypes.NewValue(tftypes.String, "Missing Template"),
		"description":   tftypes.NewValue(tftypes.String, ""),
		"body":          tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"template_type": tftypes.NewValue(tftypes.String, "page"),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected not found error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Confluence template not found") &&
			strings.Contains(d.Detail(), "tmpl-404") &&
			strings.Contains(d.Detail(), "deleted outside of Terraform") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the template and suggest it may be deleted. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluenceTemplateNotFoundRead(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 404, "Template not found")
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "tmpl-gone"),
		"name":          tftypes.NewValue(tftypes.String, "Gone Template"),
		"description":   tftypes.NewValue(tftypes.String, ""),
		"body":          tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"template_type": tftypes.NewValue(tftypes.String, "page"),
	})}
	resp := &resource.ReadResponse{State: emptyState(ctx, s)}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	// 404 on Read should remove the resource from state (no error)
	if resp.Diagnostics.HasError() {
		t.Fatal("Expected 404 Read to silently remove resource, but got error")
	}
}

func TestConfluenceTemplatePermissionDeniedCreate(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := confluencetemplateresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Forbidden Template"),
		"description":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"body":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"template_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "Confluence") &&
			strings.Contains(d.Detail(), "admin privileges") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention Confluence and admin privileges. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Confluence Space Permission: invalid, not found, conflict ---

func TestConfluenceSpacePermissionInvalid(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 400, "Invalid permission type")
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "invalid"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "invalid_op"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected invalid permission error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid Confluence space permission") &&
			strings.Contains(d.Detail(), "principal_type") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should describe the invalid permission. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluenceSpacePermissionSpaceNotFound(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 404, "Space not found")
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":       tftypes.NewValue(tftypes.String, "missing-space"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected space not found error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Confluence space not found") &&
			strings.Contains(d.Detail(), "missing-space") &&
			strings.Contains(d.Detail(), "Verify the space exists") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the missing space and suggest verification. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluenceSpacePermissionDuplicate(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 409, "Permission already exists")
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected duplicate permission error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Duplicate Confluence space permission") &&
			strings.Contains(d.Detail(), "user-1") &&
			strings.Contains(d.Detail(), "space-1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the duplicate permission. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluenceSpacePermissionPermissionDenied(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := confluencespacepermresource.NewPermissionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":       tftypes.NewValue(tftypes.String, "space-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "Confluence") &&
			strings.Contains(d.Detail(), "admin privileges") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should mention Confluence and admin privileges. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Confluence Content Restriction: conflict, permission denied ---

func TestConfluenceContentRestrictionConflict(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 409, "Restriction conflict")
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"content_id":     tftypes.NewValue(tftypes.String, "page-1"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected restriction conflict error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Confluence content restriction conflict") &&
			strings.Contains(d.Detail(), "page-1") &&
			strings.Contains(d.Detail(), "read") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the page and operation. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluenceContentRestrictionPermissionDenied(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 403, "Forbidden")
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"content_id":     tftypes.NewValue(tftypes.String, "page-1"),
		"operation":      tftypes.NewValue(tftypes.String, "update"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") &&
			strings.Contains(d.Detail(), "page-1") &&
			strings.Contains(d.Detail(), "admin privileges") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the page and suggest admin privileges. Got: %v", resp.Diagnostics.Errors())
	}
}

func TestConfluenceContentRestrictionPageNotFound(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 404, "Content not found")
	ctx := context.Background()
	r := confluencepageresource.NewRestrictionResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"content_id":     tftypes.NewValue(tftypes.String, "page-missing"),
		"operation":      tftypes.NewValue(tftypes.String, "read"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected content not found error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Confluence page not found") &&
			strings.Contains(d.Detail(), "page-missing") &&
			strings.Contains(d.Detail(), "Verify the page exists") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Error message should identify the missing page. Got: %v", resp.Diagnostics.Errors())
	}
}

// --- Error messages never expose raw API JSON ---

func TestConfluenceErrorDoesNotExposeRawJSON(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 500, "Internal server error")
	ctx := context.Background()
	r := confluencespacepermresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "ERRTEST"),
		"name":        tftypes.NewValue(tftypes.String, "Error Test"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for 500 response")
	}
	for _, d := range resp.Diagnostics.Errors() {
		detail := d.Detail()
		if strings.Contains(detail, `"errorMessages"`) ||
			strings.Contains(detail, `"errors":{`) {
			t.Fatalf("Error message exposes raw API JSON: %s", detail)
		}
		if !strings.Contains(detail, "ERRTEST") {
			t.Fatalf("Error message should identify the affected resource. Got: %s", detail)
		}
	}
}

// --- Confluence error messages have consistent format ---

func TestConfluenceErrorMessageConsistency(t *testing.T) {
	t.Parallel()
	client := confluenceErrorClient(t, 403, "Forbidden")
	ctx := context.Background()

	// Test Confluence space
	sr := confluencespacepermresource.NewResource()
	configureResource(t, sr, client)
	ss := getResourceSchema(t, sr)
	stfType := ss.Type().TerraformType(ctx)
	sPlan := tfsdk.Plan{Schema: ss, Raw: tftypes.NewValue(stfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"key":         tftypes.NewValue(tftypes.String, "CONS"),
		"name":        tftypes.NewValue(tftypes.String, "Consistent"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"type":        tftypes.NewValue(tftypes.String, "global"),
		"homepage_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	sResp := &resource.CreateResponse{State: emptyState(ctx, ss)}
	sr.Create(ctx, resource.CreateRequest{Plan: sPlan}, sResp)

	// Test Confluence page
	pr := confluencepageresource.NewResource()
	configureResource(t, pr, client)
	ps := getResourceSchema(t, pr)
	ptfType := ps.Type().TerraformType(ctx)
	pPlan := tfsdk.Plan{Schema: ps, Raw: tftypes.NewValue(ptfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":  tftypes.NewValue(tftypes.String, "space-1"),
		"title":     tftypes.NewValue(tftypes.String, "Consistent Page"),
		"body":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"parent_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"version":   tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
	})}
	pResp := &resource.CreateResponse{State: emptyState(ctx, ps)}
	pr.Create(ctx, resource.CreateRequest{Plan: pPlan}, pResp)

	// Test Confluence template
	tr := confluencetemplateresource.NewResource()
	configureResource(t, tr, client)
	ts := getResourceSchema(t, tr)
	ttfType := ts.Type().TerraformType(ctx)
	tPlan := tfsdk.Plan{Schema: ts, Raw: tftypes.NewValue(ttfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Consistent Template"),
		"description":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"body":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"template_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	tResp := &resource.CreateResponse{State: emptyState(ctx, ts)}
	tr.Create(ctx, resource.CreateRequest{Plan: tPlan}, tResp)

	// All should have "Permission denied" summary and mention "admin privileges"
	for name, resp := range map[string]*resource.CreateResponse{
		"space":    sResp,
		"page":     pResp,
		"template": tResp,
	} {
		if !resp.Diagnostics.HasError() {
			t.Fatalf("%s: Expected permission denied error", name)
		}
		for _, d := range resp.Diagnostics.Errors() {
			if d.Summary() != "Permission denied" {
				t.Fatalf("%s: Inconsistent error summary: expected 'Permission denied', got %q", name, d.Summary())
			}
			if !strings.Contains(d.Detail(), "admin privileges") {
				t.Fatalf("%s: Error message should mention admin privileges: %s", name, d.Detail())
			}
			if !strings.Contains(d.Detail(), "Confluence") {
				t.Fatalf("%s: Error message should mention Confluence: %s", name, d.Detail())
			}
		}
	}
}
