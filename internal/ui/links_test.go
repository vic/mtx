package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestURLAtClick: a click on a link's word gives its URL; on other words, none.
func TestURLAtClick(t *testing.T) {
	row := "see https://example.com/x, here"
	require.Equal(t, "https://example.com/x", urlAt(row, 6))
	require.Equal(t, "", urlAt(row, 1))
	require.Equal(t, "", urlAt(row, 26))
}
