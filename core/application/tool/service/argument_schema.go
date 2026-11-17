package service

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"unicode/utf8"
)

// validateToolArguments checks Hook rewrites against the constraints used by
// tool schemas. Descriptions, defaults and other annotation keywords do not
// affect validation. A nil schema retains compatibility with schema-less tools.
func validateToolArguments(schema any, arguments string) error {
	var value any
	if err := json.Unmarshal([]byte(arguments), &value); err != nil {
		return fmt.Errorf("must be a JSON object: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("must be a JSON object")
	}
	if schema == nil {
		return nil
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("invalid tool schema: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(encoded, &normalized); err != nil {
		return fmt.Errorf("invalid tool schema: %w", err)
	}
	return validateSchemaValue(normalized, value, "arguments")
}

func validateSchemaValue(schema, value any, path string) error {
	if allowed, ok := schema.(bool); ok {
		if !allowed {
			return fmt.Errorf("%s is not allowed", path)
		}
		return nil
	}
	rules, ok := schema.(map[string]any)
	if !ok {
		return fmt.Errorf("%s has an invalid tool schema", path)
	}
	if declared, exists := rules["type"]; exists {
		matches := false
		switch declared := declared.(type) {
		case string:
			matches = schemaTypeMatches(declared, value)
		case []any:
			for _, candidate := range declared {
				if name, ok := candidate.(string); ok && schemaTypeMatches(name, value) {
					matches = true
				}
			}
		}
		if !matches {
			return fmt.Errorf("%s has an invalid type (expected %v)", path, declared)
		}
	}
	if choices, ok := rules["enum"].([]any); ok {
		matched := false
		for _, choice := range choices {
			if reflect.DeepEqual(value, choice) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s must match an enum value", path)
		}
	}
	if constant, exists := rules["const"]; exists && !reflect.DeepEqual(value, constant) {
		return fmt.Errorf("%s must match its constant value", path)
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		if choices, ok := rules[keyword].([]any); ok {
			matches := 0
			for _, choice := range choices {
				if validateSchemaValue(choice, value, path) == nil {
					matches++
				}
			}
			if (keyword == "allOf" && matches != len(choices)) || (keyword == "anyOf" && matches == 0) || (keyword == "oneOf" && matches != 1) {
				return fmt.Errorf("%s does not satisfy %s", path, keyword)
			}
		}
	}
	switch value := value.(type) {
	case map[string]any:
		if required, ok := rules["required"].([]any); ok {
			for _, item := range required {
				if name, ok := item.(string); ok {
					if _, exists := value[name]; !exists {
						return fmt.Errorf("%s.%s is required", path, name)
					}
				}
			}
		}
		properties, _ := rules["properties"].(map[string]any)
		// Stable order keeps a failed rewrite's error reproducible.
		names := make([]string, 0, len(value))
		for name := range value {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			property, exists := properties[name]
			if !exists {
				property, exists = rules["additionalProperties"]
			}
			if exists {
				if err := validateSchemaValue(property, value[name], path+"."+name); err != nil {
					return err
				}
			}
		}
	case []any:
		if err := validateSchemaBounds(rules, float64(len(value)), path, "minItems", "maxItems"); err != nil {
			return err
		}
		if items, exists := rules["items"]; exists {
			for index, item := range value {
				if err := validateSchemaValue(items, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	case string:
		return validateSchemaBounds(rules, float64(utf8.RuneCountInString(value)), path, "minLength", "maxLength")
	case float64:
		if err := validateSchemaBounds(rules, value, path, "minimum", "maximum"); err != nil {
			return err
		}
		if minimum, ok := rules["exclusiveMinimum"].(float64); ok && value <= minimum {
			return fmt.Errorf("%s must be greater than %v", path, minimum)
		}
		if maximum, ok := rules["exclusiveMaximum"].(float64); ok && value >= maximum {
			return fmt.Errorf("%s must be less than %v", path, maximum)
		}
	}
	return nil
}

func schemaTypeMatches(name string, value any) bool {
	switch name {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && math.Trunc(number) == number
	case "null":
		return value == nil
	default:
		return false
	}
}

func validateSchemaBounds(rules map[string]any, value float64, path, lower, upper string) error {
	if minimum, ok := rules[lower].(float64); ok && value < minimum {
		return fmt.Errorf("%s does not satisfy %s %v", path, lower, minimum)
	}
	if maximum, ok := rules[upper].(float64); ok && value > maximum {
		return fmt.Errorf("%s does not satisfy %s %v", path, upper, maximum)
	}
	return nil
}
