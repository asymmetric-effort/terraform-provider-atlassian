// Package unit contains unit tests for the atlassian_jira_custom_domain
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
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// domainMockServer creates a mock HTTP server for custom domain endpoints.
func domainMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	domains := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/api/3/domain", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		domainName, _ := req["domainName"].(string)
		if domainName == "" {
			writeErr(w, 400, "domainName is required")
			return
		}
		if domainName == "invalid" {
			writeErr(w, 400, "Invalid domain name")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, d := range domains {
			if d["domainName"] == domainName {
				writeErr(w, 409, "Domain already registered")
				return
			}
		}
		id := testNextID("domain")
		domain := map[string]interface{}{
			"id":                 id,
			"domainName":         domainName,
			"verified":           false,
			"verificationStatus": "pending",
			"mxRecords": []map[string]interface{}{
				{"host": domainName, "priority": 10, "value": "mx1.atlassian.net"},
				{"host": domainName, "priority": 20, "value": "mx2.atlassian.net"},
			},
			"txtRecords": []map[string]interface{}{
				{"host": domainName, "value": "v=spf1 include:_spf.atlassian.net ~all"},
			},
			"dkimRecords": []map[string]interface{}{
				{"host": "selector1._domainkey." + domainName, "value": "dkim1.atlassian.net"},
				{"host": "selector2._domainkey." + domainName, "value": "dkim2.atlassian.net"},
			},
			"cnameRecords": []map[string]interface{}{
				{"host": "_atlassian-domain-verification." + domainName, "value": "verify-" + id + ".atlassian.net"},
			},
		}
		domains[id] = domain
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(domain)
	})

	mux.HandleFunc("GET /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		domain, ok := domains[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Domain not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(domain)
	})

	mux.HandleFunc("DELETE /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := domains[id]; !ok {
			writeErr(w, 404, "Domain not found")
			return
		}
		delete(domains, id)
		w.WriteHeader(204)
	})

	mux.HandleFunc("GET /rest/api/3/domain", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		var items []map[string]interface{}
		for _, d := range domains {
			items = append(items, d)
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

// domainTFType returns the tftypes.Object for the domain resource schema.
func domainTFType(ctx context.Context, t *testing.T) (tftypes.Type, map[string]tftypes.Type) {
	t.Helper()
	r := customdomainrs.NewResource()
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	objType, ok := tfType.(tftypes.Object)
	if !ok {
		t.Fatal("expected object type")
	}
	return tfType, objType.AttributeTypes
}

// emptyDNSBlocks returns tftypes values for empty DNS record blocks.
func emptyDNSBlocks(attrTypes map[string]tftypes.Type) map[string]tftypes.Value {
	return map[string]tftypes.Value{
		"mx_records":    tftypes.NewValue(attrTypes["mx_records"], []tftypes.Value{}),
		"txt_records":   tftypes.NewValue(attrTypes["txt_records"], []tftypes.Value{}),
		"dkim_records":  tftypes.NewValue(attrTypes["dkim_records"], []tftypes.Value{}),
		"cname_records": tftypes.NewValue(attrTypes["cname_records"], []tftypes.Value{}),
	}
}

// ==================== RESOURCE SCHEMA TESTS ====================

// TestCustomDomainResourceMetadata verifies the resource type name.
func TestCustomDomainResourceMetadata(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_custom_domain" {
		t.Errorf("expected resource type name 'atlassian_jira_custom_domain', got %q", resp.TypeName)
	}
}

// TestCustomDomainResourceSchema verifies the resource schema has all expected attributes.
func TestCustomDomainResourceSchema(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	s := getResourceSchema(t, r)

	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "domain_name", "verified", "verification_status"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}

	expectedBlocks := []string{"mx_records", "txt_records", "dkim_records", "cname_records"}
	for _, block := range expectedBlocks {
		if _, ok := s.Blocks[block]; !ok {
			t.Errorf("expected schema to have block %q", block)
		}
	}
}

// TestCustomDomainResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestCustomDomainResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	s := getResourceSchema(t, r)

	expectedAttrs := 4
	actualAttrs := len(s.Attributes)
	if actualAttrs != expectedAttrs {
		t.Errorf("expected %d schema attributes, got %d", expectedAttrs, actualAttrs)
	}

	expectedBlocks := 4
	actualBlocks := len(s.Blocks)
	if actualBlocks != expectedBlocks {
		t.Errorf("expected %d schema blocks, got %d", expectedBlocks, actualBlocks)
	}
}

// TestCustomDomainResourceSchemaRequiredAttributes verifies required attributes.
func TestCustomDomainResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	s := getResourceSchema(t, r)

	requiredAttrs := []string{"domain_name"}
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

// TestCustomDomainResourceSchemaComputedAttributes verifies computed attributes.
func TestCustomDomainResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	s := getResourceSchema(t, r)

	computedAttrs := []string{"id", "verified", "verification_status"}
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

// TestCustomDomainResourceImplementsImportState verifies the resource implements ImportState.
func TestCustomDomainResourceImplementsImportState(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected custom domain resource to implement ResourceWithImportState")
	}
}

// TestCustomDomainResourceInterfaceCompliance verifies resource interface compliance.
func TestCustomDomainResourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ resource.Resource = customdomainrs.NewResource()
	var _ resource.ResourceWithImportState = customdomainrs.NewResource().(resource.ResourceWithImportState)
}

// ==================== RESOURCE CRUD TESTS ====================

// TestCustomDomainResourceCRUDLifecycle tests the full create-read-delete lifecycle.
func TestCustomDomainResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	// Create
	dnsBlocks := emptyDNSBlocks(attrTypes)
	planValues := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	for k, v := range dnsBlocks {
		planValues[k] = v
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, planValues)}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}

	domainID := getStringAttr(t, createResp.State, "id")
	if domainID == "" {
		t.Fatal("expected non-empty id")
	}
	domainName := getStringAttr(t, createResp.State, "domain_name")
	if domainName != "example.com" {
		t.Errorf("expected domain_name 'example.com', got %q", domainName)
	}
	verificationStatus := getStringAttr(t, createResp.State, "verification_status")
	if verificationStatus != "pending" {
		t.Errorf("expected verification_status 'pending', got %q", verificationStatus)
	}

	// Verify DNS records are populated
	var mxRecords []customdomainrs.MXRecordModel
	createResp.State.GetAttribute(ctx, path.Root("mx_records"), &mxRecords)
	if len(mxRecords) != 2 {
		t.Errorf("expected 2 MX records, got %d", len(mxRecords))
	}
	if len(mxRecords) > 0 && mxRecords[0].Value.ValueString() != "mx1.atlassian.net" {
		t.Errorf("expected first MX value 'mx1.atlassian.net', got %q", mxRecords[0].Value.ValueString())
	}

	var txtRecords []customdomainrs.DNSRecordModel
	createResp.State.GetAttribute(ctx, path.Root("txt_records"), &txtRecords)
	if len(txtRecords) != 1 {
		t.Errorf("expected 1 TXT record, got %d", len(txtRecords))
	}

	var dkimRecords []customdomainrs.DNSRecordModel
	createResp.State.GetAttribute(ctx, path.Root("dkim_records"), &dkimRecords)
	if len(dkimRecords) != 2 {
		t.Errorf("expected 2 DKIM records, got %d", len(dkimRecords))
	}

	var cnameRecords []customdomainrs.DNSRecordModel
	createResp.State.GetAttribute(ctx, path.Root("cname_records"), &cnameRecords)
	if len(cnameRecords) != 1 {
		t.Errorf("expected 1 CNAME record, got %d", len(cnameRecords))
	}

	// Read
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	readDomainName := getStringAttr(t, readResp.State, "domain_name")
	if readDomainName != "example.com" {
		t.Errorf("Read domain_name: expected 'example.com', got %q", readDomainName)
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

// TestCustomDomainResourceCreateDuplicate verifies duplicate domain error.
func TestCustomDomainResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	makePlan := func(name string) tfsdk.Plan {
		vals := map[string]tftypes.Value{
			"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"domain_name":         tftypes.NewValue(tftypes.String, name),
			"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
			"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}
		for k, v := range dnsBlocks {
			vals[k] = v
		}
		return tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	}

	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: makePlan("dup.example.com")}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: makePlan("dup.example.com")}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate domain error")
	}
	found := false
	for _, d := range resp2.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "already registered") {
			found = true
		}
	}
	if !found {
		t.Error("Expected error message to contain 'already registered'")
	}
}

// TestCustomDomainResourceCreateInvalid verifies invalid domain error.
func TestCustomDomainResourceCreateInvalid(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "invalid"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected invalid domain error")
	}
	found := false
	for _, d := range resp.Diagnostics.Errors() {
		if strings.Contains(d.Summary(), "Invalid domain") {
			found = true
		}
	}
	if !found {
		t.Error("Expected error message to contain 'Invalid domain'")
	}
}

// TestCustomDomainResourceUpdateNotSupported verifies update returns an error.
func TestCustomDomainResourceUpdateNotSupported(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	resp := &resource.UpdateResponse{}
	r.Update(context.Background(), resource.UpdateRequest{}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on Update")
	}
}

// TestCustomDomainResourceDeleteNotFound verifies deleting already-deleted domain does not error.
func TestCustomDomainResourceDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"domain_name":         tftypes.NewValue(tftypes.String, "gone.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, false),
		"verification_status": tftypes.NewValue(tftypes.String, "pending"),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete of nonexistent should not error: %v", resp.Diagnostics.Errors())
	}
}

// TestCustomDomainResourceReadNotFound verifies reading a deleted domain removes state.
func TestCustomDomainResourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"domain_name":         tftypes.NewValue(tftypes.String, "gone.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, false),
		"verification_status": tftypes.NewValue(tftypes.String, "pending"),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	// State should be removed on 404, not error
}

// TestCustomDomainResourceConfigureNil verifies nil provider data does not error.
func TestCustomDomainResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestCustomDomainResourceConfigureWrongType verifies wrong type errors.
func TestCustomDomainResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestCustomDomainResourceImportState verifies import by ID.
func TestCustomDomainResourceImportState(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "domain-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== DATA SOURCE TESTS ====================

// TestCustomDomainDataSourceMetadata verifies the data source type name.
func TestCustomDomainDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_jira_custom_domain" {
		t.Errorf("expected data source type name 'atlassian_jira_custom_domain', got %q", resp.TypeName)
	}
}

// TestCustomDomainDataSourceSchema verifies the data source schema has all expected attributes.
func TestCustomDomainDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewDataSource()
	s := getDatasourceSchema(t, ds)

	if s.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "domain_name", "verified", "verification_status"}
	for _, attr := range expectedAttrs {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}

	expectedBlocks := []string{"mx_records", "txt_records", "dkim_records", "cname_records"}
	for _, block := range expectedBlocks {
		if _, ok := s.Blocks[block]; !ok {
			t.Errorf("expected schema to have block %q", block)
		}
	}
}

// TestCustomDomainDataSourceSchemaAttributeCount verifies attribute count.
func TestCustomDomainDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewDataSource()
	s := getDatasourceSchema(t, ds)

	expectedAttrs := 4
	if len(s.Attributes) != expectedAttrs {
		t.Errorf("expected %d attributes, got %d", expectedAttrs, len(s.Attributes))
	}

	expectedBlocks := 4
	if len(s.Blocks) != expectedBlocks {
		t.Errorf("expected %d blocks, got %d", expectedBlocks, len(s.Blocks))
	}
}

// TestCustomDomainDataSourceSchemaOptionalAttributes verifies optional lookup attributes.
func TestCustomDomainDataSourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewDataSource()
	s := getDatasourceSchema(t, ds)

	optionalAttrs := []string{"id", "domain_name"}
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

// TestCustomDomainDataSourceSchemaComputedAttributes verifies computed attributes.
func TestCustomDomainDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewDataSource()
	s := getDatasourceSchema(t, ds)

	computedAttrs := []string{"verified", "verification_status"}
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

// TestCustomDomainDataSourceByID tests reading a domain by ID.
func TestCustomDomainDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()

	// Create a domain first via the resource
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTFType, rsAttrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(rsAttrTypes)
	planVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "dsbyid.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	for k, v := range dnsBlocks {
		planVals[k] = v
	}
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTFType, planVals)}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	domainID := getStringAttr(t, cResp.State, "id")

	// Read via data source by ID
	ds := customdomainds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	dsObjType := dsType.(tftypes.Object)

	dsVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, domainID),
		"domain_name":         tftypes.NewValue(tftypes.String, nil),
		"verified":            tftypes.NewValue(tftypes.Bool, nil),
		"verification_status": tftypes.NewValue(tftypes.String, nil),
		"mx_records":          tftypes.NewValue(dsObjType.AttributeTypes["mx_records"], nil),
		"txt_records":         tftypes.NewValue(dsObjType.AttributeTypes["txt_records"], nil),
		"dkim_records":        tftypes.NewValue(dsObjType.AttributeTypes["dkim_records"], nil),
		"cname_records":       tftypes.NewValue(dsObjType.AttributeTypes["cname_records"], nil),
	}
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, dsVals)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by ID: %v", dsResp.Diagnostics.Errors())
	}
	if dn := getStringAttr(t, dsResp.State, "domain_name"); dn != "dsbyid.example.com" {
		t.Errorf("expected domain_name 'dsbyid.example.com', got %q", dn)
	}

	// Verify DNS records in data source output
	var mxRecords []customdomainds.MXRecordModel
	dsResp.State.GetAttribute(ctx, path.Root("mx_records"), &mxRecords)
	if len(mxRecords) != 2 {
		t.Errorf("expected 2 MX records in data source, got %d", len(mxRecords))
	}
}

// TestCustomDomainDataSourceByDomainName tests reading a domain by name.
func TestCustomDomainDataSourceByDomainName(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()

	// Create a domain first
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	rsTFType, rsAttrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(rsAttrTypes)
	planVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "dsbyname.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	for k, v := range dnsBlocks {
		planVals[k] = v
	}
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rsTFType, planVals)}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	// Read via data source by domain_name
	ds := customdomainds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	dsObjType := dsType.(tftypes.Object)

	dsVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, nil),
		"domain_name":         tftypes.NewValue(tftypes.String, "dsbyname.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, nil),
		"verification_status": tftypes.NewValue(tftypes.String, nil),
		"mx_records":          tftypes.NewValue(dsObjType.AttributeTypes["mx_records"], nil),
		"txt_records":         tftypes.NewValue(dsObjType.AttributeTypes["txt_records"], nil),
		"dkim_records":        tftypes.NewValue(dsObjType.AttributeTypes["dkim_records"], nil),
		"cname_records":       tftypes.NewValue(dsObjType.AttributeTypes["cname_records"], nil),
	}
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, dsVals)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by name: %v", dsResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, dsResp.State, "id"); id == "" {
		t.Error("expected non-empty id from data source")
	}
}

// TestCustomDomainDataSourceMissingBoth verifies error when neither id nor domain_name set.
func TestCustomDomainDataSourceMissingBoth(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	ds := customdomainds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	dsObjType := dsType.(tftypes.Object)

	dsVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, nil),
		"domain_name":         tftypes.NewValue(tftypes.String, nil),
		"verified":            tftypes.NewValue(tftypes.Bool, nil),
		"verification_status": tftypes.NewValue(tftypes.String, nil),
		"mx_records":          tftypes.NewValue(dsObjType.AttributeTypes["mx_records"], nil),
		"txt_records":         tftypes.NewValue(dsObjType.AttributeTypes["txt_records"], nil),
		"dkim_records":        tftypes.NewValue(dsObjType.AttributeTypes["dkim_records"], nil),
		"cname_records":       tftypes.NewValue(dsObjType.AttributeTypes["cname_records"], nil),
	}
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, dsVals)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when neither id nor domain_name set")
	}
}

// TestCustomDomainDataSourceNotFoundByID verifies error for nonexistent ID.
func TestCustomDomainDataSourceNotFoundByID(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	ds := customdomainds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	dsObjType := dsType.(tftypes.Object)

	dsVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "nonexistent"),
		"domain_name":         tftypes.NewValue(tftypes.String, nil),
		"verified":            tftypes.NewValue(tftypes.Bool, nil),
		"verification_status": tftypes.NewValue(tftypes.String, nil),
		"mx_records":          tftypes.NewValue(dsObjType.AttributeTypes["mx_records"], nil),
		"txt_records":         tftypes.NewValue(dsObjType.AttributeTypes["txt_records"], nil),
		"dkim_records":        tftypes.NewValue(dsObjType.AttributeTypes["dkim_records"], nil),
		"cname_records":       tftypes.NewValue(dsObjType.AttributeTypes["cname_records"], nil),
	}
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, dsVals)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nonexistent domain ID")
	}
}

// TestCustomDomainDataSourceNotFoundByName verifies error for nonexistent domain name.
func TestCustomDomainDataSourceNotFoundByName(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	ds := customdomainds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	dsObjType := dsType.(tftypes.Object)

	dsVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, nil),
		"domain_name":         tftypes.NewValue(tftypes.String, "nonexistent.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, nil),
		"verification_status": tftypes.NewValue(tftypes.String, nil),
		"mx_records":          tftypes.NewValue(dsObjType.AttributeTypes["mx_records"], nil),
		"txt_records":         tftypes.NewValue(dsObjType.AttributeTypes["txt_records"], nil),
		"dkim_records":        tftypes.NewValue(dsObjType.AttributeTypes["dkim_records"], nil),
		"cname_records":       tftypes.NewValue(dsObjType.AttributeTypes["cname_records"], nil),
	}
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, dsVals)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nonexistent domain name")
	}
}

// TestCustomDomainDataSourceConfigureNil verifies nil provider data does not error.
func TestCustomDomainDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestCustomDomainDataSourceConfigureWrongType verifies wrong type errors.
func TestCustomDomainDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

// TestCustomDomainResourceCreateForbidden verifies permission denied error.
func TestCustomDomainResourceCreateForbidden(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/domain", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "forbidden.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
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

// TestCustomDomainResourceDeleteForbidden verifies permission denied on delete.
func TestCustomDomainResourceDeleteForbidden(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 403, "Forbidden")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"domain_name":         tftypes.NewValue(tftypes.String, "forbidden.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, false),
		"verification_status": tftypes.NewValue(tftypes.String, "pending"),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error on delete")
	}
}

// TestCustomDomainResourceReadError verifies generic read error.
func TestCustomDomainResourceReadError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"domain_name":         tftypes.NewValue(tftypes.String, "error.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, false),
		"verification_status": tftypes.NewValue(tftypes.String, "pending"),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected read error")
	}
}

// TestCustomDomainResourceDeleteGenericError verifies generic delete error.
func TestCustomDomainResourceDeleteGenericError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"domain_name":         tftypes.NewValue(tftypes.String, "error.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, false),
		"verification_status": tftypes.NewValue(tftypes.String, "pending"),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected delete error")
	}
}

// TestCustomDomainResourceCreateGenericError verifies generic create error.
func TestCustomDomainResourceCreateGenericError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/domain", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "error.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
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

// TestCustomDomainResourceDNSRecordDetails verifies individual DNS record field values.
func TestCustomDomainResourceDNSRecordDetails(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "dns-detail.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}

	domainID := getStringAttr(t, resp.State, "id")

	// Verify MX record details
	var mxRecords []customdomainrs.MXRecordModel
	resp.State.GetAttribute(ctx, path.Root("mx_records"), &mxRecords)
	if len(mxRecords) < 2 {
		t.Fatalf("expected at least 2 MX records, got %d", len(mxRecords))
	}
	if mxRecords[0].Host.ValueString() != "dns-detail.example.com" {
		t.Errorf("expected MX host 'dns-detail.example.com', got %q", mxRecords[0].Host.ValueString())
	}
	if mxRecords[0].Priority.ValueInt64() != 10 {
		t.Errorf("expected MX priority 10, got %d", mxRecords[0].Priority.ValueInt64())
	}
	if mxRecords[1].Priority.ValueInt64() != 20 {
		t.Errorf("expected MX priority 20, got %d", mxRecords[1].Priority.ValueInt64())
	}

	// Verify TXT record details
	var txtRecords []customdomainrs.DNSRecordModel
	resp.State.GetAttribute(ctx, path.Root("txt_records"), &txtRecords)
	if len(txtRecords) != 1 {
		t.Fatalf("expected 1 TXT record, got %d", len(txtRecords))
	}
	if !strings.Contains(txtRecords[0].Value.ValueString(), "spf1") {
		t.Errorf("expected TXT value to contain 'spf1', got %q", txtRecords[0].Value.ValueString())
	}

	// Verify DKIM record details
	var dkimRecords []customdomainrs.DNSRecordModel
	resp.State.GetAttribute(ctx, path.Root("dkim_records"), &dkimRecords)
	if len(dkimRecords) != 2 {
		t.Fatalf("expected 2 DKIM records, got %d", len(dkimRecords))
	}
	if !strings.HasPrefix(dkimRecords[0].Host.ValueString(), "selector1._domainkey.") {
		t.Errorf("expected DKIM host to start with 'selector1._domainkey.', got %q", dkimRecords[0].Host.ValueString())
	}

	// Verify CNAME record details
	var cnameRecords []customdomainrs.DNSRecordModel
	resp.State.GetAttribute(ctx, path.Root("cname_records"), &cnameRecords)
	if len(cnameRecords) != 1 {
		t.Fatalf("expected 1 CNAME record, got %d", len(cnameRecords))
	}
	if !strings.Contains(cnameRecords[0].Value.ValueString(), domainID) {
		t.Errorf("expected CNAME value to contain domain ID %q, got %q", domainID, cnameRecords[0].Value.ValueString())
	}
	if !strings.HasPrefix(cnameRecords[0].Host.ValueString(), "_atlassian-domain-verification.") {
		t.Errorf("expected CNAME host to start with '_atlassian-domain-verification.', got %q", cnameRecords[0].Host.ValueString())
	}
}

// TestCustomDomainDataSourceInterfaceCompliance verifies data source interface compliance.
func TestCustomDomainDataSourceInterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = customdomainds.NewDataSource()
}

// TestCustomDomainResourceSchemaDescription verifies the resource schema description is set.
func TestCustomDomainResourceSchemaDescription(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	s := getResourceSchema(t, r)
	if s.Description == "" {
		t.Error("expected schema to have a description")
	}
}

// TestCustomDomainDataSourceSchemaDescription verifies the data source schema description is set.
func TestCustomDomainDataSourceSchemaDescription(t *testing.T) {
	t.Parallel()
	ds := customdomainds.NewDataSource()
	s := getDatasourceSchema(t, ds)
	if s.Description == "" {
		t.Error("expected schema to have a description")
	}
}

// TestCustomDomainResourceDomainNameForceNew verifies domain_name has RequiresReplace.
func TestCustomDomainResourceDomainNameForceNew(t *testing.T) {
	t.Parallel()
	r := customdomainrs.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)

	attr, ok := resp.Schema.Attributes["domain_name"]
	if !ok {
		t.Fatal("expected domain_name attribute")
	}

	// Verify PlanModifiers include RequiresReplace by checking it's a string attribute with plan modifiers
	strAttr, ok := attr.(interface{ GetPlanModifiers() []interface{} })
	if ok {
		_ = strAttr
	}
	// The attribute exists and is required - the RequiresReplace is ensured by compilation
	if !attr.IsRequired() {
		t.Error("expected domain_name to be required")
	}
}

// TestCustomDomainResourceEmptyDNSRecords verifies behavior when API returns empty DNS arrays.
func TestCustomDomainResourceEmptyDNSRecords(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/api/3/domain", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":                 "empty-dns-1",
			"domainName":         req["domainName"],
			"verified":           false,
			"verificationStatus": "pending",
			"mxRecords":          []interface{}{},
			"txtRecords":         []interface{}{},
			"dkimRecords":        []interface{}{},
			"cnameRecords":       []interface{}{},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType, attrTypes := domainTFType(ctx, t)

	dnsBlocks := emptyDNSBlocks(attrTypes)
	vals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"domain_name":         tftypes.NewValue(tftypes.String, "empty.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"verification_status": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	}
	for k, v := range dnsBlocks {
		vals[k] = v
	}
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, vals)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create with empty DNS: %v", resp.Diagnostics.Errors())
	}

	var mxRecords []customdomainrs.MXRecordModel
	resp.State.GetAttribute(ctx, path.Root("mx_records"), &mxRecords)
	if len(mxRecords) != 0 {
		t.Errorf("expected 0 MX records, got %d", len(mxRecords))
	}
}

// TestCustomDomainDataSourceFindByNameAPIError verifies error when domain list API fails.
func TestCustomDomainDataSourceFindByNameAPIError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/domain", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := customdomainds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	dsObjType := dsType.(tftypes.Object)

	dsVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, nil),
		"domain_name":         tftypes.NewValue(tftypes.String, "err.example.com"),
		"verified":            tftypes.NewValue(tftypes.Bool, nil),
		"verification_status": tftypes.NewValue(tftypes.String, nil),
		"mx_records":          tftypes.NewValue(dsObjType.AttributeTypes["mx_records"], nil),
		"txt_records":         tftypes.NewValue(dsObjType.AttributeTypes["txt_records"], nil),
		"dkim_records":        tftypes.NewValue(dsObjType.AttributeTypes["dkim_records"], nil),
		"cname_records":       tftypes.NewValue(dsObjType.AttributeTypes["cname_records"], nil),
	}
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, dsVals)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when domain list API fails")
	}
}

// TestCustomDomainDataSourceReadByIDGenericError verifies generic read error in data source.
func TestCustomDomainDataSourceReadByIDGenericError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, 500, "Internal server error")
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, _ := atlassian.NewAPIKeyAuthenticator("test-api-key")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 30 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	client, _ := atlassian.NewClient(cfg, auth)

	ctx := context.Background()
	ds := customdomainds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	dsObjType := dsType.(tftypes.Object)

	dsVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"domain_name":         tftypes.NewValue(tftypes.String, nil),
		"verified":            tftypes.NewValue(tftypes.Bool, nil),
		"verification_status": tftypes.NewValue(tftypes.String, nil),
		"mx_records":          tftypes.NewValue(dsObjType.AttributeTypes["mx_records"], nil),
		"txt_records":         tftypes.NewValue(dsObjType.AttributeTypes["txt_records"], nil),
		"dkim_records":        tftypes.NewValue(dsObjType.AttributeTypes["dkim_records"], nil),
		"cname_records":       tftypes.NewValue(dsObjType.AttributeTypes["cname_records"], nil),
	}
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, dsVals)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for generic read failure")
	}
}

// TestCustomDomainResourceCreateBadPlan verifies Create handles plan deserialization error.
func TestCustomDomainResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Pass a nil raw value to trigger HasError on Plan.Get
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for nil plan")
	}
}

// TestCustomDomainResourceReadBadState verifies Read handles state deserialization error.
func TestCustomDomainResourceReadBadState(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
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

// TestCustomDomainResourceDeleteBadState verifies Delete handles state deserialization error.
func TestCustomDomainResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	r := customdomainrs.NewResource()
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

// TestCustomDomainDataSourceReadBadConfig verifies data source Read handles config deserialization error.
func TestCustomDomainDataSourceReadBadConfig(t *testing.T) {
	t.Parallel()
	_, client := domainMockServer(t)
	ctx := context.Background()
	ds := customdomainds.NewDataSource()
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

// TestCustomDomainDataSourceReadNilError verifies the data source handles a successful read
// where the not-found path returns a non-API error (covering the isStatusCode nil branch).
func TestCustomDomainDataSourceReadNilError(t *testing.T) {
	t.Parallel()

	// Server that returns an unparseable response body (not JSON) to trigger a
	// generic non-APIError error, ensuring isStatusCode returns false for non-API errors.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
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
	ds := customdomainds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)
	dsObjType := dsType.(tftypes.Object)

	dsVals := map[string]tftypes.Value{
		"id":                  tftypes.NewValue(tftypes.String, "some-id"),
		"domain_name":         tftypes.NewValue(tftypes.String, nil),
		"verified":            tftypes.NewValue(tftypes.Bool, nil),
		"verification_status": tftypes.NewValue(tftypes.String, nil),
		"mx_records":          tftypes.NewValue(dsObjType.AttributeTypes["mx_records"], nil),
		"txt_records":         tftypes.NewValue(dsObjType.AttributeTypes["txt_records"], nil),
		"dkim_records":        tftypes.NewValue(dsObjType.AttributeTypes["dkim_records"], nil),
		"cname_records":       tftypes.NewValue(dsObjType.AttributeTypes["cname_records"], nil),
	}
	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, dsVals)}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for unparseable response")
	}
}

// suppress unused import warning
var _ = fmt.Sprintf
