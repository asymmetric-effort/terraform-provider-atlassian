// Package unit contains coverage-closing tests for Statuspage resources.
// These tests trigger HasError() early returns by passing invalid Raw values.
package unit

import (
	"context"
	"testing"

	spcomponentrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/component"
	sppagers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/page"
	spsubscriberrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/subscriber"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var badRaw = tftypes.NewValue(tftypes.String, "bad")

// ===== Page Resource HasError guards =====

func TestSPPageCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.CreateResponse{State: spEmptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: s, Raw: badRaw},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPPageReadStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.ReadResponse{State: spEmptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{
		State: tfsdk.State{Schema: s, Raw: badRaw},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPPageUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "name": tftypes.NewValue(tftypes.String, "x"),
		"page_description": tftypes.NewValue(tftypes.String, ""), "subdomain": tftypes.NewValue(tftypes.String, ""),
		"url": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: s, Raw: badRaw},
		State: validState,
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPPageUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "name": tftypes.NewValue(tftypes.String, "x"),
		"page_description": tftypes.NewValue(tftypes.String, ""), "subdomain": tftypes.NewValue(tftypes.String, ""),
		"url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: badRaw},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPPageDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{
		State: tfsdk.State{Schema: s, Raw: badRaw},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ===== Component Resource HasError guards =====

func TestSPComponentCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.CreateResponse{State: spEmptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPComponentReadStateGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.ReadResponse{State: spEmptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPComponentUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vs := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"name": tftypes.NewValue(tftypes.String, "x"), "description": tftypes.NewValue(tftypes.String, ""),
		"status": tftypes.NewValue(tftypes.String, ""), "group_id": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}, State: vs}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPComponentUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vp := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"name": tftypes.NewValue(tftypes.String, "x"), "description": tftypes.NewValue(tftypes.String, ""),
		"status": tftypes.NewValue(tftypes.String, ""), "group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: vp, State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPComponentDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ===== Component Group Resource HasError guards =====

func TestSPComponentGroupCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewGroupResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.CreateResponse{State: spEmptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPComponentGroupReadStateGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewGroupResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.ReadResponse{State: spEmptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPComponentGroupUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewGroupResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vs := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"name": tftypes.NewValue(tftypes.String, "x"), "description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}, State: vs}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPComponentGroupUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewGroupResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vp := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"name": tftypes.NewValue(tftypes.String, "x"), "description": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: vp, State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPComponentGroupDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewGroupResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ===== Subscriber Resource HasError guards =====

func TestSPSubscriberCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := spsubscriberrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.CreateResponse{State: spEmptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPSubscriberReadStateGetError(t *testing.T) {
	t.Parallel()
	r := spsubscriberrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.ReadResponse{State: spEmptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPSubscriberUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := spsubscriberrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vs := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"email": tftypes.NewValue(tftypes.String, ""), "endpoint": tftypes.NewValue(tftypes.String, ""),
		"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{}),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}, State: vs}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPSubscriberUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := spsubscriberrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vp := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"email": tftypes.NewValue(tftypes.String, ""), "endpoint": tftypes.NewValue(tftypes.String, ""),
		"component_ids": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, []tftypes.Value{}),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: vp, State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPSubscriberDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := spsubscriberrs.NewResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ===== Incident Template Resource HasError guards =====

func TestSPIncidentTemplateCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewIncidentTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.CreateResponse{State: spEmptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPIncidentTemplateReadStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewIncidentTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.ReadResponse{State: spEmptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPIncidentTemplateUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewIncidentTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vs := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"name": tftypes.NewValue(tftypes.String, "x"), "title": tftypes.NewValue(tftypes.String, ""),
		"body": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}, State: vs}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPIncidentTemplateUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewIncidentTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vp := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"name": tftypes.NewValue(tftypes.String, "x"), "title": tftypes.NewValue(tftypes.String, ""),
		"body": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: vp, State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPIncidentTemplateDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewIncidentTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ===== Maintenance Template Resource HasError guards =====

func TestSPMaintenanceTemplateCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewMaintenanceTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.CreateResponse{State: spEmptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPMaintenanceTemplateReadStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewMaintenanceTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.ReadResponse{State: spEmptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPMaintenanceTemplateUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewMaintenanceTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vs := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"name": tftypes.NewValue(tftypes.String, "x"), "title": tftypes.NewValue(tftypes.String, ""),
		"body": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}, State: vs}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPMaintenanceTemplateUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewMaintenanceTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vp := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"name": tftypes.NewValue(tftypes.String, "x"), "title": tftypes.NewValue(tftypes.String, ""),
		"body": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: vp, State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPMaintenanceTemplateDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewMaintenanceTemplateResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ===== Permission Resource HasError guards =====

func TestSPPermissionCreatePlanGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewPermissionResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.CreateResponse{State: spEmptyState(context.Background(), s)}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPPermissionReadStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewPermissionResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.ReadResponse{State: spEmptyState(context.Background(), s)}
	r.Read(context.Background(), resource.ReadRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPPermissionUpdatePlanGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewPermissionResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vs := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, "x"), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u"),
		"role": tftypes.NewValue(tftypes.String, "admin"),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: badRaw}, State: vs}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPPermissionUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewPermissionResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)
	vp := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue), "page_id": tftypes.NewValue(tftypes.String, "p"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"), "principal_id": tftypes.NewValue(tftypes.String, "u"),
		"role": tftypes.NewValue(tftypes.String, "admin"),
	})}
	resp := &resource.UpdateResponse{State: spEmptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: vp, State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

func TestSPPermissionDeleteStateGetError(t *testing.T) {
	t.Parallel()
	r := sppagers.NewPermissionResource()
	spConfigureResource(t, r, qc(t, okHandler))
	s := spGetResourceSchema(t, r)
	resp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: tfsdk.State{Schema: s, Raw: badRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}
