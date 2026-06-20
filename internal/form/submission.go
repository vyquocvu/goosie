package form

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type SubmissionResult struct {
	Method string
	URL    string
	Body   FormData
	Status int
}

type SubmitClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type FormSubmitter struct {
	onSuccess   SuccessCallback
	onError     ErrorCallback
	timeoutMs   int
	client      SubmitClient
	documentURL string
}

func NewFormSubmitter() *FormSubmitter {
	return &FormSubmitter{
		timeoutMs: 30000,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *FormSubmitter) SetSuccessCallback(callback SuccessCallback) {
	s.onSuccess = callback
}

func (s *FormSubmitter) SetErrorCallback(callback ErrorCallback) {
	s.onError = callback
}

func (s *FormSubmitter) SetClient(client SubmitClient) {
	if client != nil {
		s.client = client
	}
}

func (s *FormSubmitter) SetDocumentURL(rawURL string) {
	s.documentURL = rawURL
}

func EscapeForDisplay(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}

func (s *FormSubmitter) resolveAction(action string) (string, error) {
	parsed, err := url.Parse(action)
	if err != nil {
		return "", err
	}
	if parsed.IsAbs() || s.documentURL == "" {
		return parsed.String(), nil
	}
	base, err := url.Parse(s.documentURL)
	if err != nil {
		return "", err
	}
	if !base.IsAbs() {
		return "", fmt.Errorf("document URL must be absolute")
	}
	return base.ResolveReference(parsed).String(), nil
}

func (s *FormSubmitter) Submit(formNode *html.Node, data FormData) (*SubmissionResult, error) {
	action := getAttrValue(formNode, "action")
	method := strings.ToUpper(getAttrValue(formNode, "method"))
	if method == "" {
		method = "GET"
	}

	if action == "" {
		return nil, fmt.Errorf("no action URL specified")
	}
	action, err := s.resolveAction(action)
	if err != nil {
		if s.onError != nil {
			s.onError(err)
		}
		return nil, err
	}

	var body io.Reader
	var contentType string
	result := &SubmissionResult{
		Method: method,
		URL:    action,
		Body:   data,
	}

	if method == "GET" {
		actionURL, err := url.Parse(action)
		if err == nil {
			query := actionURL.Query()
			for k, v := range data {
				query.Set(k, v)
			}
			actionURL.RawQuery = query.Encode()
			result.URL = actionURL.String()
		}
	} else {
		formData := url.Values{}
		for k, v := range data {
			formData.Set(k, v)
		}
		bodyEncoded := formData.Encode()
		body = strings.NewReader(bodyEncoded)
		contentType = "application/x-www-form-urlencoded"
		result.Body = data
	}

	req, err := http.NewRequest(method, result.URL, body)
	if err != nil {
		if s.onError != nil {
			s.onError(err)
		}
		return nil, err
	}

	if method == "POST" && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		errMsg := err.Error()
		if s.onError != nil {
			s.onError(err)
		}
		if strings.Contains(errMsg, "connection refused") ||
			strings.Contains(errMsg, "connection reset") ||
			strings.Contains(errMsg, "timeout") ||
			strings.Contains(errMsg, "no such host") ||
			strings.Contains(errMsg, "network is unreachable") {
			return nil, fmt.Errorf("connection error: %w", err)
		}
		return nil, err
	}
	defer resp.Body.Close()

	result.Status = resp.StatusCode

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if s.onSuccess != nil {
			s.onSuccess(result)
		}
	} else {
		err = fmt.Errorf("HTTP error: %d", resp.StatusCode)
		if s.onError != nil {
			s.onError(err)
		}
		return result, err
	}

	return result, nil
}

func submitterJSONEncode(data FormData) string {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(jsonData)
}

func submitterFormEncode(data FormData) string {
	var pairs []string
	for k, v := range data {
		pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}
	return strings.Join(pairs, "&")
}

func submitterReadBody(resp *http.Response) string {
	if resp.Body == nil {
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

func submitterNewBuffer(data string) *bytes.Buffer {
	return bytes.NewBufferString(data)
}
