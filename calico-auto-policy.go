package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/remram44/calico-auto-policy/internal/calico-selectors"
)

func main() {
	// Create the app context
	var ctx context.Context
	{
		var ctxCancel context.CancelFunc
		ctx, ctxCancel = context.WithCancel(context.Background())

		// Cancel context on signal
		go func() {
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			sig := <-sigs
			log.Printf("Exiting on signal: %s", sig)
			ctxCancel()
		}()
	}

	// Load the config
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Can't load config: %s", err)
	}
	config.UserAgent = "calico-auto-policy"

	// Load the policy
	var policyTemplate map[string]interface{}
	{
		path := os.Getenv("CALICO_AUTO_POLICY_TEMPLATE")
		if path == "" {
			path = "/etc/calico-auto-policy/policy.yaml"
		}
		file, err := os.Open(path)
		if err != nil {
			log.Fatalf("Can't open policy template YAML: %s: %s", path, err)
		}
		decoder := yaml.NewDecoder(file)
		err = decoder.Decode(&policyTemplate)
		if err != nil {
			log.Fatalf("Can't parse policy template YAML: %s: %s", path, err)
		}
	}

	// Create the dynamic client
	dynclient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Can't create client: %s", err)
	}

	// Setup an informer
	var informerFactory dynamicinformer.DynamicSharedInformerFactory
	informerFactory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		dynclient,
		5*time.Minute,
		metav1.NamespaceAll,
		nil,
	)
	var informer cache.SharedIndexInformer
	informer = informerFactory.ForResource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}).Informer()

	// Watch for changes
	_, err = informer.AddEventHandlerWithResyncPeriod(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				k8sPolicy := obj.(*unstructured.Unstructured)

				log.Printf(
					"new NetworkPolicy: %s/%s",
					k8sPolicy.GetNamespace(),
					k8sPolicy.GetName(),
				)

				addPolicy(dynclient, k8sPolicy, policyTemplate, ctx)
			},
			UpdateFunc: func(oldObj, obj interface{}) {
				k8sPolicy := obj.(*unstructured.Unstructured)

				log.Printf(
					"updated/resynced NetworkPolicy: %s/%s",
					k8sPolicy.GetNamespace(),
					k8sPolicy.GetName(),
				)

				addPolicy(dynclient, k8sPolicy, policyTemplate, ctx)
			},
			DeleteFunc: func(obj interface{}) {
				k8sPolicy := obj.(*unstructured.Unstructured)

				log.Printf(
					"deleted NetworkPolicy: %s/%s",
					k8sPolicy.GetNamespace(),
					k8sPolicy.GetName(),
				)

				removePolicy(dynclient, k8sPolicy, ctx)
			},
		},
		5*time.Minute,
	)
	if err != nil {
		log.Fatalf("Can't setup informer: %s", err)
	}

	// Run until interrupted
	informer.Run(ctx.Done())
}

func deepCopyJsonInto(in map[string]interface{}, out map[string]interface{}) {
	for key, value := range in {
		switch v := value.(type) {
		case map[string]interface{}:
			_ = key
			_ = v
		}
	}
}

func deepCopyJsonMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for key, value := range in {
		out[key] = deepCopyJson(value)
	}
	return out
}

func deepCopyJson(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return deepCopyJsonMap(v)
	case []interface{}:
		newList := make([]interface{}, len(v))
		for i := range v {
			newList[i] = deepCopyJson(v[i])
		}
		return newList
	case int:
		return v
	case string:
		return v
	default:
		panic(fmt.Sprintf("Can't deep copy %T", v))
	}
}

func generateCalicoPolicy(
	k8sPolicy *unstructured.Unstructured,
	policyTemplate map[string]interface{},
) (*unstructured.Unstructured, error) {
	// Get the podSelector of the Kubernetes NetworkPolicy
	k8sPolicySpecUntyped, ok := k8sPolicy.Object["spec"]
	if !ok {
		return nil, fmt.Errorf("Invalid Kubernetes NetworkPolicy: no spec")
	}
	k8sPolicySpec, ok := k8sPolicySpecUntyped.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Invalid Kubernetes NetworkPolicy: invalid spec")
	}
	k8sPolicySelectorUntyped, ok := k8sPolicySpec["podSelector"]
	if !ok {
		return nil, fmt.Errorf("Invalid Kubernetes NetworkPolicy: no podSelector")
	}
	k8sPolicySelector, ok := k8sPolicySelectorUntyped.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf(
			"Invalid Kubernetes NetworkPolicy: invalid podSelector",
		)
	}

	// Convert it to a Calico NetworkPolicy selector
	calicoPolicySelector, err := calico_selectors.KubernetesToCalico(
		k8sPolicySelector,
	)
	if err != nil {
		return nil, err
	}

	// Make a policy from the template
	calicoPolicy := &unstructured.Unstructured{
		Object: policyTemplate,
	}
	// calicoPolicy = calicoPolicy.DeepCopy() // Fails: cannot deep copy int
	calicoPolicy.Object = deepCopyJsonMap(calicoPolicy.Object)

	// Put in the selector
	unstructured.SetNestedField(
		calicoPolicy.Object,
		calicoPolicySelector,
		"spec",
		"selector",
	)

	return calicoPolicy, nil
}

func addPolicy(
	dynclient *dynamic.DynamicClient,
	k8sPolicy *unstructured.Unstructured,
	policyTemplate map[string]interface{},
	ctx context.Context,
) error {
	calicoPolicy, err := generateCalicoPolicy(k8sPolicy, policyTemplate)
	if err != nil {
		return err
	}

	_, err = dynclient.Resource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}).Namespace(k8sPolicy.GetNamespace()).Create(
		ctx,
		calicoPolicy,
		metav1.CreateOptions{},
	)
	return err
}

func removePolicy(
	dynclient *dynamic.DynamicClient,
	k8sPolicy *unstructured.Unstructured,
	ctx context.Context,
) error {
	return dynclient.Resource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}).Namespace(k8sPolicy.GetNamespace()).Delete(
		ctx,
		k8sPolicy.GetName(),
		metav1.DeleteOptions{},
	)
}
