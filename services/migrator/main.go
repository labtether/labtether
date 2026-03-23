package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/labtether/labtether/internal/persistence"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
