package main

import (
	"context"
	"fmt"
	database "github.com/robertguss/go-islands-starter/db"
	"os"
	"time"
)

func main() {
	url := os.Getenv("MIGRATION_DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "MIGRATION_DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := database.Migrate(ctx, url); err != nil {
		fmt.Fprintln(os.Stderr, "Migration failed. Verify operator credentials, database availability, and schema history.")
		os.Exit(1)
	}
	fmt.Println("Application and River migrations applied.")
}
