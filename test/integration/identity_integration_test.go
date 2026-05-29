// Package integration contains integration tests for identity resources and data sources.
//
// These tests exercise the internal/client package against the mock API server,
// verifying full CRUD lifecycles and cross-resource operations for all identity
// resource types: user, group, group membership, role, role assignment, and token.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// testTimeout is the default context timeout for all integration tests.
const testTimeout = 30 * time.Second

// setupMockServer creates a mock server with auth and identity endpoints,
// and returns the httptest server and a configured client.
func setupMockServer(t *testing.T) (*httptest.Server, *client.Client) {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	auth, err := client.NewAPIKeyAuthenticator("test-api-key")
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	cfg := client.Config{
		BaseURL:        ts.URL,
		RequestTimeout: testTimeout,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}

	c, err := client.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return ts, c
}

// jsonBody marshals v to a bytes.Reader for use as a request body.
func jsonBody(t *testing.T, v interface{}) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal body: %v", err)
	}
	return bytes.NewReader(data)
}

// --- User CRUD Integration Tests ---

// TestIntegrationUserCRUDLifecycle tests the full create, read, update, delete
// lifecycle for users using the client against the mock API.
func TestIntegrationUserCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create user
	createBody := map[string]string{
		"emailAddress": "integration@example.com",
		"displayName":  "Integration User",
	}
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	accountID, ok := created["accountId"].(string)
	if !ok || accountID == "" {
		t.Fatal("create user: expected non-empty accountId")
	}
	if created["emailAddress"] != "integration@example.com" {
		t.Errorf("create user: expected email 'integration@example.com', got %v", created["emailAddress"])
	}
	if created["displayName"] != "Integration User" {
		t.Errorf("create user: expected displayName 'Integration User', got %v", created["displayName"])
	}
	if created["active"] != true {
		t.Errorf("create user: expected active=true, got %v", created["active"])
	}

	// Read user
	var readUser map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", accountID), &readUser)
	if err != nil {
		t.Fatalf("read user failed: %v", err)
	}
	if readUser["accountId"] != accountID {
		t.Errorf("read user: expected accountId %q, got %v", accountID, readUser["accountId"])
	}
	if readUser["emailAddress"] != "integration@example.com" {
		t.Errorf("read user: expected email 'integration@example.com', got %v", readUser["emailAddress"])
	}

	// Update user
	updateBody := map[string]string{
		"displayName": "Updated Integration User",
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/user/%s", accountID), jsonBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update user failed: %v", err)
	}
	if updated["displayName"] != "Updated Integration User" {
		t.Errorf("update user: expected displayName 'Updated Integration User', got %v", updated["displayName"])
	}
	if updated["accountId"] != accountID {
		t.Errorf("update user: accountId should not change, got %v", updated["accountId"])
	}

	// Verify update persisted via read
	var reread map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", accountID), &reread)
	if err != nil {
		t.Fatalf("re-read user failed: %v", err)
	}
	if reread["displayName"] != "Updated Integration User" {
		t.Errorf("re-read user: update not persisted, got %v", reread["displayName"])
	}

	// Delete user
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/user/%s", accountID))
	if err != nil {
		t.Fatalf("delete user failed: %v", err)
	}

	// Verify user is gone (read should return 404 error)
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", accountID), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted user, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404 for deleted user, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationUserSearchByQuery tests the user search endpoint via the client.
func TestIntegrationUserSearchByQuery(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create two users
	for _, name := range []string{"Alice Wonderland", "Bob Builder"} {
		body := map[string]string{
			"emailAddress": fmt.Sprintf("%s@example.com", name[:3]),
			"displayName":  name,
		}
		err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, body), nil)
		if err != nil {
			t.Fatalf("create user %q failed: %v", name, err)
		}
	}

	// Search for Alice
	var results []map[string]interface{}
	err := c.Get(ctx, "/rest/api/3/user/search?query=alice", &results)
	if err != nil {
		t.Fatalf("search users failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("search users: expected 1 result for 'alice', got %d", len(results))
	}

	// Search with empty query returns all
	var allResults []map[string]interface{}
	err = c.Get(ctx, "/rest/api/3/user/search?query=", &allResults)
	if err != nil {
		t.Fatalf("search all users failed: %v", err)
	}
	if len(allResults) < 2 {
		t.Errorf("search all users: expected at least 2, got %d", len(allResults))
	}
}

// TestIntegrationUserReadNotFound tests reading a nonexistent user via the client.
func TestIntegrationUserReadNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/rest/api/3/user?accountId=nonexistent", &result)
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationUserDeleteNotFound tests deleting a nonexistent user via the client.
func TestIntegrationUserDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/rest/api/3/user/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationUserCreateMissingFields tests user creation with missing required fields.
func TestIntegrationUserCreateMissingFields(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]string{"emailAddress": "only-email@example.com"}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for missing displayName, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

// --- Group CRUD Integration Tests ---

// TestIntegrationGroupCRUDLifecycle tests the full create, read, list, delete
// lifecycle for groups using the client against the mock API.
func TestIntegrationGroupCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create group
	createBody := map[string]string{"name": "integration-developers"}
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/group", jsonBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	groupID, ok := created["groupId"].(string)
	if !ok || groupID == "" {
		t.Fatal("create group: expected non-empty groupId")
	}
	if created["name"] != "integration-developers" {
		t.Errorf("create group: expected name 'integration-developers', got %v", created["name"])
	}

	// Read group
	var readGroup map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID), &readGroup)
	if err != nil {
		t.Fatalf("read group failed: %v", err)
	}
	if readGroup["groupId"] != groupID {
		t.Errorf("read group: expected groupId %q, got %v", groupID, readGroup["groupId"])
	}
	if readGroup["name"] != "integration-developers" {
		t.Errorf("read group: expected name 'integration-developers', got %v", readGroup["name"])
	}

	// List groups (bulk)
	var bulkResp map[string]interface{}
	err = c.Get(ctx, "/rest/api/3/group/bulk", &bulkResp)
	if err != nil {
		t.Fatalf("list groups failed: %v", err)
	}
	values, ok := bulkResp["values"].([]interface{})
	if !ok {
		t.Fatal("list groups: expected values array")
	}
	if len(values) < 1 {
		t.Errorf("list groups: expected at least 1 group, got %d", len(values))
	}

	// Delete group
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID))
	if err != nil {
		t.Fatalf("delete group failed: %v", err)
	}

	// Verify group is gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted group, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404 for deleted group, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationGroupCreateMissingName tests group creation with missing name.
func TestIntegrationGroupCreateMissingName(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]string{}
	err := c.Post(ctx, "/rest/api/3/group", jsonBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationGroupDeleteNotFound tests deleting a nonexistent group.
func TestIntegrationGroupDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/rest/api/3/group?groupId=nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent group, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// --- Group Membership Integration Tests ---

// TestIntegrationGroupMembershipLifecycle tests the full lifecycle of adding,
// listing, and removing group members via the client.
func TestIntegrationGroupMembershipLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create group
	var group map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/group", jsonBody(t, map[string]string{"name": "team-integration"}), &group)
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	groupID := group["groupId"].(string)

	// Create two users
	var user1, user2 map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "member1@example.com",
		"displayName":  "Member One",
	}), &user1)
	if err != nil {
		t.Fatalf("create user1 failed: %v", err)
	}
	userID1 := user1["accountId"].(string)

	err = c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "member2@example.com",
		"displayName":  "Member Two",
	}), &user2)
	if err != nil {
		t.Fatalf("create user2 failed: %v", err)
	}
	userID2 := user2["accountId"].(string)

	// Add first user to group
	addBody := map[string]string{"accountId": userID1}
	err = c.Post(ctx, fmt.Sprintf("/rest/api/3/group/user?groupId=%s", groupID), jsonBody(t, addBody), nil)
	if err != nil {
		t.Fatalf("add user1 to group failed: %v", err)
	}

	// Add second user to group
	addBody2 := map[string]string{"accountId": userID2}
	err = c.Post(ctx, fmt.Sprintf("/rest/api/3/group/user?groupId=%s", groupID), jsonBody(t, addBody2), nil)
	if err != nil {
		t.Fatalf("add user2 to group failed: %v", err)
	}

	// List members
	var memberResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group/member?groupId=%s", groupID), &memberResp)
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	memberValues, ok := memberResp["values"].([]interface{})
	if !ok {
		t.Fatal("list members: expected values array")
	}
	if len(memberValues) != 2 {
		t.Errorf("list members: expected 2 members, got %d", len(memberValues))
	}

	// Try adding duplicate member (should fail with 409)
	err = c.Post(ctx, fmt.Sprintf("/rest/api/3/group/user?groupId=%s", groupID), jsonBody(t, addBody), nil)
	if err == nil {
		t.Fatal("expected error adding duplicate member, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError for duplicate, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected 409 for duplicate member, got %d", apiErr.StatusCode)
	}

	// Remove first user
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/group/user?groupId=%s&accountId=%s", groupID, userID1))
	if err != nil {
		t.Fatalf("remove user1 from group failed: %v", err)
	}

	// Verify only one member remains
	var afterRemoval map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group/member?groupId=%s", groupID), &afterRemoval)
	if err != nil {
		t.Fatalf("list members after removal failed: %v", err)
	}
	afterValues, ok := afterRemoval["values"].([]interface{})
	if !ok {
		t.Fatal("list members after removal: expected values array")
	}
	if len(afterValues) != 1 {
		t.Errorf("list members after removal: expected 1 member, got %d", len(afterValues))
	}
}

// TestIntegrationGroupMembershipAddToNonexistentGroup tests adding a member to a nonexistent group.
func TestIntegrationGroupMembershipAddToNonexistentGroup(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]string{"accountId": "user-1"}
	err := c.Post(ctx, "/rest/api/3/group/user?groupId=nonexistent", jsonBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error adding to nonexistent group, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationGroupMembershipRemoveNonexistent tests removing a non-member.
func TestIntegrationGroupMembershipRemoveNonexistent(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/rest/api/3/group/user?groupId=nogroup&accountId=nouser")
	if err == nil {
		t.Fatal("expected error removing non-member, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// --- Role CRUD Integration Tests ---

// TestIntegrationRoleCRUDLifecycle tests the full create, read, update, list, delete
// lifecycle for roles using the client against the mock API.
func TestIntegrationRoleCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create role
	createBody := map[string]interface{}{
		"name":        "Integration Admin",
		"description": "Integration test admin role",
		"scope":       "org",
	}
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/role", jsonBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	roleID := fmt.Sprintf("%v", created["id"])
	ok := roleID != ""
	if !ok || roleID == "" {
		t.Fatal("create role: expected non-empty id")
	}
	if created["name"] != "Integration Admin" {
		t.Errorf("create role: expected name 'Integration Admin', got %v", created["name"])
	}

	// Read role
	var readRole map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &readRole)
	if err != nil {
		t.Fatalf("read role failed: %v", err)
	}
	if fmt.Sprintf("%v", readRole["id"]) != roleID {
		t.Errorf("read role: expected id %q, got %v", roleID, readRole["id"])
	}
	if readRole["description"] != "Integration test admin role" {
		t.Errorf("read role: expected description 'Integration test admin role', got %v", readRole["description"])
	}

	// Update role
	updateBody := map[string]string{
		"name":        "Updated Integration Admin",
		"description": "Updated description",
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), jsonBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update role failed: %v", err)
	}
	if updated["name"] != "Updated Integration Admin" {
		t.Errorf("update role: expected name 'Updated Integration Admin', got %v", updated["name"])
	}
	if updated["description"] != "Updated description" {
		t.Errorf("update role: expected description 'Updated description', got %v", updated["description"])
	}
	if fmt.Sprintf("%v", updated["id"]) != roleID {
		t.Errorf("update role: id should not change, got %v", updated["id"])
	}

	// List roles
	var roles []map[string]interface{}
	err = c.Get(ctx, "/rest/api/3/role", &roles)
	if err != nil {
		t.Fatalf("list roles failed: %v", err)
	}
	if len(roles) < 1 {
		t.Errorf("list roles: expected at least 1 role, got %d", len(roles))
	}

	// Delete role
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID))
	if err != nil {
		t.Fatalf("delete role failed: %v", err)
	}

	// Verify role is gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted role, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404 for deleted role, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationRoleCreateMissingName tests role creation with missing name.
func TestIntegrationRoleCreateMissingName(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]string{"description": "missing name"}
	err := c.Post(ctx, "/rest/api/3/role", jsonBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationRoleReadNotFound tests reading a nonexistent role.
func TestIntegrationRoleReadNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/rest/api/3/role/nonexistent", &result)
	if err == nil {
		t.Fatal("expected error for nonexistent role, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationRoleDeleteNotFound tests deleting a nonexistent role.
func TestIntegrationRoleDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/rest/api/3/role/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent role, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// --- Role Assignment Integration Tests ---

// TestIntegrationRoleAssignmentCRUDLifecycle tests the full create, read, delete
// lifecycle for role assignments using the client against the mock API.
func TestIntegrationRoleAssignmentCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a role first
	var role map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/role", jsonBody(t, map[string]interface{}{
		"name":        "Assignment Test Role",
		"description": "Role for assignment testing",
		"scope":       "org",
	}), &role)
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	roleID := fmt.Sprintf("%v", role["id"])

	// Create a user
	var user map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "assignee@example.com",
		"displayName":  "Assignee User",
	}), &user)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	userID := user["accountId"].(string)

	// Create role assignment
	assignBody := map[string]string{
		"roleId":        roleID,
		"principalType": "user",
		"principalId":   userID,
		"scope":         "org",
	}
	var assignment map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/role/assignment", jsonBody(t, assignBody), &assignment)
	if err != nil {
		t.Fatalf("create role assignment failed: %v", err)
	}
	assignmentID, ok := assignment["id"].(string)
	if !ok || assignmentID == "" {
		t.Fatal("create assignment: expected non-empty id")
	}
	if assignment["roleId"] != roleID {
		t.Errorf("create assignment: expected roleId %q, got %v", roleID, assignment["roleId"])
	}
	if assignment["principalType"] != "user" {
		t.Errorf("create assignment: expected principalType 'user', got %v", assignment["principalType"])
	}
	if assignment["principalId"] != userID {
		t.Errorf("create assignment: expected principalId %q, got %v", userID, assignment["principalId"])
	}
	if assignment["scope"] != "org" {
		t.Errorf("create assignment: expected scope 'org', got %v", assignment["scope"])
	}

	// Read role assignment
	var readAssignment map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", assignmentID), &readAssignment)
	if err != nil {
		t.Fatalf("read role assignment failed: %v", err)
	}
	if readAssignment["id"] != assignmentID {
		t.Errorf("read assignment: expected id %q, got %v", assignmentID, readAssignment["id"])
	}

	// Delete role assignment
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", assignmentID))
	if err != nil {
		t.Fatalf("delete role assignment failed: %v", err)
	}

	// Verify assignment is gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", assignmentID), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted assignment, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404 for deleted assignment, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationRoleAssignmentWithProductScope tests role assignment with product scope.
func TestIntegrationRoleAssignmentWithProductScope(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	assignBody := map[string]string{
		"roleId":        "some-role",
		"principalType": "group",
		"principalId":   "some-group",
		"scope":         "product",
		"productId":     "jira-software",
	}
	var assignment map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/role/assignment", jsonBody(t, assignBody), &assignment)
	if err != nil {
		t.Fatalf("create product-scoped assignment failed: %v", err)
	}
	if assignment["productId"] != "jira-software" {
		t.Errorf("expected productId 'jira-software', got %v", assignment["productId"])
	}
	if assignment["scope"] != "product" {
		t.Errorf("expected scope 'product', got %v", assignment["scope"])
	}
}

// TestIntegrationRoleAssignmentDeleteNotFound tests deleting a nonexistent assignment.
func TestIntegrationRoleAssignmentDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/rest/api/3/role/assignment/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent assignment, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationRoleAssignmentCreateMissingFields tests assignment creation with missing fields.
func TestIntegrationRoleAssignmentCreateMissingFields(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]string{"roleId": "some-role"}
	err := c.Post(ctx, "/rest/api/3/role/assignment", jsonBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for missing fields, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

// --- Token CRUD Integration Tests ---

// TestIntegrationTokenCRUDLifecycle tests the full create, read, delete lifecycle
// for API tokens using the client against the mock API.
func TestIntegrationTokenCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a user to own the token
	var user map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "tokenowner@example.com",
		"displayName":  "Token Owner",
	}), &user)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	accountID := user["accountId"].(string)

	// Create token
	createBody := map[string]string{"label": "Integration Test Token"}
	var created map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/rest/api/3/user/%s/token", accountID), jsonBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create token failed: %v", err)
	}
	tokenID, ok := created["id"].(string)
	if !ok || tokenID == "" {
		t.Fatal("create token: expected non-empty id")
	}
	if created["label"] != "Integration Test Token" {
		t.Errorf("create token: expected label 'Integration Test Token', got %v", created["label"])
	}
	if created["rawToken"] == nil || created["rawToken"] == "" {
		t.Error("create token: expected non-empty rawToken")
	}
	if created["userAccountId"] != accountID {
		t.Errorf("create token: expected userAccountId %q, got %v", accountID, created["userAccountId"])
	}
	if created["createdAt"] == nil || created["createdAt"] == "" {
		t.Error("create token: expected non-empty createdAt")
	}

	// Read token
	var readToken map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", accountID, tokenID), &readToken)
	if err != nil {
		t.Fatalf("read token failed: %v", err)
	}
	if readToken["id"] != tokenID {
		t.Errorf("read token: expected id %q, got %v", tokenID, readToken["id"])
	}
	if readToken["label"] != "Integration Test Token" {
		t.Errorf("read token: expected label 'Integration Test Token', got %v", readToken["label"])
	}

	// Delete token
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", accountID, tokenID))
	if err != nil {
		t.Fatalf("delete token failed: %v", err)
	}

	// Verify token is gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", accountID, tokenID), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted token, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404 for deleted token, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationTokenCreateMissingLabel tests token creation with missing label.
func TestIntegrationTokenCreateMissingLabel(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]string{}
	err := c.Post(ctx, "/rest/api/3/user/user-1/token", jsonBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for missing label, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationTokenReadNotFound tests reading a nonexistent token.
func TestIntegrationTokenReadNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/rest/api/3/user/user-1/token/nonexistent", &result)
	if err == nil {
		t.Fatal("expected error for nonexistent token, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationTokenDeleteNotFound tests deleting a nonexistent token.
func TestIntegrationTokenDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/rest/api/3/user/user-1/token/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent token, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// TestIntegrationTokenMultiplePerUser tests creating multiple tokens for the same user.
func TestIntegrationTokenMultiplePerUser(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	accountID := "multi-token-user"

	// Create first token
	var token1 map[string]interface{}
	err := c.Post(ctx, fmt.Sprintf("/rest/api/3/user/%s/token", accountID),
		jsonBody(t, map[string]string{"label": "Token One"}), &token1)
	if err != nil {
		t.Fatalf("create token1 failed: %v", err)
	}
	token1ID := token1["id"].(string)

	// Create second token
	var token2 map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/rest/api/3/user/%s/token", accountID),
		jsonBody(t, map[string]string{"label": "Token Two"}), &token2)
	if err != nil {
		t.Fatalf("create token2 failed: %v", err)
	}
	token2ID := token2["id"].(string)

	if token1ID == token2ID {
		t.Error("expected different token IDs for two tokens")
	}

	// Both should be readable
	var read1, read2 map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", accountID, token1ID), &read1)
	if err != nil {
		t.Fatalf("read token1 failed: %v", err)
	}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", accountID, token2ID), &read2)
	if err != nil {
		t.Fatalf("read token2 failed: %v", err)
	}

	// Deleting one should not affect the other
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", accountID, token1ID))
	if err != nil {
		t.Fatalf("delete token1 failed: %v", err)
	}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", accountID, token2ID), &read2)
	if err != nil {
		t.Fatalf("token2 should still be readable after deleting token1: %v", err)
	}
}

// --- Cross-Resource Integration Tests ---

// TestIntegrationCrossResourceWorkflow tests a full cross-resource workflow:
// create user, create group, add membership, create role, assign role to user.
func TestIntegrationCrossResourceWorkflow(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Step 1: Create a user
	var user map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "workflow@example.com",
		"displayName":  "Workflow User",
	}), &user)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	userID := user["accountId"].(string)
	t.Logf("created user: %s", userID)

	// Step 2: Create a group
	var group map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/group", jsonBody(t, map[string]string{
		"name": "workflow-group",
	}), &group)
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	groupID := group["groupId"].(string)
	t.Logf("created group: %s", groupID)

	// Step 3: Add user to group
	err = c.Post(ctx, fmt.Sprintf("/rest/api/3/group/user?groupId=%s", groupID),
		jsonBody(t, map[string]string{"accountId": userID}), nil)
	if err != nil {
		t.Fatalf("add user to group failed: %v", err)
	}

	// Verify membership
	var memberResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group/member?groupId=%s", groupID), &memberResp)
	if err != nil {
		t.Fatalf("list members failed: %v", err)
	}
	members, ok := memberResp["values"].([]interface{})
	if !ok || len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}

	// Step 4: Create a role
	var role map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/role", jsonBody(t, map[string]interface{}{
		"name":        "Workflow Admin",
		"description": "Admin role for workflow test",
		"scope":       "org",
	}), &role)
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	roleID := fmt.Sprintf("%v", role["id"])
	t.Logf("created role: %s", roleID)

	// Step 5: Assign role to user
	var assignment map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/role/assignment", jsonBody(t, map[string]string{
		"roleId":        roleID,
		"principalType": "user",
		"principalId":   userID,
		"scope":         "org",
	}), &assignment)
	if err != nil {
		t.Fatalf("create role assignment failed: %v", err)
	}
	assignmentID := assignment["id"].(string)
	t.Logf("created assignment: %s", assignmentID)

	// Step 6: Assign role to group
	var groupAssignment map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/role/assignment", jsonBody(t, map[string]string{
		"roleId":        roleID,
		"principalType": "group",
		"principalId":   groupID,
		"scope":         "org",
	}), &groupAssignment)
	if err != nil {
		t.Fatalf("create group role assignment failed: %v", err)
	}
	groupAssignmentID := groupAssignment["id"].(string)

	// Step 7: Create API token for user
	var token map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/rest/api/3/user/%s/token", userID),
		jsonBody(t, map[string]string{"label": "Workflow Token"}), &token)
	if err != nil {
		t.Fatalf("create token failed: %v", err)
	}
	tokenID := token["id"].(string)
	t.Logf("created token: %s", tokenID)

	// Verify all resources are readable
	var readUser map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", userID), &readUser)
	if err != nil {
		t.Fatalf("read user failed: %v", err)
	}
	var readGroup map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID), &readGroup)
	if err != nil {
		t.Fatalf("read group failed: %v", err)
	}
	var readRole map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &readRole)
	if err != nil {
		t.Fatalf("read role failed: %v", err)
	}
	var readAssignment map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", assignmentID), &readAssignment)
	if err != nil {
		t.Fatalf("read assignment failed: %v", err)
	}
	var readToken map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", userID, tokenID), &readToken)
	if err != nil {
		t.Fatalf("read token failed: %v", err)
	}

	// Cleanup in reverse order
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", userID, tokenID))
	if err != nil {
		t.Fatalf("delete token failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", groupAssignmentID))
	if err != nil {
		t.Fatalf("delete group assignment failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", assignmentID))
	if err != nil {
		t.Fatalf("delete assignment failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID))
	if err != nil {
		t.Fatalf("delete role failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/group/user?groupId=%s&accountId=%s", groupID, userID))
	if err != nil {
		t.Fatalf("remove membership failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID))
	if err != nil {
		t.Fatalf("delete group failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/user/%s", userID))
	if err != nil {
		t.Fatalf("delete user failed: %v", err)
	}

	// Verify all resources are gone
	var ghostUser map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", userID), &ghostUser)
	if err == nil {
		t.Error("expected error reading deleted user")
	}
	var ghostGroup map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID), &ghostGroup)
	if err == nil {
		t.Error("expected error reading deleted group")
	}
	var ghostRole map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &ghostRole)
	if err == nil {
		t.Error("expected error reading deleted role")
	}
}

// --- Authentication Integration Tests ---

// TestIntegrationAuthenticatedRequests tests that the mock server validates auth headers.
func TestIntegrationAuthenticatedRequests(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Verify authenticated request works against /rest/api/3/myself
	var myself map[string]interface{}
	err := c.Get(ctx, "/rest/api/3/myself", &myself)
	if err != nil {
		t.Fatalf("authenticated request to /myself failed: %v", err)
	}
	if myself["accountId"] != "mock-account-id" {
		t.Errorf("expected accountId 'mock-account-id', got %v", myself["accountId"])
	}
	if myself["emailAddress"] != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %v", myself["emailAddress"])
	}
}

// TestIntegrationAuthWithInvalidCredentials tests that invalid credentials are rejected.
func TestIntegrationAuthWithInvalidCredentials(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	auth, err := client.NewAPIKeyAuthenticator("wrong-api-key")
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	cfg := client.Config{
		BaseURL:        ts.URL,
		RequestTimeout: testTimeout,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}
	c, err := client.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]interface{}
	err = c.Get(ctx, "/rest/api/3/myself", &result)
	if err == nil {
		t.Fatal("expected error with invalid credentials, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected 401, got %d", apiErr.StatusCode)
	}
}

// --- Import-Style Read Tests ---

// TestIntegrationImportUserByAccountID simulates importing an existing user by reading by accountId.
func TestIntegrationImportUserByAccountID(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a user
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "import@example.com",
		"displayName":  "Import User",
	}), &created)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	accountID := created["accountId"].(string)

	// Simulate import: read by account ID
	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", accountID), &imported)
	if err != nil {
		t.Fatalf("import (read) user failed: %v", err)
	}
	if imported["accountId"] != accountID {
		t.Errorf("imported accountId mismatch: expected %q, got %v", accountID, imported["accountId"])
	}
	if imported["emailAddress"] != "import@example.com" {
		t.Errorf("imported email mismatch: got %v", imported["emailAddress"])
	}
	if imported["displayName"] != "Import User" {
		t.Errorf("imported displayName mismatch: got %v", imported["displayName"])
	}
}

// TestIntegrationImportGroupByID simulates importing an existing group by reading by groupId.
func TestIntegrationImportGroupByID(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a group
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/group", jsonBody(t, map[string]string{
		"name": "import-group",
	}), &created)
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	groupID := created["groupId"].(string)

	// Simulate import: read by group ID
	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID), &imported)
	if err != nil {
		t.Fatalf("import (read) group failed: %v", err)
	}
	if imported["groupId"] != groupID {
		t.Errorf("imported groupId mismatch: expected %q, got %v", groupID, imported["groupId"])
	}
	if imported["name"] != "import-group" {
		t.Errorf("imported name mismatch: got %v", imported["name"])
	}
}

// TestIntegrationImportRoleByID simulates importing an existing role by reading by role ID.
func TestIntegrationImportRoleByID(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a role
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/role", jsonBody(t, map[string]interface{}{
		"name":        "Import Role",
		"description": "Role to import",
		"scope":       "org",
	}), &created)
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	roleID := fmt.Sprintf("%v", created["id"])

	// Simulate import: read by role ID
	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &imported)
	if err != nil {
		t.Fatalf("import (read) role failed: %v", err)
	}
	if fmt.Sprintf("%v", imported["id"]) != roleID {
		t.Errorf("imported id mismatch: expected %q, got %v", roleID, imported["id"])
	}
	if imported["name"] != "Import Role" {
		t.Errorf("imported name mismatch: got %v", imported["name"])
	}
}

// TestIntegrationImportTokenByID simulates importing an existing token by reading by composite ID.
func TestIntegrationImportTokenByID(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	accountID := "import-token-user"

	// Create a token
	var created map[string]interface{}
	err := c.Post(ctx, fmt.Sprintf("/rest/api/3/user/%s/token", accountID),
		jsonBody(t, map[string]string{"label": "Import Token"}), &created)
	if err != nil {
		t.Fatalf("create token failed: %v", err)
	}
	tokenID := created["id"].(string)

	// Simulate import: read by accountId/tokenId
	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user/%s/token/%s", accountID, tokenID), &imported)
	if err != nil {
		t.Fatalf("import (read) token failed: %v", err)
	}
	if imported["id"] != tokenID {
		t.Errorf("imported id mismatch: expected %q, got %v", tokenID, imported["id"])
	}
	if imported["label"] != "Import Token" {
		t.Errorf("imported label mismatch: got %v", imported["label"])
	}
}

// TestIntegrationImportRoleAssignmentByID simulates importing a role assignment.
func TestIntegrationImportRoleAssignmentByID(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create an assignment
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/role/assignment", jsonBody(t, map[string]string{
		"roleId":        "import-role",
		"principalType": "user",
		"principalId":   "import-user",
		"scope":         "org",
	}), &created)
	if err != nil {
		t.Fatalf("create assignment failed: %v", err)
	}
	assignmentID := created["id"].(string)

	// Simulate import: read by assignment ID
	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", assignmentID), &imported)
	if err != nil {
		t.Fatalf("import (read) assignment failed: %v", err)
	}
	if imported["id"] != assignmentID {
		t.Errorf("imported id mismatch: expected %q, got %v", assignmentID, imported["id"])
	}
	if imported["roleId"] != "import-role" {
		t.Errorf("imported roleId mismatch: got %v", imported["roleId"])
	}
}

// --- Idempotency Tests ---

// TestIntegrationUserUpdateIdempotency tests that updating a user to the same values
// is idempotent and does not change the result.
func TestIntegrationUserUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var user map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "idempotent@example.com",
		"displayName":  "Idempotent User",
	}), &user)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	accountID := user["accountId"].(string)

	// Update with same display name twice
	updateBody := map[string]string{"displayName": "Idempotent User Updated"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/user/%s", accountID), jsonBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/user/%s", accountID), jsonBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["displayName"] != second["displayName"] {
		t.Errorf("idempotency: displayName differs between updates: %v vs %v",
			first["displayName"], second["displayName"])
	}
	if first["accountId"] != second["accountId"] {
		t.Errorf("idempotency: accountId differs between updates: %v vs %v",
			first["accountId"], second["accountId"])
	}
}

// TestIntegrationRoleUpdateIdempotency tests that updating a role to the same values is idempotent.
func TestIntegrationRoleUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var role map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/role", jsonBody(t, map[string]interface{}{
		"name":        "Idempotent Role",
		"description": "Original",
		"scope":       "org",
	}), &role)
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	roleID := fmt.Sprintf("%v", role["id"])

	updateBody := map[string]string{"name": "Idempotent Role", "description": "Updated"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), jsonBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), jsonBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["name"] != second["name"] {
		t.Errorf("idempotency: name differs: %v vs %v", first["name"], second["name"])
	}
	if first["description"] != second["description"] {
		t.Errorf("idempotency: description differs: %v vs %v", first["description"], second["description"])
	}
}

// --- Data Source Integration Tests ---

// TestIntegrationUserDataSourceByAccountID tests reading a user data source by account ID.
func TestIntegrationUserDataSourceByAccountID(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a user
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "datasource@example.com",
		"displayName":  "DataSource User",
	}), &created)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	accountID := created["accountId"].(string)

	// Read user (data source path)
	var ds map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", accountID), &ds)
	if err != nil {
		t.Fatalf("data source read failed: %v", err)
	}
	if ds["displayName"] != "DataSource User" {
		t.Errorf("expected displayName 'DataSource User', got %v", ds["displayName"])
	}
	if ds["emailAddress"] != "datasource@example.com" {
		t.Errorf("expected email 'datasource@example.com', got %v", ds["emailAddress"])
	}
}

// TestIntegrationUserDataSourceByEmailSearch tests finding a user by email via search.
func TestIntegrationUserDataSourceByEmailSearch(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create user
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "searchds@example.com",
		"displayName":  "Search DS User",
	}), &created)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	// Search by email
	var results []map[string]interface{}
	err = c.Get(ctx, "/rest/api/3/user/search?query=searchds@example.com", &results)
	if err != nil {
		t.Fatalf("search by email failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["emailAddress"] != "searchds@example.com" {
		t.Errorf("expected email 'searchds@example.com', got %v", results[0]["emailAddress"])
	}
}

// TestIntegrationGroupDataSource tests reading a group data source.
func TestIntegrationGroupDataSource(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create group
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/group", jsonBody(t, map[string]string{
		"name": "ds-group",
	}), &created)
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	groupID := created["groupId"].(string)

	// Read group (data source path)
	var ds map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID), &ds)
	if err != nil {
		t.Fatalf("data source read group failed: %v", err)
	}
	if ds["name"] != "ds-group" {
		t.Errorf("expected name 'ds-group', got %v", ds["name"])
	}
	if ds["groupId"] != groupID {
		t.Errorf("expected groupId %q, got %v", groupID, ds["groupId"])
	}
}

// TestIntegrationRoleDataSourceByID tests reading a role data source by ID.
func TestIntegrationRoleDataSourceByID(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create role
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/role", jsonBody(t, map[string]interface{}{
		"name":        "DS Role",
		"description": "For data source test",
		"scope":       "org",
	}), &created)
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	roleID := fmt.Sprintf("%v", created["id"])

	// Read role by ID (data source path)
	var ds map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &ds)
	if err != nil {
		t.Fatalf("data source read role failed: %v", err)
	}
	if ds["name"] != "DS Role" {
		t.Errorf("expected name 'DS Role', got %v", ds["name"])
	}
}

// TestIntegrationRoleDataSourceByNameViaList tests finding a role by name via listing all roles.
func TestIntegrationRoleDataSourceByNameViaList(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create role
	err := c.Post(ctx, "/rest/api/3/role", jsonBody(t, map[string]interface{}{
		"name":        "FindMe Role",
		"description": "Find by name",
		"scope":       "org",
	}), nil)
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}

	// List all roles and find by name (simulates data source findRoleByName)
	var roles []map[string]interface{}
	err = c.Get(ctx, "/rest/api/3/role", &roles)
	if err != nil {
		t.Fatalf("list roles failed: %v", err)
	}

	found := false
	for _, r := range roles {
		if r["name"] == "FindMe Role" {
			found = true
			if r["description"] != "Find by name" {
				t.Errorf("found role has wrong description: %v", r["description"])
			}
			break
		}
	}
	if !found {
		t.Error("role 'FindMe Role' not found in role list")
	}
}

// --- Drift Detection Tests ---

// TestIntegrationDriftDetectionUserModifiedExternally tests that reading after external
// modification returns the updated state (enabling drift detection).
func TestIntegrationDriftDetectionUserModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create user
	var user map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "drift@example.com",
		"displayName":  "Original Name",
	}), &user)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	accountID := user["accountId"].(string)

	// Simulate external modification (direct PUT to mock)
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/user/%s", accountID),
		jsonBody(t, map[string]string{"displayName": "Externally Modified"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read should reflect the external change (drift detection)
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", accountID), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["displayName"] != "Externally Modified" {
		t.Errorf("drift not detected: expected 'Externally Modified', got %v", current["displayName"])
	}
}

// TestIntegrationDriftDetectionRoleModifiedExternally tests drift detection for roles.
func TestIntegrationDriftDetectionRoleModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create role
	var role map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/role", jsonBody(t, map[string]interface{}{
		"name":        "Drift Role",
		"description": "Original",
		"scope":       "org",
	}), &role)
	if err != nil {
		t.Fatalf("create role failed: %v", err)
	}
	roleID := fmt.Sprintf("%v", role["id"])

	// Simulate external modification
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID),
		jsonBody(t, map[string]string{"description": "Externally Changed"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read should reflect external change
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["description"] != "Externally Changed" {
		t.Errorf("drift not detected: expected 'Externally Changed', got %v", current["description"])
	}
}

// TestIntegrationDriftDetectionResourceDeletedExternally tests that reading a resource
// deleted outside of Terraform returns a 404 error.
func TestIntegrationDriftDetectionResourceDeletedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create user
	var user map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/user", jsonBody(t, map[string]string{
		"emailAddress": "willdelete@example.com",
		"displayName":  "Will Delete",
	}), &user)
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	accountID := user["accountId"].(string)

	// Externally delete
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/user/%s", accountID))
	if err != nil {
		t.Fatalf("external delete failed: %v", err)
	}

	// Read should return 404
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", accountID), &ghost)
	if err == nil {
		t.Fatal("expected error for externally deleted user, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// --- Error Response Format Tests ---

// TestIntegrationErrorResponseFormat tests that the mock returns Atlassian-format errors
// and the client properly parses them.
func TestIntegrationErrorResponseFormat(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// 404 error
	var result map[string]interface{}
	err := c.Get(ctx, "/rest/api/3/user?accountId=nonexistent", &result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.Message == "" {
		t.Error("expected non-empty error message")
	}
	if apiErr.Action != "read" {
		t.Errorf("expected action 'read', got %q", apiErr.Action)
	}
}

// --- Health Check Test ---

// TestIntegrationHealthCheck tests the mock server health check endpoint.
func TestIntegrationHealthCheck(t *testing.T) {
	t.Parallel()
	_, c := setupMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]string
	err := c.Get(ctx, "/health", &result)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", result["status"])
	}
}
