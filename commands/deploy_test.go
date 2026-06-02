// Copyright (c) Alex Ellis 2017. All rights reserved.
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package commands

import (
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/openfaas/faas-cli/test"
	"github.com/openfaas/go-sdk/stack"
)

func prepareTinyFaaSDeployTest(t *testing.T) {
	t.Helper()

	oldPlatform := platform
	oldHandler := handler
	oldImage := image
	oldLanguage := language
	oldFunctionName := functionName
	oldYAMLFile := yamlFile
	oldRegex := regex
	oldFilter := filter
	oldGateway := gateway
	oldFunctionNamespace := functionNamespace
	oldToken := token
	oldTLSInsecure := tlsInsecure
	oldTimeoutOverride := timeoutOverride
	oldReadTemplate := readTemplate
	oldDeployFlags := deployFlags
	oldServices := services
	oldCPURequest := cpuRequest
	oldCPULimit := cpuLimit
	oldMemoryRequest := memoryRequest
	oldMemoryLimit := memoryLimit

	t.Cleanup(func() {
		platform = oldPlatform
		handler = oldHandler
		image = oldImage
		language = oldLanguage
		functionName = oldFunctionName
		yamlFile = oldYAMLFile
		regex = oldRegex
		filter = oldFilter
		gateway = oldGateway
		functionNamespace = oldFunctionNamespace
		token = oldToken
		tlsInsecure = oldTLSInsecure
		timeoutOverride = oldTimeoutOverride
		readTemplate = oldReadTemplate
		deployFlags = oldDeployFlags
		services = oldServices
		cpuRequest = oldCPURequest
		cpuLimit = oldCPULimit
		memoryRequest = oldMemoryRequest
		memoryLimit = oldMemoryLimit
	})

	resetForTest()
	platform = "faasd"
	handler = ""
	image = ""
	language = ""
	functionName = ""
	gateway = ""
	functionNamespace = ""
	token = ""
	tlsInsecure = false
	timeoutOverride = commandTimeout
	readTemplate = true
	deployFlags = DeployFlags{}
	services = nil
	cpuRequest = ""
	cpuLimit = ""
	memoryRequest = ""
	memoryLimit = ""
}

func newTinyFaaSUploadServer(t *testing.T, expectedName string) (*httptest.Server, *int) {
	t.Helper()

	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
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
		seenMetadata := false
		seenZip := false

		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("failed to read multipart request: %v", err)
			}

			switch part.FormName() {
			case "metadata":
				seenMetadata = true
				var metadata struct {
					Name string `json:"name"`
				}
				if err := json.NewDecoder(part).Decode(&metadata); err != nil {
					t.Fatalf("failed to decode metadata: %v", err)
				}
				if metadata.Name != expectedName {
					t.Fatalf("expected function %s, got %s", expectedName, metadata.Name)
				}
			case "zip":
				seenZip = true
				payload, err := io.ReadAll(part)
				if err != nil {
					t.Fatalf("failed to read zip part: %v", err)
				}
				if len(payload) == 0 {
					t.Fatal("expected non-empty zip payload")
				}
			}
		}

		if !seenMetadata || !seenZip {
			t.Fatalf("expected metadata and zip parts, got metadata=%v zip=%v", seenMetadata, seenZip)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Function " + expectedName + " deployed\n"))
	}))

	return server, &count
}

func Test_deploy(t *testing.T) {
	s := test.MockHttpServer(t, []test.Request{
		{
			Method:             http.MethodPut,
			Uri:                "/system/functions",
			ResponseStatusCode: http.StatusOK,
		},
	})
	defer s.Close()

	stdOut := test.CaptureStdout(func() {
		faasCmd.SetArgs([]string{
			"deploy",
			"--gateway=" + s.URL,
			"--image=golang",
			"--name=test-function",
		})
		faasCmd.Execute()
	})

	if found, err := regexp.MatchString(`(?m:Deployed)`, stdOut); err != nil || !found {
		t.Fatalf("Output is not as expected:\n%s", stdOut)
	}

	if found, err := regexp.MatchString(`(?m:200 OK)`, stdOut); err != nil || !found {
		t.Fatalf("Output is not as expected:\n%s", stdOut)
	}
}

func Test_deployFailed(t *testing.T) {

	var failedDeploy = make(map[string]int)
	var containedErrorsCount int
	failedDeploy["example1"] = 100
	failedDeploy["example2"] = 300
	failedDeploy["example3"] = 400
	failedDeploy["example4"] = 500
	err := deployFailed(failedDeploy)
	if err == nil {
		t.Errorf("\nHad to exit with errors!")
		t.Fail()
	}
	for _, theErrorCode := range failedDeploy {
		if strings.Contains(err.Error(), strconv.Itoa(theErrorCode)) {
			containedErrorsCount++
		}
	}
	if containedErrorsCount != len(failedDeploy) {
		t.Errorf("\nWanted: %d number of errors and got: %d!", len(failedDeploy), containedErrorsCount)
		t.Fail()
	}
}

func Test_deploySucceeded(t *testing.T) {
	var succededDeploy = make(map[string]int)
	if err := deployFailed(succededDeploy); err != nil {
		t.Errorf("\nHad to exit with no errors!")
		t.Fail()
	}
}
func Test_badStatusCOde(t *testing.T) {
	okStatusCode := 200
	if badStatusCode(okStatusCode) {
		t.Errorf("\nUnexpected status code - wanted:%d OK!", okStatusCode)
		t.Fail()
	}
	acceptedStatusCode := 202
	if badStatusCode(acceptedStatusCode) {
		t.Errorf("\nUnexpected status code - wanted:%d Accepted!", acceptedStatusCode)
		t.Fail()
	}
	badStatusC := 300
	if !(badStatusCode(badStatusC)) {
		t.Errorf("\nUnexpected status code - wanted: %d but got %d or %d", badStatusC, acceptedStatusCode, okStatusCode)
		t.Fail()
	}
}

func Test_resolveHandlerPath(t *testing.T) {
	tests := []struct {
		name        string
		yamlFile    string
		handlerPath string
		expected    string
	}{
		{
			name:        "empty yaml file returns handler as-is",
			yamlFile:    "",
			handlerPath: "./handler",
			expected:    "./handler",
		},
		{
			name:        "absolute handler path returns as-is",
			yamlFile:    "/some/path/stack.yml",
			handlerPath: "/absolute/handler",
			expected:    "/absolute/handler",
		},
		{
			name:        "empty handler returns as-is",
			yamlFile:    "/some/path/stack.yml",
			handlerPath: "",
			expected:    "",
		},
		{
			name:        "relative handler resolved from yaml directory",
			yamlFile:    "tests/workflows/tinyfaas/linear3/stack.yml",
			handlerPath: "./a",
			expected:    filepath.Join("tests/workflows/tinyfaas/linear3", "./a"),
		},
		{
			name:        "handler without ./ prefix",
			yamlFile:    "dir/stack.yml",
			handlerPath: "handler",
			expected:    filepath.Join("dir", "handler"),
		},
		{
			name:        "parent directory reference",
			yamlFile:    "project/configs/stack.yml",
			handlerPath: "../functions/a",
			expected:    filepath.Join("project/configs", "../functions/a"),
		},
		{
			name:        "yaml file in current directory",
			yamlFile:    "stack.yml",
			handlerPath: "./c",
			expected:    filepath.Join(".", "./c"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveHandlerPath(tt.yamlFile, tt.handlerPath)
			if result != tt.expected {
				t.Errorf("resolveHandlerPath(%q, %q) = %q; want %q",
					tt.yamlFile, tt.handlerPath, result, tt.expected)
			}
		})
	}
}

func Test_resolveStackFunctionHandlerPaths(t *testing.T) {
	t.Run("nil services is a no-op", func(t *testing.T) {
		resolveStackFunctionHandlerPaths("/tmp/stack.yml", nil)
	})

	t.Run("relative handlers resolved from stack directory", func(t *testing.T) {
		services := &stack.Services{
			Functions: map[string]stack.Function{
				"a": {Handler: "./a"},
				"b": {Handler: "handler"},
				"c": {Handler: "/opt/functions/c"},
				"d": {Handler: ""},
			},
		}

		resolveStackFunctionHandlerPaths("/workspace/workflows/stack.yml", services)

		if got, want := services.Functions["a"].Handler, filepath.Join("/workspace/workflows", "./a"); got != want {
			t.Fatalf("handler a mismatch, got %q, want %q", got, want)
		}

		if got, want := services.Functions["b"].Handler, filepath.Join("/workspace/workflows", "handler"); got != want {
			t.Fatalf("handler b mismatch, got %q, want %q", got, want)
		}

		if got, want := services.Functions["c"].Handler, "/opt/functions/c"; got != want {
			t.Fatalf("handler c mismatch, got %q, want %q", got, want)
		}

		if got, want := services.Functions["d"].Handler, ""; got != want {
			t.Fatalf("handler d mismatch, got %q, want %q", got, want)
		}
	})

	t.Run("empty yaml path leaves handlers unchanged", func(t *testing.T) {
		services := &stack.Services{
			Functions: map[string]stack.Function{
				"a": {Handler: "./a"},
			},
		}

		resolveStackFunctionHandlerPaths("", services)

		if got, want := services.Functions["a"].Handler, "./a"; got != want {
			t.Fatalf("handler mismatch, got %q, want %q", got, want)
		}
	})
}

func Test_resolveStackFunctionImageArchivePaths(t *testing.T) {
	services := &stack.Services{Functions: map[string]stack.Function{
		"archive": {Image: "./dist/fn.tar"},
		"remote":  {Image: "ghcr.io/example/fn:latest"},
	}}

	resolveStackFunctionImageArchivePaths("/workspace/workflows/stack.yml", services)

	if got := services.Functions["archive"].Image; got != "/workspace/workflows/dist/fn.tar" {
		t.Fatalf("archive image path = %q", got)
	}
	if got := services.Functions["remote"].Image; got != "ghcr.io/example/fn:latest" {
		t.Fatalf("remote image path = %q", got)
	}
}

func Test_resolveTemplateDirectory(t *testing.T) {
	tests := []struct {
		name     string
		yamlFile string
		expected string
	}{
		{
			name:     "empty yaml uses cwd template",
			yamlFile: "",
			expected: "./template",
		},
		{
			name:     "stack yaml resolves template directory",
			yamlFile: filepath.Join("tests", "workflows", "openfaas", "linear2", "stack.yaml"),
			expected: filepath.Join("tests", "workflows", "openfaas", "linear2", "template"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveTemplateDirectory(tt.yamlFile); got != tt.expected {
				t.Fatalf("resolveTemplateDirectory(%q) = %q; want %q", tt.yamlFile, got, tt.expected)
			}
		})
	}
}

func Test_stackHasLocalTemplates(t *testing.T) {
	projectDir := t.TempDir()
	stackPath := filepath.Join(projectDir, "stack.yaml")

	if err := os.WriteFile(stackPath, []byte("version: 1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	templateDir := filepath.Join(projectDir, "template", "python3-http")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(templateDir, "template.yml"), []byte("fprocess: python index.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	functions := map[string]stack.Function{
		"a": {Language: "python3-http"},
		"b": {Language: "dockerfile"},
	}

	if !stackHasLocalTemplates(stackPath, functions) {
		t.Fatal("expected stackHasLocalTemplates to be true")
	}

	delete(functions, "a")
	functions["a"] = stack.Function{Language: "node20"}

	if stackHasLocalTemplates(stackPath, functions) {
		t.Fatal("expected stackHasLocalTemplates to be false for missing language template")
	}
}

func Test_resolveTemplateYAMLPath_PrefersStackTemplate(t *testing.T) {
	stackDir := t.TempDir()
	stackPath := filepath.Join(stackDir, "stack.yaml")

	if err := os.WriteFile(stackPath, []byte("version: 1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stackTemplate := filepath.Join(stackDir, "template", "python3-http", "template.yml")
	if err := os.MkdirAll(filepath.Dir(stackTemplate), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(stackTemplate, []byte("fprocess: python index.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveTemplateYAMLPath(stackPath, "python3-http")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if got != stackTemplate {
		t.Fatalf("resolveTemplateYAMLPath picked %q, want %q", got, stackTemplate)
	}
}

func Test_resolveTemplateYAMLPath_FallsBackToCWDTemplate(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	cwd := t.TempDir()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	cwdTemplate := filepath.Join("template", "python3-http", "template.yml")
	if err := os.MkdirAll(filepath.Dir(cwdTemplate), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(cwdTemplate, []byte("fprocess: python index.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stackDir := t.TempDir()
	stackPath := filepath.Join(stackDir, "stack.yaml")
	if err := os.WriteFile(stackPath, []byte("version: 1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveTemplateYAMLPath(stackPath, "python3-http")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if filepath.Clean(got) != filepath.Clean(cwdTemplate) {
		t.Fatalf("resolveTemplateYAMLPath picked %q, want %q", got, cwdTemplate)
	}
}

func Test_deriveFprocess_FallsBackToCWDTemplate(t *testing.T) {
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	oldYAML := yamlFile
	cwd := t.TempDir()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
		yamlFile = oldYAML
	})

	cwdTemplate := filepath.Join("template", "python3-http", "template.yml")
	if err := os.MkdirAll(filepath.Dir(cwdTemplate), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(cwdTemplate, []byte("fprocess: python index.py\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stackDir := t.TempDir()
	yamlFile = filepath.Join(stackDir, "stack.yaml")
	if err := os.WriteFile(yamlFile, []byte("version: 1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fprocess, err := deriveFprocess(stack.Function{Language: "python3-http"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if fprocess != "python index.py" {
		t.Fatalf("deriveFprocess() = %q, want %q", fprocess, "python index.py")
	}
}

func Test_deployTinyFaaS_WithHandler(t *testing.T) {
	prepareTinyFaaSDeployTest(t)
	handlerDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(handlerDir, "handler.py"), []byte("print('hello')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	server, requestCount := newTinyFaaSUploadServer(t, "test-function")
	defer server.Close()

	var err error
	stdOut := test.CaptureStdout(func() {
		faasCmd.SetArgs([]string{
			"deploy",
			"--platform=tinyfaas",
			"--gateway=" + server.URL,
			"--name=test-function",
			"--lang=python",
			"--handler=" + handlerDir,
		})
		err = faasCmd.Execute()
	})

	if err != nil {
		t.Fatalf("expected deploy to succeed, got error: %v", err)
	}
	if *requestCount != 1 {
		t.Fatalf("expected 1 upload request, got %d", *requestCount)
	}
	if !strings.Contains(stdOut, "Packaging function handler from: "+handlerDir) {
		t.Fatalf("expected handler packaging output, got: %s", stdOut)
	}
	if !strings.Contains(stdOut, "Deployed. 200 OK.") {
		t.Fatalf("expected deploy success output, got: %s", stdOut)
	}
	if !strings.Contains(stdOut, "URL: "+server.URL+"/fn/test-function") {
		t.Fatalf("expected deploy success output, got: %s", stdOut)
	}
}

func Test_deployTinyFaaS_WithYAML(t *testing.T) {
	prepareTinyFaaSDeployTest(t)
	projectDir := t.TempDir()
	handlerDir := filepath.Join(projectDir, "handler")
	if err := os.Mkdir(handlerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(handlerDir, "handler.py"), []byte("print('hello')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stackYAML := strings.Join([]string{
		"provider:",
		"  name: tinyfaas",
		"functions:",
		"  yaml-function:",
		"    lang: python",
		"    handler: ./handler",
		"    image: example/yaml-function:latest",
	}, "\n")
	stackPath := filepath.Join(projectDir, "stack.yml")
	if err := os.WriteFile(stackPath, []byte(stackYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	server, requestCount := newTinyFaaSUploadServer(t, "yaml-function")
	defer server.Close()

	var err error
	stdOut := test.CaptureStdout(func() {
		faasCmd.SetArgs([]string{
			"deploy",
			"--gateway=" + server.URL,
			"--yaml=" + stackPath,
		})
		err = faasCmd.Execute()
	})

	if err != nil {
		t.Fatalf("expected YAML deploy to succeed, got error: %v", err)
	}
	if *requestCount != 1 {
		t.Fatalf("expected 1 upload request, got %d", *requestCount)
	}
	if !strings.Contains(stdOut, "Packaging function handler from: "+filepath.Join(projectDir, "./handler")) {
		t.Fatalf("expected YAML handler packaging output, got: %s", stdOut)
	}
	if !strings.Contains(stdOut, "Deployed. 200 OK.") {
		t.Fatalf("expected deploy success output, got: %s", stdOut)
	}
	if !strings.Contains(stdOut, "URL: "+server.URL+"/fn/yaml-function") {
		t.Fatalf("expected deploy success output, got: %s", stdOut)
	}
}

func Test_deployTinyFaaS_WithYAML_ExplicitPlatformOverridesProvider(t *testing.T) {
	prepareTinyFaaSDeployTest(t)

	projectDir := t.TempDir()
	handlerDir := filepath.Join(projectDir, "handler")
	if err := os.Mkdir(handlerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(handlerDir, "handler.py"), []byte("print('hello')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stackYAML := strings.Join([]string{
		"provider:",
		"  name: openfaas",
		"functions:",
		"  yaml-function:",
		"    lang: python",
		"    handler: ./handler",
	}, "\n")
	stackPath := filepath.Join(projectDir, "stack.yml")
	if err := os.WriteFile(stackPath, []byte(stackYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	server, requestCount := newTinyFaaSUploadServer(t, "yaml-function")
	defer server.Close()

	var err error
	stdOut := test.CaptureStdout(func() {
		faasCmd.SetArgs([]string{
			"deploy",
			"--platform=tinyfaas",
			"--gateway=" + server.URL,
			"--yaml=" + stackPath,
		})
		err = faasCmd.Execute()
	})

	if err != nil {
		t.Fatalf("expected deploy to succeed, got error: %v", err)
	}
	if *requestCount != 1 {
		t.Fatalf("expected 1 upload request, got %d", *requestCount)
	}
	if !strings.Contains(stdOut, "Deployed. 200 OK.") {
		t.Fatalf("expected tinyFaaS deploy output, got: %s", stdOut)
	}
	if !strings.Contains(stdOut, "URL: "+server.URL+"/fn/yaml-function") {
		t.Fatalf("expected tinyFaaS deploy output, got: %s", stdOut)
	}
}
