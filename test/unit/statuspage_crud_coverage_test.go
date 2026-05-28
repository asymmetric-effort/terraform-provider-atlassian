// Package unit contains CRUD coverage tests for all Statuspage resources and data sources.
package unit

import (
	"context"
	"testing"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	spcomponentds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/component"
	sppageds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/page"
	spsubscriberds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/subscriber"
	spcomponentrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/component"
	sppagers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/page"
	spsubscriberrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/subscriber"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rsschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// spConfigureResource configures a resource with the test client.
func spConfigureResource(t *testing.T, r resource.Resource, c *atlassian.Client) {
	t.Helper()
	rc, ok := r.(resource.ResourceWithConfigure)
	if !ok {
		t.Fatal("resource does not implement ResourceWithConfigure")
	}
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure failed: %v", resp.Diagnostics.Errors())
	}
}

// spGetResourceSchema returns the schema for a resource.
func spGetResourceSchema(t *testing.T, r resource.Resource) rsschema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	return resp.Schema
}

// spConfigureDS configures a datasource with the test client.
func spConfigureDS(t *testing.T, d datasource.DataSource, c *atlassian.Client) {
	t.Helper()
	dc, ok := d.(datasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("datasource does not implement DataSourceWithConfigure")
	}
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: c}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("configure failed: %v", resp.Diagnostics.Errors())
	}
}

// spGetDSSchema returns the schema for a datasource.
func spGetDSSchema(t *testing.T, d datasource.DataSource) dsschema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	return resp.Schema
}

func spEmptyState(ctx context.Context, s rsschema.Schema) tfsdk.State {
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
}

func spEmptyDSState(ctx context.Context, s dsschema.Schema) tfsdk.State {
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
}

func spGetStringAttr(t *testing.T, state tfsdk.State, name string) string {
	t.Helper()
	var val struct{ V *string }
	type strVal struct {
		V string `tfsdk:"v"`
	}
	// Use raw approach
	attrs := make(map[string]tftypes.Value)
	err := state.Raw.As(&attrs)
	if err != nil {
		t.Fatalf("failed to get attrs: %v", err)
	}
	v, ok := attrs[name]
	if !ok {
		t.Fatalf("attribute %q not found", name)
	}
	var s string
	err = v.As(&s)
	if err != nil {
		t.Fatalf("failed to get %q as string: %v", name, err)
	}
	_ = val
	return s
}

// ============================================================================
// Component Resource CRUD
// ============================================================================

// TestStatuspageComponentResourceCRUD tests full CRUD through the framework.
func TestStatuspageComponentResourceCRUD(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	r := spcomponentrs.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id":     tftypes.NewValue(tftypes.String, "page-1"),
		"name":        tftypes.NewValue(tftypes.String, "API Service"),
		"description": tftypes.NewValue(tftypes.String, "Main API"),
		"status":      tftypes.NewValue(tftypes.String, "operational"),
		"group_id":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	compID := spGetStringAttr(t, createResp.State, "id")

	// Read
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, compID),
		"page_id":     tftypes.NewValue(tftypes.String, "page-1"),
		"name":        tftypes.NewValue(tftypes.String, "Updated API"),
		"description": tftypes.NewValue(tftypes.String, "Updated"),
		"status":      tftypes.NewValue(tftypes.String, "degraded_performance"),
		"group_id":    tftypes.NewValue(tftypes.String, ""),
	})}
	updateResp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: createResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResp.Diagnostics.Errors())
	}

	// Delete
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)

	// Import state
	importResp := &resource.ImportStateResponse{State: spEmptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: compID}, importResp)
}

// ============================================================================
// Component Group Resource CRUD
// ============================================================================

// TestStatuspageComponentGroupResourceCRUD tests full CRUD through the framework.
func TestStatuspageComponentGroupResourceCRUD(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	r := spcomponentrs.NewGroupResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id":     tftypes.NewValue(tftypes.String, "page-2"),
		"name":        tftypes.NewValue(tftypes.String, "Infrastructure"),
		"description": tftypes.NewValue(tftypes.String, "Infra group"),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	grpID := spGetStringAttr(t, createResp.State, "id")

	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, grpID),
		"page_id":     tftypes.NewValue(tftypes.String, "page-2"),
		"name":        tftypes.NewValue(tftypes.String, "Updated Infra"),
		"description": tftypes.NewValue(tftypes.String, "Updated"),
	})}
	updateResp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: createResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResp.Diagnostics.Errors())
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResp.Diagnostics.Errors())
	}

	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)

	importResp := &resource.ImportStateResponse{State: spEmptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: grpID}, importResp)
}

// ============================================================================
// Subscriber Resource CRUD
// ============================================================================

// TestStatuspageSubscriberResourceCRUD tests full CRUD through the framework.
func TestStatuspageSubscriberResourceCRUD(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	r := spsubscriberrs.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id":  tftypes.NewValue(tftypes.String, "page-3"),
		"email":    tftypes.NewValue(tftypes.String, "sub@example.com"),
		"endpoint": tftypes.NewValue(tftypes.String, "https://hooks.example.com"),
		"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "comp-1"),
		}),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	subID := spGetStringAttr(t, createResp.State, "id")

	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, subID),
		"page_id":  tftypes.NewValue(tftypes.String, "page-3"),
		"email":    tftypes.NewValue(tftypes.String, "updated@example.com"),
		"endpoint": tftypes.NewValue(tftypes.String, "https://hooks.example.com"),
		"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "comp-2"),
		}),
	})}
	updateResp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: createResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResp.Diagnostics.Errors())
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResp.Diagnostics.Errors())
	}

	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)

	importResp := &resource.ImportStateResponse{State: spEmptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: subID}, importResp)
}

// ============================================================================
// Incident Template Resource CRUD
// ============================================================================

// TestStatuspageIncidentTemplateResourceCRUD tests full CRUD through the framework.
func TestStatuspageIncidentTemplateResourceCRUD(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	r := sppagers.NewIncidentTemplateResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id": tftypes.NewValue(tftypes.String, "page-4"),
		"name":    tftypes.NewValue(tftypes.String, "Outage Template"),
		"title":   tftypes.NewValue(tftypes.String, "Service Outage"),
		"body":    tftypes.NewValue(tftypes.String, "Investigating."),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	tmplID := spGetStringAttr(t, createResp.State, "id")

	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tmplID),
		"page_id": tftypes.NewValue(tftypes.String, "page-4"),
		"name":    tftypes.NewValue(tftypes.String, "Updated Outage"),
		"title":   tftypes.NewValue(tftypes.String, "Major Outage"),
		"body":    tftypes.NewValue(tftypes.String, "Update body."),
	})}
	updateResp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: createResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResp.Diagnostics.Errors())
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResp.Diagnostics.Errors())
	}

	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)

	importResp := &resource.ImportStateResponse{State: spEmptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: tmplID}, importResp)
}

// ============================================================================
// Maintenance Template Resource CRUD
// ============================================================================

// TestStatuspageMaintenanceTemplateResourceCRUD tests full CRUD through the framework.
func TestStatuspageMaintenanceTemplateResourceCRUD(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	r := sppagers.NewMaintenanceTemplateResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id": tftypes.NewValue(tftypes.String, "page-5"),
		"name":    tftypes.NewValue(tftypes.String, "Maint Template"),
		"title":   tftypes.NewValue(tftypes.String, "Scheduled"),
		"body":    tftypes.NewValue(tftypes.String, "Maintenance window."),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	tmplID := spGetStringAttr(t, createResp.State, "id")

	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tmplID),
		"page_id": tftypes.NewValue(tftypes.String, "page-5"),
		"name":    tftypes.NewValue(tftypes.String, "Updated Maint"),
		"title":   tftypes.NewValue(tftypes.String, "Extended"),
		"body":    tftypes.NewValue(tftypes.String, "Extended maintenance."),
	})}
	updateResp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: createResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResp.Diagnostics.Errors())
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResp.Diagnostics.Errors())
	}

	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)

	importResp := &resource.ImportStateResponse{State: spEmptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: tmplID}, importResp)
}

// ============================================================================
// Permission Resource CRUD
// ============================================================================

// TestStatuspagePermissionResourceCRUD tests full CRUD through the framework.
func TestStatuspagePermissionResourceCRUD(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	r := sppagers.NewPermissionResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id":        tftypes.NewValue(tftypes.String, "page-6"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-789"),
		"role":           tftypes.NewValue(tftypes.String, "admin"),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	permID := spGetStringAttr(t, createResp.State, "id")

	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResp.Diagnostics.Errors())
	}

	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, permID),
		"page_id":        tftypes.NewValue(tftypes.String, "page-6"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-789"),
		"role":           tftypes.NewValue(tftypes.String, "viewer"),
	})}
	updateResp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: createResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update: %v", updateResp.Diagnostics.Errors())
	}

	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete: %v", deleteResp.Diagnostics.Errors())
	}

	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp2)

	importResp := &resource.ImportStateResponse{State: spEmptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: permID}, importResp)
}

// ============================================================================
// Page Resource Import
// ============================================================================

// TestStatuspagePageResourceImport tests ImportState.
func TestStatuspagePageResourceImport(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	s := spGetResourceSchema(t, r)

	importResp := &resource.ImportStateResponse{State: spEmptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "test-import-id"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("import: %v", importResp.Diagnostics.Errors())
	}
}

// ============================================================================
// Data Source Read Tests (through framework)
// ============================================================================

// TestStatuspagePageDataSourceRead tests the page data source Read through the framework.
func TestStatuspagePageDataSourceRead(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	// First create a page
	r := sppagers.NewResource()
	spConfigureResource(t, r, c)
	rs := spGetResourceSchema(t, r)
	rtfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rtfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":             tftypes.NewValue(tftypes.String, "DS Test Page"),
		"page_description": tftypes.NewValue(tftypes.String, "DS Desc"),
		"subdomain":        tftypes.NewValue(tftypes.String, "dstest"),
		"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	pageID := spGetStringAttr(t, createResp.State, "id")

	// Now test the data source
	d := sppageds.NewDataSource()
	spConfigureDS(t, d, c)
	ds := spGetDSSchema(t, d)
	dsTfType := ds.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, pageID),
		"name":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subdomain":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("ds read: %v", readResp.Diagnostics.Errors())
	}
}

// TestStatuspagePageDataSourceReadNotFound tests 404 handling.
func TestStatuspagePageDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	d := sppageds.NewDataSource()
	spConfigureDS(t, d, c)
	ds := spGetDSSchema(t, d)
	dsTfType := ds.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "nonexistent"),
		"name":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subdomain":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if !readResp.Diagnostics.HasError() {
		t.Error("expected error for nonexistent page")
	}
}

// TestStatuspageComponentDataSourceRead tests the component data source Read.
func TestStatuspageComponentDataSourceRead(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	// Create component
	cr := spcomponentrs.NewResource()
	spConfigureResource(t, cr, c)
	rs := spGetResourceSchema(t, cr)
	rtfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rtfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id":     tftypes.NewValue(tftypes.String, "ds-page"),
		"name":        tftypes.NewValue(tftypes.String, "DS Comp"),
		"description": tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, "operational"),
		"group_id":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, rs)}
	cr.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	compID := spGetStringAttr(t, createResp.State, "id")

	d := spcomponentds.NewDataSource()
	spConfigureDS(t, d, c)
	ds := spGetDSSchema(t, d)
	dsTfType := ds.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, compID),
		"page_id":     tftypes.NewValue(tftypes.String, "ds-page"),
		"name":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"group_id":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("ds read: %v", readResp.Diagnostics.Errors())
	}
}

// TestStatuspageComponentGroupDataSourceRead tests the component group data source Read.
func TestStatuspageComponentGroupDataSourceRead(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	cr := spcomponentrs.NewGroupResource()
	spConfigureResource(t, cr, c)
	rs := spGetResourceSchema(t, cr)
	rtfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rtfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id":     tftypes.NewValue(tftypes.String, "ds-page"),
		"name":        tftypes.NewValue(tftypes.String, "DS Group"),
		"description": tftypes.NewValue(tftypes.String, ""),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, rs)}
	cr.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	grpID := spGetStringAttr(t, createResp.State, "id")

	d := spcomponentds.NewGroupDataSource()
	spConfigureDS(t, d, c)
	ds := spGetDSSchema(t, d)
	dsTfType := ds.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, grpID),
		"page_id":     tftypes.NewValue(tftypes.String, "ds-page"),
		"name":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"description": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("ds read: %v", readResp.Diagnostics.Errors())
	}
}

// TestStatuspageSubscriberDataSourceRead tests the subscriber data source Read.
func TestStatuspageSubscriberDataSourceRead(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	cr := spsubscriberrs.NewResource()
	spConfigureResource(t, cr, c)
	rs := spGetResourceSchema(t, cr)
	rtfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rtfType, map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id":  tftypes.NewValue(tftypes.String, "ds-page"),
		"email":    tftypes.NewValue(tftypes.String, "ds@example.com"),
		"endpoint": tftypes.NewValue(tftypes.String, ""),
		"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{
			tftypes.NewValue(tftypes.String, "c1"),
		}),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, rs)}
	cr.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	subID := spGetStringAttr(t, createResp.State, "id")

	d := spsubscriberds.NewDataSource()
	spConfigureDS(t, d, c)
	ds := spGetDSSchema(t, d)
	dsTfType := ds.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, subID),
		"page_id":       tftypes.NewValue(tftypes.String, "ds-page"),
		"email":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"endpoint":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("ds read: %v", readResp.Diagnostics.Errors())
	}
}

// TestStatuspageIncidentTemplateDataSourceRead tests the incident template data source Read.
func TestStatuspageIncidentTemplateDataSourceRead(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	cr := sppagers.NewIncidentTemplateResource()
	spConfigureResource(t, cr, c)
	rs := spGetResourceSchema(t, cr)
	rtfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rtfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id": tftypes.NewValue(tftypes.String, "ds-page"),
		"name":    tftypes.NewValue(tftypes.String, "DS IT"),
		"title":   tftypes.NewValue(tftypes.String, "T"),
		"body":    tftypes.NewValue(tftypes.String, "B"),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, rs)}
	cr.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	tmplID := spGetStringAttr(t, createResp.State, "id")

	d := sppageds.NewIncidentTemplateDataSource()
	spConfigureDS(t, d, c)
	ds := spGetDSSchema(t, d)
	dsTfType := ds.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tmplID),
		"page_id": tftypes.NewValue(tftypes.String, "ds-page"),
		"name":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"title":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"body":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("ds read: %v", readResp.Diagnostics.Errors())
	}
}

// TestStatuspageMaintenanceTemplateDataSourceRead tests the maintenance template data source Read.
func TestStatuspageMaintenanceTemplateDataSourceRead(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	cr := sppagers.NewMaintenanceTemplateResource()
	spConfigureResource(t, cr, c)
	rs := spGetResourceSchema(t, cr)
	rtfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rtfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id": tftypes.NewValue(tftypes.String, "ds-page"),
		"name":    tftypes.NewValue(tftypes.String, "DS MT"),
		"title":   tftypes.NewValue(tftypes.String, "T"),
		"body":    tftypes.NewValue(tftypes.String, "B"),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, rs)}
	cr.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	tmplID := spGetStringAttr(t, createResp.State, "id")

	d := sppageds.NewMaintenanceTemplateDataSource()
	spConfigureDS(t, d, c)
	ds := spGetDSSchema(t, d)
	dsTfType := ds.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, tmplID),
		"page_id": tftypes.NewValue(tftypes.String, "ds-page"),
		"name":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"title":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"body":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("ds read: %v", readResp.Diagnostics.Errors())
	}
}

// TestStatuspagePermissionDataSourceRead tests the permission data source Read.
func TestStatuspagePermissionDataSourceRead(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)
	ctx := context.Background()

	cr := sppagers.NewPermissionResource()
	spConfigureResource(t, cr, c)
	rs := spGetResourceSchema(t, cr)
	rtfType := rs.Type().TerraformType(ctx)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rtfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"page_id":        tftypes.NewValue(tftypes.String, "ds-page"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "u1"),
		"role":           tftypes.NewValue(tftypes.String, "admin"),
	})}
	createResp := &resource.CreateResponse{State: spEmptyState(ctx, rs)}
	cr.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResp.Diagnostics.Errors())
	}
	permID := spGetStringAttr(t, createResp.State, "id")

	d := sppageds.NewPermissionDataSource()
	spConfigureDS(t, d, c)
	ds := spGetDSSchema(t, d)
	dsTfType := ds.Type().TerraformType(ctx)
	config := tfsdk.Config{Schema: ds, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, permID),
		"page_id":        tftypes.NewValue(tftypes.String, "ds-page"),
		"principal_type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"principal_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role":           tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	readResp := &datasource.ReadResponse{State: spEmptyDSState(ctx, ds)}
	d.Read(ctx, datasource.ReadRequest{Config: config}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("ds read: %v", readResp.Diagnostics.Errors())
	}
}
