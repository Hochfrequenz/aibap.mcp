package tools_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// This file contains the schema validation tests that protect against the
// "no bad surprise for the clients" class of bug. The validations are split
// into two layers:
//
//  1. TestToolSchemasAreValidJSONSchema compiles every tool's input schema
//     against the JSON Schema 2020-12 meta-schema using the
//     santhosh-tekuri/jsonschema library. This catches *structural* mistakes
//     (wrong type names, malformed enums, draft-04/06/07 leftovers like
//     boolean exclusiveMaximum, dependencies-vs-dependentRequired, etc.).
//     The library walks the entire schema, so blind spots in our previous
//     hand-rolled walker (additionalProperties as schema, $defs,
//     patternProperties, propertyNames, prefixItems, if/then/else, not, …)
//     are now covered for free.
//
//  2. TestToolSchemasMatchOpenCodeProfile walks the compiled *Schema and
//     enforces the rules that real LLM clients reject when forwarded a
//     schema by opencode. Specifically, the rules below mirror what the
//     OpenAI Chat Completions tool-call endpoint refuses with HTTP 400.
//     opencode (sst/opencode) does no local validation — it spreads the raw
//     inputSchema, sets type:object + additionalProperties:false, and hands
//     it to the Vercel AI SDK which forwards verbatim to the model — so
//     "what the model rejects" is what we have to enforce ourselves.
//
//     Tier-1 rules (this PR, opencode → OpenAI):
//       - Every `type: array` MUST have an `items` schema (issue #261).
//       - `items` MUST be a single schema, not a tuple `[]*Schema` form.
//       - `prefixItems` MUST NOT be set (OpenAI rejects with the same
//         "array schema missing items" error even though it is valid
//         JSON Schema 2020-12).
//
//     Tier-2 rules (NOT enforced yet, see follow-up): the Anthropic
//     Messages API (still through opencode) additionally rejects validation
//     keywords like minLength/maxLength/pattern/format/minimum/maximum/
//     minItems/maxItems/uniqueItems when forwarded raw by the AI SDK
//     (vercel/ai#13355). Our own patch.go schema currently uses `minimum`,
//     so enforcing Tier-2 here would self-fail. That work belongs in a
//     separate issue/PR.

// compileToolSchemas turns each tool's inputSchema map into a compiled
// *jsonschema.Schema. The compile step is itself a validation: it fails the
// test on any structural problem the JSON Schema 2020-12 meta-schema would
// reject.
func compileToolSchemas(t *testing.T) map[string]*jsonschema.Schema {
	t.Helper()
	rawSchemas := listToolInputSchemas(t)
	if len(rawSchemas) == 0 {
		t.Fatal("no tools returned from tools/list")
	}

	out := make(map[string]*jsonschema.Schema, len(rawSchemas))
	names := sortedKeys(rawSchemas)
	for _, name := range names {
		c := jsonschema.NewCompiler()
		// Pin the draft to 2020-12. mcp-go does not emit `$schema` so the
		// compiler would auto-default — being explicit avoids surprise if
		// that default ever changes.
		c.DefaultDraft(jsonschema.Draft2020)
		// AssertVocabs makes the compiler enforce vocabulary keywords
		// strictly, so e.g. an unknown keyword is reported instead of
		// silently ignored.
		c.AssertVocabs()
		// AssertFormat makes the compiler populate s.Format on parsed
		// schemas. Without this, the `format` keyword is treated as
		// annotation-only and discarded — which would silently break the
		// Tier-2 check for `format` (Anthropic rejects it). The cost of
		// enabling assertion is just that unknown format names become
		// errors at compile time, which is fine for our test schemas.
		c.AssertFormat()

		// The library wants resources keyed by URL. Synthesize one.
		resourceURL := fmt.Sprintf("mem://tool/%s.json", name)
		// AddResource expects an `any` already-decoded JSON document; the
		// inputSchema map from listToolInputSchemas is exactly that.
		if err := c.AddResource(resourceURL, rawSchemas[name]); err != nil {
			t.Errorf("tool %q: AddResource failed: %v", name, err)
			continue
		}
		schema, err := c.Compile(resourceURL)
		if err != nil {
			t.Errorf("tool %q: schema does not compile against JSON Schema 2020-12: %v", name, err)
			continue
		}
		out[name] = schema
	}
	return out
}

// TestToolSchemasAreValidJSONSchema is the structural validation layer.
// Compiling each schema against the JSON Schema 2020-12 meta-schema is
// enough to flunk any tool whose schema is structurally wrong; the library
// produces detailed errors with JSON pointers, so failures point at the
// exact bad keyword.
func TestToolSchemasAreValidJSONSchema(t *testing.T) {
	// compileToolSchemas itself reports per-tool failures via t.Errorf.
	// We just need to call it.
	_ = compileToolSchemas(t)
}

// schemaProblem is a single rule violation found by the walker.
type schemaProblem struct {
	path    string // JSON pointer-like path into the schema
	message string
}

func (p schemaProblem) String() string {
	if p.path == "" {
		return p.message
	}
	return fmt.Sprintf("%s: %s", p.path, p.message)
}

// walkSchemaForOpenCodeProfile recursively walks a compiled *Schema and
// collects rule violations under the opencode profile. It uses the
// library's already-parsed AST so every JSON Schema 2020-12 keyword that
// can carry a subschema is visited (no manual keyword list to keep in
// sync with the spec).
//
// The opencode profile combines:
//
//   - Tier-1 (opencode → OpenAI): array-items requirements enforced by
//     checkArrayItemsRule. Original #261 bug class.
//   - Tier-2 (opencode → Anthropic via Vercel AI SDK): rejected validation
//     keywords enforced by checkAnthropicRejectedKeywordsRule. See
//     vercel/ai#13355 for the rejection list.
//
// Both rules apply at every node, so adding a third profile (e.g. OpenAI
// strict mode) would just be another check called from this function.
func walkSchemaForOpenCodeProfile(s *jsonschema.Schema, path string, seen map[*jsonschema.Schema]bool) []schemaProblem {
	if s == nil || seen[s] {
		return nil
	}
	seen[s] = true

	problems := checkArrayItemsRule(s, path)
	problems = append(problems, checkAnthropicRejectedKeywordsRule(s, path)...)
	problems = append(problems, descendSubschemas(s, path, seen)...)
	return problems
}

// checkArrayItemsRule enforces "type:array MUST have items" plus the
// related Tier-1 rejection rules (no tuple-form items, no prefixItems).
//
// The library splits the `items` keyword across two struct fields
// depending on the draft of the schema being compiled:
//
//   - Draft 2020-12 and later: Items2020 *Schema holds the single items
//     schema; tuple-form items moved to PrefixItems []*Schema.
//   - Draft 2019-09 and earlier: Items any holds either *Schema or
//     []*Schema (the legacy tuple form).
//
// mcp-go does not set $schema on tool input schemas, so the compiler
// parses them as 2020-12 (the configured default) — but checking both
// fields keeps this walker correct if a future contributor pins an older
// draft via $schema.
func checkArrayItemsRule(s *jsonschema.Schema, path string) []schemaProblem {
	if s.Types == nil || !containsType(s.Types, "array") {
		return nil
	}
	var problems []schemaProblem
	hasItems := false
	switch items := s.Items.(type) {
	case nil:
		// fall through; check Items2020 below
	case *jsonschema.Schema:
		hasItems = true
	case []*jsonschema.Schema:
		problems = append(problems, schemaProblem{path, fmt.Sprintf("type:array uses tuple-form items ([]Schema, %d entries) — OpenAI rejects with 'array schema missing items'", len(items))})
		hasItems = true // tuple form is "present", just wrong; don't double-report
	}
	if s.Items2020 != nil {
		hasItems = true
	}
	if len(s.PrefixItems) > 0 {
		problems = append(problems, schemaProblem{path, fmt.Sprintf("type:array uses prefixItems (%d entries) — OpenAI rejects with 'array schema missing items'", len(s.PrefixItems))})
		// prefixItems alone does NOT count as having items for the
		// purposes of the OpenAI check.
	}
	if !hasItems {
		problems = append(problems, schemaProblem{path, "type:array missing items (OpenAI rejects with 'array schema missing items')"})
	}
	return problems
}

// checkAnthropicRejectedKeywordsRule enforces the Tier-2 rule: certain
// JSON Schema *validation* keywords are rejected by the Anthropic Messages
// API when forwarded raw by the Vercel AI SDK (see vercel/ai#13355).
// Practical consequence: a schema that works fine through opencode +
// OpenAI today will 400 through opencode + Anthropic if it sets any of
// these keywords. The runtime handler is the right place to enforce
// numeric/string constraints anyway — the schema only needs to describe
// the shape, not validate the values.
//
// The library exposes each rejected keyword as a typed field on *Schema,
// so we check struct presence rather than scanning the raw map. Adding a
// new spec draft would surface as a new field, not as a silently-skipped
// map key — the same correctness story as the descend* helpers.
func checkAnthropicRejectedKeywordsRule(s *jsonschema.Schema, path string) []schemaProblem {
	var problems []schemaProblem
	report := func(keyword string) {
		problems = append(problems, schemaProblem{path, fmt.Sprintf("uses %q — Anthropic Messages API rejects this validation keyword when forwarded by @ai-sdk/anthropic (vercel/ai#13355). Move the constraint to the description string and let the runtime handler enforce it.", keyword)})
	}
	// number
	if s.Minimum != nil {
		report("minimum")
	}
	if s.Maximum != nil {
		report("maximum")
	}
	if s.ExclusiveMinimum != nil {
		report("exclusiveMinimum")
	}
	if s.ExclusiveMaximum != nil {
		report("exclusiveMaximum")
	}
	if s.MultipleOf != nil {
		report("multipleOf")
	}
	// string
	if s.MinLength != nil {
		report("minLength")
	}
	if s.MaxLength != nil {
		report("maxLength")
	}
	if s.Pattern != nil {
		report("pattern")
	}
	if s.Format != nil {
		report("format")
	}
	// array
	if s.MinItems != nil {
		report("minItems")
	}
	if s.MaxItems != nil {
		report("maxItems")
	}
	if s.UniqueItems {
		// Note: UniqueItems is bool (not *bool), so we cannot distinguish
		// "explicitly set to false" from "not set" — but `uniqueItems: false`
		// is the default and effectively imposes no constraint, so we only
		// flag the truthy case.
		report("uniqueItems")
	}
	// object
	if s.MinProperties != nil {
		report("minProperties")
	}
	if s.MaxProperties != nil {
		report("maxProperties")
	}
	return problems
}

// descendSubschemas walks every keyword on s that can carry a subschema
// and recursively applies walkSchemaForOpenCodeProfile to each. The
// library exposes each subschema-bearing keyword as a typed field, so
// adding a new spec construct surfaces as a new *Schema field rather
// than a silently-skipped map key — much more robust than the old
// hand-rolled walker that hard-coded a small list of strings.
func descendSubschemas(s *jsonschema.Schema, path string, seen map[*jsonschema.Schema]bool) []schemaProblem {
	var problems []schemaProblem
	problems = append(problems, descendObjectSubschemas(s, path, seen)...)
	problems = append(problems, descendArraySubschemas(s, path, seen)...)
	problems = append(problems, descendApplicatorSubschemas(s, path, seen)...)
	problems = append(problems, descendRefs(s, path, seen)...)
	return problems
}

// descendObjectSubschemas recurses into the object-flavored keywords. Map
// iterations are sorted so the per-tool error output stays deterministic
// across runs (Go map iteration order is randomized).
func descendObjectSubschemas(s *jsonschema.Schema, path string, seen map[*jsonschema.Schema]bool) []schemaProblem {
	var problems []schemaProblem
	for _, name := range sortedKeys(s.Properties) {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.Properties[name], path+"/properties/"+name, seen)...)
	}
	for _, pattern := range sortedPatternKeys(s.PatternProperties) {
		// Look up by re-walking — sortedPatternKeys returns string keys we
		// match against re.String() because Regexp is not directly indexable.
		for re, sub := range s.PatternProperties {
			if re.String() == pattern {
				problems = append(problems, walkSchemaForOpenCodeProfile(sub, fmt.Sprintf("%s/patternProperties/%s", path, pattern), seen)...)
				break
			}
		}
	}
	if sub, ok := s.AdditionalProperties.(*jsonschema.Schema); ok {
		problems = append(problems, walkSchemaForOpenCodeProfile(sub, path+"/additionalProperties", seen)...)
	}
	if s.PropertyNames != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.PropertyNames, path+"/propertyNames", seen)...)
	}
	for _, name := range sortedKeys(s.DependentSchemas) {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.DependentSchemas[name], path+"/dependentSchemas/"+name, seen)...)
	}
	// Dependencies is the legacy draft-4..7 form of dependentSchemas. The
	// values are either []string (dependentRequired) or *Schema (the
	// schema-form). Walk the schema-form values only; the []string form is
	// not a subschema.
	for _, name := range sortedKeys(s.Dependencies) {
		if sub, ok := s.Dependencies[name].(*jsonschema.Schema); ok {
			problems = append(problems, walkSchemaForOpenCodeProfile(sub, path+"/dependencies/"+name, seen)...)
		}
	}
	if s.UnevaluatedProperties != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.UnevaluatedProperties, path+"/unevaluatedProperties", seen)...)
	}
	return problems
}

// descendArraySubschemas recurses into the array-flavored keywords. Items
// has both legacy (any) and 2020-12 (Items2020 *Schema) representations;
// AdditionalItems is the pre-2020 trailing-element schema; UnevaluatedItems
// is the 2020-12 unevaluated-array-element schema.
func descendArraySubschemas(s *jsonschema.Schema, path string, seen map[*jsonschema.Schema]bool) []schemaProblem {
	var problems []schemaProblem
	switch items := s.Items.(type) {
	case *jsonschema.Schema:
		problems = append(problems, walkSchemaForOpenCodeProfile(items, path+"/items", seen)...)
	case []*jsonschema.Schema:
		for i, sub := range items {
			problems = append(problems, walkSchemaForOpenCodeProfile(sub, fmt.Sprintf("%s/items/%d", path, i), seen)...)
		}
	}
	if s.Items2020 != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.Items2020, path+"/items", seen)...)
	}
	if sub, ok := s.AdditionalItems.(*jsonschema.Schema); ok {
		problems = append(problems, walkSchemaForOpenCodeProfile(sub, path+"/additionalItems", seen)...)
	}
	if s.UnevaluatedItems != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.UnevaluatedItems, path+"/unevaluatedItems", seen)...)
	}
	for i, sub := range s.PrefixItems {
		problems = append(problems, walkSchemaForOpenCodeProfile(sub, fmt.Sprintf("%s/prefixItems/%d", path, i), seen)...)
	}
	if s.Contains != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.Contains, path+"/contains", seen)...)
	}
	return problems
}

// descendApplicatorSubschemas recurses into the boolean and conditional
// applicator keywords (allOf/anyOf/oneOf/not, if/then/else).
func descendApplicatorSubschemas(s *jsonschema.Schema, path string, seen map[*jsonschema.Schema]bool) []schemaProblem {
	var problems []schemaProblem
	for i, sub := range s.AllOf {
		problems = append(problems, walkSchemaForOpenCodeProfile(sub, fmt.Sprintf("%s/allOf/%d", path, i), seen)...)
	}
	for i, sub := range s.AnyOf {
		problems = append(problems, walkSchemaForOpenCodeProfile(sub, fmt.Sprintf("%s/anyOf/%d", path, i), seen)...)
	}
	for i, sub := range s.OneOf {
		problems = append(problems, walkSchemaForOpenCodeProfile(sub, fmt.Sprintf("%s/oneOf/%d", path, i), seen)...)
	}
	if s.Not != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.Not, path+"/not", seen)...)
	}
	if s.If != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.If, path+"/if", seen)...)
	}
	if s.Then != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.Then, path+"/then", seen)...)
	}
	if s.Else != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.Else, path+"/else", seen)...)
	}
	return problems
}

// descendRefs follows compiled $ref / $dynamicRef pointers. The library
// inlines them eagerly during compile, so the ref target is itself a
// *Schema we can walk. The `seen` map in the caller guards against $ref
// cycles.
func descendRefs(s *jsonschema.Schema, path string, seen map[*jsonschema.Schema]bool) []schemaProblem {
	var problems []schemaProblem
	if s.Ref != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.Ref, path+"/$ref", seen)...)
	}
	if s.DynamicRef != nil && s.DynamicRef.Ref != nil {
		problems = append(problems, walkSchemaForOpenCodeProfile(s.DynamicRef.Ref, path+"/$dynamicRef", seen)...)
	}
	return problems
}

// TestToolSchemasMatchOpenCodeProfile is the client-compatibility validation
// layer. It walks every compiled tool input schema and fails if any node
// violates a Tier-1 rule that opencode-via-OpenAI would reject. This is the
// regression-proof replacement for the previous hand-rolled
// findArraysMissingItems walker.
func TestToolSchemasMatchOpenCodeProfile(t *testing.T) {
	schemas := compileToolSchemas(t)
	if t.Failed() {
		// compileToolSchemas already reported a structural failure; running
		// the profile walker on a partially-compiled set would be noise.
		return
	}

	for _, name := range sortedKeys(schemas) {
		problems := walkSchemaForOpenCodeProfile(schemas[name], "", map[*jsonschema.Schema]bool{})
		if len(problems) > 0 {
			lines := make([]string, len(problems))
			for i, p := range problems {
				lines[i] = "  - " + p.String()
			}
			t.Errorf("tool %q violates the opencode profile:\n%s", name, strings.Join(lines, "\n"))
		}
	}
}

// sortedKeys returns the keys of m sorted ascending. Tiny generic helper
// used to make per-tool error output deterministic.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedPatternKeys returns the regex source strings of a PatternProperties
// map sorted ascending. PatternProperties is keyed by jsonschema.Regexp
// (an interface, not a string), so the generic sortedKeys helper does not
// apply directly.
func sortedPatternKeys(m map[jsonschema.Regexp]*jsonschema.Schema) []string {
	keys := make([]string, 0, len(m))
	for re := range m {
		keys = append(keys, re.String())
	}
	sort.Strings(keys)
	return keys
}

// containsType reports whether tt declares the JSON Schema type name `want`
// (e.g. "array", "object"). Handles the union form `type: ["array", "null"]`
// by checking every entry in tt.ToStrings().
func containsType(tt *jsonschema.Types, want string) bool {
	if tt == nil {
		return false
	}
	for _, ty := range tt.ToStrings() {
		if ty == want {
			return true
		}
	}
	return false
}

// compileFixture compiles a literal schema fixture for the walker self-tests.
// Mirrors compileToolSchemas (same draft, same vocab/format assertion) so
// the fixtures see the same parser state as real tool schemas.
func compileFixture(t *testing.T, raw map[string]any) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertVocabs()
	c.AssertFormat()
	if err := c.AddResource("mem://fixture.json", raw); err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	s, err := c.Compile("mem://fixture.json")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return s
}

// TestWalkerCatchesBlindSpots is a self-test for walkSchemaForOpenCodeProfile.
// Each subtest constructs a schema that exercises one of the JSON Schema
// keywords the previous hand-rolled walker missed (per the retroactive
// review of #262), and asserts the new library-backed walker reports a
// violation. The error messages are checked for substring matches so we
// know the walker reaches the right code path, not just any failure.
//
// Adding a new keyword to the spec? Add a fixture here and the walker will
// be exercised against it before any real tool ever sees the construct.
func TestWalkerCatchesBlindSpots(t *testing.T) {
	cases := []struct {
		name     string
		schema   map[string]any
		wantPath string // substring expected in the reported path
		wantMsg  string // substring expected in the reported message
	}{
		{
			name: "array missing items at top level",
			schema: map[string]any{
				"type": "array",
			},
			wantPath: "",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside properties",
			schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"xs": map[string]any{"type": "array"}},
			},
			wantPath: "/properties/xs",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items nested deeper inside properties",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"outer": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"inner": map[string]any{"type": "array"},
						},
					},
				},
			},
			wantPath: "/properties/outer/properties/inner",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside oneOf branch",
			schema: map[string]any{
				"oneOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "array"},
				},
			},
			wantPath: "/oneOf/1",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside anyOf branch",
			schema: map[string]any{
				"anyOf": []any{map[string]any{"type": "array"}},
			},
			wantPath: "/anyOf/0",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside allOf branch",
			schema: map[string]any{
				"allOf": []any{map[string]any{"type": "array"}},
			},
			wantPath: "/allOf/0",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside not",
			schema: map[string]any{
				"not": map[string]any{"type": "array"},
			},
			wantPath: "/not",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside if/then",
			schema: map[string]any{
				"if":   map[string]any{"type": "object"},
				"then": map[string]any{"type": "array"},
			},
			wantPath: "/then",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside additionalProperties (schema form)",
			schema: map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "array"},
			},
			wantPath: "/additionalProperties",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside patternProperties",
			schema: map[string]any{
				"type":              "object",
				"patternProperties": map[string]any{"^x": map[string]any{"type": "array"}},
			},
			wantPath: "/patternProperties/",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside propertyNames",
			schema: map[string]any{
				"type":          "object",
				"propertyNames": map[string]any{"type": "array"},
			},
			wantPath: "/propertyNames",
			wantMsg:  "missing items",
		},
		{
			name: "array missing items inside $defs target reached via $ref",
			schema: map[string]any{
				"$ref": "#/$defs/bad",
				"$defs": map[string]any{
					"bad": map[string]any{"type": "array"},
				},
			},
			// The library inlines the $ref target, so the path leads through
			// /$ref. The exact string depends on the library — match the
			// 'missing items' message and confirm the violation surfaces.
			wantPath: "",
			wantMsg:  "missing items",
		},
		{
			name: "prefixItems flagged regardless of items presence",
			schema: map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"prefixItems": []any{map[string]any{"type": "string"}},
			},
			wantPath: "",
			wantMsg:  "prefixItems",
		},
		{
			name: "array missing items inside unevaluatedItems (2020-12)",
			schema: map[string]any{
				"type":             "array",
				"items":            map[string]any{"type": "string"},
				"unevaluatedItems": map[string]any{"type": "array"},
			},
			wantPath: "/unevaluatedItems",
			wantMsg:  "missing items",
		},
		// Tier-2 (Anthropic-via-opencode rejected validation keywords).
		// One fixture per keyword so a future contributor can see at a
		// glance which keywords trip the rule. Each fixture exercises the
		// keyword in the context where it's most natural (e.g. minimum on
		// integer, pattern on string, minItems on array).
		{
			name:     "Tier-2: minimum on integer property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer", "minimum": 0}}},
			wantPath: "/properties/x",
			wantMsg:  "minimum",
		},
		{
			name:     "Tier-2: maximum on integer property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "integer", "maximum": 99}}},
			wantPath: "/properties/x",
			wantMsg:  "maximum",
		},
		{
			name:     "Tier-2: exclusiveMinimum on number property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "number", "exclusiveMinimum": 0.0}}},
			wantPath: "/properties/x",
			wantMsg:  "exclusiveMinimum",
		},
		{
			name:     "Tier-2: exclusiveMaximum on number property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "number", "exclusiveMaximum": 1.0}}},
			wantPath: "/properties/x",
			wantMsg:  "exclusiveMaximum",
		},
		{
			name:     "Tier-2: multipleOf on number property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "number", "multipleOf": 0.5}}},
			wantPath: "/properties/x",
			wantMsg:  "multipleOf",
		},
		{
			name:     "Tier-2: minLength on string property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "minLength": 1}}},
			wantPath: "/properties/name",
			wantMsg:  "minLength",
		},
		{
			name:     "Tier-2: maxLength on string property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "maxLength": 64}}},
			wantPath: "/properties/name",
			wantMsg:  "maxLength",
		},
		{
			name:     "Tier-2: pattern on string property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string", "pattern": "^[A-Z]+$"}}},
			wantPath: "/properties/name",
			wantMsg:  "pattern",
		},
		{
			name:     "Tier-2: format on string property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string", "format": "email"}}},
			wantPath: "/properties/email",
			wantMsg:  "format",
		},
		{
			name:     "Tier-2: minItems on array property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"xs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1}}},
			wantPath: "/properties/xs",
			wantMsg:  "minItems",
		},
		{
			name:     "Tier-2: maxItems on array property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"xs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 100}}},
			wantPath: "/properties/xs",
			wantMsg:  "maxItems",
		},
		{
			name:     "Tier-2: uniqueItems on array property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"xs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "uniqueItems": true}}},
			wantPath: "/properties/xs",
			wantMsg:  "uniqueItems",
		},
		{
			name:     "Tier-2: minProperties on object property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"o": map[string]any{"type": "object", "minProperties": 1}}},
			wantPath: "/properties/o",
			wantMsg:  "minProperties",
		},
		{
			name:     "Tier-2: maxProperties on object property",
			schema:   map[string]any{"type": "object", "properties": map[string]any{"o": map[string]any{"type": "object", "maxProperties": 10}}},
			wantPath: "/properties/o",
			wantMsg:  "maxProperties",
		},
		{
			// Even nested deep inside oneOf branches the Tier-2 walker
			// catches the violation, because the descend* helpers walk
			// every reachable subschema.
			name: "Tier-2: rejected keyword inside oneOf branch",
			schema: map[string]any{
				"oneOf": []any{
					map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}, "required": []any{"a"}},
					map[string]any{"type": "object", "properties": map[string]any{"b": map[string]any{"type": "string", "pattern": "^x"}}, "required": []any{"b"}},
				},
			},
			wantPath: "/oneOf/1/properties/b",
			wantMsg:  "pattern",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := compileFixture(t, tc.schema)
			problems := walkSchemaForOpenCodeProfile(s, "", map[*jsonschema.Schema]bool{})
			if len(problems) == 0 {
				t.Fatalf("walker returned no problems; expected one with path containing %q and message containing %q", tc.wantPath, tc.wantMsg)
			}
			found := false
			for _, p := range problems {
				if (tc.wantPath == "" || strings.Contains(p.path, tc.wantPath)) && strings.Contains(p.message, tc.wantMsg) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("walker did not report the expected violation\nwant path containing %q, message containing %q\ngot:", tc.wantPath, tc.wantMsg)
				for _, p := range problems {
					t.Errorf("  - %s", p.String())
				}
			}
		})
	}
}
