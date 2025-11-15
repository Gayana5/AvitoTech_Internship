package main

import (
	"log"
	"net/http"
	"os"

	"github.com/avito-tech/pr-reviewer-service/internal/database"
	"github.com/avito-tech/pr-reviewer-service/internal/handlers"
	"github.com/avito-tech/pr-reviewer-service/internal/service"
	"github.com/gorilla/mux"
)

func main() {
	// Get database connection string from environment
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5432/pr_reviewer?sslmode=disable"
	}

	// Connect to database
	db, err := database.NewDB(connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.RunMigrations(); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Create service
	svc := service.NewService(db.DB)

	// Create handlers
	h := handlers.NewHandlers(svc)

	// Setup router
	router := mux.NewRouter()
	h.RegisterRoutes(router)

	// Get port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

