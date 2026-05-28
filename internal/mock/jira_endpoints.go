package mock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// RegisterJiraEndpoints registers all Jira CRUD endpoints on the mock server.
func RegisterJiraEndpoints(s *Server) {
	registerProjectEndpoints(s)
	registerIssueTypeEndpoints(s)
	registerIssueTypeSchemeEndpoints(s)
	registerWorkflowEndpoints(s)
	registerWorkflowSchemeEndpoints(s)
	registerScreenEndpoints(s)
	registerScreenSchemeEndpoints(s)
	registerScreenTabFieldEndpoints(s)
	registerPermissionSchemeEndpoints(s)
	registerSecuritySchemeEndpoints(s)
	registerNotificationSchemeEndpoints(s)
	registerDashboardEndpoints(s)
	registerFilterEndpoints(s)
	registerCustomFieldEndpoints(s)
	registerBoardEndpoints(s)
	registerPriorityEndpoints(s)
	registerPrioritySchemeEndpoints(s)
	registerAutomationRuleEndpoints(s)
	registerMailHandlerEndpoints(s)
	registerCustomDomainEndpoints(s)
	registerCustomEmailEndpoints(s)
}

// registerCRUDEndpoints is a helper that registers standard CRUD endpoints for a resource type.
// It registers POST (create), GET by id, PUT by id, DELETE by id, and GET list.
func registerCRUDEndpoints(s *Server, storeName, basePath, idField, resourceName string, requiredFields []string, dupField string) {
	store := s.GetStore(storeName)

	// POST — create
	s.RegisterEndpoint("POST "+basePath, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		for _, field := range requiredFields {
			val, _ := req[field].(string)
			if val == "" {
				WriteError(w, http.StatusBadRequest, field+" is required")
				return
			}
		}

		if dupField != "" {
			dupVal, _ := req[dupField].(string)
			for _, item := range store.List() {
				var existing map[string]interface{}
				json.Unmarshal(item, &existing)
				if existing[dupField] == dupVal {
					WriteError(w, http.StatusConflict, fmt.Sprintf("A %s with this %s already exists", resourceName, dupField))
					return
				}
			}
		}

		id := nextID(storeName)
		req[idField] = id
		req["self"] = fmt.Sprintf("%s/%s", basePath, id)
		data, _ := json.Marshal(req)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, req)
	})

	// GET by id — read
	s.RegisterEndpoint("GET "+basePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, resourceName+" not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT by id — update
	s.RegisterEndpoint("PUT "+basePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, resourceName+" not found")
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
			if k != idField {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE by id
	s.RegisterEndpoint("DELETE "+basePath+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, resourceName+" not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET list
	s.RegisterEndpoint("GET "+basePath, func(w http.ResponseWriter, r *http.Request) {
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

// registerProjectEndpoints registers Jira project (space) CRUD endpoints.
func registerProjectEndpoints(s *Server) {
	store := s.GetStore("projects")

	// POST /rest/api/3/project — create project
	s.RegisterEndpoint("POST /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		name, _ := req["name"].(string)
		key, _ := req["key"].(string)
		projectTypeKey, _ := req["projectTypeKey"].(string)
		if name == "" || key == "" || projectTypeKey == "" {
			WriteError(w, http.StatusBadRequest, "name, key, and projectTypeKey are required")
			return
		}

		// Check for duplicate key
		for _, item := range store.List() {
			var existing map[string]interface{}
			json.Unmarshal(item, &existing)
			if existing["key"] == key {
				WriteError(w, http.StatusConflict, "A project with this key already exists")
				return
			}
		}

		id := nextID("project")
		project := map[string]interface{}{
			"id":             id,
			"key":            key,
			"name":           name,
			"projectTypeKey": projectTypeKey,
			"self":           fmt.Sprintf("/rest/api/3/project/%s", id),
		}
		for k, v := range req {
			if _, exists := project[k]; !exists {
				project[k] = v
			}
		}
		data, _ := json.Marshal(project)
		store.Set(id, data)
		// Also index by key for lookup
		store.Set(key, data)
		WriteJSON(w, http.StatusCreated, project)
	})

	// GET /rest/api/3/project/{idOrKey} — read project
	s.RegisterEndpoint("GET /rest/api/3/project/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
		idOrKey := r.PathValue("idOrKey")
		item, ok := store.Get(idOrKey)
		if !ok {
			WriteError(w, http.StatusNotFound, "Project not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /rest/api/3/project/{idOrKey} — update project
	s.RegisterEndpoint("PUT /rest/api/3/project/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
		idOrKey := r.PathValue("idOrKey")
		existing, ok := store.Get(idOrKey)
		if !ok {
			WriteError(w, http.StatusNotFound, "Project not found")
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
		id, _ := current["id"].(string)
		store.Set(id, data)
		newKey, _ := current["key"].(string)
		if oldKey != newKey {
			store.Delete(oldKey)
		}
		store.Set(newKey, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /rest/api/3/project/{idOrKey} — delete project
	s.RegisterEndpoint("DELETE /rest/api/3/project/{idOrKey}", func(w http.ResponseWriter, r *http.Request) {
		idOrKey := r.PathValue("idOrKey")
		item, ok := store.Get(idOrKey)
		if !ok {
			WriteError(w, http.StatusNotFound, "Project not found")
			return
		}
		var project map[string]interface{}
		json.Unmarshal(item, &project)
		id, _ := project["id"].(string)
		key, _ := project["key"].(string)
		store.Delete(id)
		store.Delete(key)
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /rest/api/3/project — list projects
	s.RegisterEndpoint("GET /rest/api/3/project", func(w http.ResponseWriter, r *http.Request) {
		allItems := store.List()
		// Deduplicate: projects are stored by both id and key, so filter to unique IDs
		seen := make(map[string]bool)
		var items []json.RawMessage
		for _, item := range allItems {
			var p map[string]interface{}
			json.Unmarshal(item, &p)
			id, _ := p["id"].(string)
			if !seen[id] {
				seen[id] = true
				items = append(items, item)
			}
		}
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

// registerIssueTypeEndpoints registers issue type CRUD endpoints.
func registerIssueTypeEndpoints(s *Server) {
	registerCRUDEndpoints(s, "issuetypes", "/rest/api/3/issuetype", "id", "Issue type", []string{"name"}, "name")
}

// registerIssueTypeSchemeEndpoints registers issue type scheme CRUD endpoints.
func registerIssueTypeSchemeEndpoints(s *Server) {
	registerCRUDEndpoints(s, "issuetypeschemes", "/rest/api/3/issuetypescheme", "id", "Issue type scheme", []string{"name"}, "name")
}

// registerWorkflowEndpoints registers workflow CRUD endpoints.
func registerWorkflowEndpoints(s *Server) {
	registerCRUDEndpoints(s, "workflows", "/rest/api/3/workflow", "id", "Workflow", []string{"name"}, "name")
}

// registerWorkflowSchemeEndpoints registers workflow scheme CRUD endpoints.
func registerWorkflowSchemeEndpoints(s *Server) {
	registerCRUDEndpoints(s, "workflowschemes", "/rest/api/3/workflowscheme", "id", "Workflow scheme", []string{"name"}, "name")
}

// registerScreenEndpoints registers screen CRUD endpoints.
func registerScreenEndpoints(s *Server) {
	registerCRUDEndpoints(s, "screens", "/rest/api/3/screen", "id", "Screen", []string{"name"}, "name")
}

// registerScreenSchemeEndpoints registers screen scheme CRUD endpoints.
func registerScreenSchemeEndpoints(s *Server) {
	registerCRUDEndpoints(s, "screenschemes", "/rest/api/3/screenscheme", "id", "Screen scheme", []string{"name"}, "name")
}

// registerScreenTabFieldEndpoints registers screen tab field CRUD endpoints.
func registerScreenTabFieldEndpoints(s *Server) {
	registerCRUDEndpoints(s, "screentabfields", "/rest/api/3/screentabfield", "id", "Screen tab field", []string{"fieldId"}, "")
}

// registerPermissionSchemeEndpoints registers permission scheme CRUD endpoints.
func registerPermissionSchemeEndpoints(s *Server) {
	registerCRUDEndpoints(s, "permissionschemes", "/rest/api/3/permissionscheme", "id", "Permission scheme", []string{"name"}, "name")
}

// registerSecuritySchemeEndpoints registers issue security scheme CRUD endpoints.
func registerSecuritySchemeEndpoints(s *Server) {
	registerCRUDEndpoints(s, "securityschemes", "/rest/api/3/issuesecurityschemes", "id", "Security scheme", []string{"name"}, "name")
}

// registerNotificationSchemeEndpoints registers notification scheme CRUD endpoints.
func registerNotificationSchemeEndpoints(s *Server) {
	registerCRUDEndpoints(s, "notificationschemes", "/rest/api/3/notificationscheme", "id", "Notification scheme", []string{"name"}, "name")
}

// registerDashboardEndpoints registers dashboard CRUD endpoints.
func registerDashboardEndpoints(s *Server) {
	registerCRUDEndpoints(s, "dashboards", "/rest/api/3/dashboard", "id", "Dashboard", []string{"name"}, "")
}

// registerFilterEndpoints registers filter CRUD endpoints.
func registerFilterEndpoints(s *Server) {
	registerCRUDEndpoints(s, "filters", "/rest/api/3/filter", "id", "Filter", []string{"name"}, "")
}

// registerCustomFieldEndpoints registers custom field CRUD endpoints.
func registerCustomFieldEndpoints(s *Server) {
	registerCRUDEndpoints(s, "fields", "/rest/api/3/field", "id", "Field", []string{"name", "type"}, "name")
}

// registerBoardEndpoints registers agile board CRUD endpoints.
func registerBoardEndpoints(s *Server) {
	registerCRUDEndpoints(s, "boards", "/rest/agile/1.0/board", "id", "Board", []string{"name", "type"}, "")
}

// registerPriorityEndpoints registers priority CRUD endpoints.
func registerPriorityEndpoints(s *Server) {
	registerCRUDEndpoints(s, "priorities", "/rest/api/3/priority", "id", "Priority", []string{"name"}, "name")
}

// registerPrioritySchemeEndpoints registers priority scheme CRUD endpoints.
func registerPrioritySchemeEndpoints(s *Server) {
	registerCRUDEndpoints(s, "priorityschemes", "/rest/api/3/priorityscheme", "id", "Priority scheme", []string{"name"}, "name")
}

// registerAutomationRuleEndpoints registers automation rule CRUD endpoints.
func registerAutomationRuleEndpoints(s *Server) {
	store := s.GetStore("automationrules")

	// POST /rest/api/3/automation/rule — create automation rule
	s.RegisterEndpoint("POST /rest/api/3/automation/rule", func(w http.ResponseWriter, r *http.Request) {
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

		// Validate trigger
		if _, hasTrigger := req["trigger"]; !hasTrigger {
			WriteError(w, http.StatusBadRequest, "trigger is required")
			return
		}

		id := nextID("automation")
		req["id"] = id
		req["self"] = fmt.Sprintf("/rest/api/3/automation/rule/%s", id)
		if req["enabled"] == nil {
			req["enabled"] = true
		}
		data, _ := json.Marshal(req)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, req)
	})

	// GET /rest/api/3/automation/rule/{id} — read
	s.RegisterEndpoint("GET /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Automation rule not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /rest/api/3/automation/rule/{id} — update
	s.RegisterEndpoint("PUT /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Automation rule not found")
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

	// DELETE /rest/api/3/automation/rule/{id}
	s.RegisterEndpoint("DELETE /rest/api/3/automation/rule/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, "Automation rule not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /rest/api/3/automation/rule — list
	s.RegisterEndpoint("GET /rest/api/3/automation/rule", func(w http.ResponseWriter, r *http.Request) {
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

// registerMailHandlerEndpoints registers incoming and outgoing mail handler CRUD endpoints.
func registerMailHandlerEndpoints(s *Server) {
	registerCRUDEndpoints(s, "mailhandlers_incoming", "/rest/api/3/mailhandler/incoming", "id", "Incoming mail handler", []string{"name"}, "name")
	registerCRUDEndpoints(s, "mailhandlers_outgoing", "/rest/api/3/mailhandler/outgoing", "id", "Outgoing mail handler", []string{"name"}, "name")
}

// registerCustomDomainEndpoints registers custom domain CRUD endpoints with DNS record generation.
func registerCustomDomainEndpoints(s *Server) {
	store := s.GetStore("domains")

	// POST /rest/api/3/domain — create domain with DNS records
	s.RegisterEndpoint("POST /rest/api/3/domain", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		domainName, _ := req["domain"].(string)
		if domainName == "" {
			WriteError(w, http.StatusBadRequest, "domain is required")
			return
		}

		// Check for duplicate domain
		for _, item := range store.List() {
			var existing map[string]interface{}
			json.Unmarshal(item, &existing)
			if existing["domain"] == domainName {
				WriteError(w, http.StatusConflict, "A domain with this name already exists")
				return
			}
		}

		id := nextID("domain")
		// Generate mock DNS records for domain verification
		dnsRecords := []map[string]string{
			{
				"type":  "TXT",
				"name":  domainName,
				"value": fmt.Sprintf("atlassian-domain-verification=%s", id),
			},
			{
				"type":  "CNAME",
				"name":  fmt.Sprintf("_atl-verify.%s", domainName),
				"value": fmt.Sprintf("%s.atlassian-verify.com", id),
			},
			{
				"type":  "MX",
				"name":  domainName,
				"value": fmt.Sprintf("10 mx.%s.atlassian-mail.com", strings.ReplaceAll(domainName, ".", "-")),
			},
			{
				"type":  "TXT",
				"name":  domainName,
				"value": fmt.Sprintf("v=spf1 include:%s.atlassian-spf.com ~all", strings.ReplaceAll(domainName, ".", "-")),
			},
		}

		domain := map[string]interface{}{
			"id":         id,
			"domain":     domainName,
			"status":     "pending",
			"dnsRecords": dnsRecords,
			"self":       fmt.Sprintf("/rest/api/3/domain/%s", id),
		}
		for k, v := range req {
			if _, exists := domain[k]; !exists {
				domain[k] = v
			}
		}

		data, _ := json.Marshal(domain)
		store.Set(id, data)
		WriteJSON(w, http.StatusCreated, domain)
	})

	// GET /rest/api/3/domain/{id} — read domain
	s.RegisterEndpoint("GET /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		item, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Domain not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(item)
	})

	// PUT /rest/api/3/domain/{id} — update domain
	s.RegisterEndpoint("PUT /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		existing, ok := store.Get(id)
		if !ok {
			WriteError(w, http.StatusNotFound, "Domain not found")
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
			if k != "id" && k != "dnsRecords" {
				current[k] = v
			}
		}
		data, _ := json.Marshal(current)
		store.Set(id, data)
		WriteJSON(w, http.StatusOK, current)
	})

	// DELETE /rest/api/3/domain/{id}
	s.RegisterEndpoint("DELETE /rest/api/3/domain/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.Delete(id) {
			WriteError(w, http.StatusNotFound, "Domain not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /rest/api/3/domain — list domains
	s.RegisterEndpoint("GET /rest/api/3/domain", func(w http.ResponseWriter, r *http.Request) {
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

// registerCustomEmailEndpoints registers custom email CRUD endpoints.
func registerCustomEmailEndpoints(s *Server) {
	registerCRUDEndpoints(s, "emails", "/rest/api/3/email", "id", "Email", []string{"emailAddress"}, "emailAddress")
}
