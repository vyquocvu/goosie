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

type FormSubmitter struct {
	onSuccess SuccessCallback
	onError   ErrorCallback
	timeoutMs int
	client    *http.Client
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

func (s *FormSubmitter) Submit(formNode *html.Node, data FormData) (*SubmissionResult, error) {
	action := getAttrValue(formNode, "action")
	method := strings.ToUpper(getAttrValue(formNode, "method"))
	if method == "" {
		method = "GET"
	}

	if action == "" {
		return nil, fmt.Errorf("no action URL specified")
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
		errMsg := strings.ToLower(err.Error())
		if s.onError != nil {
			s.onError(err)
		}
		if strings.Contains(errMsg, "connection refused") ||
			strings.Contains(errMsg, "connection reset") ||
			strings.Contains(errMsg, "timeout") ||
			strings.Contains(errMsg, "deadline exceeded") ||
			strings.Contains(errMsg, "no such host") ||
			strings.Contains(errMsg, "network is unreachable") {
			return nil, fmt.Errorf("connection error: %v", err)
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
	}

	return result, err
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
