package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Feishu sends messages via a Feishu custom bot webhook.
type Feishu struct {
	webhook string
	client  *http.Client
}

func NewFeishu(webhook string) *Feishu {
	return &Feishu{
		webhook: webhook,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *Feishu) Notify(ctx context.Context, title, text string) error {
	body, err := json.Marshal(map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": title + "\n" + text,
		},
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("feishu webhook: status %s", resp.Status)
	}
	return nil
}
