package adapter

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	configpb "arb/proto/gen/config"
)

// LoadCredentials resolves broker credentials from environment variables,
// falling back to the config file values. Environment variable format:
//   ARB_<BROKER_NAME_UPPER>_USER
//   ARB_<BROKER_NAME_UPPER>_PASSWORD
func LoadCredentials(cfg *configpb.BrokerConfig) (user int64, password string) {
	user = cfg.User
	password = cfg.Password

	prefix := "ARB_" + strings.ToUpper(strings.ReplaceAll(cfg.Name, "-", "_"))

	if envUser := os.Getenv(prefix + "_USER"); envUser != "" {
		if v, err := strconv.ParseInt(envUser, 10, 64); err == nil {
			user = v
		}
	}
	if envPass := os.Getenv(prefix + "_PASSWORD"); envPass != "" {
		password = envPass
	}
	return
}

// envKey returns the environment variable key for a broker credential.
func envKey(brokerName, suffix string) string {
	return fmt.Sprintf("ARB_%s_%s",
		strings.ToUpper(strings.ReplaceAll(brokerName, "-", "_")),
		suffix)
}
