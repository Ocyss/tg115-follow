package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// TGBot sends messages through a standalone bot via the Bot API.
type TGBot struct {
	token  string
	chatID int64
	client *http.Client
}

func NewTGBot(token string, chatID int64) *TGBot {
	return &TGBot{
		token:  token,
		chatID: chatID,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TGBot) Notify(ctx context.Context, title, text string) error {
	form := url.Values{
		"chat_id": {strconv.FormatInt(t.chatID, 10)},
		"text":    {title + "\n" + text},
	}
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage?%s", t.token, form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("tg bot api: %s", result.Description)
	}
	return nil
}
