package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"

	"github.com/openfaas/faas-cli/util"
	types "github.com/openfaas/faas-provider/types"
)

type tinyFaaSResources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
}

// TinyFaaSUploadMetadata defines the metadata part for tinyFaaS uploads.
type TinyFaaSUploadMetadata struct {
	FunctionName     string             `json:"name"`
	FunctionEnv      string             `json:"env"`
	FunctionReplicas int                `json:"replicas"`
	FunctionEnvs     []string           `json:"envs"`
	FunctionLabels   map[string]string  `json:"labels,omitempty"`
	Limits           *tinyFaaSResources `json:"limits,omitempty"`
}

// TinyFaaSDeleteRequest defines the request structure for tinyFaaS delete
type TinyFaaSDeleteRequest struct {
	FunctionName string `json:"name"`
}

func buildTinyFaaSUploadBody(metadata TinyFaaSUploadMetadata, handlerDir string) (*io.PipeReader, string, <-chan error) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	contentType := multipartWriter.FormDataContentType()
	errCh := make(chan error, 1)

	go func() {
		err := writeTinyFaaSUploadBody(multipartWriter, metadata, handlerDir)
		if err == nil {
			err = multipartWriter.Close()
		}
		if err != nil {
			_ = writer.CloseWithError(err)
			errCh <- err
			return
		}

		closeErr := writer.Close()
		if closeErr != nil && closeErr != io.ErrClosedPipe {
			errCh <- closeErr
			return
		}

		errCh <- nil
	}()

	return reader, contentType, errCh
}

func writeTinyFaaSUploadBody(writer *multipart.Writer, metadata TinyFaaSUploadMetadata, handlerDir string) error {
	metadataHeader := textproto.MIMEHeader{}
	metadataHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metadataHeader.Set("Content-Type", "application/json")

	metadataPart, err := writer.CreatePart(metadataHeader)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(metadataPart).Encode(metadata); err != nil {
		return err
	}

	zipPart, err := writer.CreateFormFile("zip", metadata.FunctionName+".zip")
	if err != nil {
		return err
	}

	return util.WriteZipDirectory(zipPart, handlerDir)
}

// DeployFunctionTinyFaaS deploys a function to tinyFaaS.
func (c *Client) DeployFunctionTinyFaaS(ctx context.Context, spec *DeployFunctionSpec, handlerDir string) (int, string) {
	// Convert environment variables to tinyFaaS format
	envKeys := make([]string, 0, len(spec.EnvVars))
	for k := range spec.EnvVars {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)

	envs := make([]string, 0, len(spec.EnvVars))
	for _, k := range envKeys {
		v := spec.EnvVars[k]
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	// Check language support
	switch spec.Language {
	case "binary":
	case "python":
		spec.Language = "python3"
	case "python3":
	case "node":
		spec.Language = "nodejs"
	case "nodejs":
	case "go":
	case "golang":
		spec.Language = "go"
	default:
		return http.StatusBadRequest, fmt.Sprintf("Unsupported language for tinyFaaS: %s", spec.Language)
	}

	uploadReq := TinyFaaSUploadMetadata{
		FunctionName:     spec.FunctionName,
		FunctionEnv:      spec.Language,
		FunctionReplicas: 1,
		FunctionEnvs:     envs,
		FunctionLabels:   spec.Labels,
	}

	// No support of requests in tinyFaaS, only limits
	if spec.FunctionResourceRequest.Limits != nil {
		uploadReq.Limits = &tinyFaaSResources{CPU: spec.FunctionResourceRequest.Limits.CPU, Memory: spec.FunctionResourceRequest.Limits.Memory}
	}

	bodyReader, contentType, bodyErrCh := buildTinyFaaSUploadBody(uploadReq, handlerDir)
	req, err := c.newRequestWithContentType(http.MethodPost, "/system/upload", nil, bodyReader, contentType)
	if err != nil {
		_ = bodyReader.CloseWithError(err)
		<-bodyErrCh
		return http.StatusInternalServerError, fmt.Sprintf("Error creating request: %s", err)
	}

	res, err := c.doRequest(ctx, req)
	if err != nil {
		_ = bodyReader.CloseWithError(err)
		if bodyErr := <-bodyErrCh; bodyErr != nil && bodyErr != io.ErrClosedPipe {
			return http.StatusInternalServerError, fmt.Sprintf("Error preparing upload: %s", bodyErr)
		}
		return http.StatusInternalServerError, fmt.Sprintf("Error deploying function: %s", err)
	}
	defer res.Body.Close()

	if bodyErr := <-bodyErrCh; bodyErr != nil {
		return http.StatusInternalServerError, fmt.Sprintf("Error preparing upload: %s", bodyErr)
	}

	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, fmt.Sprintf("Error reading response: %s", err)
	}

	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		deployOutput := fmt.Sprintf("Deployed. %s.\n", res.Status)
		deployedURL := fmt.Sprintf("URL: %s/fn/%s", c.GatewayURL.String(), generateFuncStr(spec))
		deployOutput += fmt.Sprintln(deployedURL)
		return res.StatusCode, deployOutput
	default:
		return res.StatusCode, fmt.Sprintf("tinyFaaS returned error: %s", string(responseBody))
	}
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

	req, err := c.newRequest(http.MethodPost, "/system/delete", nil, bytes.NewReader(body))
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

	return nil
}

// ListFunctionsTinyFaaS lists functions from tinyFaaS
func (c *Client) ListFunctionsTinyFaaS(context context.Context, namespace string) ([]types.FunctionStatus, error) {
	req, err := c.newRequest(http.MethodGet, "/system/list", nil, nil)
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

	var listed []struct {
		Name     string `json:"name"`
		Replicas int    `json:"replicas"`
		Running  bool   `json:"running"`
	}
	if err := json.NewDecoder(res.Body).Decode(&listed); err != nil {
		return nil, fmt.Errorf("error decoding list response: %w", err)
	}

	functions := make([]types.FunctionStatus, 0, len(listed))
	for _, fn := range listed {
		if fn.Name == "" {
			continue
		}

		desiredReplicas := fn.Replicas
		if desiredReplicas < 0 {
			desiredReplicas = 0
		}

		availableReplicas := 0
		if fn.Running {
			availableReplicas = desiredReplicas
		}

		functions = append(functions, types.FunctionStatus{
			Name:              fn.Name,
			Namespace:         namespace,
			Replicas:          uint64(desiredReplicas),
			AvailableReplicas: uint64(availableReplicas),
		})
	}

	return functions, nil
}

// GetFunctionLogsTinyFaaS retrieves logs from tinyFaaS
func (c *Client) GetFunctionLogsTinyFaaS(context context.Context, functionName string, namespace string) (io.ReadCloser, error) {
	query := url.Values{}
	if functionName != "" {
		query.Set("name", functionName)
	}

	req, err := c.newRequest(http.MethodGet, "/system/logs", query, nil)
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
	// For tinyFaaS, we need to invoke via the API gateway
	// The gateway URL should point to the API gateway (e.g., http://localhost:8888)
	// and we invoke functions at /fn/<function-name>

	functionPath := fmt.Sprintf("/fn/%s", functionName)

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
		req.Header.Set("X-Tinyfaas-Async", "true")
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error invoking function: %s", err)
	}

	return res, nil
}
