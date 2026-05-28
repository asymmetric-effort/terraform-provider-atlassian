// Package unit contains tests to cover State.Get error paths in Update functions
// and other remaining hard-to-reach coverage gaps.
package unit

import (
	"context"
	"testing"

	bbrestrictionres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/branch_restriction"
	bbdeploymentres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/deployment"
	bbpipelineres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/pipeline"
	bbreporesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/repository"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// ==================== BB Branch Restriction: Update State.Get error ====================

func TestBBBranchRestrictionUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := bbrestrictionres.NewResource()
	configureResource(t, r, cgClient(t))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)

	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "1"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"pattern":    tftypes.NewValue(tftypes.String, "develop"),
		"kind":       tftypes.NewValue(tftypes.String, "push"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== BB Deployment: Update State.Get error ====================

func TestBBDeploymentUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := bbdeploymentres.NewResource()
	configureResource(t, r, cgClient(t))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)

	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":               tftypes.NewValue(tftypes.String, "env-1"),
		"repository":       tftypes.NewValue(tftypes.String, "ws/repo"),
		"name":             tftypes.NewValue(tftypes.String, "Staging"),
		"environment_type": tftypes.NewValue(tftypes.String, "Staging"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== BB Pipeline: Update State.Get error ====================

func TestBBPipelineUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := bbpipelineres.NewResource()
	configureResource(t, r, cgClient(t))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)

	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "ws/repo"),
		"repository": tftypes.NewValue(tftypes.String, "ws/repo"),
		"enabled":    tftypes.NewValue(tftypes.Bool, false),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}

// ==================== BB Repo Permission: Update State.Get error ====================

func TestBBRepoPermissionUpdateStateGetError(t *testing.T) {
	t.Parallel()
	r := bbreporesource.NewPermissionResource()
	configureResource(t, r, cgClient(t))
	s := getResourceSchema(t, r)
	ctx := context.Background()
	tfType := s.Type().TerraformType(ctx)

	validPlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "perm-1"),
		"repository":     tftypes.NewValue(tftypes.String, "ws/repo"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"permission":     tftypes.NewValue(tftypes.String, "write"),
	})}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  validPlan,
		State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(tftypes.String, "bad")},
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected error")
	}
}
