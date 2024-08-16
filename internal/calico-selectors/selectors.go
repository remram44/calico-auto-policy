package calico_selectors

import (
	"fmt"
)

func KubernetesToCalicoNetworkPolicySelectors(k8sPolicy map[string]interface{}) (string, error) {
	// TODO: Convert selector format
	return "", fmt.Errorf("Unimplemented")
}
