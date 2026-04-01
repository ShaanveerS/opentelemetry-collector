// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package cfggen // import "go.opentelemetry.io/collector/cmd/mdatagen/internal/cfggen"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type fieldDoc struct {
	Path        string
	Type        string
	Default     string
	Required    bool
	Description string
}

type docBuilder struct {
	entries []fieldDoc
	index   map[string]int
}

// GenerateMarkdownDocumentation renders embeddable markdown documentation for a resolved config schema.
func GenerateMarkdownDocumentation(md *ConfigMetadata) ([]byte, error) {
	builder := docBuilder{
		index: make(map[string]int),
	}
	builder.collectObject("", md)

	var buf bytes.Buffer
	buf.WriteString("## Configuration\n\n")
	if description := strings.TrimSpace(md.Description); description != "" {
		buf.WriteString(description)
		buf.WriteString("\n\n")
	}

	buf.WriteString("| Name | Type | Default | Required | Description |\n")
	buf.WriteString("| --- | --- | --- | --- | --- |\n")

	for _, entry := range builder.entries {
		fmt.Fprintf(
			&buf,
			"| %s | %s | %s | %s | %s |\n",
			escapeMarkdownCell("`"+entry.Path+"`"),
			escapeMarkdownCell(entry.Type),
			escapeMarkdownCell(entry.Default),
			requiredLabel(entry.Required),
			escapeMarkdownCell(entry.Description),
		)
	}

	return buf.Bytes(), nil
}

func (b *docBuilder) collectObject(prefix string, md *ConfigMetadata) {
	if md == nil {
		return
	}

	required := collectRequiredNames(md)

	names := make([]string, 0, len(md.Properties))
	for name := range md.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		field := md.Properties[name]
		path := joinFieldPath(prefix, name)
		_, isRequired := required[name]
		b.add(fieldDoc{
			Path:        path,
			Type:        renderType(field),
			Default:     renderDefault(field.Default),
			Required:    isRequired,
			Description: renderDescription(field),
		})
		b.collectNested(path, field)
	}

	for _, item := range md.AllOf {
		b.collectObject(prefix, item)
	}

	for name := range required {
		if idx, ok := b.index[joinFieldPath(prefix, name)]; ok {
			b.entries[idx].Required = true
		}
	}

	if md.AdditionalProperties != nil && (prefix == "" || shouldDocumentAdditionalProperties(md.AdditionalProperties)) {
		keyPath := "<key>"
		if prefix != "" {
			keyPath = joinFieldPath(prefix, "<key>")
		}
		b.add(fieldDoc{
			Path:        keyPath,
			Type:        renderType(md.AdditionalProperties),
			Default:     renderDefault(md.AdditionalProperties.Default),
			Required:    false,
			Description: renderDescription(md.AdditionalProperties),
		})
		b.collectNested(keyPath, md.AdditionalProperties)
	}
}

func (b *docBuilder) collectNested(prefix string, md *ConfigMetadata) {
	if md == nil {
		return
	}

	if hasSchemaType(md.Type, "array") && md.Items != nil {
		itemPath := prefix + "[]"
		if shouldDocumentArrayItem(md.Items) {
			b.add(fieldDoc{
				Path:        itemPath,
				Type:        renderType(md.Items),
				Default:     renderDefault(md.Items.Default),
				Required:    false,
				Description: renderDescription(md.Items),
			})
		}
		b.collectNested(itemPath, md.Items)
		return
	}

	if isObjectLike(md) {
		b.collectObject(prefix, md)
	}
}

func (b *docBuilder) add(entry fieldDoc) {
	if idx, ok := b.index[entry.Path]; ok {
		existing := &b.entries[idx]
		existing.Required = existing.Required || entry.Required
		if existing.Type == "" || existing.Type == "`any`" {
			existing.Type = entry.Type
		}
		if existing.Default == "" {
			existing.Default = entry.Default
		}
		existing.Description = mergeDescriptions(existing.Description, entry.Description)
		return
	}

	b.index[entry.Path] = len(b.entries)
	b.entries = append(b.entries, entry)
}

func collectRequiredNames(md *ConfigMetadata) map[string]struct{} {
	required := make(map[string]struct{})
	collectRequiredNamesInto(md, required)
	return required
}

func collectRequiredNamesInto(md *ConfigMetadata, required map[string]struct{}) {
	if md == nil {
		return
	}

	for _, name := range md.Required {
		required[name] = struct{}{}
	}

	for _, item := range md.AllOf {
		collectRequiredNamesInto(item, required)
	}
}

func isPureMap(md *ConfigMetadata) bool {
	return md != nil && md.AdditionalProperties != nil && len(md.Properties) == 0 && len(md.AllOf) == 0
}

func isObjectLike(md *ConfigMetadata) bool {
	return md != nil && (hasSchemaType(md.Type, "object") || len(md.Properties) > 0 || len(md.AllOf) > 0 || md.AdditionalProperties != nil)
}

func shouldDocumentAdditionalProperties(md *ConfigMetadata) bool {
	return md != nil && (isObjectLike(md) || hasSchemaType(md.Type, "array") || hasOwnDocumentation(md))
}

func shouldDocumentArrayItem(md *ConfigMetadata) bool {
	return md != nil && (!isObjectLike(md) || hasOwnDocumentation(md))
}

func hasOwnDocumentation(md *ConfigMetadata) bool {
	return md != nil && (md.Default != nil || renderDescription(md) != "")
}

func hasSchemaType(schemaType any, want string) bool {
	switch t := schemaType.(type) {
	case string:
		return t == want
	case []string:
		for _, value := range t {
			if value == want {
				return true
			}
		}
	case []any:
		for _, value := range t {
			if str, ok := value.(string); ok && str == want {
				return true
			}
		}

	}

	return false
}

func joinFieldPath(prefix, name string) string {
	if prefix == "" {
		return name
	}

	return prefix + "." + name
}

func renderType(md *ConfigMetadata) string {
	switch {
	case md == nil:
		return "`any`"
	case isPureMap(md):
		return "`map[string]" + typeLabel(md.AdditionalProperties) + "`"
	case hasSchemaType(md.Type, "array"):
		return "`array[" + typeLabel(md.Items) + "]`"
	default:
		return "`" + typeLabel(md) + "`"
	}
}

func typeLabel(md *ConfigMetadata) string {
	if md == nil {
		return "any"
	}
	if isPureMap(md) {
		return "map[string]" + typeLabel(md.AdditionalProperties)
	}
	if hasSchemaType(md.Type, "array") {
		return "array[" + typeLabel(md.Items) + "]"
	}
	if hasSchemaType(md.Type, "object") || len(md.Properties) > 0 || len(md.AllOf) > 0 {
		return "object"
	}
	if hasSchemaType(md.Type, "string") && md.Pattern == goDurationPattern {
		return "duration"
	}

	labels := schemaTypeLabels(md.Type)
	if len(labels) > 0 {
		return strings.Join(labels, " | ")
	}

	return "any"
}

func schemaTypeLabels(schemaType any) []string {
	switch t := schemaType.(type) {
	case string:
		if t != "" {
			return []string{t}
		}
	case []string:
		if len(t) > 0 {
			return append([]string(nil), t...)
		}
	case []any:
		values := make([]string, 0, len(t))
		for _, item := range t {
			if str, ok := item.(string); ok && str != "" {
				values = append(values, str)
			}
		}
		if len(values) > 0 {
			return values
		}
	}

	return nil
}

func renderDefault(value any) string {
	if value == nil {
		return ""
	}

	data, err := json.Marshal(value)
	if err != nil {
		return "`" + fmt.Sprint(value) + "`"
	}

	return "`" + string(data) + "`"
}

func renderDescription(md *ConfigMetadata) string {
	if md == nil {
		return ""
	}

	parts := make([]string, 0, 8)
	if description := strings.TrimSpace(md.Description); description != "" {
		parts = append(parts, description)
	}

	if len(md.Enum) > 0 {
		values := make([]string, 0, len(md.Enum))
		for _, value := range md.Enum {
			values = append(values, renderInlineValue(value))
		}
		parts = append(parts, "Allowed values: "+strings.Join(values, ", ")+".")
	}

	if md.Const != nil {
		parts = append(parts, "Must be "+renderInlineValue(md.Const)+".")
	}

	if md.Format != "" {
		parts = append(parts, "Format: "+md.Format+".")
	}

	if md.Pattern != "" && md.Pattern != goDurationPattern {
		parts = append(parts, "Must match pattern `"+md.Pattern+"`.")
	}

	if md.MinLength != nil || md.MaxLength != nil {
		parts = append(parts, renderLengthBounds(md.MinLength, md.MaxLength))
	}

	if md.Minimum != nil {
		parts = append(parts, "Minimum: "+formatFloat(*md.Minimum)+".")
	}
	if md.Maximum != nil {
		parts = append(parts, "Maximum: "+formatFloat(*md.Maximum)+".")
	}
	if md.ExclusiveMinimum != nil {
		parts = append(parts, "Exclusive minimum: "+formatFloat(*md.ExclusiveMinimum)+".")
	}
	if md.ExclusiveMaximum != nil {
		parts = append(parts, "Exclusive maximum: "+formatFloat(*md.ExclusiveMaximum)+".")
	}
	if md.MultipleOf != nil {
		parts = append(parts, "Must be a multiple of "+formatFloat(*md.MultipleOf)+".")
	}

	if md.MinItems != nil || md.MaxItems != nil {
		parts = append(parts, renderLengthBounds(md.MinItems, md.MaxItems))
	}
	if md.UniqueItems {
		parts = append(parts, "Items must be unique.")
	}

	if md.MinProperties != nil || md.MaxProperties != nil {
		parts = append(parts, renderPropertyBounds(md.MinProperties, md.MaxProperties))
	}

	if len(md.Examples) > 0 {
		values := make([]string, 0, len(md.Examples))
		for _, value := range md.Examples {
			values = append(values, renderInlineValue(value))
		}
		parts = append(parts, "Examples: "+strings.Join(values, ", ")+".")
	}

	if md.Deprecated {
		parts = append(parts, "Deprecated.")
	}

	return strings.TrimSpace(strings.Join(parts, " "))
}

func renderLengthBounds(min, max *int) string {
	switch {
	case min != nil && max != nil:
		return "Allowed length: " + strconv.Itoa(*min) + " to " + strconv.Itoa(*max) + "."
	case min != nil:
		return "Minimum length: " + strconv.Itoa(*min) + "."
	case max != nil:
		return "Maximum length: " + strconv.Itoa(*max) + "."
	default:
		return ""
	}
}

func renderPropertyBounds(min, max *int) string {
	switch {
	case min != nil && max != nil:
		return "Allowed property count: " + strconv.Itoa(*min) + " to " + strconv.Itoa(*max) + "."
	case min != nil:
		return "Minimum property count: " + strconv.Itoa(*min) + "."
	case max != nil:
		return "Maximum property count: " + strconv.Itoa(*max) + "."
	default:
		return ""
	}
}

func renderInlineValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}

	return "`" + string(data) + "`"
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func requiredLabel(required bool) string {
	if required {
		return "Yes"
	}

	return "No"
}

func mergeDescriptions(existing, incoming string) string {
	switch {
	case existing == "":
		return incoming
	case incoming == "", existing == incoming, strings.Contains(existing, incoming):
		return existing
	default:
		return existing + " " + incoming
	}
}

func escapeMarkdownCell(value string) string {
	replacer := strings.NewReplacer(
		"\r\n", "<br>",
		"\n", "<br>",
		"|", "\\|",
	)

	return replacer.Replace(value)
}
