package banco

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_WithDefault_ZeroTTL(t *testing.T) {
	cfg := Config{}
	got := cfg.WithDefault()

	assert.Equal(t, defaultPriceCacheTTL, got.PriceCacheTTL, "zero TTL should be replaced with defaultPriceCacheTTL")
	require.NotNil(t, got.Log, "nil Log should be replaced with a default logger")
}

func TestConfig_WithDefault_CustomTTL(t *testing.T) {
	custom := 42 * time.Second
	cfg := Config{PriceCacheTTL: custom}
	got := cfg.WithDefault()

	assert.Equal(t, custom, got.PriceCacheTTL, "custom TTL should be preserved")
	require.NotNil(t, got.Log, "nil Log should be replaced with a default logger")
}

func TestConfig_WithDefault_NilLog(t *testing.T) {
	cfg := Config{Log: nil}
	got := cfg.WithDefault()

	require.NotNil(t, got.Log, "nil Log should always receive a default logger")
}

func TestConfig_WithDefault_CustomTTL_NilLog(t *testing.T) {
	custom := 10 * time.Minute
	cfg := Config{PriceCacheTTL: custom, Log: nil}
	got := cfg.WithDefault()

	assert.Equal(t, custom, got.PriceCacheTTL, "custom TTL preserved when Log is nil")
	require.NotNil(t, got.Log, "nil Log should be replaced with default logger")
}
