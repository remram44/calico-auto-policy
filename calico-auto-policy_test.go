//go:build k8s_integration
// +build k8s_integration

package main

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

const NAMESPACE = "calico-auto-policy-test"

type PodDef struct {
	Name   string
	Labels map[string]string
}

func createOrReplace(
	dynclient *dynamic.DynamicClient,
	resource schema.GroupVersionResource,
	obj *unstructured.Unstructured,
	t *testing.T,
) {
	create := func() error {
		_, err := dynclient.
			Resource(resource).
			Namespace(NAMESPACE).
			Create(
				context.TODO(),
				obj,
				metav1.CreateOptions{},
			)
		return err
	}
	log.Printf("Creating %s %s", obj.GetKind(), obj.GetName())
	err := create()
	if errors.IsAlreadyExists(err) {
		log.Printf("Already exists, deleting %s %s", obj.GetKind(), obj.GetName())
		deletePropagationForeground := metav1.DeletePropagationForeground
		err = dynclient.
			Resource(resource).
			Namespace(NAMESPACE).
			Delete(
				context.TODO(),
				obj.GetName(),
				metav1.DeleteOptions{
					PropagationPolicy: &deletePropagationForeground,
				},
			)
		if err != nil {
			t.Fatalf("Error removing existing %s %s: %s", obj.GetKind(), obj.GetName(), err)
		}

		wait := 1 * time.Second
		time.Sleep(wait)
		log.Printf("Recreating %s %s", obj.GetKind(), obj.GetName())
		err = create()
		i := 1
		for i <= 5 && errors.IsAlreadyExists(err) {
			wait *= 2
			time.Sleep(wait)
			err = create()
			i += 1
		}
	}
	if err != nil {
		t.Fatalf("Error creating %s %s: %s", obj.GetKind(), obj.GetName(), err)
	}
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

	// Create the dynamic client
	dynclient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Can't create client: %s", err)
	}

	// Create a headless service to give pods a domain name
	headlessService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name": "testpods",
			},
			"spec": map[string]interface{}{
				"clusterIP": "None",
				"selector": map[string]interface{}{
					"test": "true",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"name": "http",
						"port": 8080,
					},
				},
			},
		},
	}
	createOrReplace(
		dynclient,
		schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "services",
		},
		headlessService,
		t,
	)

	// Create some pods
	for _, podDef := range []PodDef{
		PodDef{
			Name: "nolabels",
			Labels: map[string]string{
				"test": "true",
			},
		},
		PodDef{
			Name: "prod-blue",
			Labels: map[string]string{
				"test":  "true",
				"tier":  "prod",
				"color": "blue",
			},
		},
	} {
		pod := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"name":   podDef.Name,
					"labels": podDef.Labels,
				},
				"spec": map[string]interface{}{
					"hostname":  podDef.Name,
					"subdomain": "testpods",
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "quay.io/nginx/nginx-unprivileged:1.27.0",
							"ports": []interface{}{
								map[string]interface{}{
									"name":          "http",
									"protocol":      "TCP",
									"containerPort": 8080,
								},
							},
						},
					},
				},
			},
		}
		createOrReplace(
			dynclient,
			schema.GroupVersionResource{
				Group:    "",
				Version:  "v1",
				Resource: "pods",
			},
			pod,
			t,
		)
	}

	// Test connecting to pods
	script := `
		set -eu
		sleep 10
		curl nolabels.testpods:8080
	`
	pod := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]interface{}{
				"name": "connector",
			},
			"spec": map[string]interface{}{
				"containers": []interface{}{
					map[string]interface{}{
						"name":  "connector",
						"image": "quay.io/nginx/nginx-unprivileged:1.27.0",
						"args":  []string{"sh", "-c", script},
					},
				},
				"restartPolicy": "Never",
			},
		},
	}
	createOrReplace(
		dynclient,
		schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "pods",
		},
		pod,
		t,
	)
}
