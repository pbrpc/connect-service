package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"connectrpc.com/connect/v2/connecthttp"

	foundationclient "github.com/pbrpc/connect-foundation/client"
)

// Check asks the peer at address whether its process is serving, through the
// probe route this package serves. UNKNOWN comes back with the error when the
// peer could not be asked or did not answer with a status.
func Check(ctx context.Context, httpClient connecthttp.HTTPClient, address string) (Status, error) {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet, foundationclient.BaseURL(address)+HTTPPath, nil,
	)
	if err != nil {
		return StatusUnknown, err
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return StatusUnknown, err
	}
	defer response.Body.Close()

	// 200 and 503 both carry a status in the body; anything else is the peer
	// saying something other than its health.
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusServiceUnavailable {
		return StatusUnknown, fmt.Errorf("health probe answered %s", response.Status)
	}

	var body Response
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return StatusUnknown, fmt.Errorf("health probe answer did not decode: %w", err)
	}

	return body.Status, nil
}
