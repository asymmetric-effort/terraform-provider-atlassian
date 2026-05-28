// Package unit contains unit tests for the atlassian_jira_incoming_mail_handler
// and atlassian_jira_outgoing_mail_handler resources and data sources.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	mailhandlerdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/mail_handler"
	mailhandlerresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/mail_handler"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// mailIDCounter provides unique IDs for mail handler mock server tests.
var mailIDCounter uint64

func mailNextID(prefix string) string {
	n := atomic.AddUint64(&mailIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// mailMockServer creates a mock HTTP server for Jira mail handler endpoints.
func mailMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	incomingHandlers := make(map[string]map[string]interface{})
	outgoingHandlers := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// ==================== INCOMING HANDLERS ====================

	mux.HandleFunc("POST /rest/api/3/mailhandler/incoming", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			writeErr(w, 400, "name is required")
			return
		}
		server, _ := req["server"].(string)
		if server == "" {
			writeErr(w, 400, "server is required")
			return
		}
		protocol, _ := req["protocol"].(string)
		if protocol != "IMAP" && protocol != "POP3" {
			writeErr(w, 400, fmt.Sprintf("Invalid protocol %q: must be IMAP or POP3", protocol))
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := mailNextID("incoming")
		enabled, _ := req["enabled"].(bool)
		port, _ := req["port"].(float64)
		username, _ := req["username"].(string)
		folder, _ := req["folder"].(string)
		spaceID, _ := req["spaceId"].(string)
		issueTypeID, _ := req["issueTypeId"].(string)
		handler := map[string]interface{}{
			"id":          id,
			"name":        name,
			"enabled":     enabled,
			"server":      server,
			"port":        port,
			"protocol":    protocol,
			"username":    username,
			"password":    "",
			"folder":      folder,
			"spaceId":     spaceID,
			"issueTypeId": issueTypeID,
		}
		incomingHandlers[id] = handler
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(handler)
	})

	mux.HandleFunc("GET /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		handler, ok := incomingHandlers[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Incoming mail handler not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(handler)
	})

	mux.HandleFunc("PUT /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		handler, ok := incomingHandlers[id]
		if !ok {
			mu.Unlock()
			writeErr(w, 404, "Incoming mail handler not found")
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				handler[k] = v
			}
		}
		// Don't return password
		handler["password"] = ""
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(handler)
	})

	mux.HandleFunc("DELETE /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := incomingHandlers[id]; !ok {
			writeErr(w, 404, "Incoming mail handler not found")
			return
		}
		delete(incomingHandlers, id)
		w.WriteHeader(204)
	})

	// ==================== OUTGOING HANDLERS ====================

	mux.HandleFunc("POST /rest/api/3/mailhandler/outgoing", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			writeErr(w, 400, "name is required")
			return
		}
		fromAddr, _ := req["fromAddress"].(string)
		if fromAddr == "" {
			writeErr(w, 400, "fromAddress is required")
			return
		}
		smtpHost, _ := req["smtpHost"].(string)
		if smtpHost == "" {
			writeErr(w, 400, "smtpHost is required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := mailNextID("outgoing")
		smtpPort, _ := req["smtpPort"].(float64)
		prefix, _ := req["prefix"].(string)
		protocol, _ := req["protocol"].(string)
		username, _ := req["username"].(string)
		tls, _ := req["tls"].(bool)
		handler := map[string]interface{}{
			"id":          id,
			"name":        name,
			"fromAddress": fromAddr,
			"prefix":      prefix,
			"smtpHost":    smtpHost,
			"smtpPort":    smtpPort,
			"protocol":    protocol,
			"username":    username,
			"tls":         tls,
		}
		outgoingHandlers[id] = handler
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(handler)
	})

	mux.HandleFunc("GET /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		handler, ok := outgoingHandlers[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Outgoing mail handler not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(handler)
	})

	mux.HandleFunc("PUT /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		handler, ok := outgoingHandlers[id]
		if !ok {
			mu.Unlock()
			writeErr(w, 404, "Outgoing mail handler not found")
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				handler[k] = v
			}
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(handler)
	})

	mux.HandleFunc("DELETE /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := outgoingHandlers[id]; !ok {
			writeErr(w, 404, "Outgoing mail handler not found")
			return
		}
		delete(outgoingHandlers, id)
		w.WriteHeader(204)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, err := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
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

// ==================== INCOMING RESOURCE SCHEMA TESTS ====================

// TestIncomingMailHandlerResourceMetadata verifies the resource type name.
func TestIncomingMailHandlerResourceMetadata(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewIncomingResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_incoming_mail_handler" {
		t.Errorf("expected resource type name 'atlassian_jira_incoming_mail_handler', got %q", resp.TypeName)
	}
}

// TestIncomingMailHandlerResourceSchema verifies the resource schema.
func TestIncomingMailHandlerResourceSchema(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewIncomingResource()
	s := getResourceSchema(t, r)
	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "enabled", "server", "port", "protocol", "username", "password", "folder", "space_id", "issue_type_id"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestIncomingMailHandlerResourceSchemaAttributeCount verifies attribute count.
func TestIncomingMailHandlerResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewIncomingResource()
	s := getResourceSchema(t, r)
	expected := 11
	actual := len(s.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestIncomingMailHandlerResourceSchemaRequiredAttributes verifies required attributes.
func TestIncomingMailHandlerResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewIncomingResource()
	s := getResourceSchema(t, r)
	requiredAttrs := []string{"name", "server", "port", "protocol", "username", "password"}
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

// TestIncomingMailHandlerResourceSensitiveFields verifies sensitive fields are marked.
func TestIncomingMailHandlerResourceSensitiveFields(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewIncomingResource()
	s := getResourceSchema(t, r)
	sensitiveAttrs := []string{"password"}
	for _, name := range sensitiveAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsSensitive() {
			t.Errorf("expected attribute %q to be marked sensitive", name)
		}
	}
}

// TestIncomingMailHandlerResourceImplementsImportState verifies ImportState is implemented.
func TestIncomingMailHandlerResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewIncomingResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected incoming mail handler resource to implement ResourceWithImportState")
	}
}

// TestIncomingMailHandlerResourceInterfaceCompliance verifies interface compliance.
func TestIncomingMailHandlerResourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = mailhandlerresource.NewIncomingResource()
	var _ resource.ResourceWithImportState = mailhandlerresource.NewIncomingResource().(resource.ResourceWithImportState)
}

// TestIncomingMailHandlerResourceConfigureNil verifies nil provider data is handled.
func TestIncomingMailHandlerResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewIncomingResource()
	resp := &resource.ConfigureResponse{}
	r.(interface {
		Configure(context.Context, resource.ConfigureRequest, *resource.ConfigureResponse)
	}).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("expected no error for nil provider data, got %v", resp.Diagnostics.Errors())
	}
}

// TestIncomingMailHandlerResourceConfigureWrongType verifies wrong provider data type is handled.
func TestIncomingMailHandlerResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewIncomingResource()
	resp := &resource.ConfigureResponse{}
	r.(interface {
		Configure(context.Context, resource.ConfigureRequest, *resource.ConfigureResponse)
	}).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

// ==================== INCOMING RESOURCE CRUD TESTS ====================

// TestIncomingMailHandlerResourceCRUDLifecycle tests the full create-read-update-delete lifecycle.
func TestIncomingMailHandlerResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Support Inbox"),
		"enabled":       tftypes.NewValue(tftypes.Bool, true),
		"server":        tftypes.NewValue(tftypes.String, "imap.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "support@example.com"),
		"password":      tftypes.NewValue(tftypes.String, "secret123"),
		"folder":        tftypes.NewValue(tftypes.String, "INBOX"),
		"space_id":      tftypes.NewValue(tftypes.String, "SP-1"),
		"issue_type_id": tftypes.NewValue(tftypes.String, "10001"),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty id after create")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Support Inbox" {
		t.Errorf("expected name 'Support Inbox', got %q", name)
	}

	// Read
	readResp := &resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if readID := getStringAttr(t, readResp.State, "id"); readID != id {
		t.Errorf("expected id %q, got %q", id, readID)
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, id),
		"name":          tftypes.NewValue(tftypes.String, "Updated Inbox"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "imap2.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 143),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "support@example.com"),
		"password":      tftypes.NewValue(tftypes.String, "newpassword"),
		"folder":        tftypes.NewValue(tftypes.String, "Support"),
		"space_id":      tftypes.NewValue(tftypes.String, "SP-2"),
		"issue_type_id": tftypes.NewValue(tftypes.String, "10002"),
	})}
	updateResp := &resource.UpdateResponse{State: readResp.State}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Inbox" {
		t.Errorf("expected name 'Updated Inbox', got %q", name)
	}

	// Delete
	deleteResp := &resource.DeleteResponse{State: updateResp.State}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Verify deleted
	readAfterDelete := &resource.ReadResponse{State: updateResp.State}
	r.Read(ctx, resource.ReadRequest{State: updateResp.State}, readAfterDelete)
	if readAfterDelete.Diagnostics.HasError() {
		t.Fatalf("Read after delete: %v", readAfterDelete.Diagnostics.Errors())
	}
}

// TestIncomingMailHandlerResourceCreateInvalidProtocol verifies error on invalid protocol.
func TestIncomingMailHandlerResourceCreateInvalidProtocol(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Bad Protocol"),
		"enabled":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "SMTP"),
		"username":      tftypes.NewValue(tftypes.String, "user@example.com"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for invalid protocol")
	}
}

// TestIncomingMailHandlerResourceReadNotFound verifies 404 removes resource.
func TestIncomingMailHandlerResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":          tftypes.NewValue(tftypes.String, "Gone"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Errorf("expected no error for not-found read, got %v", readResp.Diagnostics.Errors())
	}
}

// TestIncomingMailHandlerResourceUpdateNotFound verifies 404 on update.
func TestIncomingMailHandlerResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":          tftypes.NewValue(tftypes.String, "Gone"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for update on nonexistent handler")
	}
}

// TestIncomingMailHandlerResourceDeleteNotFound verifies deleting nonexistent handler does not error.
func TestIncomingMailHandlerResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":          tftypes.NewValue(tftypes.String, "Gone"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Errorf("expected no error for delete on nonexistent handler, got %v", deleteResp.Diagnostics.Errors())
	}
}

// TestIncomingMailHandlerResourceCreateInvalidPlan verifies error when plan cannot be deserialized.
func TestIncomingMailHandlerResourceCreateInvalidPlan(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for nil plan")
	}
}

// TestIncomingMailHandlerResourceReadInvalidState verifies error when state cannot be deserialized.
func TestIncomingMailHandlerResourceReadInvalidState(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Error("expected error for nil state")
	}
}

// TestIncomingMailHandlerResourceUpdateInvalidPlan verifies error when plan cannot be deserialized.
func TestIncomingMailHandlerResourceUpdateInvalidPlan(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "test-id"), "name": tftypes.NewValue(tftypes.String, "Test"),
		"enabled": tftypes.NewValue(tftypes.Bool, false), "server": tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port": tftypes.NewValue(tftypes.Number, 993), "protocol": tftypes.NewValue(tftypes.String, "IMAP"),
		"username": tftypes.NewValue(tftypes.String, "u"), "password": tftypes.NewValue(tftypes.String, "p"),
		"folder": tftypes.NewValue(tftypes.String, ""), "space_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for nil plan on update")
	}
}

// TestIncomingMailHandlerResourceUpdateInvalidState verifies error when state cannot be deserialized.
func TestIncomingMailHandlerResourceUpdateInvalidState(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "test-id"), "name": tftypes.NewValue(tftypes.String, "Test"),
		"enabled": tftypes.NewValue(tftypes.Bool, false), "server": tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port": tftypes.NewValue(tftypes.Number, 993), "protocol": tftypes.NewValue(tftypes.String, "IMAP"),
		"username": tftypes.NewValue(tftypes.String, "u"), "password": tftypes.NewValue(tftypes.String, "p"),
		"folder": tftypes.NewValue(tftypes.String, ""), "space_id": tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for nil state on update")
	}
}

// TestIncomingMailHandlerResourceDeleteInvalidState verifies error when state cannot be deserialized.
func TestIncomingMailHandlerResourceDeleteInvalidState(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for nil state on delete")
	}
}

// TestIncomingMailHandlerDataSourceReadInvalidConfig verifies error when config cannot be deserialized.
func TestIncomingMailHandlerDataSourceReadInvalidConfig(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	ds := mailhandlerdatasource.NewIncomingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dss.Type().TerraformType(ctx), nil)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for nil config on DS read")
	}
}

// TestIncomingMailHandlerResourceCreateForbidden verifies 403 error on create.
func TestIncomingMailHandlerResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/mailhandler/incoming", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Forbidden"),
		"enabled":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden create")
	}
}

// TestIncomingMailHandlerResourceCreateServerError verifies generic server error on create.
func TestIncomingMailHandlerResourceCreateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/mailhandler/incoming", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "Error"),
		"enabled":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for server error create")
	}
}

// TestIncomingMailHandlerResourceReadServerError verifies generic read error.
func TestIncomingMailHandlerResourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "test-id"),
		"name":          tftypes.NewValue(tftypes.String, "Test"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Error("expected error for server error read")
	}
}

// TestIncomingMailHandlerResourceUpdateBadRequest verifies 400 error on update.
func TestIncomingMailHandlerResourceUpdateBadRequest(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 400, "Invalid configuration")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "test-id"),
		"name":          tftypes.NewValue(tftypes.String, "Test"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for bad request update")
	}
}

// TestIncomingMailHandlerResourceUpdateForbidden verifies 403 error on update.
func TestIncomingMailHandlerResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "test-id"),
		"name":          tftypes.NewValue(tftypes.String, "Test"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden update")
	}
}

// TestIncomingMailHandlerResourceUpdateServerError verifies generic server error on update.
func TestIncomingMailHandlerResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "test-id"),
		"name":          tftypes.NewValue(tftypes.String, "Test"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for server error update")
	}
}

// TestIncomingMailHandlerResourceDeleteForbidden verifies 403 error on delete.
func TestIncomingMailHandlerResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "test-id"),
		"name":          tftypes.NewValue(tftypes.String, "Test"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden delete")
	}
}

// TestIncomingMailHandlerResourceDeleteServerError verifies generic server error on delete.
func TestIncomingMailHandlerResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "test-id"),
		"name":          tftypes.NewValue(tftypes.String, "Test"),
		"enabled":       tftypes.NewValue(tftypes.Bool, false),
		"server":        tftypes.NewValue(tftypes.String, "mail.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, ""),
		"space_id":      tftypes.NewValue(tftypes.String, ""),
		"issue_type_id": tftypes.NewValue(tftypes.String, ""),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for server error delete")
	}
}

// TestIncomingMailHandlerDataSourceReadServerError verifies generic error on DS read.
func TestIncomingMailHandlerDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/mailhandler/incoming/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := mailhandlerdatasource.NewIncomingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "test-id"),
		"name":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"enabled":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"server":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"port":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"protocol":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"folder":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for server error DS read")
	}
}

// TestIncomingMailHandlerResourceImportState verifies import state passthrough.
func TestIncomingMailHandlerResourceImportState(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)

	importable := r.(resource.ResourceWithImportState)
	s := getResourceSchema(t, r)
	importResp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	importable.ImportState(ctx, resource.ImportStateRequest{ID: "test-incoming-id"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	importedID := getStringAttr(t, importResp.State, "id")
	if importedID != "test-incoming-id" {
		t.Errorf("expected imported id 'test-incoming-id', got %q", importedID)
	}
}

// ==================== OUTGOING RESOURCE SCHEMA TESTS ====================

// TestOutgoingMailHandlerResourceMetadata verifies the resource type name.
func TestOutgoingMailHandlerResourceMetadata(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewOutgoingResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_outgoing_mail_handler" {
		t.Errorf("expected resource type name 'atlassian_jira_outgoing_mail_handler', got %q", resp.TypeName)
	}
}

// TestOutgoingMailHandlerResourceSchema verifies the resource schema.
func TestOutgoingMailHandlerResourceSchema(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewOutgoingResource()
	s := getResourceSchema(t, r)
	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "from_address", "prefix", "smtp_host", "smtp_port", "protocol", "username", "password", "tls"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestOutgoingMailHandlerResourceSchemaAttributeCount verifies attribute count.
func TestOutgoingMailHandlerResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewOutgoingResource()
	s := getResourceSchema(t, r)
	expected := 10
	actual := len(s.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestOutgoingMailHandlerResourceSchemaRequiredAttributes verifies required attributes.
func TestOutgoingMailHandlerResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewOutgoingResource()
	s := getResourceSchema(t, r)
	requiredAttrs := []string{"name", "from_address", "smtp_host", "smtp_port"}
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

// TestOutgoingMailHandlerResourceSensitiveFields verifies sensitive fields are marked.
func TestOutgoingMailHandlerResourceSensitiveFields(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewOutgoingResource()
	s := getResourceSchema(t, r)
	sensitiveAttrs := []string{"password"}
	for _, name := range sensitiveAttrs {
		attr, ok := s.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsSensitive() {
			t.Errorf("expected attribute %q to be marked sensitive", name)
		}
	}
}

// TestOutgoingMailHandlerResourceImplementsImportState verifies ImportState is implemented.
func TestOutgoingMailHandlerResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewOutgoingResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected outgoing mail handler resource to implement ResourceWithImportState")
	}
}

// TestOutgoingMailHandlerResourceInterfaceCompliance verifies interface compliance.
func TestOutgoingMailHandlerResourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = mailhandlerresource.NewOutgoingResource()
	var _ resource.ResourceWithImportState = mailhandlerresource.NewOutgoingResource().(resource.ResourceWithImportState)
}

// TestOutgoingMailHandlerResourceConfigureNil verifies nil provider data is handled.
func TestOutgoingMailHandlerResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewOutgoingResource()
	resp := &resource.ConfigureResponse{}
	r.(interface {
		Configure(context.Context, resource.ConfigureRequest, *resource.ConfigureResponse)
	}).Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("expected no error for nil provider data, got %v", resp.Diagnostics.Errors())
	}
}

// TestOutgoingMailHandlerResourceConfigureWrongType verifies wrong provider data type is handled.
func TestOutgoingMailHandlerResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := mailhandlerresource.NewOutgoingResource()
	resp := &resource.ConfigureResponse{}
	r.(interface {
		Configure(context.Context, resource.ConfigureRequest, *resource.ConfigureResponse)
	}).Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

// ==================== OUTGOING RESOURCE CRUD TESTS ====================

// TestOutgoingMailHandlerResourceCRUDLifecycle tests the full create-read-update-delete lifecycle.
func TestOutgoingMailHandlerResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":         tftypes.NewValue(tftypes.String, "Notifications"),
		"from_address": tftypes.NewValue(tftypes.String, "noreply@example.com"),
		"prefix":       tftypes.NewValue(tftypes.String, "[JIRA]"),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, "SMTPS"),
		"username":     tftypes.NewValue(tftypes.String, "smtp-user"),
		"password":     tftypes.NewValue(tftypes.String, "smtp-secret"),
		"tls":          tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	id := getStringAttr(t, createResp.State, "id")
	if id == "" {
		t.Fatal("expected non-empty id after create")
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Notifications" {
		t.Errorf("expected name 'Notifications', got %q", name)
	}

	// Read
	readResp := &resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, id),
		"name":         tftypes.NewValue(tftypes.String, "Updated Notifications"),
		"from_address": tftypes.NewValue(tftypes.String, "alerts@example.com"),
		"prefix":       tftypes.NewValue(tftypes.String, "[ALERT]"),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp2.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 465),
		"protocol":     tftypes.NewValue(tftypes.String, "SMTP"),
		"username":     tftypes.NewValue(tftypes.String, "new-user"),
		"password":     tftypes.NewValue(tftypes.String, "new-secret"),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	})}
	updateResp := &resource.UpdateResponse{State: readResp.State}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, updateResp.State, "name"); name != "Updated Notifications" {
		t.Errorf("expected name 'Updated Notifications', got %q", name)
	}

	// Delete
	deleteResp := &resource.DeleteResponse{State: updateResp.State}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

// TestOutgoingMailHandlerResourceReadNotFound verifies 404 removes resource.
func TestOutgoingMailHandlerResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":         tftypes.NewValue(tftypes.String, "Gone"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Errorf("expected no error for not-found read, got %v", readResp.Diagnostics.Errors())
	}
}

// TestOutgoingMailHandlerResourceUpdateNotFound verifies 404 on update.
func TestOutgoingMailHandlerResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":         tftypes.NewValue(tftypes.String, "Gone"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for update on nonexistent handler")
	}
}

// TestOutgoingMailHandlerResourceDeleteNotFound verifies deleting nonexistent handler does not error.
func TestOutgoingMailHandlerResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":         tftypes.NewValue(tftypes.String, "Gone"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Errorf("expected no error for delete on nonexistent handler, got %v", deleteResp.Diagnostics.Errors())
	}
}

// TestOutgoingMailHandlerResourceCreateInvalidPlan verifies error when plan cannot be deserialized.
func TestOutgoingMailHandlerResourceCreateInvalidPlan(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for nil plan")
	}
}

// TestOutgoingMailHandlerResourceReadInvalidState verifies error when state cannot be deserialized.
func TestOutgoingMailHandlerResourceReadInvalidState(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Error("expected error for nil state")
	}
}

// TestOutgoingMailHandlerResourceUpdateInvalidPlan verifies error when plan cannot be deserialized.
func TestOutgoingMailHandlerResourceUpdateInvalidPlan(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "test-id"), "name": tftypes.NewValue(tftypes.String, "Test"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"), "prefix": tftypes.NewValue(tftypes.String, ""),
		"smtp_host": tftypes.NewValue(tftypes.String, "smtp.example.com"), "smtp_port": tftypes.NewValue(tftypes.Number, 587),
		"protocol": tftypes.NewValue(tftypes.String, ""), "username": tftypes.NewValue(tftypes.String, ""),
		"password": tftypes.NewValue(tftypes.String, ""), "tls": tftypes.NewValue(tftypes.Bool, false),
	})}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for nil plan on update")
	}
}

// TestOutgoingMailHandlerResourceUpdateInvalidState verifies error when state cannot be deserialized.
func TestOutgoingMailHandlerResourceUpdateInvalidState(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "test-id"), "name": tftypes.NewValue(tftypes.String, "Test"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"), "prefix": tftypes.NewValue(tftypes.String, ""),
		"smtp_host": tftypes.NewValue(tftypes.String, "smtp.example.com"), "smtp_port": tftypes.NewValue(tftypes.Number, 587),
		"protocol": tftypes.NewValue(tftypes.String, ""), "username": tftypes.NewValue(tftypes.String, ""),
		"password": tftypes.NewValue(tftypes.String, ""), "tls": tftypes.NewValue(tftypes.Bool, false),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for nil state on update")
	}
}

// TestOutgoingMailHandlerResourceDeleteInvalidState verifies error when state cannot be deserialized.
func TestOutgoingMailHandlerResourceDeleteInvalidState(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for nil state on delete")
	}
}

// TestOutgoingMailHandlerDataSourceReadInvalidConfig verifies error when config cannot be deserialized.
func TestOutgoingMailHandlerDataSourceReadInvalidConfig(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	ds := mailhandlerdatasource.NewOutgoingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dss.Type().TerraformType(ctx), nil)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for nil config on DS read")
	}
}

// TestOutgoingMailHandlerResourceCreateForbidden verifies 403 error on create.
func TestOutgoingMailHandlerResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/mailhandler/outgoing", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":         tftypes.NewValue(tftypes.String, "Forbidden"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"password":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"tls":          tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden create")
	}
}

// TestOutgoingMailHandlerResourceCreateServerError verifies generic server error on create.
func TestOutgoingMailHandlerResourceCreateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/mailhandler/outgoing", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":         tftypes.NewValue(tftypes.String, "Error"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"password":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"tls":          tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for server error create")
	}
}

// TestOutgoingMailHandlerResourceReadServerError verifies generic read error.
func TestOutgoingMailHandlerResourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "test-id"),
		"name":         tftypes.NewValue(tftypes.String, "Test"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	})}
	readResp := &resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Error("expected error for server error read")
	}
}

// TestOutgoingMailHandlerResourceUpdateBadRequest verifies 400 error on update.
func TestOutgoingMailHandlerResourceUpdateBadRequest(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 400, "Invalid configuration")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "test-id"),
		"name":         tftypes.NewValue(tftypes.String, "Test"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for bad request update")
	}
}

// TestOutgoingMailHandlerResourceUpdateForbidden verifies 403 error on update.
func TestOutgoingMailHandlerResourceUpdateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "test-id"),
		"name":         tftypes.NewValue(tftypes.String, "Test"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden update")
	}
}

// TestOutgoingMailHandlerResourceUpdateServerError verifies generic server error on update.
func TestOutgoingMailHandlerResourceUpdateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	vals := map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "test-id"),
		"name":         tftypes.NewValue(tftypes.String, "Test"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	updateResp := &resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("expected error for server error update")
	}
}

// TestOutgoingMailHandlerResourceDeleteForbidden verifies 403 error on delete.
func TestOutgoingMailHandlerResourceDeleteForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "test-id"),
		"name":         tftypes.NewValue(tftypes.String, "Test"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for forbidden delete")
	}
}

// TestOutgoingMailHandlerResourceDeleteServerError verifies generic server error on delete.
func TestOutgoingMailHandlerResourceDeleteServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "test-id"),
		"name":         tftypes.NewValue(tftypes.String, "Test"),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, ""),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, ""),
		"username":     tftypes.NewValue(tftypes.String, ""),
		"password":     tftypes.NewValue(tftypes.String, ""),
		"tls":          tftypes.NewValue(tftypes.Bool, false),
	})}
	deleteResp := &resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if !deleteResp.Diagnostics.HasError() {
		t.Error("expected error for server error delete")
	}
}

// TestOutgoingMailHandlerDataSourceReadServerError verifies generic error on DS read.
func TestOutgoingMailHandlerDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/mailhandler/outgoing/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal Server Error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := mailhandlerdatasource.NewOutgoingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "test-id"),
		"name":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"from_address": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"prefix":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_host":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_port":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"protocol":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"tls":          tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for server error DS read")
	}
}

// TestOutgoingMailHandlerResourceImportState verifies import state passthrough.
func TestOutgoingMailHandlerResourceImportState(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)

	importable := r.(resource.ResourceWithImportState)
	s := getResourceSchema(t, r)
	importResp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	importable.ImportState(ctx, resource.ImportStateRequest{ID: "test-outgoing-id"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	importedID := getStringAttr(t, importResp.State, "id")
	if importedID != "test-outgoing-id" {
		t.Errorf("expected imported id 'test-outgoing-id', got %q", importedID)
	}
}

// TestOutgoingMailHandlerResourceCreateMissingName verifies error on missing name.
func TestOutgoingMailHandlerResourceCreateMissingName(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":         tftypes.NewValue(tftypes.String, ""),
		"from_address": tftypes.NewValue(tftypes.String, "a@b.com"),
		"prefix":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"password":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"tls":          tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Error("expected error for missing name")
	}
}

// ==================== INCOMING DATA SOURCE TESTS ====================

// TestIncomingMailHandlerDataSourceMetadata verifies the data source type name.
func TestIncomingMailHandlerDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewIncomingDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_incoming_mail_handler" {
		t.Errorf("expected data source type name 'atlassian_jira_incoming_mail_handler', got %q", resp.TypeName)
	}
}

// TestIncomingMailHandlerDataSourceSchema verifies the data source schema.
func TestIncomingMailHandlerDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewIncomingDataSource()
	s := getDatasourceSchema(t, ds)
	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "enabled", "server", "port", "protocol", "username", "folder", "space_id", "issue_type_id"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestIncomingMailHandlerDataSourceSchemaAttributeCount verifies attribute count.
func TestIncomingMailHandlerDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewIncomingDataSource()
	s := getDatasourceSchema(t, ds)
	expected := 10
	actual := len(s.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestIncomingMailHandlerDataSourceInterfaceCompliance verifies data source interface.
func TestIncomingMailHandlerDataSourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = mailhandlerdatasource.NewIncomingDataSource()
}

// TestIncomingMailHandlerDataSourceConfigureNil verifies nil provider data is handled.
func TestIncomingMailHandlerDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewIncomingDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(interface {
		Configure(context.Context, datasource.ConfigureRequest, *datasource.ConfigureResponse)
	}).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("expected no error for nil provider data, got %v", resp.Diagnostics.Errors())
	}
}

// TestIncomingMailHandlerDataSourceConfigureWrongType verifies wrong provider data type.
func TestIncomingMailHandlerDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewIncomingDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(interface {
		Configure(context.Context, datasource.ConfigureRequest, *datasource.ConfigureResponse)
	}).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

// TestIncomingMailHandlerDataSourceRead tests reading an incoming handler data source.
func TestIncomingMailHandlerDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()

	// Create a handler first
	r := mailhandlerresource.NewIncomingResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTFType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTFType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":          tftypes.NewValue(tftypes.String, "DS Test Incoming"),
		"enabled":       tftypes.NewValue(tftypes.Bool, true),
		"server":        tftypes.NewValue(tftypes.String, "imap.example.com"),
		"port":          tftypes.NewValue(tftypes.Number, 993),
		"protocol":      tftypes.NewValue(tftypes.String, "IMAP"),
		"username":      tftypes.NewValue(tftypes.String, "user@example.com"),
		"password":      tftypes.NewValue(tftypes.String, "pass"),
		"folder":        tftypes.NewValue(tftypes.String, "INBOX"),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	handlerID := getStringAttr(t, createResp.State, "id")

	// Read using data source
	ds := mailhandlerdatasource.NewIncomingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, handlerID),
		"name":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"enabled":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"server":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"port":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"protocol":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"folder":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DataSource Read: %v", dsResp.Diagnostics.Errors())
	}
}

// TestIncomingMailHandlerDataSourceReadMissingID verifies error when ID not provided.
func TestIncomingMailHandlerDataSourceReadMissingID(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()

	ds := mailhandlerdatasource.NewIncomingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, nil),
		"name":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"enabled":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"server":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"port":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"protocol":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"folder":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for missing ID on data source read")
	}
}

// TestIncomingMailHandlerDataSourceReadNotFound tests reading a nonexistent handler.
func TestIncomingMailHandlerDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()

	ds := mailhandlerdatasource.NewIncomingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"enabled":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"server":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"port":          tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"protocol":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"folder":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"space_id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"issue_type_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for nonexistent incoming mail handler")
	}
}

// ==================== OUTGOING DATA SOURCE TESTS ====================

// TestOutgoingMailHandlerDataSourceMetadata verifies the data source type name.
func TestOutgoingMailHandlerDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewOutgoingDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_outgoing_mail_handler" {
		t.Errorf("expected data source type name 'atlassian_jira_outgoing_mail_handler', got %q", resp.TypeName)
	}
}

// TestOutgoingMailHandlerDataSourceSchema verifies the data source schema.
func TestOutgoingMailHandlerDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewOutgoingDataSource()
	s := getDatasourceSchema(t, ds)
	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}
	expectedAttrs := []string{"id", "name", "from_address", "prefix", "smtp_host", "smtp_port", "protocol", "username", "tls"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestOutgoingMailHandlerDataSourceSchemaAttributeCount verifies attribute count.
func TestOutgoingMailHandlerDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewOutgoingDataSource()
	s := getDatasourceSchema(t, ds)
	expected := 9
	actual := len(s.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestOutgoingMailHandlerDataSourceInterfaceCompliance verifies data source interface.
func TestOutgoingMailHandlerDataSourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = mailhandlerdatasource.NewOutgoingDataSource()
}

// TestOutgoingMailHandlerDataSourceConfigureNil verifies nil provider data is handled.
func TestOutgoingMailHandlerDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewOutgoingDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(interface {
		Configure(context.Context, datasource.ConfigureRequest, *datasource.ConfigureResponse)
	}).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("expected no error for nil provider data, got %v", resp.Diagnostics.Errors())
	}
}

// TestOutgoingMailHandlerDataSourceConfigureWrongType verifies wrong provider data type.
func TestOutgoingMailHandlerDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := mailhandlerdatasource.NewOutgoingDataSource()
	resp := &datasource.ConfigureResponse{}
	ds.(interface {
		Configure(context.Context, datasource.ConfigureRequest, *datasource.ConfigureResponse)
	}).Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

// TestOutgoingMailHandlerDataSourceRead tests reading an outgoing handler data source.
func TestOutgoingMailHandlerDataSourceRead(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()

	// Create a handler first
	r := mailhandlerresource.NewOutgoingResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTFType := rs.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTFType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":         tftypes.NewValue(tftypes.String, "DS Test Outgoing"),
		"from_address": tftypes.NewValue(tftypes.String, "noreply@example.com"),
		"prefix":       tftypes.NewValue(tftypes.String, "[TEST]"),
		"smtp_host":    tftypes.NewValue(tftypes.String, "smtp.example.com"),
		"smtp_port":    tftypes.NewValue(tftypes.Number, 587),
		"protocol":     tftypes.NewValue(tftypes.String, "SMTP"),
		"username":     tftypes.NewValue(tftypes.String, "smtp-user"),
		"password":     tftypes.NewValue(tftypes.String, "smtp-pass"),
		"tls":          tftypes.NewValue(tftypes.Bool, true),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	handlerID := getStringAttr(t, createResp.State, "id")

	// Read using data source
	ds := mailhandlerdatasource.NewOutgoingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, handlerID),
		"name":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"from_address": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"prefix":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_host":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_port":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"protocol":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"tls":          tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DataSource Read: %v", dsResp.Diagnostics.Errors())
	}
}

// TestOutgoingMailHandlerDataSourceReadMissingID verifies error when ID not provided.
func TestOutgoingMailHandlerDataSourceReadMissingID(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()

	ds := mailhandlerdatasource.NewOutgoingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, nil),
		"name":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"from_address": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"prefix":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_host":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_port":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"protocol":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"tls":          tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for missing ID on data source read")
	}
}

// TestOutgoingMailHandlerDataSourceReadNotFound tests reading a nonexistent handler.
func TestOutgoingMailHandlerDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := mailMockServer(t)
	ctx := context.Background()

	ds := mailhandlerdatasource.NewOutgoingDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTFType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTFType, map[string]tftypes.Value{
		"id":           tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"from_address": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"prefix":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_host":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"smtp_port":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
		"protocol":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"username":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"tls":          tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Error("expected error for nonexistent outgoing mail handler")
	}
}
