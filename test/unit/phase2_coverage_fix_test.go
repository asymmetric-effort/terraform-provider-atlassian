// Package unit contains tests to close Phase 2 (Jira) coverage gaps to >= 98%.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	automationds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/automation"
	emailds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/custom_domain"
	issuetypeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/issue_type"
	issuetyperes "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/issue_type"
	screenres "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/screen"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// p2Client creates a client pointing at the given test server.
func p2Client(t *testing.T, handler http.HandlerFunc) *atlassian.Client {
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

// ==================== Automation DS: Read() generic error + trigger/condition branches ====================

func TestAutomationDSReadGenericError500(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Server error"]}`))
	})
	ctx := context.Background()
	ds := automationds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "rule-1"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"state":          tftypes.NewValue(tftypes.String, nil),
		"trigger_type":   tftypes.NewValue(tftypes.String, nil),
		"trigger_config": tftypes.NewValue(tftypes.String, nil),
		"conditions":     tftypes.NewValue(tftypes.String, nil),
		"actions":        tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

func TestAutomationDSReadWithNullTriggerAndConditions(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":            "rule-1",
			"name":          "Test Rule",
			"state":         "ENABLED",
			"triggerType":   "jira.issue.created",
			"triggerConfig": nil,
			"conditions":    nil,
			"actions":       json.RawMessage(`[{"type":"action"}]`),
		})
	})
	ctx := context.Background()
	ds := automationds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "rule-1"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"state":          tftypes.NewValue(tftypes.String, nil),
		"trigger_type":   tftypes.NewValue(tftypes.String, nil),
		"trigger_config": tftypes.NewValue(tftypes.String, nil),
		"conditions":     tftypes.NewValue(tftypes.String, nil),
		"actions":        tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

func TestAutomationDSReadWithEmptyTriggerConfig(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return with triggerConfig as "null" (JSON null)
		w.Write([]byte(`{"id":"rule-1","name":"Test","state":"ENABLED","triggerType":"t","triggerConfig":null,"conditions":null,"actions":[{"type":"a"}]}`))
	})
	ctx := context.Background()
	ds := automationds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, "rule-1"),
		"name":           tftypes.NewValue(tftypes.String, nil),
		"state":          tftypes.NewValue(tftypes.String, nil),
		"trigger_type":   tftypes.NewValue(tftypes.String, nil),
		"trigger_config": tftypes.NewValue(tftypes.String, nil),
		"conditions":     tftypes.NewValue(tftypes.String, nil),
		"actions":        tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Email DS: mapEmailDSAPIToModel with SpaceID set ====================

func TestEmailDSMapWithSpaceID(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           "email-1",
			"emailAddress": "test@example.com",
			"domainId":     "dom-1",
			"spaceId":      "space-1",
			"active":       true,
		})
	})
	ctx := context.Background()
	ds := emailds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "email-1"),
		"email_address": tftypes.NewValue(tftypes.String, nil),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

func TestEmailDSMapWithoutSpaceID(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           "email-2",
			"emailAddress": "nospace@example.com",
			"domainId":     "dom-1",
			"active":       false,
		})
	})
	ctx := context.Background()
	ds := emailds.NewEmailDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":            tftypes.NewValue(tftypes.String, "email-2"),
		"email_address": tftypes.NewValue(tftypes.String, nil),
		"domain_id":     tftypes.NewValue(tftypes.String, nil),
		"space_id":      tftypes.NewValue(tftypes.String, nil),
		"active":        tftypes.NewValue(tftypes.Bool, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Issue Type DS: Read() generic error ====================

func TestIssueTypeDSReadGenericError500(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Server error"]}`))
	})
	ctx := context.Background()
	ds := issuetypeds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "it-1"),
		"name":            tftypes.NewValue(tftypes.String, nil),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, nil),
		"subtask":         tftypes.NewValue(tftypes.Bool, nil),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== Issue Type Scheme DS: Read() generic error ====================

func TestIssueTypeSchemeDSReadGenericError500(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Server error"]}`))
	})
	ctx := context.Background()
	ds := issuetypeds.NewSchemeDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "its-1"),
		"name":                  tftypes.NewValue(tftypes.String, nil),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== Issue Type Resource: Create() 403 forbidden ====================

func TestIssueTypeResourceCreate403(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Forbidden"},
		})
	})
	ctx := context.Background()
	r := issuetyperes.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Test"),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
	found := false
	for _, e := range resp.Diagnostics.Errors() {
		if e.Summary() == "Permission denied" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'Permission denied' error")
	}
}

// ==================== Issue Type Resource: Create() 400 bad request ====================

func TestIssueTypeResourceCreate400(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Bad request"},
		})
	})
	ctx := context.Background()
	r := issuetyperes.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Bad Name"),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 400 error")
	}
}

// ==================== Issue Type Resource: Create() generic error ====================

func TestIssueTypeResourceCreateGenericError(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Server error"]}`))
	})
	ctx := context.Background()
	r := issuetyperes.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":            tftypes.NewValue(tftypes.String, "Test"),
		"description":     tftypes.NewValue(tftypes.String, nil),
		"icon_url":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"subtask":         tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"hierarchy_level": tftypes.NewValue(tftypes.Number, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== Issue Type Scheme Resource: extractIssueTypeIDs with null list ====================

func TestIssueTypeSchemeResourceCreateWithNullList(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "its-1", "name": "Scheme", "description": "d", "defaultIssueTypeId": "",
		})
	})
	ctx := context.Background()
	r := issuetyperes.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Null issue_type_ids
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":                  tftypes.NewValue(tftypes.String, "Scheme"),
		"description":           tftypes.NewValue(tftypes.String, nil),
		"issue_type_ids":        tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		"default_issue_type_id": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", resp.Diagnostics.Errors())
	}
}

// ==================== Screen Resource: Create() 400 and 403 ====================

func TestScreenResourceCreate400(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Bad request"},
		})
	})
	ctx := context.Background()
	r := screenres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Screen"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 400 error")
	}
}

func TestScreenResourceCreate403(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Forbidden"},
		})
	})
	ctx := context.Background()
	r := screenres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Screen"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

func TestScreenResourceCreateGenericError(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	r := screenres.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Screen"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== Screen Scheme Resource: Create() 400 and 403 ====================

func TestScreenSchemeResourceCreate400(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Bad request"},
		})
	})
	ctx := context.Background()
	r := screenres.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Scheme"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 400 error")
	}
}

func TestScreenSchemeResourceCreate403(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Forbidden"},
		})
	})
	ctx := context.Background()
	r := screenres.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Scheme"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected 403 error")
	}
}

func TestScreenSchemeResourceCreateGenericError(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	r := screenres.NewSchemeResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Scheme"),
		"description": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

// ==================== Tab Field Resource: Read() field not found ====================

func TestTabFieldResourceReadFieldNotFound(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return empty list - field not found
		json.NewEncoder(w).Encode([]interface{}{})
	})
	ctx := context.Background()
	r := screenres.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "1/2/customfield_1"),
		"screen_id": tftypes.NewValue(tftypes.String, "1"),
		"tab_id":    tftypes.NewValue(tftypes.String, "2"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("expected silent removal on field not found")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be removed when field not found")
	}
}

func TestTabFieldResourceReadGenericError(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		w.Write([]byte(`{"errorMessages":["Error"]}`))
	})
	ctx := context.Background()
	r := screenres.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "1/2/customfield_1"),
		"screen_id": tftypes.NewValue(tftypes.String, "1"),
		"tab_id":    tftypes.NewValue(tftypes.String, "2"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected generic error")
	}
}

func TestTabFieldResourceRead404RemovesResource(t *testing.T) {
	t.Parallel()
	client := p2Client(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		w.Write([]byte(`{"errorMessages":["Not found"]}`))
	})
	ctx := context.Background()
	r := screenres.NewTabFieldResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, "1/2/customfield_1"),
		"screen_id": tftypes.NewValue(tftypes.String, "1"),
		"tab_id":    tftypes.NewValue(tftypes.String, "2"),
		"field_id":  tftypes.NewValue(tftypes.String, "customfield_1"),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("expected 404 to silently remove resource")
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected state to be removed on 404")
	}
}
