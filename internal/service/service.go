package service

import (
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/avito-tech/pr-reviewer-service/internal/models"
)

type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// CreateTeam creates a team and its members
func (s *Service) CreateTeam(team models.Team) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if team already exists
	var exists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)", team.TeamName).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("TEAM_EXISTS: team_name already exists")
	}

	// Create team
	_, err = tx.Exec("INSERT INTO teams (team_name) VALUES ($1)", team.TeamName)
	if err != nil {
		return err
	}

	// Create/update users
	for _, member := range team.Members {
		_, err = tx.Exec(`
			INSERT INTO users (user_id, username, team_name, is_active)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id) 
			DO UPDATE SET username = EXCLUDED.username, team_name = EXCLUDED.team_name, is_active = EXCLUDED.is_active, updated_at = CURRENT_TIMESTAMP
		`, member.UserID, member.Username, team.TeamName, member.IsActive)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetTeam retrieves a team with its members
func (s *Service) GetTeam(teamName string) (*models.Team, error) {
	// Check if team exists
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)", teamName).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("NOT_FOUND: team not found")
	}

	// Get team members
	rows, err := s.db.Query(`
		SELECT user_id, username, is_active
		FROM users
		WHERE team_name = $1
		ORDER BY user_id
	`, teamName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.TeamMember
	for rows.Next() {
		var member models.TeamMember
		if err := rows.Scan(&member.UserID, &member.Username, &member.IsActive); err != nil {
			return nil, err
		}
		members = append(members, member)
	}

	return &models.Team{
		TeamName: teamName,
		Members:  members,
	}, nil
}

// SetUserActive sets the active status of a user
func (s *Service) SetUserActive(userID string, isActive bool) (*models.User, error) {
	var user models.User
	err := s.db.QueryRow(`
		UPDATE users
		SET is_active = $1, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2
		RETURNING user_id, username, team_name, is_active
	`, isActive, userID).Scan(&user.UserID, &user.Username, &user.TeamName, &user.IsActive)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("NOT_FOUND: user not found")
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// CreatePullRequest creates a PR and assigns reviewers
func (s *Service) CreatePullRequest(prID, prName, authorID string) (*models.PullRequest, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check if PR already exists
	var exists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM pull_requests WHERE pull_request_id = $1)", prID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("PR_EXISTS: PR id already exists")
	}

	// Get author's team
	var teamName string
	err = tx.QueryRow("SELECT team_name FROM users WHERE user_id = $1", authorID).Scan(&teamName)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("NOT_FOUND: author not found")
	}
	if err != nil {
		return nil, err
	}

	// Create PR
	now := time.Now()
	_, err = tx.Exec(`
		INSERT INTO pull_requests (pull_request_id, pull_request_name, author_id, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, prID, prName, authorID, models.StatusOpen, now)
	if err != nil {
		return nil, err
	}

	// Get active reviewers from author's team (excluding author)
	rows, err := tx.Query(`
		SELECT user_id
		FROM users
		WHERE team_name = $1 AND is_active = true AND user_id != $2
		ORDER BY user_id
	`, teamName, authorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		candidates = append(candidates, userID)
	}

	// Assign up to 2 reviewers randomly
	reviewers := s.selectRandomReviewers(candidates, 2)
	for _, reviewerID := range reviewers {
		_, err = tx.Exec(`
			INSERT INTO pr_reviewers (pull_request_id, reviewer_id)
			VALUES ($1, $2)
		`, prID, reviewerID)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Fetch the created PR
	return s.GetPullRequest(prID)
}

// GetPullRequest retrieves a PR with its reviewers
func (s *Service) GetPullRequest(prID string) (*models.PullRequest, error) {
	var pr models.PullRequest
	var createdAt, mergedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		FROM pull_requests
		WHERE pull_request_id = $1
	`, prID).Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status, &createdAt, &mergedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("NOT_FOUND: PR not found")
	}
	if err != nil {
		return nil, err
	}

	if createdAt.Valid {
		pr.CreatedAt = &createdAt.Time
	}
	if mergedAt.Valid {
		pr.MergedAt = &mergedAt.Time
	}

	// Get reviewers
	rows, err := s.db.Query(`
		SELECT reviewer_id
		FROM pr_reviewers
		WHERE pull_request_id = $1
		ORDER BY reviewer_id
	`, prID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var reviewerID string
		if err := rows.Scan(&reviewerID); err != nil {
			return nil, err
		}
		pr.AssignedReviewers = append(pr.AssignedReviewers, reviewerID)
	}

	return &pr, nil
}

// MergePullRequest marks a PR as merged (idempotent)
func (s *Service) MergePullRequest(prID string) (*models.PullRequest, error) {
	// Check if PR exists
	var currentStatus string
	var mergedAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT status, merged_at
		FROM pull_requests
		WHERE pull_request_id = $1
	`, prID).Scan(&currentStatus, &mergedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("NOT_FOUND: PR not found")
	}
	if err != nil {
		return nil, err
	}

	// If already merged, just return it
	if currentStatus == string(models.StatusMerged) {
		return s.GetPullRequest(prID)
	}

	// Merge the PR
	now := time.Now()
	_, err = s.db.Exec(`
		UPDATE pull_requests
		SET status = $1, merged_at = $2
		WHERE pull_request_id = $3
	`, models.StatusMerged, now, prID)
	if err != nil {
		return nil, err
	}

	return s.GetPullRequest(prID)
}

// ReassignReviewer reassigns a reviewer
func (s *Service) ReassignReviewer(prID, oldUserID string) (*models.PullRequest, string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()

	// Get PR
	var pr models.PullRequest
	var status string
	err = tx.QueryRow(`
		SELECT pull_request_id, pull_request_name, author_id, status
		FROM pull_requests
		WHERE pull_request_id = $1
	`, prID).Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &status)

	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("NOT_FOUND: PR not found")
	}
	if err != nil {
		return nil, "", err
	}

	// Check if PR is merged
	if status == string(models.StatusMerged) {
		return nil, "", fmt.Errorf("PR_MERGED: cannot reassign on merged PR")
	}

	// Check if old reviewer is assigned
	var isAssigned bool
	err = tx.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM pr_reviewers WHERE pull_request_id = $1 AND reviewer_id = $2)
	`, prID, oldUserID).Scan(&isAssigned)
	if err != nil {
		return nil, "", err
	}
	if !isAssigned {
		return nil, "", fmt.Errorf("NOT_ASSIGNED: reviewer is not assigned to this PR")
	}

	// Get old reviewer's team
	var teamName string
	err = tx.QueryRow("SELECT team_name FROM users WHERE user_id = $1", oldUserID).Scan(&teamName)
	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("NOT_FOUND: old reviewer not found")
	}
	if err != nil {
		return nil, "", err
	}

	// Get current reviewers to exclude them
	var currentReviewers []string
	rows, err := tx.Query(`
		SELECT reviewer_id
		FROM pr_reviewers
		WHERE pull_request_id = $1
	`, prID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	for rows.Next() {
		var reviewerID string
		if err := rows.Scan(&reviewerID); err != nil {
			return nil, "", err
		}
		currentReviewers = append(currentReviewers, reviewerID)
	}
	rows.Close()

	// Get active candidates from old reviewer's team (excluding current reviewers and old reviewer)
	query := `
		SELECT user_id
		FROM users
		WHERE team_name = $1 AND is_active = true AND user_id != $2
	`
	args := []interface{}{teamName, oldUserID}
	for i, reviewerID := range currentReviewers {
		if reviewerID != oldUserID {
			query += fmt.Sprintf(" AND user_id != $%d", i+3)
			args = append(args, reviewerID)
		}
	}
	query += " ORDER BY user_id"

	rows, err = tx.Query(query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var candidates []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, "", err
		}
		candidates = append(candidates, userID)
	}

	if len(candidates) == 0 {
		return nil, "", fmt.Errorf("NO_CANDIDATE: no active replacement candidate in team")
	}

	// Select random replacement
	newReviewerID := candidates[rand.Intn(len(candidates))]

	// Replace reviewer
	_, err = tx.Exec(`
		UPDATE pr_reviewers
		SET reviewer_id = $1
		WHERE pull_request_id = $2 AND reviewer_id = $3
	`, newReviewerID, prID, oldUserID)
	if err != nil {
		return nil, "", err
	}

	if err := tx.Commit(); err != nil {
		return nil, "", err
	}

	updatedPR, err := s.GetPullRequest(prID)
	if err != nil {
		return nil, "", err
	}

	return updatedPR, newReviewerID, nil
}

// GetUserReviewPRs gets PRs assigned to a user
func (s *Service) GetUserReviewPRs(userID string) ([]models.PullRequestShort, error) {
	// Check if user exists
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE user_id = $1)", userID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("NOT_FOUND: user not found")
	}

	// Get PRs
	rows, err := s.db.Query(`
		SELECT pr.pull_request_id, pr.pull_request_name, pr.author_id, pr.status
		FROM pull_requests pr
		INNER JOIN pr_reviewers prr ON pr.pull_request_id = prr.pull_request_id
		WHERE prr.reviewer_id = $1
		ORDER BY pr.created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prs []models.PullRequestShort
	for rows.Next() {
		var pr models.PullRequestShort
		if err := rows.Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status); err != nil {
			return nil, err
		}
		prs = append(prs, pr)
	}

	return prs, nil
}

// selectRandomReviewers selects up to n random reviewers from candidates
func (s *Service) selectRandomReviewers(candidates []string, n int) []string {
	if len(candidates) == 0 {
		return []string{}
	}

	if len(candidates) <= n {
		// Shuffle and return all
		shuffled := make([]string, len(candidates))
		copy(shuffled, candidates)
		rand.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		return shuffled
	}

	// Select n random reviewers
	selected := make([]string, 0, n)
	indices := rand.Perm(len(candidates))
	for i := 0; i < n && i < len(indices); i++ {
		selected = append(selected, candidates[indices[i]])
	}
	return selected
}

// Helper function to check error type
func IsErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return len(errStr) > len(code) && errStr[:len(code)] == code
}

func GetErrorCode(err error) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()
	colonIdx := -1
	for i, r := range errStr {
		if r == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx == -1 {
		return ""
	}
	return errStr[:colonIdx]
}

func GetErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()
	colonIdx := -1
	for i, r := range errStr {
		if r == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx == -1 {
		return errStr
	}
	if colonIdx+2 < len(errStr) {
		return errStr[colonIdx+2:]
	}
	return ""
}

