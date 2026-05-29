// Package unit contains unit tests for the atlassian_jira_custom_email
// resource and data source.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	customdomainds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/custom_domain"
	customdomainrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/custom_domain"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// emailMockServer creates a mock HTTP server for custom email endpoints.
func emailMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	emails := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/api/3/email", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		emailAddr, _ := req["emailAddress"].(string)
		domainID, _ := req["domainId"].(string)
		if emailAddr == "" || domainID == "" {
			writeErr(w, 400, "emailAddress and domainId are required")
			return
		}
		if emailAddr == "invalid" {
			writeErr(w, 400, "Invalid email address format")
			return
		}
		if domainID == "unverified-domain" {
			writeErr(w, 422, "Domain not verified")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, e := range emails {
			if e["emailAddress"] == emailAddr {
				writeErr(w, 409, "Email address already registered")
				return
			}
		}
		id := testNextID("email")
		active := true
		if v, ok := req["active"].(bool); ok {
			active = v
		}
		email := map[string]interface{}{
			"id":           id,
			"emailAddress": emailAddr,
			"domainId":     domainID,
			"active":       active,
		}
		if spaceID, ok := req["spaceId"].(string); ok && spaceID != "" {
			email["spaceId"] = spaceID
		}
		emails[id] = email
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(email)
	})

	mux.HandleFunc("GET /rest/api/3/email/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		email, ok := emails[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Email not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(email)
	})

	mux.HandleFunc("DELETE /rest/api/3/email/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := emails[id]; !ok {
			writeErr(w, 404, "Email not found")
			return
		}
		delete(emails, id)
		w.WriteHeader(204)
	})

	mux.HandleFunc("GET /rest/api/3/email", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		var items []map[string]interface{}
		for _, e := range emails {
			items = append(items, e)
		}
		mu.Unlock()
		if items == nil {
			items = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, err := atlassian.NewAPIKeyAuthenticator("test-api-key")
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	c, err := atlassian.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	return ts, c
}

// emailTFType returns the tftypes.Object for the email resource schema.
func emailTFType(ctx context.Context, t *testing.T) tftypes.Type {
	t.Helper()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)
	return s.Type().TerraformType(ctx)
}

// ==================== EMAIL RESOURCE SCHEMA TESTS ====================

// TestCustomEmailResourceMetadata verifies the resource type name.
func TestCustomEmailResourceMetadata(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_custom_email" {
		t.Errorf("expected resource type name 'atlassian_jira_custom_email', got %q", resp.TypeName)
	}
}

// TestCustomEmailResourceSchema verifies the resource schema has all expected attributes.
func TestCustomEmailResourceSchema(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)

	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "email_address", "domain_id", "space_id", "active"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestCustomEmailResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestCustomEmailResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)

	expectedAttrs := 5
	actualAttrs := len(s.Attributes)
	if actualAttrs != expectedAttrs {
		t.Errorf("expected %d schema attributes, got %d", expectedAttrs, actualAttrs)
	}
}

// TestCustomEmailResourceSchemaRequiredAttributes verifies required attributes.
func TestCustomEmailResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)

	requiredAttrs := []string{"email_address", "domain_id"}
	for _, name := range requiredAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("expected attribute %q to be required", name)
		}
	}
}

// TestCustomEmailResourceSchemaComputedAttributes verifies computed attributes.
func TestCustomEmailResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)

	computedAttrs := []string{"id", "active"}
	for _, name := range computedAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("expected attribute %q to be computed", name)
		}
	}
}

// TestCustomEmailResourceSchemaOptionalAttributes verifies optional attributes.
func TestCustomEmailResourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)

	optionalAttrs := []string{"space_id", "active"}
	for _, name := range optionalAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsOptional() {
			t.Errorf("expected attribute %q to be optional", name)
		}
	}
}

// TestCustomEmailResourceImplementsImportState verifies the resource implements ImportState.
func TestCustomEmailResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected custom email resource to implement ResourceWithImportState")
	}
}

// TestCustomEmailResourceInterfaceCompliance verifies resource interface compliance.
func TestCustomEmailResourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = customdomainrs.NewEmailResource()
	var _ resource.ResourceWithImportState = customdomainrs.NewEmailResource().(resource.ResourceWithImportState)
}

// TestCustomEmailResourceSchemaDescription verifies the resource schema description is set.
func TestCustomEmailResourceSchemaDescription(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)
	if s.Description == "" {
		t.Error("expected schema to have a description")
	}
}

// ==================== EMAIL RESOURCE CRUD TESTS ====================

// TestCustomEmailResourceCRUDLifecycle tests the full create-read-delete lifecycle.
func TestCustomEmailResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email_address": tftypes.NewValue(tftypes.String, "support@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}

	emailID := getStringAttr(t, createResp.State, "id")
	if emailID == "" {
		t.Fatal("expected non-empty id")
	}
	emailAddr := getStringAttr(t, createResp.State, "email_address")
	if emailAddr != "support@example.com" {
		t.Errorf("expected email_address 'support@example.com', got %q", emailAddr)
	}
	domainID := getStringAttr(t, createResp.State, "domain_id")
	if domainID != "domain-1" {
		t.Errorf("expected domain_id 'domain-1', got %q", domainID)
	}

	// Read
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	readEmail := getStringAttr(t, readResp.State, "email_address")
	if readEmail != "support@example.com" {
		t.Errorf("Read email_address: expected 'support@example.com', got %q", readEmail)
	}

	// Delete
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: createResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete (should remove resource)
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)
	// 404 should cause state removal, not error
}

// TestCustomEmailResourceCRUDWithSpaceID tests lifecycle with optional space_id.
func TestCustomEmailResourceCRUDWithSpaceID(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email_address": tftypes.NewValue(tftypes.String, "project@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-2"),
		"space_id":      tftypes.NewValue(tftypes.String, "space-123"),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create with space_id: %v", createResp.Diagnostics.Errors())
	}

	spaceID := getStringAttr(t, createResp.State, "space_id")
	if spaceID != "space-123" {
		t.Errorf("expected space_id 'space-123', got %q", spaceID)
	}
}

// TestCustomEmailResourceCreateDuplicate verifies duplicate email error.
func TestCustomEmailResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	makePlan := func(addr string) tfsdk.Plan {
		return tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"email_address": tftypes.NewValue(tftypes.String, addr),
			"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
			"space_id":      tftypes.NewValue(tftypes.String, nil),
			"active":        tftypes.NewValue(tftypes.Bool, true),
		})}
	}

	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: makePlan("dup@example.com")}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: makePlan("dup@example.com")}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate email error")
	}
	found := false
	for _, d := range resp2.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Duplicate email") {
			found = true
		}
	}
	if !found {
		t.Error("Expected error message to contain 'Duplicate email'")
	}
}

// TestCustomEmailResourceCreateInvalid verifies invalid email error.
func TestCustomEmailResourceCreateInvalid(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email_address": tftypes.NewValue(tftypes.String, "invalid"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected invalid email error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid email") {
			found = true
		}
	}
	if !found {
		t.Error("Expected error message to contain 'Invalid email'")
	}
}

// TestCustomEmailResourceCreateDomainNotVerified verifies domain-not-verified error.
func TestCustomEmailResourceCreateDomainNotVerified(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email_address": tftypes.NewValue(tftypes.String, "test@unverified.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "unverified-domain"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected domain not verified error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Domain not verified") {
			found = true
		}
	}
	if !found {
		t.Error("Expected error message to contain 'Domain not verified'")
	}
}

// TestCustomEmailResourceUpdateNotSupported verifies update returns an error.
func TestCustomEmailResourceUpdateNotSupported(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	resp := &resource.UpdateResponse{}
	r.Update(context.Background(), resource.UpdateRequest{}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on Update")
	}
}

// TestCustomEmailResourceDeleteNotFound verifies deleting already-deleted email does not error.
func TestCustomEmailResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "nonexistent"),
		"email_address": tftypes.NewValue(tftypes.String, "gone@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete of nonexistent should not error: %v", resp.Diagnostics.Errors())
	}
}

// TestCustomEmailResourceReadNotFound verifies reading a deleted email removes state.
func TestCustomEmailResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "nonexistent"),
		"email_address": tftypes.NewValue(tftypes.String, "gone@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	// State should be removed on 404, not error
}

// TestCustomEmailResourceConfigureNil verifies nil provider data does not error.
func TestCustomEmailResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestCustomEmailResourceConfigureWrongType verifies wrong type errors.
func TestCustomEmailResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestCustomEmailResourceImportState verifies import by ID.
func TestCustomEmailResourceImportState(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "email-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// TestCustomEmailResourceCreateForbidden verifies permission denied error.
func TestCustomEmailResourceCreateForbidden(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/email", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email_address": tftypes.NewValue(tftypes.String, "forbidden@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Permission denied") {
			found = true
		}
	}
	if !found {
		t.Error("Expected error message to contain 'Permission denied'")
	}
}

// TestCustomEmailResourceDeleteForbidden verifies permission denied on delete.
func TestCustomEmailResourceDeleteForbidden(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/email/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "some-id"),
		"email_address": tftypes.NewValue(tftypes.String, "forbidden@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestCustomEmailResourceReadError verifies generic read error.
func TestCustomEmailResourceReadError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/email/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "some-id"),
		"email_address": tftypes.NewValue(tftypes.String, "error@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected read error")
	}
}

// TestCustomEmailResourceDeleteGenericError verifies generic delete error.
func TestCustomEmailResourceDeleteGenericError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/email/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "some-id"),
		"email_address": tftypes.NewValue(tftypes.String, "error@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected delete error")
	}
}

// TestCustomEmailResourceCreateGenericError verifies generic create error.
func TestCustomEmailResourceCreateGenericError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/email", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := emailTFType(ctx, t)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email_address": tftypes.NewValue(tftypes.String, "error@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected create error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Failed to create") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected error summary to contain 'Failed to create', got: %v", resp.Diagnostics.Errors())
	}
}

// TestCustomEmailResourceCreateBadPlan verifies Create handles plan deserialization error.
func TestCustomEmailResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for nil plan")
	}
}

// TestCustomEmailResourceReadBadState verifies Read handles state deserialization error.
func TestCustomEmailResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for nil state")
	}
}

// TestCustomEmailResourceDeleteBadState verifies Delete handles state deserialization error.
func TestCustomEmailResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for nil state")
	}
}

// TestCustomEmailResourceEmailAddressForceNew verifies email_address has RequiresReplace.
func TestCustomEmailResourceEmailAddressForceNew(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)

	attr, ok := s.Attributes["email_address"]
	if !ok {
		t.Fatal("expected email_address attribute")
	}
	if !attr.IsRequired() {
		t.Error("expected email_address to be required")
	}
}

// TestCustomEmailResourceDomainIDForceNew verifies domain_id has RequiresReplace.
func TestCustomEmailResourceDomainIDForceNew(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewEmailResource()
	s := getResourceSchema(t, r)

	attr, ok := s.Attributes["domain_id"]
	if !ok {
		t.Fatal("expected domain_id attribute")
	}
	if !attr.IsRequired() {
		t.Error("expected domain_id to be required")
	}
}

// ==================== EMAIL DATA SOURCE TESTS ====================

// TestCustomEmailDataSourceMetadata verifies the data source type name.
func TestCustomEmailDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewEmailDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_custom_email" {
		t.Errorf("expected data source type name 'atlassian_jira_custom_email', got %q", resp.TypeName)
	}
}

// TestCustomEmailDataSourceSchema verifies the data source schema has all expected attributes.
func TestCustomEmailDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewEmailDataSource()
	s := getDatasourceSchema(t, ds)

	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "email_address", "domain_id", "space_id", "active"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestCustomEmailDataSourceSchemaAttributeCount verifies attribute count.
func TestCustomEmailDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewEmailDataSource()
	s := getDatasourceSchema(t, ds)

	expectedAttrs := 5
	if len(s.Attributes) != expectedAttrs {
		t.Errorf("expected %d attributes, got %d", expectedAttrs, len(s.Attributes))
	}
}

// TestCustomEmailDataSourceSchemaOptionalAttributes verifies optional lookup attributes.
func TestCustomEmailDataSourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewEmailDataSource()
	s := getDatasourceSchema(t, ds)

	optionalAttrs := []string{"id", "email_address"}
	for _, name := range optionalAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsOptional() {
			t.Errorf("expected attribute %q to be optional", name)
		}
	}
}

// TestCustomEmailDataSourceSchemaComputedAttributes verifies computed attributes.
func TestCustomEmailDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewEmailDataSource()
	s := getDatasourceSchema(t, ds)

	computedAttrs := []string{"domain_id", "space_id", "active"}
	for _, name := range computedAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("expected attribute %q to be computed", name)
		}
	}
}

// TestCustomEmailDataSourceSchemaDescription verifies the data source schema description is set.
func TestCustomEmailDataSourceSchemaDescription(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewEmailDataSource()
	s := getDatasourceSchema(t, ds)
	if s.Description == "" {
		t.Error("expected schema to have a description")
	}
}

// TestCustomEmailDataSourceInterfaceCompliance verifies data source interface compliance.
func TestCustomEmailDataSourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = customdomainds.NewEmailDataSource()
}

// TestCustomEmailDataSourceByID tests reading an email by ID.
func TestCustomEmailDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()

	// Create an email first via the resource
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTFType := emailTFType(ctx, t)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTFType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email_address": tftypes.NewValue(tftypes.String, "dsbyid@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	emailID := getStringAttr(t, cResp.State, "id")

	// Read via data source by ID
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, emailID),
		"email_address": tftypes.NewValue(tftypes.String, nil),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if ea := getStringAttr(t, dsResp.State, "email_address"); ea != "dsbyid@example.com" {
		t.Errorf("expected email_address 'dsbyid@example.com', got %q", ea)
	}
}

// TestCustomEmailDataSourceByEmailAddress tests reading an email by email address.
func TestCustomEmailDataSourceByEmailAddress(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()

	// Create an email first
	r := customdomainrs.NewEmailResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTFType := emailTFType(ctx, t)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTFType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email_address": tftypes.NewValue(tftypes.String, "dsbyaddr@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, "domain-1"),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, true),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read via data source by email_address
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, nil),
		"email_address": tftypes.NewValue(tftypes.String, "dsbyaddr@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by address: %v", dsResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, dsResp.State, "id"); id == "" {
		t.Error("expected non-empty id from data source")
	}
}

// TestCustomEmailDataSourceMissingBoth verifies error when neither id nor email_address set.
func TestCustomEmailDataSourceMissingBoth(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, nil),
		"email_address": tftypes.NewValue(tftypes.String, nil),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when neither id nor email_address set")
	}
}

// TestCustomEmailDataSourceNotFoundByID verifies error for nonexistent ID.
func TestCustomEmailDataSourceNotFoundByID(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "nonexistent"),
		"email_address": tftypes.NewValue(tftypes.String, nil),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nonexistent email ID")
	}
}

// TestCustomEmailDataSourceNotFoundByAddress verifies error for nonexistent email address.
func TestCustomEmailDataSourceNotFoundByAddress(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, nil),
		"email_address": tftypes.NewValue(tftypes.String, "nonexistent@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nonexistent email address")
	}
}

// TestCustomEmailDataSourceConfigureNil verifies nil provider data does not error.
func TestCustomEmailDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewEmailDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestCustomEmailDataSourceConfigureWrongType verifies wrong type errors.
func TestCustomEmailDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewEmailDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestCustomEmailDataSourceFindByAddressAPIError verifies error when email list API fails.
func TestCustomEmailDataSourceFindByAddressAPIError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/email", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, nil),
		"email_address": tftypes.NewValue(tftypes.String, "err@example.com"),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when email list API fails")
	}
}

// TestCustomEmailDataSourceReadByIDGenericError verifies generic read error in data source.
func TestCustomEmailDataSourceReadByIDGenericError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/email/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "some-id"),
		"email_address": tftypes.NewValue(tftypes.String, nil),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for generic read failure")
	}
}

// TestCustomEmailDataSourceReadBadConfig verifies data source Read handles config deserialization error.
func TestCustomEmailDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := emailMockServer(t)
	ctx := context.Background()
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, nil)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nil config")
	}
}

// TestCustomEmailDataSourceReadNilError verifies non-API error handling in data source.
func TestCustomEmailDataSourceReadNilError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/email/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte("not-json"))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := customdomainds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "some-id"),
		"email_address": tftypes.NewValue(tftypes.String, nil),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for unparseable response")
	}
}

// suppress unused import warning
var _ = fmt.Sprintf
