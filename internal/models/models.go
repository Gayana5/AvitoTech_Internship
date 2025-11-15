package models

import "time"

// User represents a user in the system
type User struct {
	UserID   string `json:"user_id" db:"user_id"`
	Username string `json:"username" db:"username"`
	TeamName string `json:"team_name" db:"team_name"`
	IsActive bool   `json:"is_active" db:"is_active"`
}

// Team represents a team with its members
type Team struct {
	TeamName string      `json:"team_name"`
	Members  []TeamMember `json:"members"`
}

// TeamMember represents a member of a team
type TeamMember struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

// PullRequestStatus represents the status of a PR
type PullRequestStatus string

const (
	StatusOpen   PullRequestStatus = "OPEN"
	StatusMerged PullRequestStatus = "MERGED"
)

// PullRequest represents a pull request
type PullRequest struct {
	PullRequestID    string             `json:"pull_request_id" db:"pull_request_id"`
	PullRequestName  string             `json:"pull_request_name" db:"pull_request_name"`
	AuthorID         string             `json:"author_id" db:"author_id"`
	Status           PullRequestStatus  `json:"status" db:"status"`
	AssignedReviewers []string          `json:"assigned_reviewers"`
	CreatedAt        *time.Time         `json:"createdAt,omitempty" db:"created_at"`
	MergedAt         *time.Time         `json:"mergedAt,omitempty" db:"merged_at"`
}

// PullRequestShort represents a short version of PR
type PullRequestShort struct {
	PullRequestID   string            `json:"pull_request_id"`
	PullRequestName string            `json:"pull_request_name"`
	AuthorID        string            `json:"author_id"`
	Status          PullRequestStatus `json:"status"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error code and message
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

