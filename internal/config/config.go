package config

import "os"

type Config struct {
	Addr        string
	SharedToken string
}

func Load() Config {
	addr := os.Getenv("HARNESS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	token := os.Getenv("HARNESS_SHARED_TOKEN")
	if token == "" {
		token = "dev-token"
	}
	return Config{Addr: addr, SharedToken: token}
}
