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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Paths used in the generated text. They are relative to the repository root
// (commands) or to docs/ (links).
const (
	specRepoPath    = "api/openapi.yaml"
	specDocLink     = "../api/openapi.yaml"
	guideDocLink    = "api-guide.md"
	regenerateCmd   = "make api-docs"
	regenerateGoCmd = "go generate ./tools/apidocs"
)

// Result is the output of Generate.
type Result struct {
	Markdown   []byte
	Operations int
	Schemas    int
}

// statusInfo describes one value of the x-status operation extension.
type statusInfo struct {
	key   string // value of x-status; empty for real operations
	label string // plain-word name
	text  string // what it means
}

// statuses lists every accepted x-status value, in reporting order.
var statuses = []statusInfo{
	{"", "real", "Backed by live node state."},
	{"stub", "stub", "Returns placeholder, hard-coded or always-empty data."},
	{"not-implemented", "not implemented", "The handler exists but its provider always fails, so no success response is ever returned."},
	{"unavailable", "unavailable", "Registered, but a dependency is never wired by `cmd/opengslb`, so it always answers 503."},
	{"unwired", "unwired", "The handler exists but `cmd/opengslb` never attaches it, so the path answers 404."},
	{"broken", "broken", "Unreachable because of a routing defect."},
}

func lookupStatus(key string) (statusInfo, bool) {
	for _, s := range statuses {
		if s.key == key {
			return s, true
		}
	}
	return statusInfo{}, false
}

// httpMethods are the path item keys that hold operations.
var httpMethods = map[string]bool{
	"get": true, "put": true, "post": true, "delete": true,
	"options": true, "head": true, "patch": true, "trace": true,
}

type operation struct {
	method string // upper case
	path   string
	id     string
	tag    string
	status statusInfo
	node   *yaml.Node
	// pathParams are the parameters declared on the path item.
	pathParams *yaml.Node
}

type generator struct {
	root       *yaml.Node
	schemas    *yaml.Node
	responses  *yaml.Node
	parameters *yaml.Node
	bodies     *yaml.Node
	buf        bytes.Buffer
	// anchors maps each heading anchor in use to the heading that owns it.
	anchors map[string]string
}

// Generate renders the Markdown reference for an OpenAPI 3.1 document.
// The output depends only on the input bytes.
func Generate(spec []byte) (*Result, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || deref(doc.Content[0]).Kind != yaml.MappingNode {
		return nil, errors.New("document root is not a mapping")
	}
	g := &generator{root: deref(doc.Content[0]), anchors: map[string]string{}}
	comps := get(g.root, "components")
	g.schemas = get(comps, "schemas")
	g.responses = get(comps, "responses")
	g.parameters = get(comps, "parameters")
	g.bodies = get(comps, "requestBodies")

	tags, byTag, ops, err := g.collect()
	if err != nil {
		return nil, err
	}
	schemaNames := sortedKeys(g.schemas)
	if err := g.claimAnchors(tags, ops, schemaNames); err != nil {
		return nil, err
	}

	if err := g.writeHeader(ops); err != nil {
		return nil, err
	}
	g.writeContents(tags, byTag, len(schemaNames))
	for _, tag := range tags {
		if err := g.writeTag(tag, byTag[tag.name]); err != nil {
			return nil, err
		}
	}
	if err := g.writeSchemas(schemaNames); err != nil {
		return nil, err
	}

	out := bytes.TrimRight(g.buf.Bytes(), "\n")
	out = append(out, '\n')
	return &Result{Markdown: out, Operations: len(ops), Schemas: len(schemaNames)}, nil
}

type tagInfo struct {
	name        string
	description string
}

// collect returns the tags in rendering order and the operations grouped by
// tag. Tags follow the order of the document's top-level "tags" list; tags
// used by operations but not declared there follow in alphabetical order.
// Operations keep the order of the document.
func (g *generator) collect() ([]tagInfo, map[string][]*operation, []*operation, error) {
	var tags []tagInfo
	seenTag := map[string]bool{}
	for _, t := range seq(get(g.root, "tags")) {
		name := str(get(t, "name"))
		if name == "" || seenTag[name] {
			return nil, nil, nil, fmt.Errorf("tags: empty or duplicate tag name %q", name)
		}
		seenTag[name] = true
		tags = append(tags, tagInfo{name: name, description: str(get(t, "description"))})
	}

	byTag := map[string][]*operation{}
	var all []*operation
	seenID := map[string]string{}
	var undeclared []string
	for _, p := range pairs(get(g.root, "paths")) {
		item := p.value
		for _, m := range pairs(item) {
			if !httpMethods[m.key] {
				continue
			}
			op := &operation{
				method:     strings.ToUpper(m.key),
				path:       p.key,
				id:         str(get(m.value, "operationId")),
				node:       m.value,
				pathParams: get(item, "parameters"),
			}
			where := op.method + " " + op.path
			if op.id == "" {
				return nil, nil, nil, fmt.Errorf("%s: missing operationId", where)
			}
			if prev, dup := seenID[op.id]; dup {
				return nil, nil, nil, fmt.Errorf("%s: operationId %q already used by %s", where, op.id, prev)
			}
			seenID[op.id] = where
			st, ok := lookupStatus(str(get(m.value, "x-status")))
			if !ok {
				return nil, nil, nil, fmt.Errorf("%s: unknown x-status %q", where, str(get(m.value, "x-status")))
			}
			op.status = st
			// An operation is listed once, under its first tag.
			op.tag = "Untagged"
			if ts := seq(get(m.value, "tags")); len(ts) > 0 && str(ts[0]) != "" {
				op.tag = str(ts[0])
			}
			if !seenTag[op.tag] {
				seenTag[op.tag] = true
				undeclared = append(undeclared, op.tag)
			}
			byTag[op.tag] = append(byTag[op.tag], op)
			all = append(all, op)
		}
	}
	sort.Strings(undeclared)
	for _, name := range undeclared {
		tags = append(tags, tagInfo{name: name})
	}
	return tags, byTag, all, nil
}

// Headings that exist in every generated document.
const (
	overviewHeading = "Overview"
	statusHeading   = "Operations by status"
	contentsHeading = "Contents"
	schemasHeading  = "Schemas"
)

func titleHeading(title string) string        { return title + " reference" }
func opHeading(op *operation) string          { return op.method + " " + op.path }
func schemaHeading(name string) string        { return "Schema: " + name }
func headingLink(text, heading string) string { return "[" + text + "](#" + slug(heading) + ")" }

// claimAnchors reserves the anchor of every heading the document will
// contain. Links target heading anchors, which GitHub, MkDocs and Sphinx
// (MyST) all derive from the heading text in the same way for the text
// produced here; two headings with the same anchor would make links
// ambiguous and renderer-dependent, so that is an error.
func (g *generator) claimAnchors(tags []tagInfo, ops []*operation, schemaNames []string) error {
	headings := []string{
		titleHeading(str(get(get(g.root, "info"), "title"))),
		overviewHeading, statusHeading, contentsHeading, schemasHeading,
	}
	for _, t := range tags {
		headings = append(headings, t.name)
	}
	for _, op := range ops {
		headings = append(headings, opHeading(op))
	}
	for _, name := range schemaNames {
		headings = append(headings, schemaHeading(name))
	}
	for _, h := range headings {
		a := slug(h)
		if a == "" {
			return fmt.Errorf("heading %q has an empty anchor", h)
		}
		if prev, taken := g.anchors[a]; taken {
			return fmt.Errorf("headings %q and %q would share the anchor #%s; rename one in the spec", prev, h, a)
		}
		g.anchors[a] = h
	}
	return nil
}

// slug returns the anchor that Markdown renderers derive from a heading:
// lower case, characters other than letters, digits, "-", "_" and spaces
// removed, spaces replaced by "-".
func slug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-', r == '_', unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (g *generator) p(format string, args ...any) {
	fmt.Fprintf(&g.buf, format, args...)
	g.buf.WriteByte('\n')
}

func (g *generator) writeHeader(ops []*operation) error {
	info := get(g.root, "info")
	title := str(get(info, "title"))
	if title == "" {
		return errors.New("info.title is missing")
	}
	g.p("<!-- Code generated by tools/apidocs from %s. DO NOT EDIT. -->", specRepoPath)
	g.p("")
	g.p("# %s", escapeText(titleHeading(title)))
	g.p("")
	g.p("> **This file is generated from [`%s`](%s). Do not edit it by hand.**", specRepoPath, specDocLink)
	g.p("> Change the spec, then run `%s` (or `%s`) and commit the result.", regenerateCmd, regenerateGoCmd)
	g.p("> A test in `tools/apidocs` fails when this file is out of date.")
	g.p(">")
	g.p("> Access control, configuration and usage examples are in the hand-maintained [API guide](%s).", guideDocLink)
	g.p("")
	if v := str(get(info, "version")); v != "" {
		g.p("API version: `%s`", v)
		g.p("")
	}
	for _, s := range seq(get(g.root, "servers")) {
		line := fmt.Sprintf("Server: `%s`", str(get(s, "url")))
		if d := str(get(s, "description")); d != "" {
			line += " - " + oneLine(d)
		}
		g.p("%s", line)
		g.p("")
	}
	if d := strings.TrimSpace(str(get(info, "description"))); d != "" {
		g.p("## %s", overviewHeading)
		g.p("")
		g.p("*This section is the `info.description` of the spec. \"This file\" in it means `%s`.*", specRepoPath)
		g.p("")
		g.p("%s", d)
		g.p("")
	}

	counts := map[string]int{}
	for _, op := range ops {
		counts[op.status.key]++
	}
	g.p("## %s", statusHeading)
	g.p("")
	g.p("| Status | Operations |")
	g.p("|---|---|")
	for _, s := range statuses {
		g.p("| %s | %d |", s.label, counts[s.key])
	}
	g.p("| **Total** | **%d** |", len(ops))
	g.p("")
	return nil
}

func (g *generator) writeContents(tags []tagInfo, byTag map[string][]*operation, schemaCount int) {
	g.p("## %s", contentsHeading)
	g.p("")
	for _, tag := range tags {
		g.p("- %s", headingLink(escapeText(tag.name), tag.name))
		for _, op := range byTag[tag.name] {
			suffix := ""
			if op.status.key != "" {
				suffix = " (" + op.status.label + ")"
			}
			g.p("  - %s%s", headingLink("`"+opHeading(op)+"`", opHeading(op)), suffix)
		}
	}
	g.p("- %s (%d)", headingLink(schemasHeading, schemasHeading), schemaCount)
	g.p("")
}

func (g *generator) writeTag(tag tagInfo, ops []*operation) error {
	g.p("## %s", escapeText(tag.name))
	g.p("")
	if tag.description != "" {
		g.p("%s", strings.TrimSpace(tag.description))
		g.p("")
	}
	for _, op := range ops {
		if err := g.writeOperation(op); err != nil {
			return fmt.Errorf("%s %s: %w", op.method, op.path, err)
		}
	}
	return nil
}

func (g *generator) writeOperation(op *operation) error {
	g.p("### %s", escapeText(opHeading(op)))
	g.p("")
	if s := str(get(op.node, "summary")); s != "" {
		g.p("%s", oneLine(s))
		g.p("")
	}
	g.p("**Status:** %s. %s", op.status.label, op.status.text)
	g.p("")
	g.p("**Operation ID:** `%s`", op.id)
	g.p("")
	if b, ok := boolean(get(op.node, "deprecated")); ok && b {
		g.p("**Deprecated.**")
		g.p("")
	}
	if d := strings.TrimSpace(str(get(op.node, "description"))); d != "" {
		g.p("%s", d)
		g.p("")
	}

	params, err := g.parameterList(op)
	if err != nil {
		return err
	}
	for _, loc := range []struct{ in, title string }{
		{"path", "Path parameters"},
		{"query", "Query parameters"},
		{"header", "Header parameters"},
		{"cookie", "Cookie parameters"},
	} {
		var rows []row
		for _, prm := range params {
			if str(get(prm, "in")) != loc.in {
				continue
			}
			schema := get(prm, "schema")
			typ, err := g.typeString(schema)
			if err != nil {
				return fmt.Errorf("parameter %q: %w", str(get(prm, "name")), err)
			}
			req, _ := boolean(get(prm, "required"))
			rows = append(rows, row{
				name:        str(get(prm, "name")),
				typ:         typ,
				required:    req,
				description: joinNotes(str(get(prm, "description")), constraintNotes(schema)),
			})
		}
		if len(rows) == 0 {
			continue
		}
		g.p("**%s**", loc.title)
		g.p("")
		g.writeRows("Name", rows)
	}

	if rb := get(op.node, "requestBody"); rb != nil {
		rb, err := g.resolve(rb, g.bodies, "#/components/requestBodies/")
		if err != nil {
			return err
		}
		need := "optional"
		if b, _ := boolean(get(rb, "required")); b {
			need = "required"
		}
		g.p("**Request body** (%s)", need)
		g.p("")
		if d := strings.TrimSpace(str(get(rb, "description"))); d != "" {
			g.p("%s", d)
			g.p("")
		}
		content := pairs(get(rb, "content"))
		for _, c := range content {
			schema := get(c.value, "schema")
			typ, err := g.typeString(schema)
			if err != nil {
				return fmt.Errorf("request body: %w", err)
			}
			g.p("- `%s`: %s", c.key, typ)
		}
		if len(content) > 0 {
			g.p("")
		}
		for _, c := range content {
			if err := g.writeInlineFields(fmt.Sprintf("Fields of the `%s` request body", c.key), get(c.value, "schema")); err != nil {
				return fmt.Errorf("request body: %w", err)
			}
		}
	}

	g.p("**Responses**")
	g.p("")
	g.p("| Status | Description | Body |")
	g.p("|---|---|---|")
	type inline struct {
		title  string
		schema *yaml.Node
	}
	var inlines []inline
	responses := pairs(get(op.node, "responses"))
	if len(responses) == 0 {
		return errors.New("no responses documented")
	}
	for _, r := range responses {
		resp, err := g.resolve(r.value, g.responses, "#/components/responses/")
		if err != nil {
			return fmt.Errorf("response %s: %w", r.key, err)
		}
		var bodies []string
		for _, c := range pairs(get(resp, "content")) {
			schema := get(c.value, "schema")
			typ, err := g.typeString(schema)
			if err != nil {
				return fmt.Errorf("response %s: %w", r.key, err)
			}
			bodies = append(bodies, fmt.Sprintf("`%s`: %s", c.key, typ))
			inlines = append(inlines, inline{fmt.Sprintf("Fields of the %s `%s` response body", r.key, c.key), schema})
		}
		body := "No body"
		if len(bodies) > 0 {
			body = strings.Join(bodies, "<br>")
		}
		desc := str(get(resp, "description"))
		if hs := pairs(get(resp, "headers")); len(hs) > 0 {
			names := make([]string, 0, len(hs))
			for _, h := range hs {
				names = append(names, "`"+h.key+"`")
			}
			desc = joinNotes(desc, []string{"Headers: " + strings.Join(names, ", ") + "."})
		}
		g.p("| %s | %s | %s |", cell(r.key), cell(desc), cell(body))
	}
	g.p("")
	for _, in := range inlines {
		if err := g.writeInlineFields(in.title, in.schema); err != nil {
			return err
		}
	}
	return nil
}

// parameterList merges path-item and operation parameters. An operation
// parameter replaces a path-item parameter with the same name and location.
func (g *generator) parameterList(op *operation) ([]*yaml.Node, error) {
	var out []*yaml.Node
	index := map[string]int{}
	for _, list := range []*yaml.Node{op.pathParams, get(op.node, "parameters")} {
		for _, prm := range seq(list) {
			prm, err := g.resolve(prm, g.parameters, "#/components/parameters/")
			if err != nil {
				return nil, err
			}
			key := str(get(prm, "in")) + "\x00" + str(get(prm, "name"))
			if i, ok := index[key]; ok {
				out[i] = prm
				continue
			}
			index[key] = len(out)
			out = append(out, prm)
		}
	}
	return out, nil
}

// resolve follows a $ref into one components section. Nodes without $ref are
// returned unchanged.
func (g *generator) resolve(n, section *yaml.Node, prefix string) (*yaml.Node, error) {
	ref := str(get(n, "$ref"))
	if ref == "" {
		return n, nil
	}
	name, ok := strings.CutPrefix(ref, prefix)
	if !ok {
		return nil, fmt.Errorf("unsupported $ref %q (want %s...)", ref, prefix)
	}
	target := get(section, name)
	if target == nil {
		return nil, fmt.Errorf("unresolved $ref %q", ref)
	}
	if str(get(target, "$ref")) != "" {
		return nil, fmt.Errorf("$ref %q points at another $ref", ref)
	}
	return target, nil
}

func (g *generator) writeSchemas(names []string) error {
	g.p("## %s", schemasHeading)
	g.p("")
	if len(names) == 0 {
		g.p("The spec defines no component schemas.")
		g.p("")
		return nil
	}
	g.p("Component schemas, sorted by name (upper case before lower case). A field marked required is always present;")
	g.p("\"or null\" means the JSON value may be `null`.")
	g.p("")
	for _, name := range names {
		schema := get(g.schemas, name)
		g.p("### %s", escapeText(schemaHeading(name)))
		g.p("")
		if d := strings.TrimSpace(str(get(schema, "description"))); d != "" {
			g.p("%s", d)
			g.p("")
		}
		rows, err := g.fieldRows(schema, "")
		if err != nil {
			return fmt.Errorf("schema %s: %w", name, err)
		}
		if len(rows) == 0 || str(get(schema, "$ref")) != "" {
			typ, err := g.typeString(schema)
			if err != nil {
				return fmt.Errorf("schema %s: %w", name, err)
			}
			g.p("%s", joinNotes("Type: "+typ+".", constraintNotes(schema)))
			g.p("")
		}
		if len(rows) > 0 {
			g.writeRows("Field", rows)
		}
		if note, err := g.additionalNote(schema); err != nil {
			return fmt.Errorf("schema %s: %w", name, err)
		} else if note != "" && len(rows) > 0 {
			g.p("%s", note)
			g.p("")
		}
	}
	return nil
}

// additionalNote describes the additionalProperties keyword of an object that
// also has named properties.
func (g *generator) additionalNote(schema *yaml.Node) (string, error) {
	ap := get(schema, "additionalProperties")
	if ap == nil {
		return "", nil
	}
	if b, ok := boolean(ap); ok {
		if b {
			return "Other fields are allowed.", nil
		}
		return "No other fields are allowed.", nil
	}
	typ, err := g.typeString(ap)
	if err != nil {
		return "", err
	}
	return "Other fields are allowed; their values are " + typ + ".", nil
}

// writeInlineFields prints a field table for a schema that is declared in
// place (not a $ref) and has named properties, directly or as array items or
// map values. Referenced schemas are documented once in the Schemas section.
func (g *generator) writeInlineFields(title string, schema *yaml.Node) error {
	rows, err := g.fieldRows(schema, "")
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	g.p("%s:", title)
	g.p("")
	g.writeRows("Field", rows)
	return nil
}

type row struct {
	name        string
	typ         string
	required    bool
	description string
}

func (g *generator) writeRows(nameHeader string, rows []row) {
	g.p("| %s | Type | Required | Description |", nameHeader)
	g.p("|---|---|---|---|")
	for _, r := range rows {
		req := "no"
		if r.required {
			req = "yes"
		}
		g.p("| `%s` | %s | %s | %s |", cell(r.name), cell(r.typ), req, cell(r.description))
	}
	g.p("")
}

// fieldRows flattens the named properties of an inline schema into table
// rows. Nested inline objects are listed with dotted names ("a.b"), array
// items with "a[].b" and map values with "a.*.b". A $ref is never expanded.
func (g *generator) fieldRows(schema *yaml.Node, name string) ([]row, error) {
	obj, name := unwrapObject(schema, name)
	if obj == nil {
		return nil, nil
	}
	prefix := strings.TrimPrefix(name, ".")
	if prefix != "" {
		prefix += "."
	}
	required := map[string]bool{}
	for _, r := range seq(get(obj, "required")) {
		required[str(r)] = true
	}
	var rows []row
	for _, prop := range pairs(get(obj, "properties")) {
		field := prefix + prop.key
		typ, err := g.typeString(prop.value)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", field, err)
		}
		rows = append(rows, row{
			name:        field,
			typ:         typ,
			required:    required[prop.key],
			description: joinNotes(str(get(prop.value, "description")), constraintNotes(prop.value)),
		})
		nested, err := g.fieldRows(prop.value, field)
		if err != nil {
			return nil, err
		}
		rows = append(rows, nested...)
	}
	return rows, nil
}

// unwrapObject finds the inline schema with named properties that a schema
// describes, looking through arrays ("[]" is appended to name), maps (".*")
// and a oneOf/anyOf whose only non-null alternative is such a schema. It
// returns nil when there is none, in particular for a $ref.
func unwrapObject(schema *yaml.Node, name string) (*yaml.Node, string) {
	for depth := 0; depth < 32; depth++ {
		if schema == nil || deref(schema).Kind != yaml.MappingNode || str(get(schema, "$ref")) != "" {
			return nil, name
		}
		if get(schema, "properties") != nil {
			return schema, name
		}
		if items := get(schema, "items"); items != nil {
			schema, name = items, name+"[]"
			continue
		}
		if ap := get(schema, "additionalProperties"); ap != nil && ap.Kind == yaml.MappingNode {
			schema, name = ap, name+".*"
			continue
		}
		var nonNull []*yaml.Node
		for _, kw := range []string{"oneOf", "anyOf"} {
			for _, alt := range seq(get(schema, kw)) {
				if !isNullSchema(alt) {
					nonNull = append(nonNull, alt)
				}
			}
		}
		if len(nonNull) != 1 {
			return nil, name
		}
		schema = nonNull[0]
	}
	return nil, name
}

// typeString renders the type of a schema as Markdown: a link for a $ref,
// "array of X", "map of string to X", "X or null" and so on.
func (g *generator) typeString(schema *yaml.Node) (string, error) {
	if schema == nil {
		return "any", nil
	}
	if b, ok := boolean(schema); ok {
		// Boolean schemas: true accepts anything, false nothing.
		if b {
			return "any", nil
		}
		return "nothing", nil
	}
	if ref := str(get(schema, "$ref")); ref != "" {
		name, ok := strings.CutPrefix(ref, "#/components/schemas/")
		if !ok {
			return "", fmt.Errorf("unsupported $ref %q (want #/components/schemas/...)", ref)
		}
		if get(g.schemas, name) == nil {
			return "", fmt.Errorf("unresolved $ref %q", ref)
		}
		return headingLink(escapeText(name), schemaHeading(name)), nil
	}

	for _, kw := range []struct{ key, sep string }{{"oneOf", " or "}, {"anyOf", " or "}, {"allOf", " and "}} {
		alts := seq(get(schema, kw.key))
		if len(alts) == 0 {
			continue
		}
		var parts []string
		nullable := false
		for _, alt := range alts {
			if isNullSchema(alt) {
				nullable = true
				continue
			}
			s, err := g.typeString(alt)
			if err != nil {
				return "", err
			}
			parts = append(parts, s)
		}
		return withNull(strings.Join(parts, kw.sep), nullable), nil
	}

	var parts []string
	nullable := false
	for _, t := range typeNames(get(schema, "type")) {
		switch t {
		case "null":
			nullable = true
		case "array":
			items, err := g.typeString(get(schema, "items"))
			if err != nil {
				return "", err
			}
			if strings.Contains(items, " or ") || strings.Contains(items, " and ") {
				items = "(" + items + ")"
			}
			parts = append(parts, "array of "+items)
		case "object":
			ap := get(schema, "additionalProperties")
			if get(schema, "properties") == nil && ap != nil && ap.Kind == yaml.MappingNode {
				values, err := g.typeString(ap)
				if err != nil {
					return "", err
				}
				if strings.Contains(values, " or ") || strings.Contains(values, " and ") {
					values = "(" + values + ")"
				}
				parts = append(parts, "map of string to "+values)
			} else {
				parts = append(parts, "object")
			}
		default:
			if f := str(get(schema, "format")); f != "" {
				t += " (" + f + ")"
			}
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		switch {
		case get(schema, "properties") != nil:
			parts = append(parts, "object")
		case get(schema, "items") != nil:
			items, err := g.typeString(get(schema, "items"))
			if err != nil {
				return "", err
			}
			parts = append(parts, "array of "+items)
		case get(schema, "enum") != nil || get(schema, "const") != nil:
			parts = append(parts, "enum")
		case !nullable:
			parts = append(parts, "any")
		}
	}
	return withNull(strings.Join(parts, " or "), nullable), nil
}

// withNull appends the null alternative to a rendered type. A comma keeps
// container types unambiguous: "array of string, or null" is a nullable
// array, while "array of (string or null)" is an array of nullable strings.
func withNull(typ string, nullable bool) string {
	switch {
	case !nullable:
		return typ
	case typ == "":
		return "null"
	case strings.Contains(typ, " of "):
		return typ + ", or null"
	default:
		return typ + " or null"
	}
}

// isNullSchema reports whether a schema accepts only null.
func isNullSchema(schema *yaml.Node) bool {
	names := typeNames(get(schema, "type"))
	return len(names) == 1 && names[0] == "null"
}

// typeNames returns the "type" keyword as a list; OpenAPI 3.1 allows a
// single name or an array of names such as [array, "null"].
func typeNames(n *yaml.Node) []string {
	n = deref(n)
	if n == nil {
		return nil
	}
	if n.Kind == yaml.ScalarNode {
		return []string{n.Value}
	}
	var out []string
	for _, c := range seq(n) {
		out = append(out, str(c))
	}
	return out
}

// constraintNotes lists the value constraints of a schema as short sentences.
func constraintNotes(schema *yaml.Node) []string {
	if schema == nil || deref(schema).Kind != yaml.MappingNode {
		return nil
	}
	var notes []string
	if enum := seq(get(schema, "enum")); len(enum) > 0 {
		vals := make([]string, 0, len(enum))
		for _, e := range enum {
			vals = append(vals, "`"+literal(e)+"`")
		}
		notes = append(notes, "One of: "+strings.Join(vals, ", ")+".")
	}
	for _, kw := range []struct{ key, label string }{
		{"const", "Always"},
		{"default", "Default:"},
		{"minimum", "Minimum:"},
		{"maximum", "Maximum:"},
		{"exclusiveMinimum", "Greater than"},
		{"exclusiveMaximum", "Less than"},
		{"minLength", "Minimum length:"},
		{"maxLength", "Maximum length:"},
		{"minItems", "Minimum items:"},
		{"maxItems", "Maximum items:"},
		{"pattern", "Pattern:"},
		{"example", "Example:"},
	} {
		if v := get(schema, kw.key); v != nil {
			notes = append(notes, kw.label+" `"+literal(v)+"`.")
		}
	}
	if b, ok := boolean(get(schema, "deprecated")); ok && b {
		notes = append(notes, "Deprecated.")
	}
	if b, ok := boolean(get(schema, "readOnly")); ok && b {
		notes = append(notes, "Read only.")
	}
	if b, ok := boolean(get(schema, "writeOnly")); ok && b {
		notes = append(notes, "Write only.")
	}
	return notes
}

// literal renders a YAML value compactly: scalars as written, collections as
// JSON (map keys sorted by encoding/json).
func literal(n *yaml.Node) string {
	n = deref(n)
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	var v any
	if err := n.Decode(&v); err != nil {
		return "?"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "?"
	}
	return string(b)
}

func joinNotes(description string, notes []string) string {
	parts := make([]string, 0, len(notes)+1)
	if d := strings.TrimSpace(description); d != "" {
		parts = append(parts, d)
	}
	parts = append(parts, notes...)
	return strings.Join(parts, " ")
}

// cell makes text safe for one Markdown table cell: pipes are escaped, line
// breaks become spaces and paragraph breaks become <br><br>.
func cell(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\r\n", "\n")
	paras := strings.Split(s, "\n\n")
	for i, p := range paras {
		paras[i] = strings.Join(strings.Fields(p), " ")
	}
	s = strings.Join(paras, "<br><br>")
	return strings.ReplaceAll(s, "|", `\|`)
}

// oneLine collapses all whitespace runs to single spaces.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// escapeText escapes characters that would start Markdown or HTML markup in
// running text such as headings ("{agent_id}", "<name>").
func escapeText(s string) string {
	return strings.NewReplacer(
		`\`, `\\`, "_", `\_`, "*", `\*`, "<", "&lt;", ">", "&gt;", "`", "\\`", "[", `\[`, "]", `\]`, "|", `\|`,
	).Replace(s)
}

// --- yaml.Node helpers -----------------------------------------------------

func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// get returns the value of key in a mapping node, or nil.
func get(n *yaml.Node, key string) *yaml.Node {
	n = deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return deref(n.Content[i+1])
		}
	}
	return nil
}

type pair struct {
	key   string
	value *yaml.Node
}

// pairs returns the entries of a mapping node in document order.
func pairs(n *yaml.Node) []pair {
	n = deref(n)
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]pair, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		out = append(out, pair{n.Content[i].Value, deref(n.Content[i+1])})
	}
	return out
}

// seq returns the items of a sequence node.
func seq(n *yaml.Node) []*yaml.Node {
	n = deref(n)
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]*yaml.Node, 0, len(n.Content))
	for _, c := range n.Content {
		out = append(out, deref(c))
	}
	return out
}

// str returns the value of a scalar node, or "".
func str(n *yaml.Node) string {
	n = deref(n)
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// boolean returns the value of a boolean scalar node.
func boolean(n *yaml.Node) (value, ok bool) {
	n = deref(n)
	if n == nil || n.Kind != yaml.ScalarNode || n.Tag != "!!bool" {
		return false, false
	}
	return n.Value == "true", true
}

func sortedKeys(n *yaml.Node) []string {
	ps := pairs(n)
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.key)
	}
	sort.Strings(out)
	return out
}
