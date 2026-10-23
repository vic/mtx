package matrix

import (
	"context"
	"errors"
	"time"

	"maunium.net/go/mautrix"
)

// RetryAfter reports how long the server asked us to wait after M_LIMIT_EXCEEDED.
// ok is false for any other error, or when the server gave no retry_after_ms.
func RetryAfter(err error) (wait time.Duration, ok bool) {
	var re mautrix.RespError
	if !errors.As(err, &re) || re.ErrCode != "M_LIMIT_EXCEEDED" {
		return 0, false
	}
	ms, isNum := re.ExtraData["retry_after_ms"].(float64)
	if !isNum || ms < 0 {
		return 0, false
	}
	return time.Duration(ms) * time.Millisecond, true
}

// Retry runs call; on M_LIMIT_EXCEEDED it waits the server's retry_after_ms and tries again,
// up to attempts calls in total. Any other error, or ctx ending, returns at once.
func Retry(ctx context.Context, attempts int, call func() error) error {
	for {
		err := call()
		attempts--
		wait, limited := RetryAfter(err)
		if err == nil || !limited || attempts <= 0 {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(wait):
		}
	}
}
