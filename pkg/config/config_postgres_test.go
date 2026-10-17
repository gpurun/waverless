package config

import (
	"os"
	"testing"
)

func TestMetadataDriverDefaultsToPostgres(t *testing.T) {
	cfg := &Config{}
	if got := cfg.MetadataDriver(); got != "postgres" {
		t.Fatalf("MetadataDriver() = %q, want postgres", got)
	}
}

func TestMetadataDriverNormalizesPostgresql(t *testing.T) {
	cfg := &Config{Providers: &ProvidersConfig{Metadata: "postgresql"}}
	if got := cfg.MetadataDriver(); got != "postgres" {
		t.Fatalf("MetadataDriver() = %q, want postgres", got)
	}
}

func TestMetadataDSNPrefersDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@db.example.test:5432/waverless?sslmode=disable")
	cfg := &Config{Postgres: PostgreSQLConfig{Host: "ignored", Database: "ignored"}}
	if got := cfg.MetadataDSN(); got != "postgres://user:pass@db.example.test:5432/waverless?sslmode=disable" {
		t.Fatalf("MetadataDSN() = %q", got)
	}
}

func TestMetadataDSNFallsBackToPostgresDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	cfg := &Config{Postgres: PostgreSQLConfig{
		Host: "db.internal", Port: 5432, User: "u", Password: "p", Database: "waverless",
	}}
	got := cfg.MetadataDSN()
	if got == "" || got == os.Getenv("DATABASE_URL") {
		t.Fatalf("MetadataDSN() = %q, expected libpq DSN from fields", got)
	}
}
