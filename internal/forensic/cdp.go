package forensic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TabInfo holds browser tab metadata from Chrome DevTools Protocol.
type TabInfo struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// cdpTarget is the upstream CDP response shape. Unexported to isolate
// the codebase from CDP's response format.
type cdpTarget struct {
	Title string `json:"title"`
	Type  string `json:"type"`
	URL   string `json:"url"`
}

// CaptureTabs connects to the Chrome DevTools Protocol endpoint and returns
// open browser targets. Returns (nil, err) if unreachable or unparseable.
func CaptureTabs(port int) ([]TabInfo, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("cdp: invalid port %d", port)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://localhost:%d/json", port)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("cdp: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cdp: HTTP %d from port %d", resp.StatusCode, port)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cdp: reading response: %w", err)
	}

	var targets []cdpTarget
	if err := json.Unmarshal(body, &targets); err != nil {
		return nil, fmt.Errorf("cdp: parsing JSON: %w", err)
	}

	tabs := make([]TabInfo, len(targets))
	for i, t := range targets {
		tabs[i] = TabInfo{URL: t.URL, Title: t.Title, Type: t.Type}
	}
	return tabs, nil
}
