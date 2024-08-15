package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// Load the config
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Can't load config: %s", err)
	}
	config.UserAgent = "calico-auto-policy"

	// Create the dynamic clientset
	dynclientset, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Can't create clientset: %s", err)
	}

	// Setup an informer
	var informerFactory dynamicinformer.DynamicSharedInformerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynclientset,
		5*time.Minute,
		metav1.NamespaceAll,
		nil,
	)
	var informer cache.SharedIndexInformer = informerFactory.ForResource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}).Informer()

	// Watch for changes
	_, err = informer.AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				typedObj := obj.(*unstructured.Unstructured)

				log.Printf("new NetworkPolicy: %s/%s", typedObj.GetNamespace(), typedObj.GetName())

				addPolicy(dynclientset, typedObj)
			},
			UpdateFunc: func(oldObj, obj interface{}) {
				typedObj := obj.(*unstructured.Unstructured)

				log.Printf("NetworkPolicy: %s/%s", typedObj.GetNamespace(), typedObj.GetName())

				addPolicy(dynclientset, typedObj)
			},
			DeleteFunc: func(obj interface{}) {
				typedObj := obj.(*unstructured.Unstructured)

				log.Printf("deleted NetworkPolicy: %s/%s", typedObj.GetNamespace(), typedObj.GetName())

				removePolicy(dynclientset, typedObj.GetNamespace(), typedObj.GetName())
			},
		},
		5*time.Minute,
	)
	if err != nil {
		log.Fatalf("Can't setup informer: %s", err)
	}

	// Create a channel, closed on signal
	stopChannel := make(chan struct{})

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigs
		log.Printf("Exiting on signal: %s", sig)
		close(stopChannel)
	}()

	// Run until interrupted
	informer.Run(stopChannel)
}

func generateCalicoPolicy(k8sPolicy *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	calicoPolicy := &unstructured.Unstructured{}
	calicoPolicy.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "crd.projectcalico.org",
		Version: "v1",
		Kind:    "NetworkPolicy",
	})

	// TODO: Fill in the instance

	return calicoPolicy, nil
}

func addPolicy(dynclientset *dynamic.DynamicClient, k8sPolicy *unstructured.Unstructured) error {
	calicoPolicy, err := generateCalicoPolicy(k8sPolicy)
	if err != nil {
		return err
	}

	_, err = dynclientset.Resource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}).Namespace(k8sPolicy.GetNamespace()).Create(
		context.TODO(),
		calicoPolicy,
		metav1.CreateOptions{},
	)
	return err
}

func removePolicy(dynclientset *dynamic.DynamicClient, namespace string, k8sPolicyName string) error {
	return dynclientset.Resource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}).Namespace(namespace).Delete(
		context.TODO(),
		k8sPolicyName,
		metav1.DeleteOptions{},
	)
}
