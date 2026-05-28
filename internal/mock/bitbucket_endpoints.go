// Package mock implements Bitbucket Cloud API mock endpoints for testing.
//
// These endpoints simulate the Bitbucket 2.0 REST API for repositories,
// branch restrictions, pipeline configurations, deployments, and
// repository permissions. They provide realistic CRUD behavior including
// duplicate detection, required field validation, and proper error responses.
package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// RegisterBitbucketEndpoints registers all Bitbucket CRUD endpoints on the mock server.
func RegisterBitbucketEndpoints(s *Server) {
	registerBitbucketRepositoryEndpoints(s)
	registerBitbucketBranchRestrictionEndpoints(s)
	registerBitbucketPipelineEndpoints(s)
	registerBitbucketDeploymentEndpoints(s)
	registerBitbucketRepositoryPermissionEndpoints(s)
}

// registerBitbucketRepositoryEndpoints registers /2.0/repositories/{workspace}/{repo_slug} endpoints.
func registerBitbucketRepositoryEndpoints(s *Server) {
	store := s.GetStore("bb_repository")

	// POST — create repository
	s.RegisterEndpoint("POST /2.0/repositories/{workspace}/{repo_slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")

		if workspace == "" || repoSlug == "" {
			WriteError(w, http.StatusBadRequest, "workspace and repo_slug are required")
			return
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		key := workspace + "/" + repoSlug
		if _, exists := store.Get(key); exists {
			WriteError(w, http.StatusConflict, fmt.Sprintf("Repository with slug '%s' already exists in workspace '%s'", repoSlug, workspace))
			return
		}

		req["uuid"] = nextID("bb_repo")
		req["slug"] = repoSlug
		req["full_name"] = key
		req["workspace"] = map[string]interface{}{"slug": workspace}
		if req["scm"] == nil {
			req["scm"] = "git"
		}
		if req["is_private"] == nil {
			req["is_private"] = true
		}
		req["links"] = map[string]interface{}{
			"self": map[string]interface{}{"href": fmt.Sprintf("/2.0/repositories/%s", key)},
		}

		data, _ := json.Marshal(req)
		store.Set(key, data)
		WriteJSON(w, http.StatusCreated, req)
	})

	// GET — read repository
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{repo_slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")
		key := workspace + "/" + repoSlug

		item, ok := store.Get(key)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Repository '%s/%s' not found", workspace, repoSlug))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT — update repository
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{repo_slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")
		key := workspace + "/" + repoSlug

		existing, ok := store.Get(key)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Repository '%s/%s' not found", workspace, repoSlug))
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
			current[k] = v
		}
		// Preserve immutable fields
		current["slug"] = repoSlug
		current["full_name"] = key

		data, _ := json.Marshal(current)
		store.Set(key, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE — delete repository
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{repo_slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")
		key := workspace + "/" + repoSlug

		if !store.Delete(key) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Repository '%s/%s' not found", workspace, repoSlug))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// registerBitbucketBranchRestrictionEndpoints registers /2.0/repositories/{workspace}/{repo_slug}/branch-restrictions endpoints.
func registerBitbucketBranchRestrictionEndpoints(s *Server) {
	store := s.GetStore("bb_branch_restriction")

	// POST — create branch restriction
	s.RegisterEndpoint("POST /2.0/repositories/{workspace}/{repo_slug}/branch-restrictions", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		kind, _ := req["kind"].(string)
		if kind == "" {
			WriteError(w, http.StatusBadRequest, "kind is required")
			return
		}

		pattern, _ := req["pattern"].(string)
		if pattern == "" {
			WriteError(w, http.StatusBadRequest, "pattern is required")
			return
		}

		// Validate pattern format
		if strings.ContainsAny(pattern, "{}[]") {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid branch pattern '%s': must not contain special characters {, }, [, ]", pattern))
			return
		}

		id := nextID("bb_branch_restriction")
		req["id"] = id
		repoKey := workspace + "/" + repoSlug
		req["_repo"] = repoKey

		data, _ := json.Marshal(req)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, req)
	})

	// GET by id
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{repo_slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Branch restriction '%s' not found", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT by id
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{repo_slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Branch restriction '%s' not found", id))
			return
		}

		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		// Validate pattern if provided
		if pattern, ok := updates["pattern"].(string); ok && strings.ContainsAny(pattern, "{}[]") {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid branch pattern '%s': must not contain special characters {, }, [, ]", pattern))
			return
		}

		for k, v := range updates {
			current[k] = v
		}
		current["id"] = id

		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE by id
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{repo_slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Branch restriction '%s' not found", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// registerBitbucketPipelineEndpoints registers /2.0/repositories/{workspace}/{repo_slug}/pipelines_config endpoints.
func registerBitbucketPipelineEndpoints(s *Server) {
	store := s.GetStore("bb_pipeline")

	// PUT — create/update pipeline config (Bitbucket uses PUT for pipeline config)
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{repo_slug}/pipelines_config", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		enabled, hasEnabled := req["enabled"]
		if !hasEnabled {
			WriteError(w, http.StatusBadRequest, "enabled field is required for pipeline configuration")
			return
		}

		key := workspace + "/" + repoSlug
		req["repository"] = map[string]interface{}{"full_name": key}

		// Validate that enabled is a bool
		switch enabled.(type) {
		case bool:
			// valid
		default:
			WriteError(w, http.StatusBadRequest, "enabled must be a boolean value")
			return
		}

		data, _ := json.Marshal(req)
		store.Set(key, data)
		WriteJSON(w, http.StatusOK, req)
	})

	// GET — read pipeline config
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{repo_slug}/pipelines_config", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")
		key := workspace + "/" + repoSlug

		item, ok := store.Get(key)
		if !ok {
			// Pipeline config doesn't exist yet — return default disabled
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"enabled":    false,
				"repository": map[string]interface{}{"full_name": key},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE — disable pipeline config
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{repo_slug}/pipelines_config", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")
		key := workspace + "/" + repoSlug
		store.Delete(key)
		w.WriteHeader(http.StatusNoContent)
	})
}

// registerBitbucketDeploymentEndpoints registers /2.0/repositories/{workspace}/{repo_slug}/environments endpoints.
func registerBitbucketDeploymentEndpoints(s *Server) {
	store := s.GetStore("bb_deployment")

	// POST — create deployment environment
	s.RegisterEndpoint("POST /2.0/repositories/{workspace}/{repo_slug}/environments", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")

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

		envType, _ := req["environment_type"].(map[string]interface{})
		if envType == nil {
			WriteError(w, http.StatusBadRequest, "environment_type is required")
			return
		}

		id := nextID("bb_deployment")
		repoKey := workspace + "/" + repoSlug
		req["uuid"] = id
		req["_repo"] = repoKey

		// Check for duplicate name in same repo
		for _, item := range store.List() {
			var existing map[string]interface{}
			json.Unmarshal(item, &existing)
			if existing["_repo"] == repoKey && existing["name"] == name {
				WriteError(w, http.StatusConflict, fmt.Sprintf("Deployment environment '%s' already exists in repository '%s'", name, repoKey))
				return
			}
		}

		data, _ := json.Marshal(req)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, req)
	})

	// GET by id
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{repo_slug}/environments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Deployment environment '%s' not found", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT by id — update deployment environment
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{repo_slug}/environments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Deployment environment '%s' not found", id))
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
			current[k] = v
		}
		current["uuid"] = id

		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE by id
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{repo_slug}/environments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Deployment environment '%s' not found", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// registerBitbucketRepositoryPermissionEndpoints registers /2.0/repositories/{workspace}/{repo_slug}/permissions-config/users and groups endpoints.
func registerBitbucketRepositoryPermissionEndpoints(s *Server) {
	store := s.GetStore("bb_permission")

	// POST — add user permission
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{repo_slug}/permissions-config/users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")
		userID := r.PathValue("user_id")

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		permission, _ := req["permission"].(string)
		if permission == "" {
			WriteError(w, http.StatusBadRequest, "permission is required (read, write, or admin)")
			return
		}
		if permission != "read" && permission != "write" && permission != "admin" {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid permission '%s': must be 'read', 'write', or 'admin'", permission))
			return
		}

		key := workspace + "/" + repoSlug + "/user/" + userID
		entry := map[string]interface{}{
			"permission": permission,
			"user":       map[string]interface{}{"account_id": userID},
			"repository": map[string]interface{}{"full_name": workspace + "/" + repoSlug},
		}

		data, _ := json.Marshal(entry)
		store.Set(key, data)
		WriteJSON(w, http.StatusOK, entry)
	})

	// GET user permission
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{repo_slug}/permissions-config/users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")
		userID := r.PathValue("user_id")
		key := workspace + "/" + repoSlug + "/user/" + userID

		item, ok := store.Get(key)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Permission not found for user '%s' on repository '%s/%s'", userID, workspace, repoSlug))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE user permission
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{repo_slug}/permissions-config/users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		repoSlug := r.PathValue("repo_slug")
		userID := r.PathValue("user_id")
		key := workspace + "/" + repoSlug + "/user/" + userID

		if !store.Delete(key) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Permission not found for user '%s' on repository '%s/%s'", userID, workspace, repoSlug))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
