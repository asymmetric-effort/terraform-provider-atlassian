// Package unit contains unit tests for the mock API identity endpoints.
package unit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// newIdentityServer creates a mock server with identity endpoints registered.
func newIdentityServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	return httptest.NewServer(s.Handler())
}

// postJSON sends a POST request with a JSON body and returns the response.
func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	return resp
}

// putJSON sends a PUT request with a JSON body and returns the response.
func putJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to create PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s failed: %v", url, err)
	}
	return resp
}

// doDelete sends a DELETE request and returns the response.
func doDelete(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("failed to create DELETE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s failed: %v", url, err)
	}
	return resp
}

// decodeJSON decodes a JSON response body into the given target.
func decodeJSON(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

// TestUserCRUDLifecycle tests create, read, update, search, and delete for users.
func TestUserCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	// Create user
	resp := postJSON(t, ts.URL+"/rest/api/3/user", map[string]string{
		"emailAddress": "alice@example.com",
		"displayName":  "Alice Smith",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: expected 201, got %d", resp.StatusCode)
	}
	var user map[string]interface{}
	decodeJSON(t, resp, &user)
	accountID, ok := user["accountId"].(string)
	if !ok || accountID == "" {
		t.Fatal("create user: expected non-empty accountId")
	}
	if user["emailAddress"] != "alice@example.com" {
		t.Errorf("create user: expected email 'alice@example.com', got %v", user["emailAddress"])
	}
	if user["active"] != true {
		t.Errorf("create user: expected active=true, got %v", user["active"])
	}

	// Read user
	resp, err := http.Get(ts.URL + "/rest/api/3/user?accountId=" + accountID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read user: expected 200, got %d", resp.StatusCode)
	}
	var readUser map[string]interface{}
	decodeJSON(t, resp, &readUser)
	if readUser["accountId"] != accountID {
		t.Errorf("read user: expected accountId %q, got %v", accountID, readUser["accountId"])
	}

	// Update user
	resp = putJSON(t, ts.URL+"/rest/api/3/user/"+accountID, map[string]string{
		"displayName": "Alice Johnson",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update user: expected 200, got %d", resp.StatusCode)
	}
	var updatedUser map[string]interface{}
	decodeJSON(t, resp, &updatedUser)
	if updatedUser["displayName"] != "Alice Johnson" {
		t.Errorf("update user: expected displayName 'Alice Johnson', got %v", updatedUser["displayName"])
	}
	if updatedUser["accountId"] != accountID {
		t.Errorf("update user: accountId should not change, got %v", updatedUser["accountId"])
	}

	// Search users
	resp, err = http.Get(ts.URL + "/rest/api/3/user/search?query=alice")
	if err != nil {
		t.Fatalf("search users: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search users: expected 200, got %d", resp.StatusCode)
	}
	var searchResults []map[string]interface{}
	decodeJSON(t, resp, &searchResults)
	if len(searchResults) != 1 {
		t.Fatalf("search users: expected 1 result, got %d", len(searchResults))
	}
	if searchResults[0]["accountId"] != accountID {
		t.Errorf("search users: expected accountId %q, got %v", accountID, searchResults[0]["accountId"])
	}

	// Delete user
	resp = doDelete(t, ts.URL+"/rest/api/3/user/"+accountID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete user: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify user is deleted
	resp, err = http.Get(ts.URL + "/rest/api/3/user?accountId=" + accountID)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestUserCreateMissingFields tests user creation with missing required fields.
func TestUserCreateMissingFields(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/user", map[string]string{
		"emailAddress": "bob@example.com",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing displayName, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestUserReadNotFound tests reading a non-existent user.
func TestUserReadNotFound(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/user?accountId=nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// TestUserDeleteNotFound tests deleting a non-existent user.
func TestUserDeleteNotFound(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := doDelete(t, ts.URL+"/rest/api/3/user/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestUserSearchEmpty tests searching with no matches.
func TestUserSearchEmpty(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/user/search?query=nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var results []map[string]interface{}
	decodeJSON(t, resp, &results)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestGroupCRUDLifecycle tests create, read, list, and delete for groups.
func TestGroupCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	// Create group
	resp := postJSON(t, ts.URL+"/rest/api/3/group", map[string]string{
		"name": "developers",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d", resp.StatusCode)
	}
	var group map[string]interface{}
	decodeJSON(t, resp, &group)
	groupID, ok := group["groupId"].(string)
	if !ok || groupID == "" {
		t.Fatal("create group: expected non-empty groupId")
	}
	if group["name"] != "developers" {
		t.Errorf("create group: expected name 'developers', got %v", group["name"])
	}

	// Read group
	resp, err := http.Get(ts.URL + "/rest/api/3/group?groupId=" + groupID)
	if err != nil {
		t.Fatalf("read group: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read group: expected 200, got %d", resp.StatusCode)
	}
	var readGroup map[string]interface{}
	decodeJSON(t, resp, &readGroup)
	if readGroup["groupId"] != groupID {
		t.Errorf("read group: expected groupId %q, got %v", groupID, readGroup["groupId"])
	}

	// List groups (bulk)
	resp, err = http.Get(ts.URL + "/rest/api/3/group/bulk")
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list groups: expected 200, got %d", resp.StatusCode)
	}
	var bulkResp map[string]interface{}
	decodeJSON(t, resp, &bulkResp)
	values, ok := bulkResp["values"].([]interface{})
	if !ok {
		t.Fatal("list groups: expected values array")
	}
	if len(values) != 1 {
		t.Errorf("list groups: expected 1 group, got %d", len(values))
	}

	// Delete group
	resp = doDelete(t, ts.URL+"/rest/api/3/group?groupId="+groupID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete group: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify group is deleted
	resp, err = http.Get(ts.URL + "/rest/api/3/group?groupId=" + groupID)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestGroupCreateMissingName tests group creation with missing name.
func TestGroupCreateMissingName(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/group", map[string]string{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestGroupDeleteNotFound tests deleting a non-existent group.
func TestGroupDeleteNotFound(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := doDelete(t, ts.URL+"/rest/api/3/group?groupId=nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestGroupMembershipLifecycle tests adding, listing, and removing group members.
func TestGroupMembershipLifecycle(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	// Create a group first
	resp := postJSON(t, ts.URL+"/rest/api/3/group", map[string]string{
		"name": "team-alpha",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d", resp.StatusCode)
	}
	var group map[string]interface{}
	decodeJSON(t, resp, &group)
	groupID := group["groupId"].(string)

	// Create a user
	resp = postJSON(t, ts.URL+"/rest/api/3/user", map[string]string{
		"emailAddress": "member@example.com",
		"displayName":  "Team Member",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: expected 201, got %d", resp.StatusCode)
	}
	var user map[string]interface{}
	decodeJSON(t, resp, &user)
	userID := user["accountId"].(string)

	// Add member to group
	resp = postJSON(t, ts.URL+"/rest/api/3/group/user?groupId="+groupID, map[string]string{
		"accountId": userID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("add member: expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List members
	resp, err := http.Get(ts.URL + "/rest/api/3/group/member?groupId=" + groupID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list members: expected 200, got %d", resp.StatusCode)
	}
	var memberResp map[string]interface{}
	decodeJSON(t, resp, &memberResp)
	memberValues, ok := memberResp["values"].([]interface{})
	if !ok {
		t.Fatal("list members: expected values array")
	}
	if len(memberValues) != 1 {
		t.Errorf("list members: expected 1 member, got %d", len(memberValues))
	}

	// Try adding duplicate member
	resp = postJSON(t, ts.URL+"/rest/api/3/group/user?groupId="+groupID, map[string]string{
		"accountId": userID,
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("add duplicate member: expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Remove member
	resp = doDelete(t, fmt.Sprintf("%s/rest/api/3/group/user?groupId=%s&accountId=%s", ts.URL, groupID, userID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("remove member: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify member removed
	resp, err = http.Get(ts.URL + "/rest/api/3/group/member?groupId=" + groupID)
	if err != nil {
		t.Fatalf("verify removal: %v", err)
	}
	var afterRemoval map[string]interface{}
	decodeJSON(t, resp, &afterRemoval)
	afterValues, _ := afterRemoval["values"].([]interface{})
	if len(afterValues) != 0 {
		t.Errorf("verify removal: expected 0 members, got %d", len(afterValues))
	}
}

// TestGroupMembershipAddToNonexistentGroup tests adding a member to a non-existent group.
func TestGroupMembershipAddToNonexistentGroup(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/group/user?groupId=nonexistent", map[string]string{
		"accountId": "user-1",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent group, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestGroupMembershipRemoveNonexistent tests removing a non-member.
func TestGroupMembershipRemoveNonexistent(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := doDelete(t, ts.URL+"/rest/api/3/group/user?groupId=nogroup&accountId=nouser")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestRoleCRUDLifecycle tests create, read, update, list, and delete for roles.
func TestRoleCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	// Create role
	resp := postJSON(t, ts.URL+"/rest/api/3/role", map[string]interface{}{
		"name":        "Administrators",
		"description": "Admin role",
		"scope":       "global",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create role: expected 201, got %d", resp.StatusCode)
	}
	var role map[string]interface{}
	decodeJSON(t, resp, &role)
	roleID, ok := role["id"].(string)
	if !ok || roleID == "" {
		t.Fatal("create role: expected non-empty id")
	}
	if role["name"] != "Administrators" {
		t.Errorf("create role: expected name 'Administrators', got %v", role["name"])
	}

	// Read role
	resp, err := http.Get(ts.URL + "/rest/api/3/role/" + roleID)
	if err != nil {
		t.Fatalf("read role: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read role: expected 200, got %d", resp.StatusCode)
	}
	var readRole map[string]interface{}
	decodeJSON(t, resp, &readRole)
	if readRole["id"] != roleID {
		t.Errorf("read role: expected id %q, got %v", roleID, readRole["id"])
	}

	// Update role
	resp = putJSON(t, ts.URL+"/rest/api/3/role/"+roleID, map[string]string{
		"description": "Updated admin role",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update role: expected 200, got %d", resp.StatusCode)
	}
	var updatedRole map[string]interface{}
	decodeJSON(t, resp, &updatedRole)
	if updatedRole["description"] != "Updated admin role" {
		t.Errorf("update role: expected description 'Updated admin role', got %v", updatedRole["description"])
	}
	if updatedRole["id"] != roleID {
		t.Errorf("update role: id should not change, got %v", updatedRole["id"])
	}

	// List roles
	resp, err = http.Get(ts.URL + "/rest/api/3/role")
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list roles: expected 200, got %d", resp.StatusCode)
	}
	var roles []map[string]interface{}
	decodeJSON(t, resp, &roles)
	if len(roles) != 1 {
		t.Errorf("list roles: expected 1 role, got %d", len(roles))
	}

	// Delete role
	resp = doDelete(t, ts.URL+"/rest/api/3/role/"+roleID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete role: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify role is deleted
	resp, err = http.Get(ts.URL + "/rest/api/3/role/" + roleID)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestRoleCreateMissingName tests role creation with missing name.
func TestRoleCreateMissingName(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/role", map[string]string{
		"description": "no name",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestRoleReadNotFound tests reading a non-existent role.
func TestRoleReadNotFound(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/role/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// TestRoleDeleteNotFound tests deleting a non-existent role.
func TestRoleDeleteNotFound(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := doDelete(t, ts.URL+"/rest/api/3/role/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestTokenCRUDLifecycle tests create, read, and delete for API tokens.
func TestTokenCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	accountID := "user-account-1"

	// Create token
	resp := postJSON(t, ts.URL+"/rest/api/3/user/"+accountID+"/token", map[string]string{
		"label": "CI/CD Token",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create token: expected 201, got %d", resp.StatusCode)
	}
	var token map[string]interface{}
	decodeJSON(t, resp, &token)
	tokenID, ok := token["id"].(string)
	if !ok || tokenID == "" {
		t.Fatal("create token: expected non-empty id")
	}
	if token["label"] != "CI/CD Token" {
		t.Errorf("create token: expected label 'CI/CD Token', got %v", token["label"])
	}
	if token["rawToken"] == nil || token["rawToken"] == "" {
		t.Error("create token: expected non-empty rawToken")
	}
	if token["userAccountId"] != accountID {
		t.Errorf("create token: expected userAccountId %q, got %v", accountID, token["userAccountId"])
	}
	if token["createdAt"] == nil || token["createdAt"] == "" {
		t.Error("create token: expected non-empty createdAt")
	}

	// Read token
	resp, err := http.Get(fmt.Sprintf("%s/rest/api/3/user/%s/token/%s", ts.URL, accountID, tokenID))
	if err != nil {
		t.Fatalf("read token: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read token: expected 200, got %d", resp.StatusCode)
	}
	var readToken map[string]interface{}
	decodeJSON(t, resp, &readToken)
	if readToken["id"] != tokenID {
		t.Errorf("read token: expected id %q, got %v", tokenID, readToken["id"])
	}

	// Delete token
	resp = doDelete(t, fmt.Sprintf("%s/rest/api/3/user/%s/token/%s", ts.URL, accountID, tokenID))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete token: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify token is deleted
	resp, err = http.Get(fmt.Sprintf("%s/rest/api/3/user/%s/token/%s", ts.URL, accountID, tokenID))
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestTokenCreateMissingLabel tests token creation with missing label.
func TestTokenCreateMissingLabel(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/user/user-1/token", map[string]string{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing label, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestTokenReadNotFound tests reading a non-existent token.
func TestTokenReadNotFound(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/user/user-1/token/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// TestTokenDeleteNotFound tests deleting a non-existent token.
func TestTokenDeleteNotFound(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp := doDelete(t, ts.URL+"/rest/api/3/user/user-1/token/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestErrorResponseFormat tests that errors are in Atlassian format.
func TestErrorResponseFormat(t *testing.T) {
	t.Parallel()
	ts := newIdentityServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/user?accountId=nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var errResp map[string]interface{}
	decodeJSON(t, resp, &errResp)

	msgs, ok := errResp["errorMessages"].([]interface{})
	if !ok {
		t.Fatal("expected errorMessages array in error response")
	}
	if len(msgs) == 0 {
		t.Error("expected at least one error message")
	}

	errs, ok := errResp["errors"].(map[string]interface{})
	if !ok {
		t.Fatal("expected errors object in error response")
	}
	if errs == nil {
		t.Error("expected errors to be a map, not nil")
	}
}
