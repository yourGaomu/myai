package agent

import (
	"errors"
	"fmt"
	mathRand "math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type permanentConnectionError struct{ error }

func relayDialError(err error, response *http.Response) error {
	if response == nil {
		return fmt.Errorf("connect relay failed: %w", err)
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	wrapped := fmt.Errorf("connect relay failed: %w, status: %s", err, response.Status)
	// Authentication, identity, path and protocol failures need configuration
	// changes. Retry timeouts, throttling and server outages.
	if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusTooManyRequests {
		return permanentConnectionError{wrapped}
	}
	return wrapped
}

func permanentRelayError(err error) bool {
	var permanent permanentConnectionError
	if errors.As(err, &permanent) {
		return true
	}
	return websocket.IsCloseError(err, websocket.ClosePolicyViolation, websocket.CloseProtocolError, websocket.CloseUnsupportedData)
}

func nextReconnectBackoff(current time.Duration) time.Duration {
	if current < time.Second {
		return time.Second
	}
	if current >= 15*time.Second {
		return 30 * time.Second
	}
	return current * 2
}

func reconnectDelay(backoff time.Duration) time.Duration {
	// Equal jitter grows with the exponential cap and stays between 0.5x and 1x.
	return backoff/2 + time.Duration(mathRand.Float64()*float64(backoff/2))
}
