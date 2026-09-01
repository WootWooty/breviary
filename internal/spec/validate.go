// YAML + JSON Schema validation
// Input: YAML file. Output: Runbook model or error with position.
package spec

import (
	"encoding/json"
	"fmt"
	"os"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// ValidateYAML reads YAML, validates against JSON Schema, returns the model
func ValidateYAML(path string) (*Runbook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("spec: read %s: %w", path, err)
	}
	return Validate(data)
}

// Validate validates raw YAML and returns the model
func Validate(data []byte) (*Runbook, error) {
	// 1. Parse YAML
	raw := make(map[string]interface{})
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("spec: yaml parse: %w", err)
	}

	// 2. Convert YAML → JSON for validation
	jsonData, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("spec: json marshal: %w", err)
	}

	// 3. Validate against JSON Schema
	if err := validateJSON(jsonData); err != nil {
		return nil, fmt.Errorf("spec: validation: %w", err)
	}

	// 4. Deserialize into the model
	var book Runbook
	if err := yaml.Unmarshal(data, &book); err != nil {
		return nil, fmt.Errorf("spec: unmarshal: %w", err)
	}

	if book.Kind != "Runbook" {
		return nil, fmt.Errorf("spec: kind must be 'Runbook', got %q", book.Kind)
	}
	if book.Metadata.Name == "" {
		return nil, fmt.Errorf("spec: metadata.name is required")
	}

	return &book, nil
}

var compiledSchema *jsonschema.Schema

func validateJSON(data []byte) error {
	if compiledSchema == nil {
		var err error
		compiledSchema, err = compileSchema()
		if err != nil {
			return err
		}
	}

	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("json parse: %w", err)
	}

	return compiledSchema.Validate(v)
}

func compileSchema() (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()

	// Parse the embedded schema
	var doc interface{}
	schemaData := embeddedSchema()
	if err := json.Unmarshal([]byte(schemaData), &doc); err != nil {
		return nil, fmt.Errorf("parse schema json: %w", err)
	}

	if err := c.AddResource("breviary-schema.json", doc); err != nil {
		return nil, fmt.Errorf("add resource: %w", err)
	}

	return c.Compile("breviary-schema.json")
}

// embeddedSchema returns the JSON Schema embedded in the Go binary
func embeddedSchema() string {
	return `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://breviary.io/schema/v1",
  "title": "Breviary Runbook",
  "type": "object",
  "required": ["apiVersion", "kind", "metadata", "spec"],
  "properties": {
    "apiVersion": {
      "type": "string",
      "pattern": "^breviary\\.io/v[0-9]+$"
    },
    "kind": {
      "type": "string",
      "enum": ["Runbook"]
    },
    "metadata": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": { "type": "string", "minLength": 1 },
        "owner": { "type": "string" }
      }
    },
    "spec": {
      "type": "object",
      "required": ["steps"],
      "properties": {
        "trigger": {
          "type": "object",
          "properties": {
            "alert": { "type": "string" },
            "severity": {
              "type": "string",
              "enum": ["critical", "warning", "info"]
            },
            "concurrency": { "type": "integer", "minimum": 1 },
            "dedup": { "type": "string", "pattern": "^[0-9]+[smhd]$" },
            "throttle": { "type": "string", "pattern": "^[0-9]+[smhd]$" }
          }
        },
        "steps": {
          "type": "array",
          "minItems": 1,
          "items": { "$ref": "#/$defs/step" }
        }
      }
    }
  },
  "$defs": {
    "step": {
      "type": "object",
      "required": ["id", "action"],
      "properties": {
        "id": { "type": "string", "minLength": 1 },
        "action": {
          "type": "string",
          "enum": ["exec", "http", "script", "notify"]
        },
        "exec": { "type": "string" },
        "url": { "type": "string", "format": "uri" },
        "script": { "type": "string" },
        "timeout": { "type": "string", "pattern": "^[0-9]+[sm]$" },
        "runas": { "type": "string" },
        "when": { "type": "string" },
        "retry": {
          "oneOf": [
            {
              "type": "object",
              "properties": {
                "max": { "type": "integer", "minimum": 1 },
                "backoff": { "type": "string", "pattern": "^[0-9]+[sm]$" }
              }
            }
          ]
        },
        "approval": {
          "type": "object",
          "properties": {
            "channel": { "type": "string" },
            "timeout": { "type": "string", "pattern": "^[0-9]+[sm]+$" },
            "escalate_after": { "type": "string", "pattern": "^[0-9]+[sm]+$" },
            "show": { "type": "string" }
          }
        },
        "undo": { "$ref": "#/$defs/step" },
        "on-failure": {
          "type": "string",
          "enum": ["rollback", "stop", "continue"]
        },
        "notify": {
          "type": "object",
          "properties": {
            "channel": { "type": "string" },
            "msg": { "type": "string" }
          }
        },
        "parallel": { "type": "boolean" }
      }
    }
  }
}`
}
