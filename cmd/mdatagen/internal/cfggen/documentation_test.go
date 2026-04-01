// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package cfggen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateMarkdownDocumentation(t *testing.T) {
	md := &ConfigMetadata{
		Type:        "object",
		Description: "Component settings.",
		Required:    []string{"endpoint"},
		Properties: map[string]*ConfigMetadata{
			"endpoint": {
				Type:        "string",
				Description: "Target endpoint.",
				Default:     "localhost:4317",
			},
			"headers": {
				Type:                 "object",
				Description:          "Additional request headers.",
				AdditionalProperties: &ConfigMetadata{Type: "string"},
			},
			"targets": {
				Type:        "array",
				Description: "Target list.",
				Items: &ConfigMetadata{
					Type:     "object",
					Required: []string{"host"},
					Properties: map[string]*ConfigMetadata{
						"host": {
							Type:        "string",
							Description: "Host to scrape.",
						},
						"interval": {
							Type:        "string",
							Pattern:     goDurationPattern,
							Description: "Scrape interval.",
						},
					},
				},
			},
		},
	}

	got, err := GenerateMarkdownDocumentation(md)
	require.NoError(t, err)
	require.Equal(t, "## Configuration\n\n"+
		"Component settings.\n\n"+
		"| Name | Type | Default | Required | Description |\n"+
		"| --- | --- | --- | --- | --- |\n"+
		"| `endpoint` | `string` | `\"localhost:4317\"` | Yes | Target endpoint. |\n"+
		"| `headers` | `map[string]string` |  | No | Additional request headers. |\n"+
		"| `targets` | `array[object]` |  | No | Target list. |\n"+
		"| `targets[].host` | `string` |  | Yes | Host to scrape. |\n"+
		"| `targets[].interval` | `duration` |  | No | Scrape interval. |\n", string(got))
}

func TestGenerateMarkdownDocumentation_AllOf(t *testing.T) {
	md := &ConfigMetadata{
		Type: "object",
		Properties: map[string]*ConfigMetadata{
			"timeout": {
				Type:        "string",
				Description: "Request timeout.",
			},
		},
		AllOf: []*ConfigMetadata{
			{
				Type: "object",
				Properties: map[string]*ConfigMetadata{
					"endpoint": {
						Type:        "string",
						Description: "Server endpoint.",
					},
				},
			},
		},
	}

	got, err := GenerateMarkdownDocumentation(md)
	require.NoError(t, err)
	require.Contains(t, string(got), "`endpoint`")
	require.Contains(t, string(got), "`timeout`")
	require.Equal(t, 1, strings.Count(string(got), "| `endpoint` |"))
}

func TestGenerateMarkdownDocumentation_AllOfRequiredOnly(t *testing.T) {
	md := &ConfigMetadata{
		Type: "object",
		Properties: map[string]*ConfigMetadata{
			"endpoint": {
				Type:        "string",
				Description: "Server endpoint.",
			},
		},
		AllOf: []*ConfigMetadata{
			{
				Required: []string{"endpoint"},
			},
		},
	}

	got, err := GenerateMarkdownDocumentation(md)
	require.NoError(t, err)
	require.Contains(t, string(got), "| `endpoint` | `string` |  | Yes | Server endpoint. |")
}

func TestGenerateMarkdownDocumentation_AllOfSiblingRequired(t *testing.T) {
	md := &ConfigMetadata{
		Type: "object",
		AllOf: []*ConfigMetadata{
			{
				Properties: map[string]*ConfigMetadata{
					"endpoint": {
						Type:        "string",
						Description: "Server endpoint.",
					},
				},
			},
			{
				Required: []string{"endpoint"},
			},
		},
	}

	got, err := GenerateMarkdownDocumentation(md)
	require.NoError(t, err)
	require.Contains(t, string(got), "| `endpoint` | `string` |  | Yes | Server endpoint. |")
}

func TestGenerateMarkdownDocumentation_PureMapObjectValues(t *testing.T) {
	md := &ConfigMetadata{
		Type: "object",
		Properties: map[string]*ConfigMetadata{
			"clients": {
				Type:        "object",
				Description: "Named clients.",
				AdditionalProperties: &ConfigMetadata{
					Type: "object",
					Properties: map[string]*ConfigMetadata{
						"endpoint": {
							Type:        "string",
							Description: "Client endpoint.",
						},
					},
				},
			},
		},
	}

	got, err := GenerateMarkdownDocumentation(md)
	require.NoError(t, err)
	require.Contains(t, string(got), "| `clients` | `map[string]object` |  | No | Named clients. |")
	require.Contains(t, string(got), "| `clients.<key>` | `object` |  | No |  |")
	require.Contains(t, string(got), "| `clients.<key>.endpoint` | `string` |  | No | Client endpoint. |")
}

func TestGenerateMarkdownDocumentation_ArrayScalarItems(t *testing.T) {
	minimum := 1.0
	maximum := 65535.0
	md := &ConfigMetadata{
		Type: "object",
		Properties: map[string]*ConfigMetadata{
			"ports": {
				Type:        "array",
				Description: "Ports to expose.",
				Items: &ConfigMetadata{
					Type:        "integer",
					Description: "Port number.",
					Enum:        []any{4317, 4318},
					Minimum:     &minimum,
					Maximum:     &maximum,
				},
			},
		},
	}

	got, err := GenerateMarkdownDocumentation(md)
	require.NoError(t, err)
	require.Contains(t, string(got), "| `ports` | `array[integer]` |  | No | Ports to expose. |")
	require.Contains(t, string(got), "| `ports[]` | `integer` |  | No | Port number. Allowed values: `4317`, `4318`. Minimum: 1. Maximum: 65535. |")
}
