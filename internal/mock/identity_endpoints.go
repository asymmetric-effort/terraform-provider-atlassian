package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// idCounter is a global atomic counter for generating unique IDs.
var idCounter uint64

// nextID generates a unique ID with the given prefix.
func nextID(prefix string) string {
	n := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// RegisterIdentityEndpoints registers identity CRUD endpoints on the mock server.
func RegisterIdentityEndpoints(s *Server) {
	registerUserEndpoints(s)
	registerGroupEndpoints(s)
	registerGroupMembershipEndpoints(s)
	registerRoleEndpoints(s)
	registerRoleAssignmentEndpoints(s)
	registerTokenEndpoints(s)
}

// registerUserEndpoints registers user CRUD and search endpoints.
func registerUserEndpoints(s *Server) {
	store := s.GetStore("users")

	// POST /rest/api/3/user — create user
	s.RegisterEndpoint("POST /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		email, _ := req["emailAddress"].(string)
		displayName, _ := req["displayName"].(string)
		if email == "" || displayName == "" {
			WriteError(w, http.StatusBadRequest, "emailAddress and displayName are required")
			return
		}

		// Check for duplicate email
		for _, item := range store.List() {
			var existing map[string]interface{}
			json.Unmarshal(item, &existing)
			if existing["emailAddress"] == email {
				WriteError(w, http.StatusConflict, "A user with this email address already exists")
				return
			}
		}

		// Simulate permission denied via special header
		if r.Header.Get("X-Mock-Forbidden") == "true" {
			WriteError(w, http.StatusForbidden, "Insufficient permissions")
			return
		}

		accountID := nextID("user")
		user := map[string]interface{}{
			"accountId":    accountID,
			"emailAddress": email,
			"displayName":  displayName,
			"active":       true,
		}

		data, _ := json.Marshal(user)
		store.Set(accountID, data)
		WriteJSON(w, http.StatusCreated, user)
	})

	// GET /rest/api/3/user — read user by accountId query param
	s.RegisterEndpoint("GET /rest/api/3/user", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.URL.Query().Get("accountId")
		if accountID == "" {
			WriteError(w, http.StatusBadRequest, "accountId query parameter is required")
			return
		}

		item, ok := store.Get(accountID)
		if !ok {
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /rest/api/3/user/{id} — update user
	s.RegisterEndpoint("PUT /rest/api/3/user/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}

		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		for k, v := range updates {
			if k != "accountId" {
				current[k] = v
			}
		}

		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /rest/api/3/user/{id} — delete user
	s.RegisterEndpoint("DELETE /rest/api/3/user/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, "User not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /rest/api/3/user/search — search users by query
	s.RegisterEndpoint("GET /rest/api/3/user/search", func(w http.ResponseWriter, r *http.Request) {
		query := strings.ToLower(r.URL.Query().Get("query"))
		var results []json.RawMessage
		for _, item := range store.List() {
			if query == "" {
				results = append(results, item)
				continue
			}
			var user map[string]interface{}
			json.Unmarshal(item, &user)
			email, _ := user["emailAddress"].(string)
			displayName, _ := user["displayName"].(string)
			if strings.Contains(strings.ToLower(email), query) ||
				strings.Contains(strings.ToLower(displayName), query) {
				results = append(results, item)
			}
		}
		if results == nil {
			results = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, results)
	})
}

// registerGroupEndpoints registers group CRUD endpoints.
func registerGroupEndpoints(s *Server) {
	store := s.GetStore("groups")

	// POST /rest/api/3/group — create group
	s.RegisterEndpoint("POST /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		name, _ := req["name"].(string)
		if name == "" {
			WriteError(w, http.StatusBadRequest, "name is required")
			return
		}

		// Check for duplicate group name
		for _, item := range store.List() {
			var existing map[string]interface{}
			json.Unmarshal(item, &existing)
			if existing["name"] == name {
				WriteError(w, http.StatusConflict, "A group with this name already exists")
				return
			}
		}

		groupID := nextID("group")
		group := map[string]interface{}{
			"groupId": groupID,
			"name":    name,
			"self":    fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID),
		}

		data, _ := json.Marshal(group)
		store.Set(groupID, data)
		WriteJSON(w, http.StatusCreated, group)
	})

	// GET /rest/api/3/group — read group by groupId query param
	s.RegisterEndpoint("GET /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		if groupID == "" {
			WriteError(w, http.StatusBadRequest, "groupId query parameter is required")
			return
		}

		item, ok := store.Get(groupID)
		if !ok {
			WriteError(w, http.StatusNotFound, "Group not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE /rest/api/3/group — delete group by groupId query param
	s.RegisterEndpoint("DELETE /rest/api/3/group", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		if groupID == "" {
			WriteError(w, http.StatusBadRequest, "groupId query parameter is required")
			return
		}

		if !store.Delete(groupID) {
			WriteError(w, http.StatusNotFound, "Group not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /rest/api/3/group/bulk — list all groups
	s.RegisterEndpoint("GET /rest/api/3/group/bulk", func(w http.ResponseWriter, r *http.Request) {
		items := store.List()
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"maxResults": len(items),
			"startAt":    0,
			"total":      len(items),
			"isLast":     true,
			"values":     items,
		})
	})
}

// registerGroupMembershipEndpoints registers group membership endpoints.
func registerGroupMembershipEndpoints(s *Server) {
	store := s.GetStore("group_members")

	// POST /rest/api/3/group/user — add member to group
	s.RegisterEndpoint("POST /rest/api/3/group/user", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		if groupID == "" {
			WriteError(w, http.StatusBadRequest, "groupId query parameter is required")
			return
		}

		// Verify group exists
		groupStore := s.GetStore("groups")
		if _, ok := groupStore.Get(groupID); !ok {
			WriteError(w, http.StatusNotFound, "Group not found")
			return
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		accountID, _ := req["accountId"].(string)
		if accountID == "" {
			WriteError(w, http.StatusBadRequest, "accountId is required")
			return
		}

		// Get existing members for this group
		var members []string
		raw, ok := store.Get(groupID)
		if ok {
			json.Unmarshal(raw, &members)
		}

		// Check if already a member
		for _, m := range members {
			if m == accountID {
				WriteError(w, http.StatusConflict, "User is already a member of this group")
				return
			}
		}

		members = append(members, accountID)
		data, _ := json.Marshal(members)
		store.Set(groupID, data)

		w.WriteHeader(http.StatusCreated)
	})

	// DELETE /rest/api/3/group/user — remove member from group
	s.RegisterEndpoint("DELETE /rest/api/3/group/user", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		accountID := r.URL.Query().Get("accountId")
		if groupID == "" || accountID == "" {
			WriteError(w, http.StatusBadRequest, "groupId and accountId query parameters are required")
			return
		}

		var members []string
		raw, ok := store.Get(groupID)
		if !ok {
			WriteError(w, http.StatusNotFound, "Group membership not found")
			return
		}
		json.Unmarshal(raw, &members)

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
			WriteError(w, http.StatusNotFound, "User is not a member of this group")
			return
		}

		if len(updated) == 0 {
			store.Delete(groupID)
		} else {
			data, _ := json.Marshal(updated)
			store.Set(groupID, data)
		}

		w.WriteHeader(http.StatusNoContent)
	})

	// GET /rest/api/3/group/member — list group members
	s.RegisterEndpoint("GET /rest/api/3/group/member", func(w http.ResponseWriter, r *http.Request) {
		groupID := r.URL.Query().Get("groupId")
		if groupID == "" {
			WriteError(w, http.StatusBadRequest, "groupId query parameter is required")
			return
		}

		var members []string
		raw, ok := store.Get(groupID)
		if ok {
			json.Unmarshal(raw, &members)
		}

		// Look up user details from user store
		userStore := s.GetStore("users")
		var values []json.RawMessage
		for _, accountID := range members {
			userData, found := userStore.Get(accountID)
			if found {
				values = append(values, userData)
			} else {
				stub, _ := json.Marshal(map[string]interface{}{
					"accountId": accountID,
					"active":    true,
				})
				values = append(values, stub)
			}
		}

		if values == nil {
			values = []json.RawMessage{}
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"maxResults": len(values),
			"startAt":    0,
			"total":      len(values),
			"isLast":     true,
			"values":     values,
		})
	})
}

// registerRoleEndpoints registers role CRUD endpoints.
func registerRoleEndpoints(s *Server) {
	store := s.GetStore("roles")

	// POST /rest/api/3/role — create role
	s.RegisterEndpoint("POST /rest/api/3/role", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		name, _ := req["name"].(string)
		if name == "" {
			WriteError(w, http.StatusBadRequest, "name is required")
			return
		}

		// Check for duplicate role name
		for _, item := range store.List() {
			var existing map[string]interface{}
			json.Unmarshal(item, &existing)
			if existing["name"] == name {
				WriteError(w, http.StatusConflict, "A role with this name already exists")
				return
			}
		}

		roleID := nextID("role")
		role := map[string]interface{}{
			"id":          roleID,
			"name":        name,
			"description": req["description"],
			"scope":       req["scope"],
			"self":        fmt.Sprintf("/rest/api/3/role/%s", roleID),
		}

		data, _ := json.Marshal(role)
		store.Set(roleID, data)
		WriteJSON(w, http.StatusCreated, role)
	})

	// GET /rest/api/3/role — list all roles
	s.RegisterEndpoint("GET /rest/api/3/role", func(w http.ResponseWriter, r *http.Request) {
		items := store.List()
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, items)
	})

	// GET /rest/api/3/role/{id} — read role
	s.RegisterEndpoint("GET /rest/api/3/role/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Role not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /rest/api/3/role/{id} — update role
	s.RegisterEndpoint("PUT /rest/api/3/role/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Role not found")
			return
		}

		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		for k, v := range updates {
			if k != "id" {
				current[k] = v
			}
		}

		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /rest/api/3/role/{id} — delete role
	s.RegisterEndpoint("DELETE /rest/api/3/role/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, "Role not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// registerTokenEndpoints registers API token CRUD endpoints.
func registerTokenEndpoints(s *Server) {
	store := s.GetStore("tokens")

	// POST /rest/api/3/user/{accountId}/token — create token
	s.RegisterEndpoint("POST /rest/api/3/user/{accountId}/token", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("accountId")

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		label, _ := req["label"].(string)
		if label == "" {
			WriteError(w, http.StatusBadRequest, "label is required")
			return
		}

		// Check token limit per user (max 5)
		tokenCount := 0
		for _, item := range store.List() {
			var existing map[string]interface{}
			json.Unmarshal(item, &existing)
			if uid, _ := existing["userAccountId"].(string); uid == accountID {
				tokenCount++
			}
		}
		if tokenCount >= 5 {
			WriteError(w, http.StatusConflict, "Token limit exceeded: maximum number of API tokens reached for this user")
			return
		}

		tokenID := nextID("token")
		tokenValue := nextID("secret")
		token := map[string]interface{}{
			"id":            tokenID,
			"label":         label,
			"userAccountId": accountID,
			"rawToken":      tokenValue,
			"createdAt":     time.Now().UTC().Format(time.RFC3339),
		}

		// Store with composite key: accountId/tokenId
		storeKey := fmt.Sprintf("%s/%s", accountID, tokenID)
		data, _ := json.Marshal(token)
		store.Set(storeKey, data)
		WriteJSON(w, http.StatusCreated, token)
	})

	// GET /rest/api/3/user/{accountId}/token/{tokenId} — read token
	s.RegisterEndpoint("GET /rest/api/3/user/{accountId}/token/{tokenId}", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("accountId")
		tokenID := r.PathValue("tokenId")
		storeKey := fmt.Sprintf("%s/%s", accountID, tokenID)

		item, ok := store.Get(storeKey)
		if !ok {
			WriteError(w, http.StatusNotFound, "Token not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE /rest/api/3/user/{accountId}/token/{tokenId} — delete token
	s.RegisterEndpoint("DELETE /rest/api/3/user/{accountId}/token/{tokenId}", func(w http.ResponseWriter, r *http.Request) {
		accountID := r.PathValue("accountId")
		tokenID := r.PathValue("tokenId")
		storeKey := fmt.Sprintf("%s/%s", accountID, tokenID)

		if !store.Delete(storeKey) {
			WriteError(w, http.StatusNotFound, "Token not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// registerRoleAssignmentEndpoints registers role assignment CRUD endpoints.
func registerRoleAssignmentEndpoints(s *Server) {
	store := s.GetStore("role_assignments")

	// POST /rest/api/3/role/assignment — create role assignment
	s.RegisterEndpoint("POST /rest/api/3/role/assignment", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		roleID, _ := req["roleId"].(string)
		principalType, _ := req["principalType"].(string)
		principalID, _ := req["principalId"].(string)
		scope, _ := req["scope"].(string)
		productID, _ := req["productId"].(string)

		if roleID == "" || principalType == "" || principalID == "" || scope == "" {
			WriteError(w, http.StatusBadRequest, "roleId, principalType, principalId, and scope are required")
			return
		}

		assignmentID := nextID("assign")
		assignment := map[string]interface{}{
			"id":            assignmentID,
			"roleId":        roleID,
			"principalType": principalType,
			"principalId":   principalID,
			"scope":         scope,
		}
		if productID != "" {
			assignment["productId"] = productID
		}

		data, _ := json.Marshal(assignment)
		store.Set(assignmentID, data)
		WriteJSON(w, http.StatusCreated, assignment)
	})

	// GET /rest/api/3/role/assignment/{id} — read role assignment
	s.RegisterEndpoint("GET /rest/api/3/role/assignment/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Role assignment not found")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE /rest/api/3/role/assignment/{id} — delete role assignment
	s.RegisterEndpoint("DELETE /rest/api/3/role/assignment/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, "Role assignment not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
