package mock

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock/specs"
)

// RegisterConfluenceEndpoints registers all Confluence CRUD endpoints
// on the mock server and adds OpenAPI request validation.
func RegisterConfluenceEndpoints(s *Server) {
	specData, err := specs.SpecFS.ReadFile("confluence.yaml")
	if err != nil {
		log.Printf("WARNING: could not load Confluence OpenAPI spec: %v", err)
	} else {
		v, err := NewRequestValidatorFromBytes(specData)
		if err != nil {
			log.Printf("WARNING: could not parse Confluence OpenAPI spec: %v", err)
		} else {
			s.AddValidator(v)
		}
	}
	registerConfluenceSpaceEndpoints(s)
	registerConfluencePageEndpoints(s)
	registerConfluenceTemplateEndpoints(s)
	registerSpacePermissionEndpoints(s)
	registerContentRestrictionEndpoints(s)
}

// registerConfluenceSpaceEndpoints registers Confluence space CRUD endpoints.
// Spaces are stored by both id and key for lookup. Duplicate key detection is enforced.
// Supports GET by key query param on the list endpoint.
func registerConfluenceSpaceEndpoints(s *Server) {
	store := s.GetStore("confluence_spaces")

	// POST /wiki/api/v2/spaces — create space
	s.RegisterEndpoint("POST /wiki/api/v2/spaces", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		name, _ := req["name"].(string)
		key, _ := req["key"].(string)
		if name == "" || key == "" {
			WriteError(w, http.StatusBadRequest, "name and key are required")
			return
		}

		// Check for duplicate key
		for _, item := range store.List() {
			var existing map[string]interface{}
			json.Unmarshal(item, &existing)
			if existing["key"] == key {
				WriteError(w, http.StatusConflict, "A space with this key already exists")
				return
			}
		}

		id := nextID("confluence_space")
		space := map[string]interface{}{
			"id":   id,
			"key":  key,
			"name": name,
			"type": "global",
			"self": fmt.Sprintf("/wiki/api/v2/spaces/%s", id),
		}
		for k, v := range req {
			if _, exists := space[k]; !exists {
				space[k] = v
			}
		}
		data, _ := json.Marshal(space)
		store.Set(id, data)
		store.Set(key, data)
		WriteJSON(w, http.StatusCreated, space)
	})

	// GET /wiki/api/v2/spaces/{id} — read space
	s.RegisterEndpoint("GET /wiki/api/v2/spaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Space not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /wiki/api/v2/spaces/{id} — update space
	s.RegisterEndpoint("PUT /wiki/api/v2/spaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Space not found")
			return
		}
		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		oldKey, _ := current["key"].(string)
		for k, v := range updates {
			if k != "id" {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		currentID, _ := current["id"].(string)
		store.Set(currentID, data)
		newKey, _ := current["key"].(string)
		if oldKey != newKey {
			store.Delete(oldKey)
		}
		store.Set(newKey, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /wiki/api/v2/spaces/{id} — delete space
	s.RegisterEndpoint("DELETE /wiki/api/v2/spaces/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Space not found")
			return
		}
		var space map[string]interface{}
		json.Unmarshal(item, &space)
		spaceID, _ := space["id"].(string)
		spaceKey, _ := space["key"].(string)
		store.Delete(spaceID)
		store.Delete(spaceKey)
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /wiki/api/v2/spaces — list spaces (supports ?key= query param)
	s.RegisterEndpoint("GET /wiki/api/v2/spaces", func(w http.ResponseWriter, r *http.Request) {
		keyFilter := r.URL.Query().Get("key")

		allItems := store.List()
		// Deduplicate: spaces are stored by both id and key
		seen := make(map[string]bool)
		var items []json.RawMessage
		for _, item := range allItems {
			var sp map[string]interface{}
			json.Unmarshal(item, &sp)
			id, _ := sp["id"].(string)
			if seen[id] {
				continue
			}
			seen[id] = true
			if keyFilter != "" {
				spKey, _ := sp["key"].(string)
				if spKey != keyFilter {
					continue
				}
			}
			items = append(items, item)
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"results": items,
			"_links":  map[string]interface{}{},
		})
	})
}

// registerConfluencePageEndpoints registers Confluence page CRUD endpoints.
// Pages support filtering by space_id on the list endpoint.
func registerConfluencePageEndpoints(s *Server) {
	store := s.GetStore("confluence_pages")

	// POST /wiki/api/v2/pages — create page
	s.RegisterEndpoint("POST /wiki/api/v2/pages", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		title, _ := req["title"].(string)
		spaceID, _ := req["spaceId"].(string)
		if title == "" || spaceID == "" {
			WriteError(w, http.StatusBadRequest, "title and spaceId are required")
			return
		}

		id := nextID("confluence_page")
		page := map[string]interface{}{
			"id":      id,
			"title":   title,
			"spaceId": spaceID,
			"status":  "current",
			"version": map[string]interface{}{"number": float64(1)},
			"self":    fmt.Sprintf("/wiki/api/v2/pages/%s", id),
		}
		for k, v := range req {
			if _, exists := page[k]; !exists {
				page[k] = v
			}
		}
		data, _ := json.Marshal(page)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, page)
	})

	// GET /wiki/api/v2/pages/{id} — read page
	s.RegisterEndpoint("GET /wiki/api/v2/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Page not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /wiki/api/v2/pages/{id} — update page
	s.RegisterEndpoint("PUT /wiki/api/v2/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Page not found")
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

	// DELETE /wiki/api/v2/pages/{id} — delete page
	s.RegisterEndpoint("DELETE /wiki/api/v2/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, "Page not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /wiki/api/v2/pages — list pages (supports ?space_id= filter)
	s.RegisterEndpoint("GET /wiki/api/v2/pages", func(w http.ResponseWriter, r *http.Request) {
		spaceFilter := r.URL.Query().Get("space_id")

		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			if spaceFilter != "" {
				var pg map[string]interface{}
				json.Unmarshal(item, &pg)
				pgSpaceID, _ := pg["spaceId"].(string)
				if pgSpaceID != spaceFilter {
					continue
				}
			}
			items = append(items, item)
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"results": items,
			"_links":  map[string]interface{}{},
		})
	})
}

// registerConfluenceTemplateEndpoints registers Confluence template CRUD endpoints.
func registerConfluenceTemplateEndpoints(s *Server) {
	registerCRUDEndpoints(s, "confluence_templates", "/wiki/api/v2/templates", "id", "Template", []string{"name"}, "name")
}

// registerSpacePermissionEndpoints registers Confluence space permission endpoints.
// Permissions are scoped to a space and support create, delete, and list.
func registerSpacePermissionEndpoints(s *Server) {
	store := s.GetStore("confluence_space_permissions")

	// POST /wiki/api/v2/spaces/{id}/permissions — create permission
	s.RegisterEndpoint("POST /wiki/api/v2/spaces/{id}/permissions", func(w http.ResponseWriter, r *http.Request) {
		spaceID := r.PathValue("id")

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		principalType, _ := req["principalType"].(string)
		if principalType == "" {
			WriteError(w, http.StatusBadRequest, "principalType is required")
			return
		}

		permID := nextID("confluence_space_perm")
		perm := map[string]interface{}{
			"id":      permID,
			"spaceId": spaceID,
			"self":    fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions/%s", spaceID, permID),
		}
		for k, v := range req {
			if _, exists := perm[k]; !exists {
				perm[k] = v
			}
		}
		data, _ := json.Marshal(perm)
		store.Set(permID, data)
		WriteJSON(w, http.StatusCreated, perm)
	})

	// DELETE /wiki/api/v2/spaces/{id}/permissions/{permId} — delete permission
	s.RegisterEndpoint("DELETE /wiki/api/v2/spaces/{id}/permissions/{permId}", func(w http.ResponseWriter, r *http.Request) {
		permID := r.PathValue("permId")
		if !store.Delete(permID) {
			WriteError(w, http.StatusNotFound, "Permission not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /wiki/api/v2/spaces/{id}/permissions — list permissions for space
	s.RegisterEndpoint("GET /wiki/api/v2/spaces/{id}/permissions", func(w http.ResponseWriter, r *http.Request) {
		spaceID := r.PathValue("id")

		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var p map[string]interface{}
			json.Unmarshal(item, &p)
			if p["spaceId"] == spaceID {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"results": items,
			"_links":  map[string]interface{}{},
		})
	})
}

// registerContentRestrictionEndpoints registers Confluence content restriction endpoints.
// Restrictions are scoped to a content (page) ID and support create, delete, and list.
func registerContentRestrictionEndpoints(s *Server) {
	store := s.GetStore("confluence_content_restrictions")

	// POST /wiki/api/v2/content/{id}/restrictions — create restriction
	s.RegisterEndpoint("POST /wiki/api/v2/content/{id}/restrictions", func(w http.ResponseWriter, r *http.Request) {
		contentID := r.PathValue("id")

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		operation, _ := req["operation"].(string)
		if operation == "" {
			WriteError(w, http.StatusBadRequest, "operation is required")
			return
		}

		restrictionID := nextID("confluence_restriction")
		restriction := map[string]interface{}{
			"id":        restrictionID,
			"contentId": contentID,
			"self":      fmt.Sprintf("/wiki/api/v2/content/%s/restrictions/%s", contentID, restrictionID),
		}
		for k, v := range req {
			if _, exists := restriction[k]; !exists {
				restriction[k] = v
			}
		}
		data, _ := json.Marshal(restriction)
		store.Set(restrictionID, data)
		WriteJSON(w, http.StatusCreated, restriction)
	})

	// DELETE /wiki/api/v2/content/{id}/restrictions/{restrictionId} — delete restriction
	s.RegisterEndpoint("DELETE /wiki/api/v2/content/{id}/restrictions/{restrictionId}", func(w http.ResponseWriter, r *http.Request) {
		restrictionID := r.PathValue("restrictionId")
		if !store.Delete(restrictionID) {
			WriteError(w, http.StatusNotFound, "Restriction not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /wiki/api/v2/content/{id}/restrictions — list restrictions for content
	s.RegisterEndpoint("GET /wiki/api/v2/content/{id}/restrictions", func(w http.ResponseWriter, r *http.Request) {
		contentID := r.PathValue("id")

		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var res map[string]interface{}
			json.Unmarshal(item, &res)
			if res["contentId"] == contentID {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"results": items,
			"_links":  map[string]interface{}{},
		})
	})
}
