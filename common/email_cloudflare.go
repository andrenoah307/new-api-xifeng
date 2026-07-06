package common

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const cloudflareEmailAPIFormat = "https://api.cloudflare.com/client/v4/accounts/%s/email/sending/send"

var (
	cloudflareEmailHTTPClient     *http.Client
	cloudflareEmailHTTPClientOnce sync.Once
)

func getCloudflareEmailHTTPClient() *http.Client {
	cloudflareEmailHTTPClientOnce.Do(func() {
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		}
		if TLSInsecureSkipVerify {
			transport.TLSClientConfig = InsecureTLSConfig
		}
		cloudflareEmailHTTPClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
	})
	return cloudflareEmailHTTPClient
}

type cloudflareEmailAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

type cloudflareEmailRequest struct {
	From    cloudflareEmailAddress `json:"from"`
	To      []string               `json:"to"`
	Subject string                 `json:"subject"`
	HTML    string                 `json:"html"`
}

// Cloudflare 部分错误响应中 code 为数字，部分为字符串，用 any 兼容两者
type cloudflareAPIError struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
}

type cloudflareEmailResponse struct {
	Success bool                 `json:"success"`
	Errors  []cloudflareAPIError `json:"errors"`
}

func cloudflareEmailFromAddress() string {
	if CloudflareEmailFrom != "" {
		return CloudflareEmailFrom
	}
	if SMTPFrom != "" {
		return SMTPFrom
	}
	return SMTPAccount
}

func sendEmailViaCloudflare(subject string, receiver string, content string) error {
	if CloudflareEmailAccountId == "" || CloudflareEmailAPIToken == "" {
		return fmt.Errorf("Cloudflare 邮件服务未配置（缺少 Account ID 或 API Token）")
	}
	from := cloudflareEmailFromAddress()
	if from == "" {
		return fmt.Errorf("Cloudflare 邮件服务未配置发件人地址")
	}
	var to []string
	for _, addr := range strings.Split(receiver, ";") {
		if addr = strings.TrimSpace(addr); addr != "" {
			to = append(to, addr)
		}
	}
	if len(to) == 0 {
		return fmt.Errorf("收件人地址无效")
	}
	payload, err := Marshal(cloudflareEmailRequest{
		From:    cloudflareEmailAddress{Address: from, Name: SystemName},
		To:      to,
		Subject: subject,
		HTML:    content,
	})
	if err != nil {
		return err
	}
	url := fmt.Sprintf(cloudflareEmailAPIFormat, CloudflareEmailAccountId)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+CloudflareEmailAPIToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := getCloudflareEmailHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare email request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var cfResp cloudflareEmailResponse
	_ = Unmarshal(body, &cfResp)
	if resp.StatusCode != http.StatusOK || !cfResp.Success {
		if len(cfResp.Errors) > 0 {
			msgs := make([]string, 0, len(cfResp.Errors))
			for _, e := range cfResp.Errors {
				msgs = append(msgs, fmt.Sprintf("[%v] %s", e.Code, e.Message))
			}
			return fmt.Errorf("cloudflare email API error (status %d): %s", resp.StatusCode, strings.Join(msgs, "; "))
		}
		return fmt.Errorf("cloudflare email API error (status %d)", resp.StatusCode)
	}
	return nil
}
