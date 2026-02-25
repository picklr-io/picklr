package engine

import (
	"fmt"
	"strings"

	"github.com/picklr-io/picklr/internal/ir"
)

// ExpandForEach expands resources with ForEach or Count fields into individual resources.
// This must be called before plan creation to flatten iterated resources.
func ExpandForEach(resources []*ir.Resource) []*ir.Resource {
	var expanded []*ir.Resource

	for _, res := range resources {
		if res.Count > 0 {
			for i := 0; i < res.Count; i++ {
				clone := cloneResource(res)
				clone.Name = fmt.Sprintf("%s[%d]", res.Name, i)
				// Substitute count.index in properties
				clone.Properties = substituteIndex(clone.Properties, i)
				expanded = append(expanded, clone)
			}
		} else if len(res.ForEach) > 0 {
			for key, val := range res.ForEach {
				clone := cloneResource(res)
				clone.Name = fmt.Sprintf("%s[%q]", res.Name, key)
				// Substitute each.key and each.value in properties
				clone.Properties = substituteEach(clone.Properties, key, val)
				expanded = append(expanded, clone)
			}
		} else {
			expanded = append(expanded, res)
		}
	}

	return expanded
}

func cloneResource(res *ir.Resource) *ir.Resource {
	clone := &ir.Resource{
		Type:     res.Type,
		Name:     res.Name,
		Provider: res.Provider,
	}
	if res.Lifecycle != nil {
		clone.Lifecycle = &ir.Lifecycle{
			CreateBeforeDestroy: res.Lifecycle.CreateBeforeDestroy,
			PreventDestroy:      res.Lifecycle.PreventDestroy,
			IgnoreChanges:       append([]string{}, res.Lifecycle.IgnoreChanges...),
		}
	}
	clone.DependsOn = append([]string{}, res.DependsOn...)

	// Deep copy properties
	clone.Properties = deepCopyMap(res.Properties)

	return clone
}

func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range m {
		result[k] = deepCopyValue(v)
	}
	return result
}

func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return deepCopyMap(val)
	case []any:
		clone := make([]any, len(val))
		for i, item := range val {
			clone[i] = deepCopyValue(item)
		}
		return clone
	default:
		return v
	}
}

func substituteIndex(props map[string]any, index int) map[string]any {
	return substituteAll(props, map[string]string{
		"${count.index}": fmt.Sprintf("%d", index),
	})
}

func substituteEach(props map[string]any, key string, value any) map[string]any {
	return substituteAll(props, map[string]string{
		"${each.key}":   key,
		"${each.value}": fmt.Sprintf("%v", value),
	})
}

func substituteAll(props map[string]any, replacements map[string]string) map[string]any {
	result := make(map[string]any)
	for k, v := range props {
		result[k] = substituteValue(v, replacements)
	}
	return result
}

func substituteValue(v any, replacements map[string]string) any {
	switch val := v.(type) {
	case string:
		result := val
		for old, newVal := range replacements {
			result = strings.ReplaceAll(result, old, newVal)
		}
		return result
	case map[string]any:
		return substituteAll(val, replacements)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = substituteValue(item, replacements)
		}
		return result
	default:
		return v
	}
}

