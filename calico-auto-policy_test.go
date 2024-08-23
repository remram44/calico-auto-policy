//go:build k8s_integration
// +build k8s_integration

package main

import (
	"context"
	"log"
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

const NAMESPACE = "calico-auto-policy-test"

type PodDef struct{
	Name string
	Labels map[string]string
}

func TestIntegration(t *testing.T) {
	// This requires that a Kubernetes cluster be available to test against
	// It needs to have Calico installed

	// Load the config
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Can't load config: %s", err)
	}
	config.UserAgent = "calico-auto-policy_test"

	// Create the dynamic clientset
	dynclientset, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Can't create clientset: %s", err)
	}

	// Create some pods
	for _, podDef := range []PodDef{
		PodDef{
			Name: "nolabels",
			Labels: map[string]string{},
		},
		PodDef{
			Name: "prod-blue",
			Labels: map[string]string{
				"tier": "prod",
				"color": "blue",
			},
		},
	} {
		pod := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind": "Pod",
				"metadata": map[string]interface{}{
					"name": podDef.Name,
					"labels": podDef.Labels,
					"namespace": NAMESPACE,
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name": "nginx",
							"image": "quay.io/nginx/nginx-unprivileged:1.27.0",
							"ports": []interface{}{
								map[string]interface{}{
									"name": "http",
									"protocol": "TCP",
									"containerPort": 8080,
								},
							},
						},
					},
				},
			},
		}
		_, err := dynclientset.Resource(schema.GroupVersionResource{
			Group: "",
			Version: "v1",
			Resource: "pods",
		}).Namespace(NAMESPACE).Create(
			context.TODO(),
			pod,
			metav1.CreateOptions{},
		)
		if err != nil {
			t.Fatalf("Error creating pod %s: %s", podDef.Name, err)
		}
	}
}
