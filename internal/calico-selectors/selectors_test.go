package calico_selectors

import (
	"encoding/json"
	"testing"
)

func mustDecode(jsonString string) map[string]interface{} {
	var result map[string]interface{}
	err := json.Unmarshal([]byte(jsonString), &result)
	if err != nil {
		panic(err)
	}
	return result
}

func translate(jsonString string, t *testing.T) string {
	calicoSelector, err := KubernetesToCalico(mustDecode(jsonString))
	if err != nil {
		t.Fatal(err)
	}
	return calicoSelector
}

func testTranslate(jsonString string, expectedCalico string, t *testing.T) {
	result := translate(jsonString, t)
	if result != expectedCalico {
		t.Errorf("%#v != %#v", expectedCalico, result)
	}
}

func TestLabels(t *testing.T) {
	testTranslate(
		"{\"matchLabels\": {\"app\": \"email\"}}",
		"app == 'email'",
		t,
	)
	testTranslate(
		"{\"matchLabels\": {\"app\": \"email\", \"tier\": \"prod\"}}",
		"app == 'email' && tier == 'prod'",
		t,
	)
}

func TestExpressions(t *testing.T) {
	testTranslate(
		"{\"matchExpressions\": [{\"key\": \"app\", \"operator\": \"In\", \"values\": [\"email\", \"chat\"]}, {\"key\": \"tier\", \"operator\": \"Exists\"}]}",
		"app in {'email', 'chat'} && has(tier)",
		t,
	)
	testTranslate(
		"{\"matchExpressions\": [{\"key\": \"app\", \"operator\": \"NotIn\", \"values\": [\"chat\"]}]}",
		"app not in {'chat'}",
		t,
	)
}

func TestBoth(t *testing.T) {
	testTranslate(
		"{\"matchLabels\": {\"tier\": \"prod\"}, \"matchExpressions\": [{\"key\": \"app\", \"operator\": \"NotIn\", \"values\": [\"chat\"]}]}",
		"tier == 'prod' && app not in {'chat'}",
		t,
	)
}
