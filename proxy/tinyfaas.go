package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	types "github.com/openfaas/faas-provider/types"
)

// TinyFaaSUploadRequest defines the request structure for tinyFaaS upload
type TinyFaaSUploadRequest struct {
	FunctionName    string   `json:"name"`
	FunctionEnv     string   `json:"env"`
	FunctionThreads int      `json:"threads"`
	FunctionZip     string   `json:"zip"`
	FunctionEnvs    []string `json:"envs"`
}

// TinyFaaSDeleteRequest defines the request structure for tinyFaaS delete
type TinyFaaSDeleteRequest struct {
	FunctionName string `json:"name"`
}

// DeployFunctionTinyFaaS deploys a function to tinyFaaS
func (c *Client) DeployFunctionTinyFaaS(context context.Context, spec *DeployFunctionSpec, functionZip []byte) (int, string) {
	// Convert environment variables to tinyFaaS format
	envs := []string{}
	for k, v := range spec.EnvVars {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	// Base64 encode the zip file
	zipBase64 := base64.StdEncoding.EncodeToString(functionZip)

	// Check language support
	switch spec.Language {
	case "binary":
	case "python":
		spec.Language = "python3"
	case "python3":
	case "node":
		spec.Language = "nodejs"
	case "nodejs":
	default:
		return http.StatusBadRequest, fmt.Sprintf("Unsupported language for tinyFaaS: %s", spec.Language)
	}

	uploadReq := TinyFaaSUploadRequest{
		FunctionName:    spec.FunctionName,
		FunctionEnv:     spec.Language,
		FunctionThreads: 1,
		FunctionZip:     zipBase64,
		FunctionEnvs:    envs,
	}

	body, err := json.Marshal(uploadReq)
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Error marshaling request: %s", err)
	}

	req, err := c.newRequest(http.MethodPost, "/upload", nil, bytes.NewReader(body))
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Error creating request: %s", err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Error deploying function: %s", err)
	}
	defer res.Body.Close()

	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, fmt.Sprintf("Error reading response: %s", err)
	}

	if res.StatusCode != http.StatusOK {
		return res.StatusCode, fmt.Sprintf("tinyFaaS returned error: %s", string(responseBody))
	}

	return res.StatusCode, fmt.Sprintf("Function %s deployed successfully\n%s", spec.FunctionName, string(responseBody))
}

// DeleteFunctionTinyFaaS deletes a function from tinyFaaS
func (c *Client) DeleteFunctionTinyFaaS(context context.Context, functionName string, namespace string) error {
	deleteReq := TinyFaaSDeleteRequest{
		FunctionName: functionName,
	}

	body, err := json.Marshal(deleteReq)
	if err != nil {
		return fmt.Errorf("error marshaling request: %s", err)
	}

	req, err := c.newRequest(http.MethodPost, "/delete", nil, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("error creating request: %s", err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error deleting function: %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(res.Body)
		return fmt.Errorf("tinyFaaS returned error (status %d): %s", res.StatusCode, string(responseBody))
	}

	fmt.Printf("Function %s deleted successfully\n", functionName)
	return nil
}

// ListFunctionsTinyFaaS lists functions from tinyFaaS
func (c *Client) ListFunctionsTinyFaaS(context context.Context, namespace string) ([]types.FunctionStatus, error) {
	req, err := c.newRequest(http.MethodGet, "/list", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %s", err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error listing functions: %s", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tinyFaaS returned error status: %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %s", err)
	}

	// Parse the response - tinyFaaS returns function names line by line
	functionNames := strings.Split(strings.TrimSpace(string(body)), "\n")

	var functions []types.FunctionStatus
	for _, name := range functionNames {
		if name != "" {
			functions = append(functions, types.FunctionStatus{
				Name:              name,
				Namespace:         namespace,
				Replicas:          1,
				AvailableReplicas: 1,
			})
		}
	}

	return functions, nil
}

// GetFunctionLogsTinyFaaS retrieves logs from tinyFaaS
func (c *Client) GetFunctionLogsTinyFaaS(context context.Context, functionName string, namespace string) (io.ReadCloser, error) {
	query := url.Values{}
	if functionName != "" {
		query.Set("name", functionName)
	}

	req, err := c.newRequest(http.MethodGet, "/logs", query, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %s", err)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error getting logs: %s", err)
	}

	if res.StatusCode != http.StatusOK {
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("tinyFaaS returned error (status %d): %s", res.StatusCode, string(body))
	}

	return res.Body, nil
}

// InvokeFunctionTinyFaaS invokes a function on tinyFaaS via the rproxy
func (c *Client) InvokeFunctionTinyFaaS(context context.Context, functionName, namespace string, body io.Reader, contentType string, query url.Values, headers map[string]string, async bool, httpMethod string) (*http.Response, error) {
	// For tinyFaaS, we need to invoke via the rproxy endpoint
	// The gateway URL should point to the rproxy (e.g., http://localhost:8000)
	// and we invoke functions at /<function-name>

	functionPath := fmt.Sprintf("/%s", functionName)

	req, err := c.newRequest(httpMethod, functionPath, query, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %s", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Add custom headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Handle async invocation
	if async {
		req.Header.Set("X-tinyFaaS-Async", "true")
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error invoking function: %s", err)
	}

	return res, nil
}
