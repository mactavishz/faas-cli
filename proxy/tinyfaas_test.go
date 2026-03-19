package proxy

import (
	"context"
	"net/http"
	"regexp"
	"testing"

	"github.com/openfaas/faas-cli/test"
)

func Test_ListFunctionsTinyFaaS(t *testing.T) {
	response := []map[string]interface{}{
		{
			"name":     "func-test1",
			"replicas": 1,
			"running":  true,
		},
		{
			"name":     "func-test2",
			"replicas": 2,
			"running":  false,
		},
		{
			"name":     "func-test3",
			"replicas": -1,
			"running":  true,
		},
	}

	s := test.MockHttpServer(t, []test.Request{
		{
			Method:             http.MethodGet,
			Uri:                "/system/list",
			ResponseStatusCode: http.StatusOK,
			ResponseBody:       response,
		},
	})
	defer s.Close()

	cliAuth := NewTestAuth(nil)
	client, _ := NewClient(cliAuth, s.URL, nil, &defaultCommandTimeout)

	result, err := client.ListFunctionsTinyFaaS(context.Background(), "test-ns")
	if err != nil {
		t.Fatalf("Error returned: %s", err)
	}

	if len(result) != 3 {
		t.Fatalf("Expected 3 functions but got %d", len(result))
	}

	if result[0].Name != "func-test1" || result[0].Replicas != 1 || result[0].AvailableReplicas != 1 {
		t.Fatalf("Unexpected first function: %#v", result[0])
	}

	if result[1].Name != "func-test2" || result[1].Replicas != 2 || result[1].AvailableReplicas != 0 {
		t.Fatalf("Unexpected second function: %#v", result[1])
	}

	if result[2].Name != "func-test3" || result[2].Replicas != 0 || result[2].AvailableReplicas != 0 {
		t.Fatalf("Unexpected third function: %#v", result[2])
	}
}

func Test_ListFunctionsTinyFaaS_Not200(t *testing.T) {
	s := test.MockHttpServerStatus(t, http.StatusBadRequest)
	defer s.Close()

	cliAuth := NewTestAuth(nil)
	client, _ := NewClient(cliAuth, s.URL, nil, &defaultCommandTimeout)
	_, err := client.ListFunctionsTinyFaaS(context.Background(), "")

	if err == nil {
		t.Fatal("Error was not returned")
	}

	r := regexp.MustCompile(`(?m:tinyFaaS returned error status)`)
	if !r.MatchString(err.Error()) {
		t.Fatalf("Error not matched: %s", err)
	}
}

func Test_ListFunctionsTinyFaaS_InvalidJSON(t *testing.T) {
	s := test.MockHttpServer(t, []test.Request{
		{
			Method:             http.MethodGet,
			Uri:                "/system/list",
			ResponseStatusCode: http.StatusOK,
			ResponseBody:       "{",
		},
	})
	defer s.Close()

	cliAuth := NewTestAuth(nil)
	client, _ := NewClient(cliAuth, s.URL, nil, &defaultCommandTimeout)
	_, err := client.ListFunctionsTinyFaaS(context.Background(), "")

	if err == nil {
		t.Fatal("Error was not returned")
	}

	r := regexp.MustCompile(`(?m:error decoding list response)`)
	if !r.MatchString(err.Error()) {
		t.Fatalf("Error not matched: %s", err)
	}
}
