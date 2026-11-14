package transport

import (
	"context"
	"io"
	"net/http"
	"time"
)

// RetryHTTPReads bounds JWKS fetches and retries only safe HTTP methods.
type RetryHTTPReads struct{ Next http.RoundTripper }

func (t RetryHTTPReads) RoundTrip(request *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return next.RoundTrip(request)
	}
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(request.Context(), time.Second)
		response, err := next.RoundTrip(request.Clone(ctx))
		if err == nil && response == nil {
			cancel()
			return nil, io.ErrUnexpectedEOF
		}
		retryable := err != nil || response.StatusCode == http.StatusBadGateway || response.StatusCode == http.StatusServiceUnavailable || response.StatusCode == http.StatusGatewayTimeout
		if !retryable || attempt == 1 || request.Context().Err() != nil {
			if response != nil && response.Body != nil {
				response.Body = &cancelBody{ReadCloser: response.Body, cancel: cancel}
			} else {
				cancel()
			}
			return response, err
		}
		if response != nil && response.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
		}
		cancel()
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-request.Context().Done():
			timer.Stop()
			return nil, request.Context().Err()
		case <-timer.C:
		}
	}
	return nil, io.ErrUnexpectedEOF
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { err := b.ReadCloser.Close(); b.cancel(); return err }
