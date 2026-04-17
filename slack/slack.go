package slack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// Send posts a plain-text message to a Slack Incoming Webhook URL.
func Send(webhookURL, message string) error {
	body, err := json.Marshal(map[string]string{"text": message})
	if err != nil {
		return err
	}
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}
