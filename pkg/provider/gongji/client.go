package gongji

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"waverless/pkg/config"
	"waverless/pkg/logger"
)

// Client is the Gongji OpenAPI HTTP client (token + optional RSA sign).
type Client struct {
	baseURL    string
	token      string
	version    string
	tenantID   int64
	httpClient *http.Client
	privateKey *rsa.PrivateKey // nil => token-only (simple) mode
}

// NewClient creates a Gongji API client from config.
func NewClient(cfg *config.GongjiConfig) (*Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	version := cfg.APIVersion
	if version == "" {
		version = APIVersion
	}

	timeout := time.Duration(cfg.RequestTimeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	c := &Client{
		baseURL:  baseURL,
		token:    cfg.Token,
		version:  version,
		tenantID: cfg.TenantID,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	if strings.TrimSpace(cfg.SigningKey) != "" {
		key, err := parseRSAPrivateKey(cfg.SigningKey)
		if err != nil {
			return nil, fmt.Errorf("gongji signing_key: %w", err)
		}
		c.privateKey = key
	}

	return c, nil
}

func parseRSAPrivateKey(pemOrRaw string) (*rsa.PrivateKey, error) {
	data := []byte(strings.TrimSpace(pemOrRaw))
	// Support literal \n in env vars
	if bytes.Contains(data, []byte(`\n`)) {
		data = bytes.ReplaceAll(data, []byte(`\n`), []byte("\n"))
	}
	block, _ := pem.Decode(data)
	if block == nil {
		// try raw base64 PKCS8
		raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(data), "\n", ""))
		if err != nil {
			return nil, fmt.Errorf("invalid PEM or base64 private key")
		}
		key, err := x509.ParsePKCS8PrivateKey(raw)
		if err != nil {
			if pkcs1, err2 := x509.ParsePKCS1PrivateKey(raw); err2 == nil {
				return pkcs1, nil
			}
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("unsupported private key format")
}

func (c *Client) sign(path, timestamp, body string) (string, error) {
	if c.privateKey == nil {
		return "", nil
	}
	content := path + "\n" + c.version + "\n" + timestamp + "\n" + c.token + "\n" + body
	sum := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, reqBody any) (*APIResponse, error) {
	var bodyBytes []byte
	var err error
	if reqBody != nil {
		bodyBytes, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
	}
	bodyStr := string(bodyBytes)

	signPath := path
	if len(query) > 0 {
		signPath = path + "?" + query.Encode()
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signStr, err := c.sign(signPath, timestamp, bodyStr)
	if err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}

	fullURL := c.baseURL + signPath
	var reader io.Reader
	if len(bodyBytes) > 0 {
		reader = bytes.NewReader(bodyBytes)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("token", c.token)
	httpReq.Header.Set("version", c.version)
	httpReq.Header.Set("timestamp", timestamp)
	if signStr != "" {
		httpReq.Header.Set("sign_str", signStr)
	}

	logger.Debugf("gongji %s %s", method, signPath)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, classifyTransport(err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &Error{Code: CodeProviderRateLimited, Message: "gongji rate limited"}
	}
	if resp.StatusCode == http.StatusGatewayTimeout || resp.StatusCode == http.StatusRequestTimeout {
		return nil, &Error{Code: CodeProviderTimeout, Message: "gongji request timed out"}
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		if resp.StatusCode >= 500 {
			return nil, &Error{Code: CodeProviderTimeout, Message: "gongji returned an unreadable error"}
		}
		return nil, fmt.Errorf("parse response (http %d): %w", resp.StatusCode, err)
	}
	if apiResp.Code != SuccessCode {
		return &apiResp, vendorError(apiResp.Code, apiResp.Text())
	}
	return &apiResp, nil
}

func classifyTransport(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: CodeProviderTimeout, Message: "gongji request timed out"}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Code: CodeProviderTimeout, Message: "gongji request timed out"}
	}
	return fmt.Errorf("gongji http: %w", err)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// CreateTask creates a deployment task.
func (c *Client) CreateTask(ctx context.Context, req *CreateTaskRequest) (*CreateTaskData, error) {
	apiResp, err := c.doJSON(ctx, http.MethodPost, PathTaskCreate, nil, req)
	if err != nil {
		return nil, err
	}
	var data CreateTaskData
	if len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, &data); err != nil {
			return nil, fmt.Errorf("parse create data: %w", err)
		}
	}
	if data.TaskID == 0 {
		return nil, fmt.Errorf("create task returned empty task_id")
	}
	return &data, nil
}

// ChangePoints sets absolute desired points (ScaleApp).
func (c *Client) ChangePoints(ctx context.Context, taskID int64, points int) error {
	_, err := c.doJSON(ctx, http.MethodPost, PathTaskChangePoints, nil, &ChangePointsRequest{
		TaskID: taskID,
		Points: points,
	})
	return err
}

// GetTaskDetail fetches task detail via GET /api/deployment/task/detail?task_id=.
func (c *Client) GetTaskDetail(ctx context.Context, taskID int64) (*TaskDetail, error) {
	q := url.Values{}
	q.Set("task_id", strconv.FormatInt(taskID, 10))
	apiResp, err := c.doJSON(ctx, http.MethodGet, PathTaskDetail, q, nil)
	if err != nil {
		return nil, err
	}
	var detail TaskDetail
	if err := json.Unmarshal(apiResp.Data, &detail); err != nil {
		return nil, fmt.Errorf("parse task detail: %w", err)
	}
	if detail.TaskID == 0 {
		detail.TaskID = taskID
	}
	return &detail, nil
}

// PauseTask pauses a running deployment. Vendor releases the points.
func (c *Client) PauseTask(ctx context.Context, taskID int64) error {
	_, err := c.doJSON(ctx, http.MethodPost, PathTaskPause, nil, &TaskIDRequest{TaskID: taskID})
	return err
}

// RecoverTask resumes a paused deployment.
func (c *Client) RecoverTask(ctx context.Context, taskID int64) error {
	_, err := c.doJSON(ctx, http.MethodPost, PathTaskRecover, nil, &TaskIDRequest{TaskID: taskID})
	return err
}

// UpdateTask replaces a deployment configuration.
// body must be the full task document from GetTaskDetail, with edits applied.
// Partial documents are rejected by the vendor.
func (c *Client) UpdateTask(ctx context.Context, body json.RawMessage) error {
	if len(body) == 0 || string(body) == "null" {
		return fmt.Errorf("update task body is required")
	}
	_, err := c.doJSON(ctx, http.MethodPost, PathTaskUpdate, nil, json.RawMessage(body))
	return err
}

// StopTask deletes/stops a deployment task.
func (c *Client) StopTask(ctx context.Context, taskID int64) error {
	_, err := c.doJSON(ctx, http.MethodPost, PathTaskStop, nil, &TaskIDRequest{TaskID: taskID})
	return err
}

// DeletePod terminates a single point.
func (c *Client) DeletePod(ctx context.Context, pointID int64) error {
	_, err := c.doJSON(ctx, http.MethodPost, PathTaskDeletePod, nil, &DeletePodRequest{PointID: pointID})
	return err
}

// ListPoints lists points for a task.
func (c *Client) ListPoints(ctx context.Context, taskID int64, page, pageSize int) (*PointsListData, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 100
	}
	q := url.Values{}
	if c.tenantID > 0 {
		q.Set("tenant_id", strconv.FormatInt(c.tenantID, 10))
	}
	q.Set("task_id", strconv.FormatInt(taskID, 10))
	q.Set("page", strconv.Itoa(page))
	q.Set("page_size", strconv.Itoa(pageSize))

	apiResp, err := c.doJSON(ctx, http.MethodGet, PathTaskPoints, q, nil)
	if err != nil {
		return nil, err
	}
	var data PointsListData
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("parse points: %w", err)
	}
	return &data, nil
}

// SearchTasks lists deployment tasks. The vendor API is GET with query filters.
func (c *Client) SearchTasks(ctx context.Context, filter TaskListFilter) (*TaskSearchData, error) {
	q := taskListQuery(filter.Type, filter.Status, filter.SearchValue, filter.Page, filter.PageSize)
	apiResp, err := c.doJSON(ctx, http.MethodGet, PathTaskSearch, q, nil)
	if err != nil {
		return nil, err
	}
	var data TaskSearchData
	if len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return &TaskSearchData{}, nil
	}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("parse task search: %w", err)
	}
	return &data, nil
}

// SearchResources loads device inventory, price, and resource marks.
func (c *Client) SearchResources(ctx context.Context, filter ResourceSearchFilter) (*ResourceSearchData, error) {
	q := url.Values{}
	taskType := filter.TaskType
	if taskType == "" {
		taskType = TaskTypeDeploy
	}
	q.Set("task_type", taskType)
	if filter.DeviceType != "" {
		q.Set("device_type", filter.DeviceType)
	}
	apiResp, err := c.doJSON(ctx, http.MethodGet, PathResourceSearch, q, nil)
	if err != nil {
		return nil, err
	}
	var data ResourceSearchData
	if len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return &ResourceSearchData{}, nil
	}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		return nil, fmt.Errorf("parse resource search: %w", err)
	}
	return &data, nil
}

// CreateJob creates a Job batch task.
func (c *Client) CreateJob(ctx context.Context, req *CreateJobRequest) (*CreateTaskData, error) {
	apiResp, err := c.doJSON(ctx, http.MethodPost, PathJobCreate, nil, req)
	if err != nil {
		return nil, err
	}
	var data CreateTaskData
	if len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, &data); err != nil {
			return nil, fmt.Errorf("parse job create: %w", err)
		}
	}
	if data.TaskID == 0 {
		return nil, fmt.Errorf("create job returned empty task_id")
	}
	return &data, nil
}

// GetJob returns the raw Job detail document.
func (c *Client) GetJob(ctx context.Context, taskID int64) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("task_id", strconv.FormatInt(taskID, 10))
	apiResp, err := c.doJSON(ctx, http.MethodGet, PathJobDetail, q, nil)
	if err != nil {
		return nil, err
	}
	if len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return nil, fmt.Errorf("job %d not found", taskID)
	}
	return apiResp.Data, nil
}

// SearchJobs lists Job batch tasks.
func (c *Client) SearchJobs(ctx context.Context, filter JobListFilter) (*JobListData, error) {
	q := taskListQuery("", filter.Status, filter.SearchValue, filter.Page, filter.PageSize)
	if filter.GroupTask != nil {
		q.Set("group_task", strconv.FormatBool(*filter.GroupTask))
	}
	if filter.GroupID > 0 {
		q.Set("group_id", strconv.FormatInt(filter.GroupID, 10))
	}
	apiResp, err := c.doJSON(ctx, http.MethodGet, PathJobSearch, q, nil)
	if err != nil {
		return nil, err
	}
	return decodeJobList(apiResp.Data)
}

// StopJob stops a Job batch task.
func (c *Client) StopJob(ctx context.Context, taskID int64) error {
	_, err := c.doJSON(ctx, http.MethodPost, PathJobStop, nil, &TaskIDRequest{TaskID: taskID})
	return err
}

func taskListQuery(taskType, status, search string, page, pageSize int) url.Values {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	q := url.Values{}
	if taskType != "" {
		q.Set("type", taskType)
	}
	if status != "" {
		q.Set("status", status)
	}
	if search != "" {
		q.Set("search_value", search)
	}
	q.Set("page", strconv.Itoa(page))
	q.Set("page_size", strconv.Itoa(pageSize))
	return q
}

func decodeJobList(raw json.RawMessage) (*JobListData, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return &JobListData{}, nil
	}
	var envelope struct {
		Count   int64             `json:"count"`
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse job search: %w", err)
	}
	out := &JobListData{Count: envelope.Count, Results: make([]JobSummary, 0, len(envelope.Results))}
	for _, item := range envelope.Results {
		var summary JobSummary
		if err := json.Unmarshal(item, &summary); err != nil {
			return nil, fmt.Errorf("parse job row: %w", err)
		}
		summary.Raw = item
		out.Results = append(out.Results, summary)
	}
	return out, nil
}
