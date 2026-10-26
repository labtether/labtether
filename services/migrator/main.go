package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

func main() {
	// Large upgrades can spend minutes building indexes; the store applies
	// LABTETHER_DB_MIGRATION_TIMEOUT to the migration run itself, and this
	// outer deadline only adds headroom for connecting.
	ctx, cancel := context.WithTimeout(context.Background(), persistence.SchemaMigrationTimeout()+time.Minute)
	defer cancel()

	log.Println("labtether migrator: applying pending schema migrations")
	log.Println("labtether migrator: IMPORTANT — ensure you have a recent database backup before upgrading")
	log.Println("labtether migrator: run 'make db-backup' or 'scripts/db-backup.sh' to create one")

	databaseURL := envOrDefault("DATABASE_URL", persistence.DefaultDatabaseURL("localhost"))
	store, err := persistence.NewPostgresStore(ctx, databaseURL)
	if err != nil {
		log.Fatalf("migrator failed: %v", err)
	}
	defer store.Close()

	log.Printf("migrations applied successfully")
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
