package matrix

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
)

func TestRetryAfter(t *testing.T) {
	limited := mautrix.RespError{ErrCode: "M_LIMIT_EXCEEDED", ExtraData: map[string]any{"retry_after_ms": float64(1500)}}
	d, ok := retryAfter(limited)
	require.True(t, ok)
	require.Equal(t, 1500*time.Millisecond, d)

	_, ok = retryAfter(mautrix.RespError{ErrCode: "M_LIMIT_EXCEEDED"})
	require.False(t, ok, "no retry_after_ms: caller backs off")

	_, ok = retryAfter(errors.New("network"))
	require.False(t, ok)

	_, ok = retryAfter(mautrix.RespError{ErrCode: "M_FORBIDDEN"})
	require.False(t, ok)
}
