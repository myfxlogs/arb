package config

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/encoding/prototext"

	configpb "arb/proto/gen/config"
)

// Load reads and parses a protobuf text format config file.
func Load(path string) (*configpb.SystemConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := &configpb.SystemConfig{}
	if err := prototext.Unmarshal(b, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}
