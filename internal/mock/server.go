// Package mock implements a mock Atlassian Cloud API server for testing.
//
// The server supports pluggable endpoint registration, in-memory state for
// CRUD operations, and request validation. It grows incrementally per phase.
package mock

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

// Server is the mock Atlassian API server.
type Server struct {
	mux        *http.ServeMux
	mu         sync.RWMutex
	stores     map[string]*Store
	validators []*RequestValidator
}

// AddValidator adds an OpenAPI request validator to the server.
// Requests matching routes in the spec will be validated before
// being passed to the handler.
func (s *Server) AddValidator(v *RequestValidator) {
	s.validators = append(s.validators, v)
}

// Store holds in-memory state for a resource type.
type Store struct {
	mu    sync.RWMutex
	items map[string]json.RawMessage
}

// NewStore creates a new in-memory store.
func NewStore() *Store {
	return &Store{items: make(map[string]json.RawMessage)}
}

// Get retrieves an item by ID.
func (s *Store) Get(id string) (json.RawMessage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	item, ok := s.items[id]
	return item, ok
}

// Set stores an item by ID.
func (s *Store) Set(id string, data json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = data
}

// Delete removes an item by ID.
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[id]
	if ok {
		delete(s.items, id)
	}
	return ok
}

// List returns all items.
func (s *Store) List() []json.RawMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]json.RawMessage, 0, len(s.items))
	for _, v := range s.items {
		result = append(result, v)
	}
	return result
}

// NewServer creates a new mock API server with health check endpoint.
func NewServer() *Server {
	s := &Server{
		mux:    http.NewServeMux(),
		stores: make(map[string]*Store),
	}

	// Health check endpoint
	s.mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	return s
}

// GetStore returns the store for a resource type, creating it if needed.
func (s *Server) GetStore(name string) *Store {
	s.mu.Lock()
	defer s.mu.Unlock()
	if store, ok := s.stores[name]; ok {
		return store
	}
	store := NewStore()
	s.stores[name] = store
	return store
}

// RegisterEndpoint adds a handler to the mock server.
func (s *Server) RegisterEndpoint(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, handler)
}

// Handler returns the HTTP handler for the server.
// If validators have been added, requests are validated against
// OpenAPI specs before reaching the handlers.
func (s *Server) Handler() http.Handler {
	if len(s.validators) == 0 {
		return s.mux
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, v := range s.validators {
			if err := v.ValidateRequest(r); err != nil {
				errStr := err.Error()
				if !strings.Contains(errStr, "no matching route") {
					WriteError(w, http.StatusBadRequest,
						"Request validation failed: "+errStr)
					return
				}
			}
		}
		s.mux.ServeHTTP(w, r)
	})
}

// ListenAndServe starts the mock server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	log.Printf("Mock Atlassian API server listening on %s", addr)
	return http.ListenAndServe(addr, s.Handler())
}

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// WriteError writes an Atlassian-format error response.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]interface{}{
		"errorMessages": []string{message},
		"errors":        map[string]string{},
	})
}

// RequireAuth is middleware that validates authentication headers.
func RequireAuth(next http.HandlerFunc, validTokens map[string]bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		// Accept any Bearer token or Basic auth for mock purposes
		if !validTokens[auth] && len(validTokens) > 0 {
			WriteError(w, http.StatusUnauthorized, "Invalid authentication credentials")
			return
		}

		next(w, r)
	}
}

// ErrorResponse creates an Atlassian-format error response body.
func ErrorResponse(message string) map[string]interface{} {
	return map[string]interface{}{
		"errorMessages": []string{message},
		"errors":        map[string]string{},
	}
}

// Run starts the mock server with default configuration.
func Run(addr string) error {
	s := NewServer()
	RegisterAuthEndpoints(s)
	RegisterIdentityEndpoints(s)
	RegisterJiraEndpoints(s)
	RegisterConfluenceEndpoints(s)
	RegisterBitbucketEndpoints(s)
	RegisterStatuspageEndpoints(s)
	RegisterAdminEndpoints(s)
	return s.ListenAndServe(addr)
}

// RegisterAuthEndpoints is defined in auth_endpoints.go.
// This forward declaration documents the dependency.
var _ = fmt.Sprintf // ensure fmt is used
