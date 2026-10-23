package matrix

import (
	"errors"
	"time"

	"maunium.net/go/mautrix"
)

// retryAfter reports how long the server asked us to wait after M_LIMIT_EXCEEDED.
// ok is false for any other error, or when the server gave no retry_after_ms.
func retryAfter(err error) (wait time.Duration, ok bool) {
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
