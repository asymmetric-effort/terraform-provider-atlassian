package mock

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock/specs"
)

// RegisterStatuspageEndpoints registers all Statuspage CRUD endpoints
// on the mock server and adds OpenAPI request validation.
func RegisterStatuspageEndpoints(s *Server) {
	specData, err := specs.SpecFS.ReadFile("statuspage.yaml")
	if err != nil {
		log.Printf("WARNING: could not load Statuspage OpenAPI spec: %v", err)
	} else {
		v, err := NewRequestValidatorFromBytes(specData)
		if err != nil {
			log.Printf("WARNING: could not parse Statuspage OpenAPI spec: %v", err)
		} else {
			s.AddValidator(v)
		}
	}
	registerPageEndpoints(s)
	registerComponentEndpoints(s)
	registerComponentGroupEndpoints(s)
	registerSubscriberEndpoints(s)
	registerIncidentTemplateEndpoints(s)
	registerMaintenanceTemplateEndpoints(s)
	registerStatuspagePermissionEndpoints(s)
}

// registerPageEndpoints registers Statuspage page CRUD endpoints.
func registerPageEndpoints(s *Server) {
	store := s.GetStore("sp_pages")

	// POST /v1/pages -- create
	s.RegisterEndpoint("POST /v1/pages", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for Statuspage page creation")
			return
		}
		pageData, _ := req["page"].(map[string]interface{})
		if pageData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'page' field in request body")
			return
		}
		name, _ := pageData["name"].(string)
		if name == "" {
			WriteError(w, http.StatusBadRequest, "name is required when creating a Statuspage page")
			return
		}

		id := nextID("sp-page")
		subdomain, _ := pageData["subdomain"].(string)
		if subdomain == "" {
			subdomain = id
		}
		description, _ := pageData["page_description"].(string)

		page := map[string]interface{}{
			"id":               id,
			"name":             name,
			"page_description": description,
			"subdomain":        subdomain,
			"url":              fmt.Sprintf("https://%s.statuspage.io", subdomain),
		}
		data, _ := json.Marshal(page)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, page)
	})

	// GET /v1/pages/{page_id} -- read
	s.RegisterEndpoint("GET /v1/pages/{page_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("page_id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Statuspage page %q not found. Verify the page ID is correct.", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /v1/pages/{page_id} -- update
	s.RegisterEndpoint("PUT /v1/pages/{page_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("page_id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Statuspage page %q not found. The page may have been deleted outside of Terraform.", id))
			return
		}
		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for Statuspage page update")
			return
		}
		pageData, _ := req["page"].(map[string]interface{})
		if pageData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'page' field in request body")
			return
		}
		for k, v := range pageData {
			if k != "id" {
				current[k] = v
			}
		}
		if sd, ok := current["subdomain"].(string); ok {
			current["url"] = fmt.Sprintf("https://%s.statuspage.io", sd)
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /v1/pages/{page_id} -- delete
	s.RegisterEndpoint("DELETE /v1/pages/{page_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("page_id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Statuspage page %q not found. The page may have already been deleted.", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /v1/pages -- list
	s.RegisterEndpoint("GET /v1/pages", func(w http.ResponseWriter, r *http.Request) {
		items := store.List()
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, items)
	})
}

// registerComponentEndpoints registers Statuspage component CRUD endpoints.
func registerComponentEndpoints(s *Server) {
	store := s.GetStore("sp_components")

	// POST /v1/pages/{page_id}/components -- create
	s.RegisterEndpoint("POST /v1/pages/{page_id}/components", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for component creation")
			return
		}
		compData, _ := req["component"].(map[string]interface{})
		if compData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'component' field in request body")
			return
		}
		name, _ := compData["name"].(string)
		if name == "" {
			WriteError(w, http.StatusBadRequest, "name is required when creating a Statuspage component")
			return
		}

		id := nextID("sp-comp")
		description, _ := compData["description"].(string)
		status, _ := compData["status"].(string)
		if status == "" {
			status = "operational"
		}
		groupID, _ := compData["group_id"].(string)

		comp := map[string]interface{}{
			"id":          id,
			"page_id":     pageID,
			"name":        name,
			"description": description,
			"status":      status,
			"group_id":    groupID,
		}
		data, _ := json.Marshal(comp)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, comp)
	})

	// GET /v1/pages/{page_id}/components/{component_id} -- read
	s.RegisterEndpoint("GET /v1/pages/{page_id}/components/{component_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("component_id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Statuspage component %q not found. Verify the component ID is correct.", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /v1/pages/{page_id}/components/{component_id} -- update
	s.RegisterEndpoint("PUT /v1/pages/{page_id}/components/{component_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("component_id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Statuspage component %q not found. The component may have been deleted outside of Terraform.", id))
			return
		}
		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for component update")
			return
		}
		compData, _ := req["component"].(map[string]interface{})
		if compData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'component' field in request body")
			return
		}
		for k, v := range compData {
			if k != "id" && k != "page_id" {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /v1/pages/{page_id}/components/{component_id} -- delete
	s.RegisterEndpoint("DELETE /v1/pages/{page_id}/components/{component_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("component_id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Statuspage component %q not found. The component may have already been deleted.", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /v1/pages/{page_id}/components -- list
	s.RegisterEndpoint("GET /v1/pages/{page_id}/components", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var comp map[string]interface{}
			json.Unmarshal(item, &comp)
			if comp["page_id"] == pageID {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, items)
	})
}

// registerComponentGroupEndpoints registers component group CRUD endpoints.
func registerComponentGroupEndpoints(s *Server) {
	store := s.GetStore("sp_component_groups")

	// POST /v1/pages/{page_id}/component-groups -- create
	s.RegisterEndpoint("POST /v1/pages/{page_id}/component-groups", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for component group creation")
			return
		}
		groupData, _ := req["component_group"].(map[string]interface{})
		if groupData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'component_group' field in request body")
			return
		}
		name, _ := groupData["name"].(string)
		if name == "" {
			WriteError(w, http.StatusBadRequest, "name is required when creating a component group")
			return
		}

		id := nextID("sp-cg")
		description, _ := groupData["description"].(string)

		group := map[string]interface{}{
			"id":          id,
			"page_id":     pageID,
			"name":        name,
			"description": description,
		}
		data, _ := json.Marshal(group)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, group)
	})

	// GET /v1/pages/{page_id}/component-groups/{group_id} -- read
	s.RegisterEndpoint("GET /v1/pages/{page_id}/component-groups/{group_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("group_id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Component group %q not found. Verify the group ID is correct.", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /v1/pages/{page_id}/component-groups/{group_id} -- update
	s.RegisterEndpoint("PUT /v1/pages/{page_id}/component-groups/{group_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("group_id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Component group %q not found. The group may have been deleted outside of Terraform.", id))
			return
		}
		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for component group update")
			return
		}
		groupData, _ := req["component_group"].(map[string]interface{})
		if groupData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'component_group' field in request body")
			return
		}
		for k, v := range groupData {
			if k != "id" && k != "page_id" {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /v1/pages/{page_id}/component-groups/{group_id} -- delete
	s.RegisterEndpoint("DELETE /v1/pages/{page_id}/component-groups/{group_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("group_id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Component group %q not found. The group may have already been deleted.", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /v1/pages/{page_id}/component-groups -- list
	s.RegisterEndpoint("GET /v1/pages/{page_id}/component-groups", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var group map[string]interface{}
			json.Unmarshal(item, &group)
			if group["page_id"] == pageID {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, items)
	})
}

// registerSubscriberEndpoints registers subscriber CRUD endpoints.
func registerSubscriberEndpoints(s *Server) {
	store := s.GetStore("sp_subscribers")

	// POST /v1/pages/{page_id}/subscribers -- create
	s.RegisterEndpoint("POST /v1/pages/{page_id}/subscribers", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for subscriber creation")
			return
		}
		subData, _ := req["subscriber"].(map[string]interface{})
		if subData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'subscriber' field in request body")
			return
		}

		id := nextID("sp-sub")
		email, _ := subData["email"].(string)
		endpoint, _ := subData["endpoint"].(string)

		var componentIDs []string
		if cids, ok := subData["component_ids"].([]interface{}); ok {
			for _, cid := range cids {
				if s, ok := cid.(string); ok {
					componentIDs = append(componentIDs, s)
				}
			}
		}
		if componentIDs == nil {
			componentIDs = []string{}
		}

		sub := map[string]interface{}{
			"id":            id,
			"page_id":       pageID,
			"email":         email,
			"endpoint":      endpoint,
			"component_ids": componentIDs,
		}
		data, _ := json.Marshal(sub)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, sub)
	})

	// GET /v1/pages/{page_id}/subscribers/{subscriber_id} -- read
	s.RegisterEndpoint("GET /v1/pages/{page_id}/subscribers/{subscriber_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("subscriber_id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Subscriber %q not found. Verify the subscriber ID is correct.", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /v1/pages/{page_id}/subscribers/{subscriber_id} -- update
	s.RegisterEndpoint("PUT /v1/pages/{page_id}/subscribers/{subscriber_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("subscriber_id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Subscriber %q not found. The subscriber may have been removed outside of Terraform.", id))
			return
		}
		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for subscriber update")
			return
		}
		subData, _ := req["subscriber"].(map[string]interface{})
		if subData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'subscriber' field in request body")
			return
		}
		for k, v := range subData {
			if k != "id" && k != "page_id" {
				current[k] = v
			}
		}
		// Normalize component_ids
		if cids, ok := current["component_ids"].([]interface{}); ok {
			var normalized []string
			for _, cid := range cids {
				if s, ok := cid.(string); ok {
					normalized = append(normalized, s)
				}
			}
			if normalized == nil {
				normalized = []string{}
			}
			current["component_ids"] = normalized
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /v1/pages/{page_id}/subscribers/{subscriber_id} -- delete
	s.RegisterEndpoint("DELETE /v1/pages/{page_id}/subscribers/{subscriber_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("subscriber_id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Subscriber %q not found. The subscriber may have already been removed.", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /v1/pages/{page_id}/subscribers -- list
	s.RegisterEndpoint("GET /v1/pages/{page_id}/subscribers", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var sub map[string]interface{}
			json.Unmarshal(item, &sub)
			if sub["page_id"] == pageID {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, items)
	})
}

// registerIncidentTemplateEndpoints registers incident template CRUD endpoints.
func registerIncidentTemplateEndpoints(s *Server) {
	store := s.GetStore("sp_incident_templates")

	// POST /v1/pages/{page_id}/incident_templates -- create
	s.RegisterEndpoint("POST /v1/pages/{page_id}/incident_templates", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for incident template creation")
			return
		}
		tmplData, _ := req["template"].(map[string]interface{})
		if tmplData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'template' field in request body")
			return
		}
		name, _ := tmplData["name"].(string)
		if name == "" {
			WriteError(w, http.StatusBadRequest, "name is required when creating an incident template")
			return
		}

		id := nextID("sp-it")
		title, _ := tmplData["title"].(string)
		body, _ := tmplData["body"].(string)

		tmpl := map[string]interface{}{
			"id":      id,
			"page_id": pageID,
			"name":    name,
			"title":   title,
			"body":    body,
		}
		data, _ := json.Marshal(tmpl)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, tmpl)
	})

	// GET /v1/pages/{page_id}/incident_templates/{template_id} -- read
	s.RegisterEndpoint("GET /v1/pages/{page_id}/incident_templates/{template_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("template_id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Incident template %q not found. Verify the template ID is correct.", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /v1/pages/{page_id}/incident_templates/{template_id} -- update
	s.RegisterEndpoint("PUT /v1/pages/{page_id}/incident_templates/{template_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("template_id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Incident template %q not found. The template may have been deleted outside of Terraform.", id))
			return
		}
		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for incident template update")
			return
		}
		tmplData, _ := req["template"].(map[string]interface{})
		if tmplData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'template' field in request body")
			return
		}
		for k, v := range tmplData {
			if k != "id" && k != "page_id" {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /v1/pages/{page_id}/incident_templates/{template_id} -- delete
	s.RegisterEndpoint("DELETE /v1/pages/{page_id}/incident_templates/{template_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("template_id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Incident template %q not found. The template may have already been deleted.", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /v1/pages/{page_id}/incident_templates -- list
	s.RegisterEndpoint("GET /v1/pages/{page_id}/incident_templates", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var tmpl map[string]interface{}
			json.Unmarshal(item, &tmpl)
			if tmpl["page_id"] == pageID {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, items)
	})
}

// registerMaintenanceTemplateEndpoints registers maintenance template CRUD endpoints.
func registerMaintenanceTemplateEndpoints(s *Server) {
	store := s.GetStore("sp_maintenance_templates")

	// POST /v1/pages/{page_id}/maintenance_templates -- create
	s.RegisterEndpoint("POST /v1/pages/{page_id}/maintenance_templates", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for maintenance template creation")
			return
		}
		tmplData, _ := req["template"].(map[string]interface{})
		if tmplData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'template' field in request body")
			return
		}
		name, _ := tmplData["name"].(string)
		if name == "" {
			WriteError(w, http.StatusBadRequest, "name is required when creating a maintenance template")
			return
		}

		id := nextID("sp-mt")
		title, _ := tmplData["title"].(string)
		body, _ := tmplData["body"].(string)

		tmpl := map[string]interface{}{
			"id":      id,
			"page_id": pageID,
			"name":    name,
			"title":   title,
			"body":    body,
		}
		data, _ := json.Marshal(tmpl)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, tmpl)
	})

	// GET /v1/pages/{page_id}/maintenance_templates/{template_id} -- read
	s.RegisterEndpoint("GET /v1/pages/{page_id}/maintenance_templates/{template_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("template_id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Maintenance template %q not found. Verify the template ID is correct.", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /v1/pages/{page_id}/maintenance_templates/{template_id} -- update
	s.RegisterEndpoint("PUT /v1/pages/{page_id}/maintenance_templates/{template_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("template_id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Maintenance template %q not found. The template may have been deleted outside of Terraform.", id))
			return
		}
		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for maintenance template update")
			return
		}
		tmplData, _ := req["template"].(map[string]interface{})
		if tmplData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'template' field in request body")
			return
		}
		for k, v := range tmplData {
			if k != "id" && k != "page_id" {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /v1/pages/{page_id}/maintenance_templates/{template_id} -- delete
	s.RegisterEndpoint("DELETE /v1/pages/{page_id}/maintenance_templates/{template_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("template_id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Maintenance template %q not found. The template may have already been deleted.", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /v1/pages/{page_id}/maintenance_templates -- list
	s.RegisterEndpoint("GET /v1/pages/{page_id}/maintenance_templates", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var tmpl map[string]interface{}
			json.Unmarshal(item, &tmpl)
			if tmpl["page_id"] == pageID {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, items)
	})
}

// registerStatuspagePermissionEndpoints registers permission CRUD endpoints.
func registerStatuspagePermissionEndpoints(s *Server) {
	store := s.GetStore("sp_permissions")

	// POST /v1/pages/{page_id}/permissions -- create
	s.RegisterEndpoint("POST /v1/pages/{page_id}/permissions", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for permission creation")
			return
		}
		permData, _ := req["permission"].(map[string]interface{})
		if permData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'permission' field in request body")
			return
		}
		principalType, _ := permData["principal_type"].(string)
		principalID, _ := permData["principal_id"].(string)
		role, _ := permData["role"].(string)
		if principalType == "" || principalID == "" || role == "" {
			WriteError(w, http.StatusBadRequest, "principal_type, principal_id, and role are required when creating a Statuspage permission")
			return
		}

		id := nextID("sp-perm")

		perm := map[string]interface{}{
			"id":             id,
			"page_id":        pageID,
			"principal_type": principalType,
			"principal_id":   principalID,
			"role":           role,
		}
		data, _ := json.Marshal(perm)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, perm)
	})

	// GET /v1/pages/{page_id}/permissions/{permission_id} -- read
	s.RegisterEndpoint("GET /v1/pages/{page_id}/permissions/{permission_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("permission_id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Permission %q not found. Verify the permission ID is correct.", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /v1/pages/{page_id}/permissions/{permission_id} -- update
	s.RegisterEndpoint("PUT /v1/pages/{page_id}/permissions/{permission_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("permission_id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Permission %q not found. The permission may have been revoked outside of Terraform.", id))
			return
		}
		var current map[string]interface{}
		json.Unmarshal(existing, &current)

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body for permission update")
			return
		}
		permData, _ := req["permission"].(map[string]interface{})
		if permData == nil {
			WriteError(w, http.StatusBadRequest, "Missing 'permission' field in request body")
			return
		}
		for k, v := range permData {
			if k != "id" && k != "page_id" {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /v1/pages/{page_id}/permissions/{permission_id} -- delete
	s.RegisterEndpoint("DELETE /v1/pages/{page_id}/permissions/{permission_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("permission_id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("Permission %q not found. The permission may have already been revoked.", id))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /v1/pages/{page_id}/permissions -- list
	s.RegisterEndpoint("GET /v1/pages/{page_id}/permissions", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		allItems := store.List()
		var items []json.RawMessage
		for _, item := range allItems {
			var perm map[string]interface{}
			json.Unmarshal(item, &perm)
			if perm["page_id"] == pageID {
				items = append(items, item)
			}
		}
		if items == nil {
			items = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, items)
	})
}
