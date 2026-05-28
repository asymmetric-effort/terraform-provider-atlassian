// Package mock provides OpenAPI-driven request validation for the mock
// server. This is a purpose-built lightweight validator that parses
// OpenAPI 3.0 specs (YAML) and validates request bodies against the
// defined schemas. It uses only the Go standard library and
// gopkg.in/yaml.v3 (already a transitive dependency).
package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// OpenAPISpec represents a parsed OpenAPI 3.0 specification.
type OpenAPISpec struct {
	Paths      map[string]map[string]Operation `yaml:"paths"`
	Components struct {
		Schemas map[string]Schema `yaml:"schemas"`
	} `yaml:"components"`
}

// Operation represents an OpenAPI operation (GET, POST, etc.).
type Operation struct {
	OperationID string      `yaml:"operationId"`
	RequestBody RequestBody `yaml:"requestBody"`
}

// RequestBody represents an OpenAPI request body definition.
type RequestBody struct {
	Required bool                       `yaml:"required"`
	Content  map[string]MediaTypeObject `yaml:"content"`
}

// MediaTypeObject represents a media type in OpenAPI.
type MediaTypeObject struct {
	Schema SchemaRef `yaml:"schema"`
}

// SchemaRef is a schema that may be a $ref or inline.
type SchemaRef struct {
	Ref        string            `yaml:"$ref"`
	Type       string            `yaml:"type"`
	Required   []string          `yaml:"required"`
	Properties map[string]Schema `yaml:"properties"`
	Enum       []string          `yaml:"enum"`
}

// Schema represents an OpenAPI schema definition.
type Schema struct {
	Type                 string            `yaml:"type"`
	Required             []string          `yaml:"required"`
	Properties           map[string]Schema `yaml:"properties"`
	Enum                 []string          `yaml:"enum"`
	AdditionalProperties *Schema           `yaml:"additionalProperties"`
	Items                *Schema           `yaml:"items"`
}

// RequestValidator validates incoming HTTP requests against an
// OpenAPI spec.
type RequestValidator struct {
	spec OpenAPISpec
}

// NewRequestValidator creates a validator from an OpenAPI spec file.
func NewRequestValidator(specPath string) (*RequestValidator, error) {
	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read OpenAPI spec %s: %w", specPath, err)
	}
	return NewRequestValidatorFromBytes(data)
}

// NewRequestValidatorFromBytes creates a validator from raw YAML.
func NewRequestValidatorFromBytes(
	data []byte,
) (*RequestValidator, error) {
	var spec OpenAPISpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}
	return &RequestValidator{spec: spec}, nil
}

// ValidateRequest checks an HTTP request against the OpenAPI spec.
// Returns nil if valid or if no matching route exists in the spec.
func (v *RequestValidator) ValidateRequest(
	r *http.Request,
) error {
	method := strings.ToLower(r.Method)
	for pathPattern, methods := range v.spec.Paths {
		if !matchPath(r.URL.Path, pathPattern) {
			continue
		}
		op, ok := methods[method]
		if !ok {
			return nil
		}
		return v.validateOperation(r, op)
	}
	return nil
}

// validateOperation validates a request against a specific operation.
func (v *RequestValidator) validateOperation(
	r *http.Request,
	op Operation,
) error {
	hasBody := r.Body != nil && r.Body != http.NoBody &&
		r.ContentLength != 0

	if op.RequestBody.Required && !hasBody {
		return fmt.Errorf("request body is required for %s",
			op.OperationID)
	}

	if !hasBody {
		return nil
	}

	jsonContent, ok := op.RequestBody.Content["application/json"]
	if !ok {
		return nil
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	var body map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return fmt.Errorf("request body is not valid JSON: %w", err)
	}

	schema := v.resolveSchema(jsonContent.Schema)
	return v.validateAgainstSchema(body, schema, "body")
}

// resolveSchema resolves a $ref to the actual schema definition.
func (v *RequestValidator) resolveSchema(ref SchemaRef) Schema {
	if ref.Ref != "" {
		parts := strings.Split(ref.Ref, "/")
		name := parts[len(parts)-1]
		if s, ok := v.spec.Components.Schemas[name]; ok {
			return s
		}
	}
	return Schema{
		Type:       ref.Type,
		Required:   ref.Required,
		Properties: ref.Properties,
	}
}

// validateAgainstSchema validates a JSON object against a schema.
func (v *RequestValidator) validateAgainstSchema(
	body map[string]interface{},
	schema Schema,
	path string,
) error {
	for _, reqField := range schema.Required {
		if _, ok := body[reqField]; !ok {
			return fmt.Errorf("%s: missing required field %q",
				path, reqField)
		}
	}

	for fieldName, fieldSchema := range schema.Properties {
		val, ok := body[fieldName]
		if !ok {
			continue
		}
		if len(fieldSchema.Enum) > 0 {
			strVal, ok := val.(string)
			if !ok {
				return fmt.Errorf(
					"%s.%s: expected string for enum field",
					path, fieldName)
			}
			found := false
			for _, e := range fieldSchema.Enum {
				if e == strVal {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf(
					"%s.%s: value %q is not one of %v",
					path, fieldName, strVal, fieldSchema.Enum)
			}
		}
	}
	return nil
}

// matchPath checks if a request path matches an OpenAPI path pattern.
// Supports {param} placeholders.
func matchPath(requestPath, pattern string) bool {
	reqParts := strings.Split(
		strings.Trim(requestPath, "/"), "/")
	patParts := strings.Split(strings.Trim(pattern, "/"), "/")

	if len(reqParts) != len(patParts) {
		return false
	}
	for i, pat := range patParts {
		if strings.HasPrefix(pat, "{") && strings.HasSuffix(pat, "}") {
			continue
		}
		if pat != reqParts[i] {
			return false
		}
	}
	return true
}
