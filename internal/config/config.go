package config

import "os"

type Config struct {
	Addr         string
	SharedToken  string
	StorageMode  string
	PostgresDSN  string
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
	storageMode := os.Getenv("HARNESS_STORAGE_MODE")
	if storageMode == "" {
		storageMode = "memory"
	}
	postgresDSN := os.Getenv("HARNESS_POSTGRES_DSN")
	if postgresDSN == "" {
		postgresDSN = os.Getenv("DATABASE_URL")
	}
	return Config{
		Addr:        addr,
		SharedToken: token,
		StorageMode: storageMode,
		PostgresDSN: postgresDSN,
	}
}
