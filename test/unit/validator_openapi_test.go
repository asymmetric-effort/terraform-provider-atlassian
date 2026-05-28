// Package unit tests OpenAPI request validation.
package unit

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

const testSpec = `---
openapi: "3.0.3"
info:
  title: Test API
  version: "0.1.0"
paths:
  /test/resource:
    post:
      operationId: createResource
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateRequest"
      responses:
        "200":
          description: OK
  /test/optional:
    post:
      operationId: optionalBody
      requestBody:
        required: false
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateRequest"
      responses:
        "200":
          description: OK
  /test/noop:
    get:
      operationId: getNoBody
      responses:
        "200":
          description: OK
components:
  schemas:
    CreateRequest:
      type: object
      required:
        - name
        - kind
      properties:
        name:
          type: string
        kind:
          type: string
          enum:
            - typeA
            - typeB
        description:
          type: string
`

// TestValidatorFromBytes verifies spec parsing.
func TestValidatorFromBytes(t *testing.T) {
	t.Parallel()
	v, err := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil validator")
	}
}

// TestValidatorInvalidYAML rejects bad YAML.
func TestValidatorInvalidYAML(t *testing.T) {
	t.Parallel()
	_, err := mock.NewRequestValidatorFromBytes([]byte("{{bad"))
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// TestValidatorValidRequest passes valid request.
func TestValidatorValidRequest(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	body := `{"name":"test","kind":"typeA"}`
	req := httptest.NewRequest(http.MethodPost, "/test/resource",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	err := v.ValidateRequest(req)
	if err != nil {
		t.Fatalf("expected valid request, got: %v", err)
	}
}

// TestValidatorMissingRequiredField rejects missing required field.
func TestValidatorMissingRequiredField(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	body := `{"description":"no name or kind"}`
	req := httptest.NewRequest(http.MethodPost, "/test/resource",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	err := v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected error to mention 'name', got: %s",
			err.Error())
	}
}

// TestValidatorInvalidEnum rejects invalid enum value.
func TestValidatorInvalidEnum(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	body := `{"name":"test","kind":"typeC"}`
	req := httptest.NewRequest(http.MethodPost, "/test/resource",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	err := v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid enum value")
	}
	if !strings.Contains(err.Error(), "typeC") {
		t.Errorf("expected error to mention 'typeC', got: %s",
			err.Error())
	}
}

// TestValidatorInvalidJSON rejects non-JSON body.
func TestValidatorInvalidJSON(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	req := httptest.NewRequest(http.MethodPost, "/test/resource",
		strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 8
	err := v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestValidatorNoMatchingRoute passes unmatched routes.
func TestValidatorNoMatchingRoute(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	req := httptest.NewRequest(http.MethodGet, "/unknown/path", nil)
	err := v.ValidateRequest(req)
	if err != nil {
		t.Fatalf("expected nil for unmatched route, got: %v", err)
	}
}

// TestValidatorGETNoBody passes GET with no body.
func TestValidatorGETNoBody(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	req := httptest.NewRequest(http.MethodGet, "/test/noop", nil)
	err := v.ValidateRequest(req)
	if err != nil {
		t.Fatalf("expected nil for GET, got: %v", err)
	}
}

// TestValidatorOptionalBody passes when body is optional and absent.
func TestValidatorOptionalBodyAbsent(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	req := httptest.NewRequest(http.MethodPost, "/test/optional", nil)
	err := v.ValidateRequest(req)
	if err != nil {
		t.Fatalf("expected nil for optional body, got: %v", err)
	}
}

// TestValidatorIntegrationWithServer verifies the validator works with
// the mock server's AddValidator + Handler chain.
func TestValidatorIntegrationWithServer(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))

	s := mock.NewServer()
	s.AddValidator(v)
	s.RegisterEndpoint("POST /test/resource",
		func(w http.ResponseWriter, r *http.Request) {
			mock.WriteJSON(w, http.StatusOK,
				map[string]string{"status": "created"})
		})

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Valid request
	resp, err := http.Post(ts.URL+"/test/resource",
		"application/json",
		strings.NewReader(`{"name":"x","kind":"typeA"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Invalid request (missing required field)
	resp, err = http.Post(ts.URL+"/test/resource",
		"application/json",
		strings.NewReader(`{"description":"missing name"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400 for invalid request, got %d",
			resp.StatusCode)
	}
}

// TestValidatorFileNotFound returns error for missing file.
func TestValidatorFileNotFound(t *testing.T) {
	t.Parallel()
	_, err := mock.NewRequestValidator("/nonexistent/spec.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestValidatorFromFile loads from actual file.
func TestValidatorFromFile(t *testing.T) {
	t.Parallel()
	specFile := t.TempDir() + "/test.yaml"
	os.WriteFile(specFile, []byte(testSpec), 0644)
	v, err := mock.NewRequestValidator(specFile)
	if err != nil {
		t.Fatalf("failed to load from file: %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil validator")
	}
}

// TestValidatorRequiredBodyNil rejects nil body when required.
func TestValidatorRequiredBodyNil(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	req := httptest.NewRequest(http.MethodPost, "/test/resource", nil)
	req.Header.Set("Content-Type", "application/json")
	err := v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for nil body when required")
	}
}

// TestValidatorEnumNonString rejects non-string for enum field.
func TestValidatorEnumNonString(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	body := `{"name":"test","kind":123}`
	req := httptest.NewRequest(http.MethodPost, "/test/resource",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	err := v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for non-string enum value")
	}
	if !strings.Contains(err.Error(), "enum") {
		t.Errorf("expected 'enum' in error, got: %s", err.Error())
	}
}

// TestValidatorPathLengthMismatch doesn't match wrong-length paths.
func TestValidatorPathLengthMismatch(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	req := httptest.NewRequest(http.MethodPost,
		"/test/resource/extra", nil)
	err := v.ValidateRequest(req)
	if err != nil {
		t.Fatalf("expected nil for non-matching path, got: %v", err)
	}
}

// TestValidatorMethodNotInSpec passes when method not in spec.
func TestValidatorMethodNotInSpec(t *testing.T) {
	t.Parallel()
	v, _ := mock.NewRequestValidatorFromBytes([]byte(testSpec))
	req := httptest.NewRequest(http.MethodDelete,
		"/test/resource", nil)
	err := v.ValidateRequest(req)
	if err != nil {
		t.Fatalf("expected nil for unlisted method, got: %v", err)
	}
}

// TestValidatorNonJSONContentType passes non-JSON content.
func TestValidatorNonJSONContentType(t *testing.T) {
	t.Parallel()

	specNoJSON := `---
openapi: "3.0.3"
info:
  title: Test
  version: "0.1.0"
paths:
  /test/upload:
    post:
      operationId: upload
      requestBody:
        required: true
        content:
          multipart/form-data:
            schema:
              type: object
      responses:
        "200":
          description: OK
`
	v, _ := mock.NewRequestValidatorFromBytes([]byte(specNoJSON))
	req := httptest.NewRequest(http.MethodPost, "/test/upload",
		strings.NewReader("file data"))
	req.Header.Set("Content-Type", "multipart/form-data")
	req.ContentLength = 9
	err := v.ValidateRequest(req)
	if err != nil {
		t.Fatalf("expected nil for non-JSON content, got: %v", err)
	}
}

// TestMatchPathWithParams verifies path matching with parameters.
func TestMatchPathWithParams(t *testing.T) {
	t.Parallel()

	paramSpec := `---
openapi: "3.0.3"
info:
  title: Test
  version: "0.1.0"
paths:
  /api/{id}/sub:
    post:
      operationId: paramOp
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - value
              properties:
                value:
                  type: string
      responses:
        "200":
          description: OK
`
	v, _ := mock.NewRequestValidatorFromBytes([]byte(paramSpec))

	// Should match
	req := httptest.NewRequest(http.MethodPost, "/api/123/sub",
		strings.NewReader(`{"value":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 16
	err := v.ValidateRequest(req)
	if err != nil {
		t.Fatalf("expected match, got: %v", err)
	}

	// Missing required field
	req = httptest.NewRequest(http.MethodPost, "/api/123/sub",
		strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 2
	err = v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
}
