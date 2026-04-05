package httpclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"tracto/internal/utils"
)

const (
	MaxReadSize      = 100 * 1024 * 1024
	MaxDisplayLength = 50000
)

func ExecuteRequest(method, reqUrl, reqBody string, headers map[string]string) (string, string, error) {
	client := &http.Client{}

	var bodyReader io.Reader
	if reqBody != "" {
		bodyReader = strings.NewReader(reqBody)
	}

	req, err := http.NewRequest(method, reqUrl, bodyReader)
	if err != nil {
		return "", "", err
	}

	for k, v := range headers {
		if k != "" {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, MaxReadSize))
	if err != nil {
		return "", resp.Status, err
	}

	var finalData string
	if json.Valid(respBytes) {
		var prettyJSON bytes.Buffer
		if err := json.Indent(&prettyJSON, respBytes, "", "  "); err == nil {
			finalData = prettyJSON.String()
		} else {
			finalData = string(respBytes)
		}
	} else {
		finalData = string(respBytes)
	}

	text := utils.SanitizeText(finalData)
	runes := []rune(text)
	if len(runes) > MaxDisplayLength {
		text = string(runes[:MaxDisplayLength]) + "\n\n... [Response truncated due to limits]"
	}

	return text, resp.Status, nil
}
