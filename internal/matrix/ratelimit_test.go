package matrix

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"maunium.net/go/mautrix"
)

func TestRetryAfter(t *testing.T) {
	limited := mautrix.RespError{ErrCode: "M_LIMIT_EXCEEDED", ExtraData: map[string]any{"retry_after_ms": float64(1500)}}
	d, ok := RetryAfter(limited)
	require.True(t, ok)
	require.Equal(t, 1500*time.Millisecond, d)

	_, ok = RetryAfter(mautrix.RespError{ErrCode: "M_LIMIT_EXCEEDED"})
	require.False(t, ok, "no retry_after_ms: caller backs off")

	_, ok = RetryAfter(errors.New("network"))
	require.False(t, ok)

	_, ok = RetryAfter(mautrix.RespError{ErrCode: "M_FORBIDDEN"})
	require.False(t, ok)
}

func TestRetryStopsOnOtherErrors(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 3, func() error {
		calls++
		return mautrix.RespError{ErrCode: "M_FORBIDDEN"}
	})
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func TestRetryRetriesLimitThenSucceeds(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 3, func() error {
		calls++
		if calls == 1 {
			return mautrix.RespError{ErrCode: "M_LIMIT_EXCEEDED", ExtraData: map[string]any{"retry_after_ms": float64(1)}}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}
