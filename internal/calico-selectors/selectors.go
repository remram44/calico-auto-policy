package calico_selectors

import (
	"fmt"
	"strings"
)

func escape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "'", "\\'")
	return value
}

func processMatchLabels(k8sPolicy map[string]interface{}, selector *[]string) error {
	labelsUntyped, ok := k8sPolicy["matchLabels"]
	if !ok {
		return nil
	}
	labels, ok := labelsUntyped.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Unexpected type for matchLabels")
	}
	for key, valueUntyped := range labels {
		value, ok := valueUntyped.(string)
		if !ok {
			return fmt.Errorf("Unexpected type for matchLabels value %#v", key)
		}
		*selector = append(*selector, key+" == '"+escape(value)+"'")
	}
	return nil
}

func processMatchExpressions(k8sPolicy map[string]interface{}, selector *[]string) error {
	expressionsUntyped, ok := k8sPolicy["matchExpressions"]
	if !ok {
		// It's not there, that's fine
		return nil
	}
	expressions, ok := expressionsUntyped.([]interface{})
	if !ok {
		return fmt.Errorf("Unexpected type for matchExpressions")
	}
	for i, expressionUntyped := range expressions {
		expression, ok := expressionUntyped.(map[string]interface{})
		if !ok {
			return fmt.Errorf("Unexpected type for matchExpressions[%v]", i)
		}

		keyUntyped, ok := expression["key"]
		if !ok {
			return fmt.Errorf("matchExpressions[%v] missing key", i)
		}
		key, ok := keyUntyped.(string)
		if !ok {
			return fmt.Errorf("Unexpected type for matchExpressions[%v].key", i)
		}

		operatorUntyped, ok := expression["operator"]
		if !ok {
			return fmt.Errorf("matchExpressions[%v} missing operator", i)
		}
		operator, ok := operatorUntyped.(string)
		if !ok {
			return fmt.Errorf("Unexpected type for matchExpressions[%v].operator", i)
		}

		if operator == "In" || operator == "NotIn" {
			valuesUntyped, ok := expression["values"]
			if !ok {
				return fmt.Errorf("matchExpressions[%v] mising values", i)
			}
			values, ok := valuesUntyped.([]interface{})
			if !ok {
				return fmt.Errorf("Unexpected type for matchExpressions[%v].values", i)
			}
			valuesString := make([]string, len(values))
			for j := range values {
				valueUntyped := values[j]
				value, ok := valueUntyped.(string)
				if !ok {
					return fmt.Errorf("Unexpected type for matchExpressions[%v].values[%v]", i, j)
				}
				valuesString[j] = "'" + escape(value) + "'"
			}
			op := "in"
			if operator == "NotIn" {
				op = "not in"
			}
			*selector = append(
				*selector,
				key+" "+op+" {"+strings.Join(valuesString, ", ")+"}",
			)
		} else if operator == "Exists" || operator == "DoesNotExist" {
			_, ok = expression["values"]
			if ok {
				return fmt.Errorf("Unexpected matchExpressions[%v].values for operator %#v", i, operator)
			}
			op := ""
			if operator == "DoesNotExist" {
				op = "!"
			}
			*selector = append(*selector, op+"has("+key+")")
		} else {
			return fmt.Errorf("Unexpected value for matchExpressions[%v].operator: %#v", i, operator)
		}
	}
	return nil
}

func KubernetesToCalico(k8sPolicy map[string]interface{}) (string, error) {
	var selector []string

	// Convert matchLabels section
	processMatchLabels(k8sPolicy, &selector)

	// Convert matchExpressions section
	processMatchExpressions(k8sPolicy, &selector)

	return strings.Join(selector, " && "), nil
}
