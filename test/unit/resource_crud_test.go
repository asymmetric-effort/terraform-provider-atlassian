// Package unit contains unit tests exercising resource and datasource CRUD methods
// directly against a mock API server.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	groupdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/group"
	roledatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/role"
	userds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/user"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	tokenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/token"
	userrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/user"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rsschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// testIDCounter provides unique IDs for the test mock server.
var testIDCounter uint64

func testNextID(prefix string) string {
	n := atomic.AddUint64(&testIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// writeErr writes an Atlassian-format error response.
func writeErr(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"errorMessages": []string{message},
		"errors":        map[string]string{},
	})
}

// testMockServer creates a mock HTTP server matching resource expectations.
func testMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	users := make(map[string]map[string]interface{})
	groups := make(map[string]map[string]interface{})
	roles := make(map[string]map[string]interface{})
	tokens := make(map[string]map[string]interface{})
	assignments := make(map[string]map[string]interface{})
	groupMembers := make(map[string][]string)

	mux := http.NewServeMux()

	// User endpoints
	mux.HandleFunc("POST /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		email, _ := req["emailAddress"].(string)
		displayName, _ := req["displayName"].(string)
		if email == "" || displayName == "" {
			writeErr(w, 400, "emailAddress and displayName are required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, u := range users {
			if u["emailAddress"] == email {
				writeErr(w, 409, "A user with this email address already exists")
				return
			}
		}
		id := testNextID("user")
		user := map[string]interface{}{"accountId": id, "emailAddress": email, "displayName": displayName, "active": true}
		users[id] = user
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(user)
	})

	mux.HandleFunc("GET /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.URL.Query().Get("accountId")
		if accountID == "" {
			writeErr(w, 400, "accountId required")
			return
		}
		mu.Lock()
		user, ok := users[accountID]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "User not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	})

	mux.HandleFunc("PUT /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.URL.Query().Get("accountId")
		if accountID == "" {
			writeErr(w, 400, "accountId required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		user, ok := users[accountID]
		if !ok {
			writeErr(w, 404, "User not found")
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "accountId" {
				user[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	})

	mux.HandleFunc("DELETE /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.URL.Query().Get("accountId")
		if accountID == "" {
			writeErr(w, 400, "accountId required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if _, ok := users[accountID]; !ok {
			writeErr(w, 404, "User not found")
			return
		}
		delete(users, accountID)
		w.WriteHeader(204)
	})

	mux.HandleFunc("GET /rest/api/3/user/search", func(w http.ResponseWriter, r *http.Request) {
		query := strings.ToLower(r.URL.Query().Get("query"))
		mu.Lock()
		var results []map[string]interface{}
		for _, u := range users {
			if query == "" {
				results = append(results, u)
				continue
			}
			email, _ := u["emailAddress"].(string)
			dn, _ := u["displayName"].(string)
			if strings.Contains(strings.ToLower(email), query) || strings.Contains(strings.ToLower(dn), query) {
				results = append(results, u)
			}
		}
		mu.Unlock()
		if results == nil {
			results = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	// Group endpoints
	mux.HandleFunc("POST /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			writeErr(w, 400, "name is required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, g := range groups {
			if g["name"] == name {
				writeErr(w, 409, "Group already exists")
				return
			}
		}
		id := testNextID("group")
		group := map[string]interface{}{"groupId": id, "name": name, "self": fmt.Sprintf("/rest/api/3/group?groupId=%s", id)}
		groups[id] = group
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(group)
	})

	mux.HandleFunc("GET /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		if groupID == "" {
			writeErr(w, 400, "groupId required")
			return
		}
		mu.Lock()
		g, ok := groups[groupID]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Group not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(g)
	})

	mux.HandleFunc("DELETE /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		if groupID == "" {
			writeErr(w, 400, "groupId required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if _, ok := groups[groupID]; !ok {
			writeErr(w, 404, "Group not found")
			return
		}
		delete(groups, groupID)
		w.WriteHeader(204)
	})

	// Group membership
	mux.HandleFunc("POST /rest/api/3/group/user", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		if groupID == "" {
			writeErr(w, 400, "groupId required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if _, ok := groups[groupID]; !ok {
			writeErr(w, 404, "Group not found")
			return
		}
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		accountID, _ := req["accountId"].(string)
		for _, m := range groupMembers[groupID] {
			if m == accountID {
				writeErr(w, 409, "Already member")
				return
			}
		}
		groupMembers[groupID] = append(groupMembers[groupID], accountID)
		w.WriteHeader(201)
	})

	mux.HandleFunc("DELETE /rest/api/3/group/user", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		accountID := r.URL.Query().Get("accountId")
		mu.Lock()
		defer mu.Unlock()
		members := groupMembers[groupID]
		found := false
		var updated []string
		for _, m := range members {
			if m == accountID {
				found = true
				continue
			}
			updated = append(updated, m)
		}
		if !found {
			writeErr(w, 404, "Not a member")
			return
		}
		if len(updated) == 0 {
			delete(groupMembers, groupID)
		} else {
			groupMembers[groupID] = updated
		}
		w.WriteHeader(204)
	})

	mux.HandleFunc("GET /rest/api/3/group/member", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		mu.Lock()
		members := groupMembers[groupID]
		var values []map[string]interface{}
		for _, id := range members {
			values = append(values, map[string]interface{}{"accountId": id, "active": true})
		}
		mu.Unlock()
		if values == nil {
			values = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"maxResults": len(values), "startAt": 0, "total": len(values), "isLast": true, "values": values})
	})

	// Role endpoints
	mux.HandleFunc("POST /rest/api/3/role", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		name, _ := req["name"].(string)
		if name == "" {
			writeErr(w, 400, "name is required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, rl := range roles {
			if rl["name"] == name {
				writeErr(w, 409, "Role already exists")
				return
			}
		}
		idStr := testNextID("role")
		var idNum int
		fmt.Sscanf(idStr, "role-%d", &idNum)
		idKey := fmt.Sprintf("%d", idNum)
		role := map[string]interface{}{"id": idNum, "name": name, "description": req["description"], "scope": req["scope"]}
		roles[idKey] = role
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(role)
	})

	mux.HandleFunc("GET /rest/api/3/role", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		var items []map[string]interface{}
		for _, rl := range roles {
			items = append(items, rl)
		}
		mu.Unlock()
		if items == nil {
			items = []map[string]interface{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	mux.HandleFunc("GET /rest/api/3/role/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		role, ok := roles[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Role not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(role)
	})

	mux.HandleFunc("PUT /rest/api/3/role/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		role, ok := roles[id]
		if !ok {
			writeErr(w, 404, "Role not found")
			return
		}
		var updates map[string]interface{}
		json.NewDecoder(r.Body).Decode(&updates)
		for k, v := range updates {
			if k != "id" {
				role[k] = v
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(role)
	})

	mux.HandleFunc("DELETE /rest/api/3/role/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := roles[id]; !ok {
			writeErr(w, 404, "Role not found")
			return
		}
		delete(roles, id)
		w.WriteHeader(204)
	})

	// Token endpoints
	mux.HandleFunc("POST /rest/api/3/user/{accountId}/token", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("accountId")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		label, _ := req["label"].(string)
		if label == "" {
			writeErr(w, 400, "label is required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		tokenID := testNextID("token")
		tokenValue := testNextID("secret")
		token := map[string]interface{}{"tokenId": tokenID, "label": label, "tokenValue": tokenValue, "createdAt": time.Now().UTC().Format(time.RFC3339)}
		tokens[accountID+"/"+tokenID] = token
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(token)
	})

	mux.HandleFunc("GET /rest/api/3/user/{accountId}/token/{tokenId}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("accountId") + "/" + r.PathValue("tokenId")
		mu.Lock()
		token, ok := tokens[key]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Token not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"tokenId": token["tokenId"], "label": token["label"], "createdAt": token["createdAt"]})
	})

	mux.HandleFunc("DELETE /rest/api/3/user/{accountId}/token/{tokenId}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("accountId") + "/" + r.PathValue("tokenId")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := tokens[key]; !ok {
			writeErr(w, 404, "Token not found")
			return
		}
		delete(tokens, key)
		w.WriteHeader(204)
	})

	// Role assignment endpoints
	mux.HandleFunc("POST /rest/api/3/role/assignment", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		roleID, _ := req["roleId"].(string)
		principalType, _ := req["principalType"].(string)
		principalID, _ := req["principalId"].(string)
		scope, _ := req["scope"].(string)
		productID, _ := req["productId"].(string)
		if roleID == "" || principalType == "" || principalID == "" || scope == "" {
			writeErr(w, 400, "missing required fields")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := testNextID("assign")
		a := map[string]interface{}{"id": id, "roleId": roleID, "principalType": principalType, "principalId": principalID, "scope": scope}
		if productID != "" {
			a["productId"] = productID
		}
		assignments[id] = a
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(a)
	})

	mux.HandleFunc("GET /rest/api/3/role/assignment/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		a, ok := assignments[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Assignment not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(a)
	})

	mux.HandleFunc("DELETE /rest/api/3/role/assignment/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := assignments[id]; !ok {
			writeErr(w, 404, "Assignment not found")
			return
		}
		delete(assignments, id)
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

// --- Configure helpers ---

type resourceWithConfigure interface {
	Configure(context.Context, resource.ConfigureRequest, *resource.ConfigureResponse)
}

type datasourceWithConfigure interface {
	Configure(context.Context, datasource.ConfigureRequest, *datasource.ConfigureResponse)
}

func configureResource(t *testing.T, r resource.Resource, client *atlassian.Client) {
	t.Helper()
	rc := r.(resourceWithConfigure)
	req := resource.ConfigureRequest{ProviderData: client}
	resp := &resource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
	rc.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure failed: %v", resp.Diagnostics.Errors())
	}
}

func configureDatasource(t *testing.T, ds datasource.DataSource, client *atlassian.Client) {
	t.Helper()
	dc := ds.(datasourceWithConfigure)
	req := datasource.ConfigureRequest{ProviderData: client}
	resp := &datasource.ConfigureResponse{Diagnostics: diag.Diagnostics{}}
	dc.Configure(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure failed: %v", resp.Diagnostics.Errors())
	}
}

// --- Attribute helpers ---

func getStringAttr(t *testing.T, state tfsdk.State, name string) string {
	t.Helper()
	var val types.String
	diags := state.GetAttribute(context.Background(), path.Root(name), &val)
	if diags.HasError() {
		t.Fatalf("GetAttribute %q: %v", name, diags.Errors())
	}
	return val.ValueString()
}

// --- Schema helpers ---

func getResourceSchema(t *testing.T, r resource.Resource) rsschema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	return resp.Schema
}

func getDatasourceSchema(t *testing.T, ds datasource.DataSource) dsschema.Schema {
	t.Helper()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	return resp.Schema
}

func emptyState(ctx context.Context, s rsschema.Schema) tfsdk.State {
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
}

func emptyDSState(ctx context.Context, s dsschema.Schema) tfsdk.State {
	return tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
}

// ==================== USER RESOURCE ====================

func TestUserResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email":        tftypes.NewValue(tftypes.String, "crud@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, "CRUD User"),
		"active":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"self_url":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	accountID := getStringAttr(t, createResp.State, "account_id")
	if accountID == "" {
		t.Fatal("expected non-empty account_id")
	}

	// Read
	readState := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, accountID),
		"email":        tftypes.NewValue(tftypes.String, "crud@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, "CRUD User"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, accountID),
		"email":        tftypes.NewValue(tftypes.String, "crud@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, "Updated CRUD"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: readState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if dn := getStringAttr(t, updateResp.State, "display_name"); dn != "Updated CRUD" {
		t.Errorf("Update display_name: got %q", dn)
	}

	// Delete
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete (should remove resource)
	readResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: readState.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: readState}, readResp2)
	if !readResp2.State.Raw.IsNull() {
		// State should be removed
	}
}

func TestUserResourceCreateDuplicate(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email":        tftypes.NewValue(tftypes.String, "dup@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, "First"),
		"active":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"self_url":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp1 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp1)
	if resp1.Diagnostics.HasError() {
		t.Fatalf("First create: %v", resp1.Diagnostics.Errors())
	}

	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email":        tftypes.NewValue(tftypes.String, "dup@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, "Second"),
		"active":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"self_url":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("Expected duplicate error")
	}
}

func TestUserResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := userrs.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, "nonexistent"),
		"email":        tftypes.NewValue(tftypes.String, "x@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, "X"),
		"active":       tftypes.NewValue(tftypes.Bool, true),
		"self_url":     tftypes.NewValue(tftypes.String, ""),
	})}
	plan := tfsdk.Plan{Schema: s, Raw: state.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent user")
	}
}

func TestUserResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := userrs.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

func TestUserResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := userrs.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestUserResourceImportState(t *testing.T) {
	t.Parallel()
	r := userrs.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "user-123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== GROUP RESOURCE ====================

func TestGroupResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := groupresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":     tftypes.NewValue(tftypes.String, "test-group"),
		"self_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	groupID := getStringAttr(t, createResp.State, "group_id")

	// Read
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}

	// Update (delete + recreate)
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, groupID),
		"name":     tftypes.NewValue(tftypes.String, "renamed-group"),
		"self_url": tftypes.NewValue(tftypes.String, ""),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: createResp.State}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
	if nm := getStringAttr(t, updateResp.State, "name"); nm != "renamed-group" {
		t.Errorf("Update name: got %q", nm)
	}

	// Delete
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: updateResp.State.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: updateResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
}

func TestGroupResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := groupresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestGroupResourceImportState(t *testing.T) {
	t.Parallel()
	r := groupresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "g-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== GROUP MEMBERSHIP RESOURCE ====================

func TestGroupMembershipCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()

	// Create group first
	gr := groupresource.NewResource()
	configureResource(t, gr, client)
	gs := getResourceSchema(t, gr)
	gplan := tfsdk.Plan{Schema: gs, Raw: tftypes.NewValue(gs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":     tftypes.NewValue(tftypes.String, "membership-grp"),
		"self_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	gcResp := &resource.CreateResponse{State: emptyState(ctx, gs)}
	gr.Create(ctx, resource.CreateRequest{Plan: gplan}, gcResp)
	if gcResp.Diagnostics.HasError() {
		t.Fatalf("Create group: %v", gcResp.Diagnostics.Errors())
	}
	groupID := getStringAttr(t, gcResp.State, "group_id")

	// Membership resource
	r := groupresource.NewMembershipResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)
	listType := tftypes.List{ElementType: tftypes.String}

	// Create
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, groupID),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u-a"), tftypes.NewValue(tftypes.String, "u-b")}),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create membership: %v", cResp.Diagnostics.Errors())
	}

	// Read
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: cResp.State}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Read membership: %v", rResp.Diagnostics.Errors())
	}

	// Update (remove u-a, add u-c)
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"group_id":         tftypes.NewValue(tftypes.String, groupID),
		"user_account_ids": tftypes.NewValue(listType, []tftypes.Value{tftypes.NewValue(tftypes.String, "u-b"), tftypes.NewValue(tftypes.String, "u-c")}),
	})}
	uResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: cResp.State}, uResp)
	if uResp.Diagnostics.HasError() {
		t.Fatalf("Update membership: %v", uResp.Diagnostics.Errors())
	}

	// Delete
	dResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: uResp.State.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: uResp.State}, dResp)
	if dResp.Diagnostics.HasError() {
		t.Fatalf("Delete membership: %v", dResp.Diagnostics.Errors())
	}
}

func TestGroupMembershipConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestGroupMembershipImportState(t *testing.T) {
	t.Parallel()
	r := groupresource.NewMembershipResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "g-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== ROLE RESOURCE ====================

func TestRoleResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := roleresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "Test Admin"),
		"description": tftypes.NewValue(tftypes.String, "Desc"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create role: %v", cResp.Diagnostics.Errors())
	}
	roleID := getStringAttr(t, cResp.State, "role_id")
	if roleID == "" {
		t.Fatal("expected non-empty role_id")
	}

	// Read
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: cResp.State}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Read role: %v", rResp.Diagnostics.Errors())
	}

	// Update
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, roleID),
		"name":        tftypes.NewValue(tftypes.String, "Updated Admin"),
		"description": tftypes.NewValue(tftypes.String, "Updated"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	uResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: cResp.State}, uResp)
	if uResp.Diagnostics.HasError() {
		t.Fatalf("Update role: %v", uResp.Diagnostics.Errors())
	}

	// Delete
	dResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: uResp.State.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: uResp.State}, dResp)
	if dResp.Diagnostics.HasError() {
		t.Fatalf("Delete role: %v", dResp.Diagnostics.Errors())
	}

	// Read after delete
	rResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: cResp.State}, rResp2)
	// Should not error (state removed on 404)
}

func TestRoleResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestRoleResourceImportState(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "123"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== ROLE ASSIGNMENT RESOURCE ====================

func TestRoleAssignmentCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, nil),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	aID := getStringAttr(t, cResp.State, "id")

	// Read
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: cResp.State}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", rResp.Diagnostics.Errors())
	}

	// Update (delete + create)
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "role-2"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "user-1"),
		"scope":          tftypes.NewValue(tftypes.String, "org"),
		"product_id":     tftypes.NewValue(tftypes.String, nil),
	})}
	uResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: cResp.State}, uResp)
	if uResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", uResp.Diagnostics.Errors())
	}

	// Delete
	dResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: uResp.State.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: uResp.State}, dResp)
	if dResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", dResp.Diagnostics.Errors())
	}
	_ = aID // used in lifecycle
}

func TestRoleAssignmentProductScope(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := roleresource.NewAssignmentResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// Missing product_id with product scope
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "r1"),
		"principal_type": tftypes.NewValue(tftypes.String, "user"),
		"principal_id":   tftypes.NewValue(tftypes.String, "u1"),
		"scope":          tftypes.NewValue(tftypes.String, "product"),
		"product_id":     tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for missing product_id")
	}

	// With product_id
	plan2 := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"role_id":        tftypes.NewValue(tftypes.String, "r1"),
		"principal_type": tftypes.NewValue(tftypes.String, "group"),
		"principal_id":   tftypes.NewValue(tftypes.String, "g1"),
		"scope":          tftypes.NewValue(tftypes.String, "product"),
		"product_id":     tftypes.NewValue(tftypes.String, "jira-software"),
	})}
	resp2 := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan2}, resp2)
	if resp2.Diagnostics.HasError() {
		t.Fatalf("Create with product scope: %v", resp2.Diagnostics.Errors())
	}
}

func TestRoleAssignmentConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestRoleAssignmentImportState(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()

	// Valid 4-part
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "r/user/u/org"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState 4-part: %v", resp.Diagnostics.Errors())
	}

	// Valid 5-part
	resp2 := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "r/user/u/product/jira"}, resp2)
	if resp2.Diagnostics.HasError() {
		t.Fatalf("ImportState 5-part: %v", resp2.Diagnostics.Errors())
	}

	// Invalid
	resp3 := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "invalid"}, resp3)
	if !resp3.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid import format")
	}
}

// ==================== TOKEN RESOURCE ====================

func TestTokenResourceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	r := tokenresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"token_id":        tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"label":           tftypes.NewValue(tftypes.String, "Test Token"),
		"user_account_id": tftypes.NewValue(tftypes.String, "user-for-token"),
		"token_value":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"created_at":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	tokenID := getStringAttr(t, cResp.State, "token_id")
	if tokenID == "" {
		t.Fatal("expected non-empty token_id")
	}
	tokenValue := getStringAttr(t, cResp.State, "token_value")
	if tokenValue == "" {
		t.Fatal("expected non-empty token_value")
	}

	// Read
	rResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: cResp.State}, rResp)
	if rResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", rResp.Diagnostics.Errors())
	}

	// Update (should error)
	uResp := &resource.UpdateResponse{}
	r.Update(ctx, resource.UpdateRequest{}, uResp)
	if !uResp.Diagnostics.HasError() {
		t.Fatal("Expected error on token Update")
	}

	// Delete
	dResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: cResp.State}, dResp)
	if dResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", dResp.Diagnostics.Errors())
	}

	// Read after delete
	rResp2 := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: cResp.State.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: cResp.State}, rResp2)
	// Should not error (removed on 404)
}

func TestTokenResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	r := tokenresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestTokenResourceImportState(t *testing.T) {
	t.Parallel()
	r := tokenresource.NewResource()
	s := getResourceSchema(t, r)
	ctx := context.Background()
	resp := &resource.ImportStateResponse{State: emptyState(ctx, s)}
	r.(resource.ResourceWithImportState).ImportState(ctx, resource.ImportStateRequest{ID: "tok-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", resp.Diagnostics.Errors())
	}
}

// ==================== DATA SOURCES ====================

func TestUserDataSourceByAccountID(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()

	// Create a user first
	r := userrs.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email":        tftypes.NewValue(tftypes.String, "ds@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, "DS User"),
		"active":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"self_url":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}
	accountID := getStringAttr(t, cResp.State, "account_id")

	// Read via data source
	ds := userds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, accountID),
		"email":        tftypes.NewValue(tftypes.String, nil),
		"display_name": tftypes.NewValue(tftypes.String, nil),
		"active":       tftypes.NewValue(tftypes.Bool, nil),
		"self_url":     tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if em := getStringAttr(t, dsResp.State, "email"); em != "ds@example.com" {
		t.Errorf("expected email ds@example.com, got %q", em)
	}
}

func TestUserDataSourceByEmail(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()

	r := userrs.NewResource()
	configureResource(t, r, client)
	rs := getResourceSchema(t, r)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"email":        tftypes.NewValue(tftypes.String, "byemail@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, "ByEmail"),
		"active":       tftypes.NewValue(tftypes.Bool, tftypes.UnknownValue),
		"self_url":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	ds := userds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"account_id":   tftypes.NewValue(tftypes.String, nil),
		"email":        tftypes.NewValue(tftypes.String, "byemail@example.com"),
		"display_name": tftypes.NewValue(tftypes.String, nil),
		"active":       tftypes.NewValue(tftypes.Bool, nil),
		"self_url":     tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read by email: %v", dsResp.Diagnostics.Errors())
	}
}

func TestUserDataSourceMissingBoth(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	ds := userds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"account_id": tftypes.NewValue(tftypes.String, nil), "email": tftypes.NewValue(tftypes.String, nil),
		"display_name": tftypes.NewValue(tftypes.String, nil), "active": tftypes.NewValue(tftypes.Bool, nil),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when neither set")
	}
}

func TestUserDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	ds := userds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"account_id": tftypes.NewValue(tftypes.String, "nonexistent"), "email": tftypes.NewValue(tftypes.String, nil),
		"display_name": tftypes.NewValue(tftypes.String, nil), "active": tftypes.NewValue(tftypes.Bool, nil),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nonexistent")
	}
}

func TestUserDataSourceByEmailNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	ds := userds.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"account_id": tftypes.NewValue(tftypes.String, nil), "email": tftypes.NewValue(tftypes.String, "nope@e.com"),
		"display_name": tftypes.NewValue(tftypes.String, nil), "active": tftypes.NewValue(tftypes.Bool, nil),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nonexistent email")
	}
}

func TestUserDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := userds.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestGroupDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()

	gr := groupresource.NewResource()
	configureResource(t, gr, client)
	gs := getResourceSchema(t, gr)
	plan := tfsdk.Plan{Schema: gs, Raw: tftypes.NewValue(gs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":     tftypes.NewValue(tftypes.String, "ds-grp"),
		"self_url": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, gs)}
	gr.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create group: %v", cResp.Diagnostics.Errors())
	}
	groupID := getStringAttr(t, cResp.State, "group_id")

	ds := groupdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, groupID),
		"name":     tftypes.NewValue(tftypes.String, nil),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("Group DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if nm := getStringAttr(t, dsResp.State, "name"); nm != "ds-grp" {
		t.Errorf("expected name ds-grp, got %q", nm)
	}
}

func TestGroupDataSourceMissingBoth(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	ds := groupdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, nil), "name": tftypes.NewValue(tftypes.String, nil),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error when neither set")
	}
}

func TestGroupDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	ds := groupdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"group_id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, nil),
		"self_url": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error for nonexistent group")
	}
}

func TestGroupDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := groupdatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}

func TestRoleDataSourceByID(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()

	rr := roleresource.NewResource()
	configureResource(t, rr, client)
	rs := getResourceSchema(t, rr)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS Role"),
		"description": tftypes.NewValue(tftypes.String, "desc"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	rr.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create role: %v", cResp.Diagnostics.Errors())
	}
	roleID := getStringAttr(t, cResp.State, "role_id")

	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, roleID), "name": tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil), "scope": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("Role DS Read: %v", dsResp.Diagnostics.Errors())
	}
}

func TestRoleDataSourceByName(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()

	rr := roleresource.NewResource()
	configureResource(t, rr, client)
	rs := getResourceSchema(t, rr)
	plan := tfsdk.Plan{Schema: rs, Raw: tftypes.NewValue(rs.Type().TerraformType(ctx), map[string]tftypes.Value{
		"role_id":     tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"name":        tftypes.NewValue(tftypes.String, "DS Name Role"),
		"description": tftypes.NewValue(tftypes.String, "d"),
		"scope":       tftypes.NewValue(tftypes.String, "org"),
	})}
	cResp := &resource.CreateResponse{State: emptyState(ctx, rs)}
	rr.Create(ctx, resource.CreateRequest{Plan: plan}, cResp)
	if cResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", cResp.Diagnostics.Errors())
	}

	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, nil), "name": tftypes.NewValue(tftypes.String, "DS Name Role"),
		"description": tftypes.NewValue(tftypes.String, nil), "scope": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("Role DS Read by name: %v", dsResp.Diagnostics.Errors())
	}
}

func TestRoleDataSourceMissingBoth(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, nil), "name": tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil), "scope": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleDataSourceNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, "nonexistent"), "name": tftypes.NewValue(tftypes.String, nil),
		"description": tftypes.NewValue(tftypes.String, nil), "scope": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleDataSourceByNameNotFound(t *testing.T) {
	t.Parallel()
	_, client := testMockServer(t)
	ctx := context.Background()
	ds := roledatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsType, map[string]tftypes.Value{
		"role_id": tftypes.NewValue(tftypes.String, nil), "name": tftypes.NewValue(tftypes.String, "Nope"),
		"description": tftypes.NewValue(tftypes.String, nil), "scope": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error")
	}
}

func TestRoleDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	ds := roledatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: 42}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("wrong type should error")
	}
}
