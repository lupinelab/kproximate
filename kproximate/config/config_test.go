package config

import (
	"testing"
)

func TestValidateConfig(t *testing.T) {
	cfg := &KproximateConfig{
		LoadHeadroom:     0.1,
		PollInterval:       5,
		WaitSecondsForJoin: 30,
	}

	*cfg = validateConfig(cfg)

	if cfg.LoadHeadroom != 0.2 {
		t.Errorf("Expected 0.2, got %f", cfg.LoadHeadroom)
	}

	if cfg.PollInterval != 10 {
		t.Errorf("Expected 10, got %d", cfg.PollInterval)
	}

	if cfg.WaitSecondsForJoin != 60 {
		t.Errorf("Expected 60, got %d", cfg.WaitSecondsForJoin)
	}
}
