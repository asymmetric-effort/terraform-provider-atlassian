// Package mock provides mock Atlassian Admin API endpoints for testing.
package mock

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock/specs"
)

// RegisterAdminEndpoints registers all Atlassian Admin API mock endpoints.
func RegisterAdminEndpoints(s *Server) {
	specData, err := specs.SpecFS.ReadFile("admin.yaml")
	if err != nil {
		log.Printf("WARNING: could not load admin OpenAPI spec: %v", err)
	} else {
		v, err := NewRequestValidatorFromBytes(specData)
		if err != nil {
			log.Printf("WARNING: could not parse admin OpenAPI spec: %v", err)
		} else {
			s.AddValidator(v)
		}
	}

	registerOrganizationEndpoints(s)
	registerWorkspaceEndpoints(s)
	registerProductProvisioningEndpoints(s)
}

// registerOrganizationEndpoints registers organization list and get endpoints.
func registerOrganizationEndpoints(s *Server) {
	store := s.GetStore("organizations")

	// GET /v1/orgs — list organizations
	s.RegisterEndpoint("GET /v1/orgs", func(w http.ResponseWriter, r *http.Request) {
		items := store.List()
		var orgs []json.RawMessage
		for _, item := range items {
			orgs = append(orgs, item)
		}
		if orgs == nil {
			orgs = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": orgs,
		})
	})

	// GET /v1/orgs/{orgId} — get organization by ID
	s.RegisterEndpoint("GET /v1/orgs/{orgId}", func(w http.ResponseWriter, r *http.Request) {
		orgID := r.PathValue("orgId")
		item, ok := store.Get(orgID)
		if !ok {
			WriteError(w, http.StatusNotFound, "Organization not found")
			return
		}
		var org map[string]interface{}
		json.Unmarshal(item, &org)
		WriteJSON(w, http.StatusOK, map[string]interface{}{"data": org})
	})
}

// registerWorkspaceEndpoints registers workspace query endpoints.
func registerWorkspaceEndpoints(s *Server) {
	store := s.GetStore("workspaces")

	// POST /admin/v2/orgs/{orgId}/workspaces — query workspaces
	s.RegisterEndpoint("POST /admin/v2/orgs/{orgId}/workspaces", func(w http.ResponseWriter, r *http.Request) {
		orgID := r.PathValue("orgId")

		// Parse optional query filter
		var reqBody struct {
			Query struct {
				Field struct {
					Name   string   `json:"name"`
					Values []string `json:"values"`
				} `json:"field"`
			} `json:"query"`
		}
		json.NewDecoder(r.Body).Decode(&reqBody)

		var workspaces []json.RawMessage
		for _, item := range store.List() {
			var ws map[string]interface{}
			json.Unmarshal(item, &ws)

			// Filter by orgId
			if ws["orgId"] != orgID {
				continue
			}

			// Filter by name if query provided
			if reqBody.Query.Field.Name == "attributes.name" && len(reqBody.Query.Field.Values) > 0 {
				attrs, _ := ws["attributes"].(map[string]interface{})
				name, _ := attrs["name"].(string)
				matched := false
				for _, v := range reqBody.Query.Field.Values {
					if v == name {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}

			workspaces = append(workspaces, item)
		}
		if workspaces == nil {
			workspaces = []json.RawMessage{}
		}
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": workspaces,
		})
	})
}

// registerProductProvisioningEndpoints registers product provisioning and status endpoints.
func registerProductProvisioningEndpoints(s *Server) {
	provisionStore := s.GetStore("provisions")
	workspaceStore := s.GetStore("workspaces")

	// POST /admin/installations/v2/orgs/{orgId}/products — provision product
	s.RegisterEndpoint("POST /admin/installations/v2/orgs/{orgId}/products", func(w http.ResponseWriter, r *http.Request) {
		orgID := r.PathValue("orgId")

		var req struct {
			Offerings []struct {
				ID       string `json:"id"`
				Location string `json:"location"`
			} `json:"offerings"`
			Parameters struct {
				AdminEmail string `json:"adminEmail"`
				Name       string `json:"name"`
				Timezone   string `json:"timezone"`
			} `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Could not parse request body")
			return
		}

		if req.Parameters.Name == "" {
			WriteError(w, http.StatusBadRequest, "parameters.name is required")
			return
		}
		if len(req.Offerings) == 0 {
			WriteError(w, http.StatusBadRequest, "at least one offering is required")
			return
		}

		// Generate request ID
		requestID := nextID("provision")

		// In the mock, immediately complete provisioning and create a workspace
		wsID := nextID("workspace")
		siteURL := fmt.Sprintf("https://%s.atlassian.net", req.Parameters.Name)
		workspace := map[string]interface{}{
			"id":    wsID,
			"orgId": orgID,
			"attributes": map[string]interface{}{
				"name": req.Parameters.Name,
				"url":  siteURL,
			},
		}
		wsData, _ := json.Marshal(workspace)
		workspaceStore.Set(wsID, wsData)

		// Store provisioning status as COMPLETED
		provision := map[string]interface{}{
			"requestId":   requestID,
			"status":      "COMPLETED",
			"workspaceId": wsID,
			"siteUrl":     siteURL,
		}
		provData, _ := json.Marshal(provision)
		provisionStore.Set(requestID, provData)

		WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"requestId": requestID,
			"statusUrl": fmt.Sprintf("/admin/installations/v2/orgs/%s/products/status/%s", orgID, requestID),
		})
	})

	// GET /admin/installations/v2/orgs/{orgId}/products/status/{requestId} — provisioning status
	s.RegisterEndpoint("GET /admin/installations/v2/orgs/{orgId}/products/status/{requestId}", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.PathValue("requestId")
		item, ok := provisionStore.Get(requestID)
		if !ok {
			WriteError(w, http.StatusNotFound, "Provisioning request not found")
			return
		}
		var provision map[string]interface{}
		json.Unmarshal(item, &provision)
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": provision,
		})
	})
}
