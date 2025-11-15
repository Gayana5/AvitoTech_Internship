package service

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/avito-tech/pr-reviewer-service/internal/database"
	"github.com/avito-tech/pr-reviewer-service/internal/models"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	connStr := os.Getenv("TEST_DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5432/pr_reviewer_test?sslmode=disable"
	}

	db, err := database.NewDB(connStr)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Run migrations
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Cleanup function
	cleanup := func() {
		// Clean up test data
		db.Exec("DELETE FROM pr_reviewers")
		db.Exec("DELETE FROM pull_requests")
		db.Exec("DELETE FROM users")
		db.Exec("DELETE FROM teams")
		db.Close()
	}

	return db.DB, cleanup
}

func TestCreateTeam(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	svc := NewService(db)

	team := models.Team{
		TeamName: "backend",
		Members: []models.TeamMember{
			{UserID: "u1", Username: "Alice", IsActive: true},
			{UserID: "u2", Username: "Bob", IsActive: true},
		},
	}

	err := svc.CreateTeam(team)
	if err != nil {
		t.Fatalf("Failed to create team: %v", err)
	}

	// Try to create again - should fail
	err = svc.CreateTeam(team)
	if err == nil {
		t.Fatal("Expected error when creating duplicate team")
	}
	if !IsErrorCode(err, "TEAM_EXISTS") {
		t.Fatalf("Expected TEAM_EXISTS error, got: %v", err)
	}
}

func TestCreatePullRequest(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	svc := NewService(db)

	// Create team first
	team := models.Team{
		TeamName: "backend",
		Members: []models.TeamMember{
			{UserID: "u1", Username: "Alice", IsActive: true},
			{UserID: "u2", Username: "Bob", IsActive: true},
			{UserID: "u3", Username: "Charlie", IsActive: true},
		},
	}
	if err := svc.CreateTeam(team); err != nil {
		t.Fatalf("Failed to create team: %v", err)
	}

	// Create PR
	pr, err := svc.CreatePullRequest("pr-1", "Test PR", "u1")
	if err != nil {
		t.Fatalf("Failed to create PR: %v", err)
	}

	if pr.PullRequestID != "pr-1" {
		t.Errorf("Expected PR ID pr-1, got %s", pr.PullRequestID)
	}

	if len(pr.AssignedReviewers) == 0 {
		t.Error("Expected at least one reviewer")
	}

	if len(pr.AssignedReviewers) > 2 {
		t.Errorf("Expected at most 2 reviewers, got %d", len(pr.AssignedReviewers))
	}

	// Author should not be in reviewers
	for _, reviewer := range pr.AssignedReviewers {
		if reviewer == "u1" {
			t.Error("Author should not be assigned as reviewer")
		}
	}
}

func TestMergePullRequest(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	svc := NewService(db)

	// Setup
	team := models.Team{
		TeamName: "backend",
		Members: []models.TeamMember{
			{UserID: "u1", Username: "Alice", IsActive: true},
			{UserID: "u2", Username: "Bob", IsActive: true},
		},
	}
	if err := svc.CreateTeam(team); err != nil {
		t.Fatalf("Failed to create team: %v", err)
	}

	pr, err := svc.CreatePullRequest("pr-1", "Test PR", "u1")
	if err != nil {
		t.Fatalf("Failed to create PR: %v", err)
	}

	// Merge PR
	mergedPR, err := svc.MergePullRequest("pr-1")
	if err != nil {
		t.Fatalf("Failed to merge PR: %v", err)
	}

	if mergedPR.Status != models.StatusMerged {
		t.Errorf("Expected status MERGED, got %s", mergedPR.Status)
	}

	if mergedPR.MergedAt == nil {
		t.Error("Expected mergedAt to be set")
	}

	// Merge again - should be idempotent
	mergedPR2, err := svc.MergePullRequest("pr-1")
	if err != nil {
		t.Fatalf("Failed to merge PR again: %v", err)
	}

	if mergedPR2.Status != models.StatusMerged {
		t.Errorf("Expected status MERGED on second merge, got %s", mergedPR2.Status)
	}
}

func TestReassignReviewer(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	svc := NewService(db)

	// Setup
	team := models.Team{
		TeamName: "backend",
		Members: []models.TeamMember{
			{UserID: "u1", Username: "Alice", IsActive: true},
			{UserID: "u2", Username: "Bob", IsActive: true},
			{UserID: "u3", Username: "Charlie", IsActive: true},
		},
	}
	if err := svc.CreateTeam(team); err != nil {
		t.Fatalf("Failed to create team: %v", err)
	}

	pr, err := svc.CreatePullRequest("pr-1", "Test PR", "u1")
	if err != nil {
		t.Fatalf("Failed to create PR: %v", err)
	}

	if len(pr.AssignedReviewers) == 0 {
		t.Fatal("No reviewers assigned")
	}

	oldReviewer := pr.AssignedReviewers[0]

	// Reassign
	newPR, newReviewer, err := svc.ReassignReviewer("pr-1", oldReviewer)
	if err != nil {
		t.Fatalf("Failed to reassign reviewer: %v", err)
	}

	if newReviewer == oldReviewer {
		t.Error("New reviewer should be different from old reviewer")
	}

	// Check new reviewer is in the list
	found := false
	for _, reviewer := range newPR.AssignedReviewers {
		if reviewer == newReviewer {
			found = true
			break
		}
	}
	if !found {
		t.Error("New reviewer should be in assigned reviewers list")
	}

	// Old reviewer should not be in the list
	for _, reviewer := range newPR.AssignedReviewers {
		if reviewer == oldReviewer {
			t.Error("Old reviewer should not be in assigned reviewers list")
		}
	}
}

func TestReassignOnMergedPR(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	svc := NewService(db)

	// Setup
	team := models.Team{
		TeamName: "backend",
		Members: []models.TeamMember{
			{UserID: "u1", Username: "Alice", IsActive: true},
			{UserID: "u2", Username: "Bob", IsActive: true},
			{UserID: "u3", Username: "Charlie", IsActive: true},
		},
	}
	if err := svc.CreateTeam(team); err != nil {
		t.Fatalf("Failed to create team: %v", err)
	}

	pr, err := svc.CreatePullRequest("pr-1", "Test PR", "u1")
	if err != nil {
		t.Fatalf("Failed to create PR: %v", err)
	}

	// Merge PR
	if _, err := svc.MergePullRequest("pr-1"); err != nil {
		t.Fatalf("Failed to merge PR: %v", err)
	}

	// Try to reassign - should fail
	_, _, err = svc.ReassignReviewer("pr-1", pr.AssignedReviewers[0])
	if err == nil {
		t.Fatal("Expected error when reassigning on merged PR")
	}
	if !IsErrorCode(err, "PR_MERGED") {
		t.Fatalf("Expected PR_MERGED error, got: %v", err)
	}
}

