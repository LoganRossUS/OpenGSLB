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

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Paths relative to this package directory, which is where "go test" runs.
const (
	repoSpec = "../../api/openapi.yaml"
	repoDoc  = "../../docs/api.md"
)

// TestDocsUpToDate fails when docs/api.md is not what the generator produces
// from the current api/openapi.yaml.
func TestDocsUpToDate(t *testing.T) {
	spec, err := os.ReadFile(repoSpec)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	res, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	committed, err := os.ReadFile(repoDoc)
	if err != nil {
		t.Fatalf("read %s: %v\nRun `%s` (or `%s`) from the repository root and commit docs/api.md.",
			filepath.Clean(repoDoc), err, regenerateCmd, regenerateGoCmd)
	}
	if bytes.Equal(committed, res.Markdown) {
		return
	}
	t.Fatalf("docs/api.md is out of date with %s.\n%s\n"+
		"Run `%s` (or `%s`) from the repository root and commit docs/api.md.",
		specRepoPath, firstDifference(committed, res.Markdown), regenerateCmd, regenerateGoCmd)
}

// firstDifference describes the first line at which two texts differ.
func firstDifference(committed, generated []byte) string {
	a := strings.Split(string(committed), "\n")
	b := strings.Split(string(generated), "\n")
	for i := 0; i < len(a) || i < len(b); i++ {
		var la, lb string
		if i < len(a) {
			la = a[i]
		}
		if i < len(b) {
			lb = b[i]
		}
		if la != lb {
			return fmt.Sprintf("First difference at line %d:\n  committed: %s\n  generated: %s", i+1, la, lb)
		}
	}
	return "No line differs (line-ending or trailing-newline difference)."
}

// TestRealSpecCoverage checks that every operation and component schema of
// the real spec is rendered exactly once.
func TestRealSpecCoverage(t *testing.T) {
	spec, err := os.ReadFile(repoSpec)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	res, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	md := string(res.Markdown)

	var doc struct {
		Paths      map[string]map[string]yaml.Node `yaml:"paths"`
		Components struct {
			Schemas map[string]yaml.Node `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	ops := 0
	for path, item := range doc.Paths {
		for method, node := range item {
			if !httpMethods[method] {
				continue
			}
			ops++
			var op struct {
				OperationID string `yaml:"operationId"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s %s: %v", method, path, err)
			}
			heading := "\n### " + strings.ToUpper(method) + " " + escapeText(path) + "\n"
			if n := strings.Count(md, heading); n != 1 {
				t.Errorf("heading for %s %s appears %d times, want 1", method, path, n)
			}
			if !strings.Contains(md, "](#"+slug(strings.ToUpper(method)+" "+path)+")") {
				t.Errorf("operation %s is missing from the table of contents", op.OperationID)
			}
			if !strings.Contains(md, "**Operation ID:** `"+op.OperationID+"`\n") {
				t.Errorf("operation ID %s is not rendered", op.OperationID)
			}
		}
	}
	if res.Operations != ops {
		t.Errorf("Result.Operations = %d, spec has %d", res.Operations, ops)
	}
	if res.Schemas != len(doc.Components.Schemas) {
		t.Errorf("Result.Schemas = %d, spec has %d", res.Schemas, len(doc.Components.Schemas))
	}
	for name := range doc.Components.Schemas {
		heading := "\n### " + escapeText(schemaHeading(name)) + "\n"
		if n := strings.Count(md, heading); n != 1 {
			t.Errorf("heading for schema %s appears %d times, want 1", name, n)
		}
	}

	// Every in-page link must point at the anchor of exactly one heading.
	headings := map[string]int{}
	for _, line := range strings.Split(md, "\n") {
		if text := strings.TrimLeft(line, "#"); text != line && strings.HasPrefix(text, " ") {
			headings[slug(unescapeText(strings.TrimSpace(text)))]++
		}
	}
	for a, n := range headings {
		if n != 1 {
			t.Errorf("anchor #%s belongs to %d headings", a, n)
		}
	}
	links := 0
	for _, part := range strings.Split(md, "](#")[1:] {
		links++
		target := part[:strings.Index(part, ")")]
		if headings[target] != 1 {
			t.Errorf("link to #%s matches %d headings, want 1", target, headings[target])
		}
	}
	if links == 0 {
		t.Error("no in-page links found")
	}

	for i, line := range strings.Split(md, "\n") {
		if strings.TrimRight(line, " \t\r") != line {
			t.Errorf("line %d has trailing whitespace", i+1)
		}
	}
	if !strings.HasSuffix(md, "\n") || strings.HasSuffix(md, "\n\n") {
		t.Error("output must end with exactly one newline")
	}
}

func TestGenerateDeterministic(t *testing.T) {
	spec, err := os.ReadFile(repoSpec)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	first, err := Generate(spec)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for i := 0; i < 5; i++ {
		again, err := Generate(spec)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !bytes.Equal(first.Markdown, again.Markdown) {
			t.Fatalf("run %d produced different output", i+2)
		}
	}
}

const miniSpec = `
openapi: 3.1.0
info:
  title: Mini API
  version: 0.0.1
  description: About the API.
tags:
- name: Second
- name: First
  description: First things.
paths:
  /things/{thing_id}:
    parameters:
    - name: thing_id
      in: path
      required: true
      schema: {type: string}
    get:
      operationId: getThing
      tags: [First]
      summary: Get a thing
      parameters:
      - name: verbose
        in: query
        required: false
        description: More | detail.
        schema: {type: boolean, default: false}
      responses:
        '200':
          description: Success.
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Thing'
        '404':
          $ref: '#/components/responses/NotFound'
    delete:
      operationId: deleteThing
      tags: [Second, First]
      summary: Delete a thing
      x-status: not-implemented
      responses:
        '204':
          description: No content.
  /things:
    post:
      operationId: createThing
      tags: [Other]
      summary: Create a thing
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name: {type: string}
                owner:
                  type: object
                  properties:
                    id: {type: integer, format: int64}
      responses:
        '201':
          description: Created.
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/Thing'
components:
  responses:
    NotFound:
      description: Not found.
      content:
        text/plain:
          schema: {type: string}
  schemas:
    Thing:
      type: object
      description: A thing.
      required: [id]
      additionalProperties: false
      properties:
        id: {type: string}
        kind: {type: string, enum: [a, b]}
        parts:
          type: [array, "null"]
          items:
            $ref: '#/components/schemas/Part'
        labels:
          type: object
          additionalProperties: {type: string}
        parent:
          oneOf:
          - $ref: '#/components/schemas/Thing'
          - type: "null"
    Part:
      type: object
      properties:
        size: {type: number}
    Names:
      type: array
      items: {type: string}
`

func TestGenerateMiniSpec(t *testing.T) {
	res, err := Generate([]byte(miniSpec))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Operations != 3 || res.Schemas != 3 {
		t.Fatalf("got %d operations and %d schemas, want 3 and 3", res.Operations, res.Schemas)
	}
	md := string(res.Markdown)

	// Declared tags keep the spec's order, undeclared tags follow, and an
	// operation is listed once under its first tag.
	order := []string{
		"\n## Second\n", `### DELETE /things/{thing\_id}`,
		"\n## First\n", "First things.", `### GET /things/{thing\_id}`,
		"\n## Other\n", "### POST /things",
		"\n## Schemas\n", "### Schema: Names", "### Schema: Part", "### Schema: Thing",
	}
	pos := 0
	for _, want := range order {
		i := strings.Index(md[pos:], want)
		if i < 0 {
			t.Fatalf("%q not found after offset %d in:\n%s", want, pos, md)
		}
		pos += i + len(want)
	}

	for _, want := range []string{
		"# Mini API reference",
		"Do not edit it by hand",
		"`make api-docs`",
		"About the API.",
		"| not implemented | 1 |",
		"| **Total** | **3** |",
		"  - [`DELETE /things/{thing_id}`](#delete-thingsthing_id) (not implemented)",
		"  - [`GET /things/{thing_id}`](#get-thingsthing_id)\n",
		"**Status:** real. Backed by live node state.",
		"**Status:** not implemented. The handler exists",
		// Path-item parameters are merged into the operation.
		"**Path parameters**\n\n| Name | Type | Required | Description |\n|---|---|---|---|\n| `thing_id` | string | yes |  |",
		"| `verbose` | boolean | no | More \\| detail. Default: `false`. |",
		// $ref responses are resolved.
		"| 404 | Not found. | `text/plain`: string |",
		"| 200 | Success. | `application/json`: [Thing](#schema-thing) |",
		"| 204 | No content. | No body |",
		"| 201 | Created. | `application/json`: array of [Thing](#schema-thing) |",
		// Inline request body.
		"**Request body** (required)\n\n- `application/json`: object\n\nFields of the `application/json` request body:",
		"| `name` | string | yes |  |",
		"| `owner` | object | no |  |",
		"| `owner.id` | integer (int64) | no |  |",
		// Component schemas.
		"A thing.",
		"| `kind` | string | no | One of: `a`, `b`. |",
		"| `parts` | array of [Part](#schema-part), or null | no |  |",
		"| `labels` | map of string to string | no |  |",
		"| `parent` | [Thing](#schema-thing) or null | no |  |",
		"No other fields are allowed.",
		"Type: array of string.",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("output does not contain %q", want)
		}
	}
	if t.Failed() {
		t.Logf("output:\n%s", md)
	}
}

func TestGenerateErrors(t *testing.T) {
	const head = "openapi: 3.1.0\ninfo: {title: T, version: '1'}\n"
	tests := []struct {
		name, spec, want string
	}{
		{"not yaml", "a: [", "parse"},
		{"not a mapping", "- a", "not a mapping"},
		{"no title", "openapi: 3.1.0\npaths: {}\n", "info.title"},
		{"missing operationId", head + "paths:\n  /a:\n    get:\n      responses: {'200': {description: ok}}\n", "missing operationId"},
		{"duplicate operationId", head + "paths:\n  /a:\n    get: {operationId: x, responses: {'200': {description: ok}}}\n" +
			"    put: {operationId: x, responses: {'200': {description: ok}}}\n", "already used by GET /a"},
		{"unknown status", head + "paths:\n  /a:\n    get: {operationId: x, x-status: flaky, responses: {'200': {description: ok}}}\n", `unknown x-status "flaky"`},
		{"no responses", head + "paths:\n  /a:\n    get: {operationId: x}\n", "no responses"},
		{"unresolved schema", head + "paths:\n  /a:\n    get:\n      operationId: x\n      responses:\n        '200':\n          description: ok\n" +
			"          content: {application/json: {schema: {$ref: '#/components/schemas/Nope'}}}\n", `unresolved $ref "#/components/schemas/Nope"`},
		{"unresolved response", head + "paths:\n  /a:\n    get: {operationId: x, responses: {'200': {$ref: '#/components/responses/Nope'}}}\n", `unresolved $ref "#/components/responses/Nope"`},
		{"external ref", head + "paths:\n  /a:\n    get: {operationId: x, responses: {'200': {$ref: 'other.yaml#/x'}}}\n", "unsupported $ref"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Generate([]byte(tt.spec))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

func schemaNode(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return doc.Content[0]
}

func testGenerator(t *testing.T) *generator {
	t.Helper()
	return &generator{schemas: schemaNode(t, "A: {type: object}\nB_c: {type: string}")}
}

func TestTypeString(t *testing.T) {
	g := testGenerator(t)
	tests := []struct{ schema, want string }{
		{`{type: string}`, "string"},
		{`{type: string, format: date-time}`, "string (date-time)"},
		{`{type: integer, format: int64}`, "integer (int64)"},
		{`{type: [string, "null"]}`, "string or null"},
		{`{type: ["null", string]}`, "string or null"},
		{`{type: [string, integer]}`, "string or integer"},
		{`{type: object}`, "object"},
		{`{type: [object, "null"]}`, "object or null"},
		{`{type: object, properties: {a: {type: string}}}`, "object"},
		{`{properties: {a: {type: string}}}`, "object"},
		{`{type: object, additionalProperties: false}`, "object"},
		{`{type: object, additionalProperties: true}`, "object"},
		{`{type: object, additionalProperties: {}}`, "map of string to any"},
		{`{type: object, additionalProperties: {type: integer}}`, "map of string to integer"},
		{`{type: [object, "null"], additionalProperties: {type: integer}}`, "map of string to integer, or null"},
		{`{type: object, additionalProperties: {$ref: '#/components/schemas/A'}}`, "map of string to [A](#schema-a)"},
		{`{type: object, additionalProperties: {type: array, items: {type: string}}}`, "map of string to array of string"},
		{`{type: array, items: {type: string}}`, "array of string"},
		{`{type: array}`, "array of any"},
		{`{type: [array, "null"], items: {type: string}}`, "array of string, or null"},
		{`{type: array, items: {type: [string, "null"]}}`, "array of (string or null)"},
		{`{type: array, items: {$ref: '#/components/schemas/A'}}`, "array of [A](#schema-a)"},
		{`{type: [array, "null"], items: {$ref: '#/components/schemas/A'}}`, "array of [A](#schema-a), or null"},
		{`{type: array, items: {type: array, items: {type: number}}}`, "array of array of number"},
		{`{$ref: '#/components/schemas/A'}`, "[A](#schema-a)"},
		{`{$ref: '#/components/schemas/B_c'}`, `[B\_c](#schema-b_c)`},
		{`{oneOf: [{$ref: '#/components/schemas/A'}, {type: "null"}]}`, "[A](#schema-a) or null"},
		{`{oneOf: [{type: "null"}, {$ref: '#/components/schemas/A'}]}`, "[A](#schema-a) or null"},
		{`{anyOf: [{type: string}, {type: integer}]}`, "string or integer"},
		{`{oneOf: [{type: array, items: {type: string}}, {type: "null"}]}`, "array of string, or null"},
		{`{allOf: [{$ref: '#/components/schemas/A'}, {type: object}]}`, "[A](#schema-a) and object"},
		{`{enum: [a, b]}`, "enum"},
		{`{type: "null"}`, "null"},
		{`{}`, "any"},
		{`true`, "any"},
		{`false`, "nothing"},
	}
	for _, tt := range tests {
		got, err := g.typeString(schemaNode(t, tt.schema))
		if err != nil {
			t.Errorf("typeString(%s): %v", tt.schema, err)
			continue
		}
		if got != tt.want {
			t.Errorf("typeString(%s) = %q, want %q", tt.schema, got, tt.want)
		}
	}
	if got, err := g.typeString(nil); err != nil || got != "any" {
		t.Errorf("typeString(nil) = %q, %v", got, err)
	}
	for _, bad := range []string{
		`{$ref: '#/components/schemas/Missing'}`,
		`{$ref: 'other.yaml#/A'}`,
		`{type: array, items: {$ref: '#/components/schemas/Missing'}}`,
		`{oneOf: [{$ref: '#/components/schemas/Missing'}]}`,
	} {
		if got, err := g.typeString(schemaNode(t, bad)); err == nil {
			t.Errorf("typeString(%s) = %q, want an error", bad, got)
		}
	}
}

func TestFieldRows(t *testing.T) {
	g := testGenerator(t)
	schema := schemaNode(t, `
type: object
required: [id, items]
properties:
  id: {type: string, description: "Identifier."}
  ref: {$ref: '#/components/schemas/A'}
  nested:
    type: object
    required: [x]
    properties:
      x: {type: integer}
      deeper:
        type: object
        properties:
          y: {type: boolean}
  items:
    type: [array, "null"]
    items:
      type: object
      properties:
        n: {type: number}
  by_name:
    type: object
    additionalProperties:
      type: object
      properties:
        v: {type: string}
  maybe:
    oneOf:
    - type: object
      properties:
        z: {type: string}
    - type: "null"
`)
	rows, err := g.fieldRows(schema, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []row{
		{"id", "string", true, "Identifier."},
		{"ref", "[A](#schema-a)", false, ""},
		{"nested", "object", false, ""},
		{"nested.x", "integer", true, ""},
		{"nested.deeper", "object", false, ""},
		{"nested.deeper.y", "boolean", false, ""},
		{"items", "array of object, or null", true, ""},
		{"items[].n", "number", false, ""},
		{"by_name", "map of string to object", false, ""},
		{"by_name.*.v", "string", false, ""},
		{"maybe", "object or null", false, ""},
		{"maybe.z", "string", false, ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}

	// Top-level containers and schemas without named properties.
	for _, tt := range []struct {
		schema string
		names  []string
	}{
		{`{type: array, items: {type: object, properties: {a: {type: string}}}}`, []string{"[].a"}},
		{`{type: object, additionalProperties: {type: object, properties: {a: {type: string}}}}`, []string{"*.a"}},
		{`{$ref: '#/components/schemas/A'}`, nil},
		{`{type: array, items: {$ref: '#/components/schemas/A'}}`, nil},
		{`{type: string}`, nil},
		{`{type: object, additionalProperties: false}`, nil},
	} {
		rows, err := g.fieldRows(schemaNode(t, tt.schema), "")
		if err != nil {
			t.Errorf("fieldRows(%s): %v", tt.schema, err)
			continue
		}
		var names []string
		for _, r := range rows {
			names = append(names, r.name)
		}
		if strings.Join(names, ",") != strings.Join(tt.names, ",") {
			t.Errorf("fieldRows(%s) names = %v, want %v", tt.schema, names, tt.names)
		}
	}

	if _, err := g.fieldRows(schemaNode(t, `{type: object, properties: {a: {$ref: '#/components/schemas/Missing'}}}`), ""); err == nil {
		t.Error("expected an error for an unresolved field $ref")
	}
}

func TestConstraintNotes(t *testing.T) {
	tests := []struct{ schema, want string }{
		{`{type: string}`, ""},
		{`{type: string, enum: [a, b]}`, "One of: `a`, `b`."},
		{`{type: integer, default: 30, minimum: 1, maximum: 60}`, "Default: `30`. Minimum: `1`. Maximum: `60`."},
		{`{type: object, default: {b: 1, a: [x]}}`, "Default: `{\"a\":[\"x\"],\"b\":1}`."},
		{`{type: string, deprecated: true}`, "Deprecated."},
		{`{type: string, deprecated: false}`, ""},
		{`{type: string, const: v1, example: v1}`, "Always `v1`. Example: `v1`."},
	}
	for _, tt := range tests {
		got := strings.Join(constraintNotes(schemaNode(t, tt.schema)), " ")
		if got != tt.want {
			t.Errorf("constraintNotes(%s) = %q, want %q", tt.schema, got, tt.want)
		}
	}
}

func TestCell(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"plain", "plain"},
		{"a | b", `a \| b`},
		{"wrapped\n  line", "wrapped line"},
		{"  para one\n\npara\ntwo \n", "para one<br><br>para two"},
		{"crlf\r\n\r\nnext", "crlf<br><br>next"},
	}
	for _, tt := range tests {
		if got := cell(tt.in); got != tt.want {
			t.Errorf("cell(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// unescapeText reverses escapeText, giving the heading text as rendered.
func unescapeText(s string) string {
	return strings.NewReplacer(
		`\\`, `\`, `\_`, "_", `\*`, "*", "&lt;", "<", "&gt;", ">", "\\`", "`", `\[`, "[", `\]`, "]", `\|`, "|",
	).Replace(s)
}

func TestEscapeText(t *testing.T) {
	const in = "/a/{agent_id}/<x>*[y]|`z`\\"
	const want = `/a/{agent\_id}/&lt;x&gt;\*\[y\]\|` + "\\`z\\`" + `\\`
	got := escapeText(in)
	if got != want {
		t.Errorf("escapeText = %q, want %q", got, want)
	}
	if back := unescapeText(got); back != in {
		t.Errorf("unescapeText(escapeText(x)) = %q, want %q", back, in)
	}
}

func TestSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Schemas", "schemas"},
		{"Operations by status", "operations-by-status"},
		{"GET /api/v1/domains/{name}", "get-apiv1domainsname"},
		{"POST /api/v1/overwatch/agents/{agent_id}/revoke", "post-apiv1overwatchagentsagent_idrevoke"},
		{"PUT /api/v1/servers/{id}/health-check", "put-apiv1serversidhealth-check"},
		{"Schema: DNSSECStatusResponse", "schema-dnssecstatusresponse"},
		{"OpenGSLB Overwatch API reference", "opengslb-overwatch-api-reference"},
	}
	for _, tt := range tests {
		if got := slug(tt.in); got != tt.want {
			t.Errorf("slug(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAnchorCollisions(t *testing.T) {
	const head = "openapi: 3.1.0\ninfo: {title: T, version: '1'}\n"
	const ok = "{description: ok}"
	tests := []struct{ name, spec, want string }{
		{"paths differing only in punctuation",
			head + "paths:\n  /a/{b}:\n    get: {operationId: x, responses: {'200': " + ok + "}}\n" +
				"  /a/b:\n    get: {operationId: y, responses: {'200': " + ok + "}}\n",
			`"GET /a/{b}" and "GET /a/b" would share the anchor #get-ab`},
		{"schemas differing only in case",
			head + "paths: {}\ncomponents:\n  schemas:\n    Foo: {type: string}\n    FOO: {type: string}\n",
			"would share the anchor #schema-foo"},
		{"tag named like a fixed heading",
			head + "tags:\n- name: Schemas\npaths: {}\n",
			"would share the anchor #schemas"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Generate([]byte(tt.spec))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

func TestFirstDifference(t *testing.T) {
	got := firstDifference([]byte("a\nb\nc\n"), []byte("a\nB\nc\n"))
	if !strings.Contains(got, "line 2") || !strings.Contains(got, "committed: b") || !strings.Contains(got, "generated: B") {
		t.Errorf("unexpected description: %s", got)
	}
	if got := firstDifference([]byte("a\n"), []byte("a\nb\n")); !strings.Contains(got, "line 2") {
		t.Errorf("unexpected description for a longer text: %s", got)
	}
}
