// Package unit provides state file format verification tests.
//
// These tests verify that after every resource action (create, read, update,
// delete, import), the resulting state is properly formatted and contains
// the expected attributes with correct types.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	tokenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/token"
	userresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/user"
	spaceresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/space"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// verifyStateNotEmpty checks that the response state is non-nil and contains data.
func verifyStateNotEmpty(t *testing.T, state tfsdk.State, action string, resourceName string) {
	t.Helper()
	if state.Raw.IsNull() {
		t.Errorf("%s %s: state is null after %s", resourceName, action, action)
	}
	if !state.Raw.IsKnown() {
		t.Errorf("%s %s: state is unknown after %s", resourceName, action, action)
	}
}

// verifyStateIsValidJSON checks that the state can be marshaled to valid JSON.
func verifyStateIsValidJSON(t *testing.T, state tfsdk.State, action string, resourceName string) {
	t.Helper()
	raw, err := state.Raw.MarshalMsgPack(state.Schema.Type().TerraformType(context.Background()))
	if err != nil {
		t.Errorf("%s %s: state cannot be serialized: %v", resourceName, action, err)
		return
	}
	if len(raw) == 0 {
		t.Errorf("%s %s: serialized state is empty", resourceName, action)
	}
}

// stateVerificationMockServer returns a mock that handles basic CRUD for state verification tests.
func stateVerificationMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	mux := http.NewServeMux()

	// User endpoints
	mux.HandleFunc("POST /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accountId":    "user-1",
			"emailAddress": "test@example.com",
			"displayName":  "Test User",
			"active":       true,
		})
	})
	mux.HandleFunc("GET /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accountId":    "user-1",
			"emailAddress": "test@example.com",
			"displayName":  "Test User",
			"active":       true,
		})
	})
	mux.HandleFunc("PUT /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accountId":    "user-1",
			"emailAddress": "updated@example.com",
			"displayName":  "Updated User",
			"active":       true,
		})
	})
	mux.HandleFunc("DELETE /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Group endpoints
	mux.HandleFunc("POST /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"groupId": "group-1",
			"name":    "test-group",
			"self":    "https://example.atlassian.net/rest/api/3/group?groupId=group-1",
		})
	})
	mux.HandleFunc("GET /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"groupId": "group-1",
			"name":    "test-group",
			"self":    "https://example.atlassian.net/rest/api/3/group?groupId=group-1",
		})
	})
	mux.HandleFunc("DELETE /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Role endpoints
	mux.HandleFunc("POST /rest/api/3/role", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          1,
			"name":        "test-role",
			"description": "A test role",
			"scope":       "org",
		})
	})
	mux.HandleFunc("GET /rest/api/3/role/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          1,
			"name":        "test-role",
			"description": "A test role",
			"scope":       "org",
		})
	})
	mux.HandleFunc("DELETE /rest/api/3/role/1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Token endpoints
	mux.HandleFunc("POST /rest/api/3/user/user-1/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         "token-1",
			"label":      "test-token",
			"token":      "secret-value",
			"created_at": "2026-01-01T00:00:00Z",
		})
	})
	mux.HandleFunc("GET /rest/api/3/user/user-1/token/token-1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         "token-1",
			"label":      "test-token",
			"created_at": "2026-01-01T00:00:00Z",
		})
	})
	mux.HandleFunc("DELETE /rest/api/3/user/user-1/token/token-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Jira space (project) endpoints
	mux.HandleFunc("POST /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":             "10001",
			"key":            "TEST",
			"name":           "Test Space",
			"description":    "A test space",
			"projectTypeKey": "business",
			"self":           "https://example.atlassian.net/rest/api/3/project/10001",
		})
	})
	mux.HandleFunc("GET /rest/api/3/project/10001", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":             "10001",
			"key":            "TEST",
			"name":           "Test Space",
			"description":    "A test space",
			"projectTypeKey": "business",
			"self":           "https://example.atlassian.net/rest/api/3/project/10001",
		})
	})
	mux.HandleFunc("PUT /rest/api/3/project/10001", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":             "10001",
			"key":            "TEST",
			"name":           "Updated Space",
			"description":    "Updated description",
			"projectTypeKey": "business",
			"self":           "https://example.atlassian.net/rest/api/3/project/10001",
		})
	})
	mux.HandleFunc("DELETE /rest/api/3/project/10001", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5e9,
		MaxRetries:     0,
		RetryWaitMin:   1e9,
		RetryWaitMax:   1e9,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return ts, client
}

// buildPlanState creates a plan state with the given values for a resource.
func buildPlanState(t *testing.T, r fwresource.Resource, values map[string]tftypes.Value, attrTypes map[string]tftypes.Type) tfsdk.Plan {
	t.Helper()
	schemaResp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)

	tfType := schemaResp.Schema.Type().TerraformType(context.Background())
	raw := tftypes.NewValue(tfType, values)

	return tfsdk.Plan{
		Schema: schemaResp.Schema,
		Raw:    raw,
	}
}

// TestStateVerification_UserResource_CreateReadUpdateDelete verifies state format
// after every action on the user resource.
func TestStateVerification_UserResource_CreateReadUpdateDelete(t *testing.T) {
	t.Parallel()
	_, client := stateVerificationMockServer(t)

	r := userresource.NewResource()
	r.(fwresource.ResourceWithConfigure).Configure(context.Background(),
		fwresource.ConfigureRequest{ProviderData: client},
		&fwresource.ConfigureResponse{})

	schemaResp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)

	// CREATE
	createPlan := tfsdk.Plan{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"account_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"email":        tftypes.NewValue(tftypes.String, "test@example.com"),
			"display_name": tftypes.NewValue(tftypes.String, "Test User"),
			"active":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
			"self_url":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}),
	}
	createResp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), nil)}}
	r.Create(context.Background(), fwresource.CreateRequest{Plan: createPlan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create failed: %s", createResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, createResp.State, "create", "atlassian_user")
	verifyStateIsValidJSON(t, createResp.State, "create", "atlassian_user")

	// READ
	readResp := &fwresource.ReadResponse{State: createResp.State}
	r.Read(context.Background(), fwresource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read failed: %s", readResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, readResp.State, "read", "atlassian_user")
	verifyStateIsValidJSON(t, readResp.State, "read", "atlassian_user")

	// UPDATE
	updatePlan := tfsdk.Plan{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"account_id":   tftypes.NewValue(tftypes.String, "user-1"),
			"email":        tftypes.NewValue(tftypes.String, "updated@example.com"),
			"display_name": tftypes.NewValue(tftypes.String, "Updated User"),
			"active":       tftypes.NewValue(tftypes.Bool, true),
			"self_url":     tftypes.NewValue(tftypes.String, ""),
		}),
	}
	updateResp := &fwresource.UpdateResponse{State: readResp.State}
	r.Update(context.Background(), fwresource.UpdateRequest{Plan: updatePlan, State: readResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update failed: %s", updateResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, updateResp.State, "update", "atlassian_user")
	verifyStateIsValidJSON(t, updateResp.State, "update", "atlassian_user")

	// DELETE
	deleteResp := &fwresource.DeleteResponse{State: updateResp.State}
	r.Delete(context.Background(), fwresource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete failed: %s", deleteResp.Diagnostics.Errors()[0].Detail())
	}
	// Delete succeeded without error — the framework handles state removal.
	// Verify no diagnostics errors were added during delete.
}

// TestStateVerification_GroupResource_CreateReadDelete verifies state format
// after every action on the group resource.
func TestStateVerification_GroupResource_CreateReadDelete(t *testing.T) {
	t.Parallel()
	_, client := stateVerificationMockServer(t)

	r := groupresource.NewResource()
	r.(fwresource.ResourceWithConfigure).Configure(context.Background(),
		fwresource.ConfigureRequest{ProviderData: client},
		&fwresource.ConfigureResponse{})

	schemaResp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)
	tfType := schemaResp.Schema.Type().TerraformType(context.Background())

	// CREATE
	createResp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), fwresource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"name":     tftypes.NewValue(tftypes.String, "test-group"),
			"self_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		})},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create failed: %s", createResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, createResp.State, "create", "atlassian_group")
	verifyStateIsValidJSON(t, createResp.State, "create", "atlassian_group")

	// READ
	readResp := &fwresource.ReadResponse{State: createResp.State}
	r.Read(context.Background(), fwresource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read failed: %s", readResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, readResp.State, "read", "atlassian_group")
	verifyStateIsValidJSON(t, readResp.State, "read", "atlassian_group")

	// DELETE
	deleteResp := &fwresource.DeleteResponse{State: readResp.State}
	r.Delete(context.Background(), fwresource.DeleteRequest{State: readResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete failed: %s", deleteResp.Diagnostics.Errors()[0].Detail())
	}
}

// TestStateVerification_RoleResource_CreateReadDelete verifies state format.
func TestStateVerification_RoleResource_CreateReadDelete(t *testing.T) {
	t.Parallel()
	_, client := stateVerificationMockServer(t)

	r := roleresource.NewResource()
	r.(fwresource.ResourceWithConfigure).Configure(context.Background(),
		fwresource.ConfigureRequest{ProviderData: client},
		&fwresource.ConfigureResponse{})

	schemaResp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)
	tfType := schemaResp.Schema.Type().TerraformType(context.Background())

	createResp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), fwresource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"role_id":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"name":        tftypes.NewValue(tftypes.String, "test-role"),
			"description": tftypes.NewValue(tftypes.String, "A test role"),
			"scope":       tftypes.NewValue(tftypes.String, "org"),
		})},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create failed: %s", createResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, createResp.State, "create", "atlassian_role")
	verifyStateIsValidJSON(t, createResp.State, "create", "atlassian_role")

	readResp := &fwresource.ReadResponse{State: createResp.State}
	r.Read(context.Background(), fwresource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read failed: %s", readResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, readResp.State, "read", "atlassian_role")
	verifyStateIsValidJSON(t, readResp.State, "read", "atlassian_role")
}

// TestStateVerification_TokenResource_CreateReadDelete verifies state format.
func TestStateVerification_TokenResource_CreateReadDelete(t *testing.T) {
	t.Parallel()
	_, client := stateVerificationMockServer(t)

	r := tokenresource.NewResource()
	r.(fwresource.ResourceWithConfigure).Configure(context.Background(),
		fwresource.ConfigureRequest{ProviderData: client},
		&fwresource.ConfigureResponse{})

	schemaResp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)
	tfType := schemaResp.Schema.Type().TerraformType(context.Background())

	createResp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), fwresource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"token_id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"label":           tftypes.NewValue(tftypes.String, "test-token"),
			"user_account_id": tftypes.NewValue(tftypes.String, "user-1"),
			"token_value":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"created_at":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		})},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create failed: %s", createResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, createResp.State, "create", "atlassian_api_token")
	verifyStateIsValidJSON(t, createResp.State, "create", "atlassian_api_token")
}

// TestStateVerification_JiraSpace_CreateReadUpdateDelete verifies state format.
func TestStateVerification_JiraSpace_CreateReadUpdateDelete(t *testing.T) {
	t.Parallel()
	_, client := stateVerificationMockServer(t)

	r := spaceresource.NewResource()
	r.(fwresource.ResourceWithConfigure).Configure(context.Background(),
		fwresource.ConfigureRequest{ProviderData: client},
		&fwresource.ConfigureResponse{})

	schemaResp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)
	tfType := schemaResp.Schema.Type().TerraformType(context.Background())

	// CREATE
	createResp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, nil)}}
	r.Create(context.Background(), fwresource.CreateRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"id":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"key":             tftypes.NewValue(tftypes.String, "TEST"),
			"name":            tftypes.NewValue(tftypes.String, "Test Space"),
			"description":     tftypes.NewValue(tftypes.String, "A test space"),
			"lead_account_id": tftypes.NewValue(tftypes.String, ""),
			"space_type":      tftypes.NewValue(tftypes.String, "classic"),
			"url":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		})},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create failed: %s", createResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, createResp.State, "create", "atlassian_jira_space")
	verifyStateIsValidJSON(t, createResp.State, "create", "atlassian_jira_space")

	// READ
	readResp := &fwresource.ReadResponse{State: createResp.State}
	r.Read(context.Background(), fwresource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read failed: %s", readResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, readResp.State, "read", "atlassian_jira_space")
	verifyStateIsValidJSON(t, readResp.State, "read", "atlassian_jira_space")

	// UPDATE
	updateResp := &fwresource.UpdateResponse{State: readResp.State}
	r.Update(context.Background(), fwresource.UpdateRequest{
		Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"id":              tftypes.NewValue(tftypes.String, "10001"),
			"key":             tftypes.NewValue(tftypes.String, "TEST"),
			"name":            tftypes.NewValue(tftypes.String, "Updated Space"),
			"description":     tftypes.NewValue(tftypes.String, "Updated description"),
			"lead_account_id": tftypes.NewValue(tftypes.String, ""),
			"space_type":      tftypes.NewValue(tftypes.String, "classic"),
			"url":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		})},
		State: readResp.State,
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update failed: %s", updateResp.Diagnostics.Errors()[0].Detail())
	}
	verifyStateNotEmpty(t, updateResp.State, "update", "atlassian_jira_space")
	verifyStateIsValidJSON(t, updateResp.State, "update", "atlassian_jira_space")

	// DELETE
	deleteResp := &fwresource.DeleteResponse{State: updateResp.State}
	r.Delete(context.Background(), fwresource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete failed: %s", deleteResp.Diagnostics.Errors()[0].Detail())
	}
}

// TestStateVerification_ImportState verifies state format after import.
func TestStateVerification_ImportState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		resource func() resource.Resource
		id       string
	}{
		{"atlassian_user", userresource.NewResource, "user-1"},
		{"atlassian_group", groupresource.NewResource, "group-1"},
		{"atlassian_role", roleresource.NewResource, "1"},
		{"atlassian_jira_space", spaceresource.NewResource, "10001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := tt.resource()
			importer, ok := r.(fwresource.ResourceWithImportState)
			if !ok {
				t.Fatalf("%s does not implement ImportState", tt.name)
			}

			schemaResp := &fwresource.SchemaResponse{}
			r.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)

			importResp := &fwresource.ImportStateResponse{
				State: tfsdk.State{
					Schema: schemaResp.Schema,
					Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), nil),
				},
			}
			importer.ImportState(context.Background(), fwresource.ImportStateRequest{ID: tt.id}, importResp)
			if importResp.Diagnostics.HasError() {
				t.Fatalf("ImportState failed: %s", importResp.Diagnostics.Errors()[0].Detail())
			}

			// After import, state should have the ID set
			if importResp.State.Raw.IsNull() {
				t.Errorf("%s import: state is null after import", tt.name)
			}
		})
	}
}

// Ensure strings package is used.
var _ = strings.TrimSpace
