package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	endpoint string
	client   *http.Client
}

func NewClient(address string) *Client {
	address = strings.TrimRight(address, "/")
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	return &Client{endpoint: address + "/control/monitoring/snapshot",
		client: &http.Client{Timeout: 2 * time.Second}}
}

func (client *Client) Fetch(ctx context.Context) (Snapshot, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.endpoint, nil)
	if err != nil {
		return Snapshot{}, err
	}
	response, err := client.client.Do(request)
	if err != nil {
		return Snapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("monitoring endpoint returned %s", response.Status)
	}
	var snapshot Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		return Snapshot{}, err
	}
	if snapshot.Schema != Schema {
		return Snapshot{}, fmt.Errorf("unsupported monitoring schema %q", snapshot.Schema)
	}
	return snapshot, nil
}
