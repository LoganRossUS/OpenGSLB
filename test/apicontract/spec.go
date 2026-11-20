// Copyright (C) 2025 Logan Ross
//
// This file is part of OpenGSLB – https://opengslb.org
//
// OpenGSLB is dual-licensed:
//
// 1. GNU Affero General Public License v3.0 (AGPLv3)
//    Free forever for open-source and internal use. You may copy, modify,
//    and distribute this software under the terms of the AGPLv3.
//    → https://www.gnu.org/licenses/agpl-3.0.html
//
// 2. Commercial License
//    Commercial licenses are available for proprietary integration,
//    closed-source appliances, SaaS offerings, and dedicated support.
//    Contact: licensing@opengslb.org
//
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-OpenGSLB-Commercial

// Package apicontract loads api/openapi.yaml and validates real HTTP responses
// against it. It is used by the in-process contract test in cmd/opengslb.
package apicontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const specResource = "https://opengslb.org/api/openapi.json"

// Operation is one method on one path in the spec.
type Operation struct {
	ID     string
	Method string
	Path   string
	// Status is the x-status extension ("" means real).
	Status string
	// responses maps status code -> media type -> compiled schema (nil when the
	// response is documented without a schema).
	responses map[string]map[string]*jsonschema.Schema
}

// Statuses returns the documented status codes, sorted.
func (o *Operation) Statuses() []string {
	out := make([]string, 0, len(o.responses))
	for code := range o.responses {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// Spec is a loaded OpenAPI document.
type Spec struct {
	ops map[string]*Operation
}

// Load reads and compiles the OpenAPI document at path.
func Load(path string) (*Spec, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- test helper, path supplied by the test
	if err != nil {
		return nil, err
	}
	var y any
	if err := yaml.Unmarshal(raw, &y); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Round-trip through JSON so numbers and maps have the types jsonschema expects.
	jb, err := json.Marshal(y)
	if err != nil {
		return nil, fmt.Errorf("convert %s to JSON: %w", path, err)
	}
	docAny, err := jsonschema.UnmarshalJSON(bytes.NewReader(jb))
	if err != nil {
		return nil, err
	}
	doc, ok := docAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: top level is not an object", path)
	}

	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(specResource, doc); err != nil {
		return nil, err
	}

	s := &Spec{ops: map[string]*Operation{}}
	paths, _ := doc["paths"].(map[string]any)
	n := 0
	for p, item := range paths {
		methods, _ := item.(map[string]any)
		for m, opAny := range methods {
			opMap, ok := opAny.(map[string]any)
			if !ok {
				continue
			}
			id, _ := opMap["operationId"].(string)
			if id == "" {
				return nil, fmt.Errorf("%s %s: missing operationId", m, p)
			}
			if _, dup := s.ops[id]; dup {
				return nil, fmt.Errorf("duplicate operationId %q", id)
			}
			op := &Operation{ID: id, Method: strings.ToUpper(m), Path: p, responses: map[string]map[string]*jsonschema.Schema{}}
			op.Status, _ = opMap["x-status"].(string)
			resps, _ := opMap["responses"].(map[string]any)
			for code, rAny := range resps {
				r, err := resolveResponse(doc, rAny)
				if err != nil {
					return nil, fmt.Errorf("%s %s %s: %w", m, p, code, err)
				}
				byType := map[string]*jsonschema.Schema{}
				content, _ := r["content"].(map[string]any)
				for mediaType, mtAny := range content {
					mt, _ := mtAny.(map[string]any)
					schema, has := mt["schema"]
					if !has {
						byType[mediaType] = nil
						continue
					}
					n++
					res := fmt.Sprintf("https://opengslb.org/api/response-%d.json", n)
					if err := c.AddResource(res, absolutize(schema)); err != nil {
						return nil, err
					}
					compiled, err := c.Compile(res)
					if err != nil {
						return nil, fmt.Errorf("%s %s %s %s: %w", m, p, code, mediaType, err)
					}
					byType[mediaType] = compiled
				}
				op.responses[code] = byType
			}
			s.ops[id] = op
		}
	}
	return s, nil
}

// resolveResponse follows a "#/components/responses/X" reference.
func resolveResponse(doc map[string]any, r any) (map[string]any, error) {
	m, ok := r.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("response is not an object")
	}
	ref, ok := m["$ref"].(string)
	if !ok {
		return m, nil
	}
	const prefix = "#/components/responses/"
	if !strings.HasPrefix(ref, prefix) {
		return nil, fmt.Errorf("unsupported response reference %q", ref)
	}
	comps, _ := doc["components"].(map[string]any)
	resps, _ := comps["responses"].(map[string]any)
	target, ok := resps[strings.TrimPrefix(ref, prefix)].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unresolved response reference %q", ref)
	}
	return target, nil
}

// absolutize copies a schema, rewriting document-local references so that they
// resolve against the spec resource rather than the per-response wrapper.
func absolutize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if s, ok := val.(string); ok && k == "$ref" && strings.HasPrefix(s, "#/") {
				out[k] = specResource + s
				continue
			}
			out[k] = absolutize(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = absolutize(t[i])
		}
		return out
	default:
		return v
	}
}

// Operation returns the operation with the given operationId.
func (s *Spec) Operation(id string) (*Operation, bool) {
	op, ok := s.ops[id]
	return op, ok
}

// Operations returns every operation, sorted by operationId.
func (s *Spec) Operations() []*Operation {
	out := make([]*Operation, 0, len(s.ops))
	for _, op := range s.ops {
		out = append(out, op)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ValidateResponse checks that a real response is one the spec documents for
// the operation: the status code is listed, the media type is listed for that
// status, and the body matches the schema.
func (s *Spec) ValidateResponse(opID string, status int, contentType string, body []byte) error {
	op, ok := s.ops[opID]
	if !ok {
		return fmt.Errorf("operation %q is not in the spec", opID)
	}
	code := strconv.Itoa(status)
	byType, ok := op.responses[code]
	if !ok {
		return fmt.Errorf("%s %s: status %d is not documented (documented: %s)", op.Method, op.Path, status, strings.Join(op.Statuses(), ", "))
	}
	if len(byType) == 0 {
		if len(bytes.TrimSpace(body)) != 0 {
			return fmt.Errorf("%s %s %d: spec documents no body, got %q", op.Method, op.Path, status, truncate(body))
		}
		return nil
	}
	mediaType := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	schema, ok := byType[mediaType]
	if !ok {
		return fmt.Errorf("%s %s %d: media type %q is not documented", op.Method, op.Path, status, mediaType)
	}
	if schema == nil {
		return nil
	}
	var inst any
	if mediaType == "application/json" {
		var err error
		inst, err = jsonschema.UnmarshalJSON(bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("%s %s %d: body is not JSON: %w (%q)", op.Method, op.Path, status, err, truncate(body))
		}
	} else {
		inst = string(body)
	}
	if err := schema.Validate(inst); err != nil {
		return fmt.Errorf("%s %s %d: response does not match the spec: %w\nbody: %s", op.Method, op.Path, status, err, truncate(body))
	}
	return nil
}

func truncate(b []byte) string {
	const limit = 600
	if len(b) > limit {
		return string(b[:limit]) + "..."
	}
	return string(b)
}
