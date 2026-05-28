package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// RegisterBitbucketEndpoints registers all Bitbucket CRUD endpoints on the mock server.
func RegisterBitbucketEndpoints(s *Server) {
	registerRepositoryEndpoints(s)
	registerBranchRestrictionEndpoints(s)
	registerPipelineConfigEndpoints(s)
	registerDeploymentEnvironmentEndpoints(s)
	registerRepoPermissionUserEndpoints(s)
	registerRepoPermissionGroupEndpoints(s)
}

// registerRepositoryEndpoints registers Bitbucket repository CRUD endpoints.
func registerRepositoryEndpoints(s *Server) {
	store := s.GetStore("bb_repositories")

	// PUT /2.0/repositories/{workspace}/{slug} -- create or update repository
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		existing, exists := store.Get(key)
		if exists {
			// Update existing
			var current map[string]interface{}
			json.Unmarshal(existing, &current)
			for k, v := range req {
				if k != "uuid" && k != "slug" && k != "full_name" {
					current[k] = v
				}
			}
			data, _ := json.Marshal(current)
			store.Set(key, data)
			WriteJSON(w, http.StatusOK, current)
			return
		}

		// Create new
		name, _ := req["name"].(string)
		if name == "" {
			WriteError(w, http.StatusBadRequest, "name is required")
			return
		}

		id := nextID("bb_repo")
		uuid := fmt.Sprintf("{%s}", id)
		isPrivate, _ := req["is_private"].(bool)
		forkPolicy, _ := req["fork_policy"].(string)
		if forkPolicy == "" {
			forkPolicy = "allow_forks"
		}
		language, _ := req["language"].(string)
		description, _ := req["description"].(string)
		hasIssues, _ := req["has_issues"].(bool)
		hasWiki, _ := req["has_wiki"].(bool)

		repo := map[string]interface{}{
			"uuid":        uuid,
			"slug":        slug,
			"name":        name,
			"full_name":   workspace + "/" + slug,
			"description": description,
			"is_private":  isPrivate,
			"fork_policy": forkPolicy,
			"language":    language,
			"has_issues":  hasIssues,
			"has_wiki":    hasWiki,
			"workspace": map[string]interface{}{
				"slug": workspace,
			},
			"links": map[string]interface{}{
				"html": map[string]interface{}{
					"href": fmt.Sprintf("https://bitbucket.org/%s/%s", workspace, slug),
				},
				"clone": []map[string]interface{}{
					{
						"name": "https",
						"href": fmt.Sprintf("https://bitbucket.org/%s/%s.git", workspace, slug),
					},
					{
						"name": "ssh",
						"href": fmt.Sprintf("git@bitbucket.org:%s/%s.git", workspace, slug),
					},
				},
			},
		}

		// Handle mainbranch
		if mb, ok := req["mainbranch"]; ok {
			repo["mainbranch"] = mb
		}

		data, _ := json.Marshal(repo)
		store.Set(key, data)
		WriteJSON(w, http.StatusOK, repo)
	})

	// GET /2.0/repositories/{workspace}/{slug} -- read repository
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		item, ok := store.Get(key)
		if !ok {
			WriteError(w, http.StatusNotFound, "Repository not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE /2.0/repositories/{workspace}/{slug} -- delete repository
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		if !store.Delete(key) {
			WriteError(w, http.StatusNotFound, "Repository not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /2.0/repositories/{workspace} -- list repositories in workspace
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var repo map[string]interface{}
			json.Unmarshal(item, &repo)
			fn, _ := repo["full_name"].(string)
			if len(fn) > len(workspace) && fn[:len(workspace)+1] == workspace+"/" {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"size":   len(items),
			"page":   1,
			"values": items,
		})
	})
}

// registerBranchRestrictionEndpoints registers branch restriction CRUD endpoints.
func registerBranchRestrictionEndpoints(s *Server) {
	store := s.GetStore("bb_branch_restrictions")

	// POST /2.0/repositories/{workspace}/{slug}/branch-restrictions -- create
	s.RegisterEndpoint("POST /2.0/repositories/{workspace}/{slug}/branch-restrictions", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")

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

		id := nextID("bb_branchrestrict")
		req["id"] = id
		req["_workspace"] = workspace
		req["_slug"] = slug
		data, _ := json.Marshal(req)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, req)
	})

	// GET /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id} -- read
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Branch restriction not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id} -- update
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Branch restriction not found")
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

	// DELETE /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id}
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{slug}/branch-restrictions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, "Branch restriction not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /2.0/repositories/{workspace}/{slug}/branch-restrictions -- list
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/branch-restrictions", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var br map[string]interface{}
			json.Unmarshal(item, &br)
			if br["_workspace"] == workspace && br["_slug"] == slug {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"size":   len(items),
			"page":   1,
			"values": items,
		})
	})
}

// registerPipelineConfigEndpoints registers pipeline configuration CRUD endpoints.
func registerPipelineConfigEndpoints(s *Server) {
	store := s.GetStore("bb_pipelines")

	// PUT /2.0/repositories/{workspace}/{slug}/pipelines_config -- create/update
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{slug}/pipelines_config", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		req["_key"] = key
		data, _ := json.Marshal(req)
		store.Set(key, data)
		WriteJSON(w, http.StatusOK, req)
	})

	// GET /2.0/repositories/{workspace}/{slug}/pipelines_config -- read
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/pipelines_config", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		item, ok := store.Get(key)
		if !ok {
			WriteError(w, http.StatusNotFound, "Pipeline configuration not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE /2.0/repositories/{workspace}/{slug}/pipelines_config -- delete
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{slug}/pipelines_config", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		key := workspace + "/" + slug

		if !store.Delete(key) {
			WriteError(w, http.StatusNotFound, "Pipeline configuration not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// registerDeploymentEnvironmentEndpoints registers deployment environment CRUD endpoints.
func registerDeploymentEnvironmentEndpoints(s *Server) {
	store := s.GetStore("bb_environments")

	// POST /2.0/repositories/{workspace}/{slug}/environments -- create
	s.RegisterEndpoint("POST /2.0/repositories/{workspace}/{slug}/environments", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")

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

		id := nextID("bb_env")
		req["uuid"] = fmt.Sprintf("{%s}", id)
		req["_workspace"] = workspace
		req["_slug"] = slug
		data, _ := json.Marshal(req)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, req)
	})

	// GET /2.0/repositories/{workspace}/{slug}/environments/{id} -- read
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/environments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Deployment environment not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /2.0/repositories/{workspace}/{slug}/environments/{id} -- update (POST in real API but PUT for mock)
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{slug}/environments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Deployment environment not found")
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
			if k != "uuid" {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /2.0/repositories/{workspace}/{slug}/environments/{id}
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{slug}/environments/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, "Deployment environment not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /2.0/repositories/{workspace}/{slug}/environments -- list
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/environments", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var env map[string]interface{}
			json.Unmarshal(item, &env)
			if env["_workspace"] == workspace && env["_slug"] == slug {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"size":   len(items),
			"page":   1,
			"values": items,
		})
	})
}

// registerRepoPermissionUserEndpoints registers repository user permission CRUD endpoints.
func registerRepoPermissionUserEndpoints(s *Server) {
	store := s.GetStore("bb_perm_users")

	// PUT /2.0/repositories/{workspace}/{slug}/permissions-config/users/{user_id} -- create/update
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{slug}/permissions-config/users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		userID := r.PathValue("user_id")
		key := workspace + "/" + slug + "/user/" + userID

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		req["_key"] = key
		req["_workspace"] = workspace
		req["_slug"] = slug
		req["user_id"] = userID
		data, _ := json.Marshal(req)
		store.Set(key, data)
		WriteJSON(w, http.StatusOK, req)
	})

	// GET /2.0/repositories/{workspace}/{slug}/permissions-config/users/{user_id} -- read
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/permissions-config/users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		userID := r.PathValue("user_id")
		key := workspace + "/" + slug + "/user/" + userID

		item, ok := store.Get(key)
		if !ok {
			WriteError(w, http.StatusNotFound, "User permission not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE /2.0/repositories/{workspace}/{slug}/permissions-config/users/{user_id}
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{slug}/permissions-config/users/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		userID := r.PathValue("user_id")
		key := workspace + "/" + slug + "/user/" + userID

		if !store.Delete(key) {
			WriteError(w, http.StatusNotFound, "User permission not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /2.0/repositories/{workspace}/{slug}/permissions-config/users -- list
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/permissions-config/users", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var perm map[string]interface{}
			json.Unmarshal(item, &perm)
			if perm["_workspace"] == workspace && perm["_slug"] == slug {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"size":   len(items),
			"page":   1,
			"values": items,
		})
	})
}

// registerRepoPermissionGroupEndpoints registers repository group permission CRUD endpoints.
func registerRepoPermissionGroupEndpoints(s *Server) {
	store := s.GetStore("bb_perm_groups")

	// PUT /2.0/repositories/{workspace}/{slug}/permissions-config/groups/{group_slug} -- create/update
	s.RegisterEndpoint("PUT /2.0/repositories/{workspace}/{slug}/permissions-config/groups/{group_slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		groupSlug := r.PathValue("group_slug")
		key := workspace + "/" + slug + "/group/" + groupSlug

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		req["_key"] = key
		req["_workspace"] = workspace
		req["_slug"] = slug
		req["group_slug"] = groupSlug
		data, _ := json.Marshal(req)
		store.Set(key, data)
		WriteJSON(w, http.StatusOK, req)
	})

	// GET /2.0/repositories/{workspace}/{slug}/permissions-config/groups/{group_slug} -- read
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/permissions-config/groups/{group_slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		groupSlug := r.PathValue("group_slug")
		key := workspace + "/" + slug + "/group/" + groupSlug

		item, ok := store.Get(key)
		if !ok {
			WriteError(w, http.StatusNotFound, "Group permission not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// DELETE /2.0/repositories/{workspace}/{slug}/permissions-config/groups/{group_slug}
	s.RegisterEndpoint("DELETE /2.0/repositories/{workspace}/{slug}/permissions-config/groups/{group_slug}", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		groupSlug := r.PathValue("group_slug")
		key := workspace + "/" + slug + "/group/" + groupSlug

		if !store.Delete(key) {
			WriteError(w, http.StatusNotFound, "Group permission not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /2.0/repositories/{workspace}/{slug}/permissions-config/groups -- list
	s.RegisterEndpoint("GET /2.0/repositories/{workspace}/{slug}/permissions-config/groups", func(w http.ResponseWriter, r *http.Request) {
		workspace := r.PathValue("workspace")
		slug := r.PathValue("slug")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var perm map[string]interface{}
			json.Unmarshal(item, &perm)
			if perm["_workspace"] == workspace && perm["_slug"] == slug {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"size":   len(items),
			"page":   1,
			"values": items,
		})
	})
}
