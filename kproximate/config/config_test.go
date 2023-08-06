package config

import (
	"testing"
)

func TestValidateConfig(t *testing.T) {
	cfg := &Config{
		KpLoadHeadroom: 0.1,
	}

	*cfg = validateConfig(cfg)

	if cfg.KpLoadHeadroom != 0.2 {
		t.Errorf("Expected 0.2, got %f", cfg.KpLoadHeadroom)
	}
}