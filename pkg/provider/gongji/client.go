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
	"fmt"
	"io"
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
		return nil, fmt.Errorf("gongji http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response (http %d): %w; body=%s", resp.StatusCode, err, truncate(string(respBody), 256))
	}
	if apiResp.Code != SuccessCode {
		return &apiResp, fmt.Errorf("gongji api code=%s message=%s", apiResp.Code, apiResp.Text())
	}
	return &apiResp, nil
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

// GetTaskDetail fetches task detail (desired/observed points).
func (c *Client) GetTaskDetail(ctx context.Context, taskID int64) (*TaskDetail, error) {
	apiResp, err := c.doJSON(ctx, http.MethodPost, PathTaskDetail, nil, &TaskIDRequest{TaskID: taskID})
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

// SearchTasks searches tasks by name (best-effort).
func (c *Client) SearchTasks(ctx context.Context, taskName string) ([]TaskDetail, error) {
	apiResp, err := c.doJSON(ctx, http.MethodPost, PathTaskSearch, nil, &TaskSearchRequest{
		TaskName: taskName,
		Page:     1,
		PageSize: 50,
	})
	if err != nil {
		return nil, err
	}
	var data TaskSearchData
	if len(apiResp.Data) == 0 || string(apiResp.Data) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(apiResp.Data, &data); err != nil {
		// Some APIs return a bare array
		var list []TaskDetail
		if err2 := json.Unmarshal(apiResp.Data, &list); err2 == nil {
			return list, nil
		}
		return nil, fmt.Errorf("parse search: %w", err)
	}
	return data.Results, nil
}
