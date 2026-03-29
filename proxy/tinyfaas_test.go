package proxy

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/openfaas/faas-cli/test"
	"github.com/openfaas/go-sdk/stack"
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

func Test_DeployFunctionTinyFaaS_MultipartRequest(t *testing.T) {
	handlerDir := t.TempDir()
	requireNoError := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	requireNoError(os.WriteFile(filepath.Join(handlerDir, "handler.py"), []byte("print('hello')\n"), 0o644))
	requireNoError(os.Mkdir(filepath.Join(handlerDir, "sub"), 0o755))
	requireNoError(os.WriteFile(filepath.Join(handlerDir, "sub", "config.txt"), []byte("value\n"), 0o644))

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/system/upload" {
			t.Fatalf("expected /system/upload, got %s", r.URL.Path)
		}

		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("failed to parse content type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("expected multipart/form-data, got %s", mediaType)
		}

		reader := multipart.NewReader(r.Body, params["boundary"])
		var metadata TinyFaaSUploadMetadata
		var archiveData []byte

		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("failed to read multipart body: %v", err)
			}

			switch part.FormName() {
			case "metadata":
				if err := json.NewDecoder(part).Decode(&metadata); err != nil {
					t.Fatalf("failed to decode metadata: %v", err)
				}
			case "zip":
				archiveData, err = io.ReadAll(part)
				if err != nil {
					t.Fatalf("failed to read zip part: %v", err)
				}
			default:
				t.Fatalf("unexpected multipart field %q", part.FormName())
			}
		}

		if metadata.FunctionName != "echo" {
			t.Fatalf("unexpected function name: %s", metadata.FunctionName)
		}
		if metadata.FunctionEnv != "python3" {
			t.Fatalf("unexpected function env: %s", metadata.FunctionEnv)
		}
		if metadata.FunctionReplicas != 1 {
			t.Fatalf("unexpected replicas: %d", metadata.FunctionReplicas)
		}
		envs := append([]string(nil), metadata.FunctionEnvs...)
		sort.Strings(envs)
		if strings.Join(envs, ",") != "A=1,B=2" {
			t.Fatalf("unexpected envs: %#v", metadata.FunctionEnvs)
		}
		if metadata.Limits == nil || metadata.Limits.CPU != "50m" || metadata.Limits.Memory != "128Mi" {
			t.Fatalf("unexpected limits: %#v", metadata.Limits)
		}
		if metadata.FunctionLabels["region"] != "eu" {
			t.Fatalf("unexpected labels: %#v", metadata.FunctionLabels)
		}
		if len(archiveData) == 0 {
			t.Fatal("expected zip data")
		}

		archive, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
		if err != nil {
			t.Fatalf("failed to parse zip archive: %v", err)
		}

		entries := make([]string, 0, len(archive.File))
		for _, file := range archive.File {
			entries = append(entries, file.Name)
		}
		sort.Strings(entries)
		joinedEntries := strings.Join(entries, ",")
		if !strings.Contains(joinedEntries, "handler.py") || !strings.Contains(joinedEntries, "sub/config.txt") {
			t.Fatalf("unexpected archive entries: %s", joinedEntries)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Function echo deployed\n"))
	}))
	defer s.Close()

	cliAuth := NewTestAuth(nil)
	client, _ := NewClient(cliAuth, s.URL, nil, &defaultCommandTimeout)
	statusCode, output := client.DeployFunctionTinyFaaS(context.Background(), &DeployFunctionSpec{
		FunctionName: "echo",
		Language:     "python",
		EnvVars: map[string]string{
			"B": "2",
			"A": "1",
		},
		Labels: map[string]string{"region": "eu"},
		FunctionResourceRequest: FunctionResourceRequest{
			Limits: &stack.FunctionResources{CPU: "50m", Memory: "128Mi"},
		},
	}, handlerDir)

	if statusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", statusCode, output)
	}
	if !strings.Contains(output, "Function echo deployed successfully") {
		t.Fatalf("unexpected output: %s", output)
	}
}

func Test_DeployFunctionTinyFaaS_UnsupportedLanguage(t *testing.T) {
	cliAuth := NewTestAuth(nil)
	client, _ := NewClient(cliAuth, "http://127.0.0.1", nil, &defaultCommandTimeout)
	statusCode, output := client.DeployFunctionTinyFaaS(context.Background(), &DeployFunctionSpec{
		FunctionName: "echo",
		Language:     "ruby",
	}, t.TempDir())

	if statusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", statusCode)
	}
	if !strings.Contains(output, "Unsupported language for tinyFaaS") {
		t.Fatalf("unexpected output: %s", output)
	}
}
