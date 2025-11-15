package service

import (
	"database/sql"
)

// Statistics represents statistics about the service
type Statistics struct {
	UserAssignments    []UserAssignmentStats `json:"user_assignments"`
	PRStats            PRStatistics          `json:"pr_statistics"`
}

// UserAssignmentStats represents assignment statistics for a user
type UserAssignmentStats struct {
	UserID          string `json:"user_id"`
	Username        string `json:"username"`
	TotalAssignments int   `json:"total_assignments"`
	OpenPRs         int   `json:"open_prs"`
	MergedPRs       int   `json:"merged_prs"`
}

// PRStatistics represents overall PR statistics
type PRStatistics struct {
	TotalPRs        int `json:"total_prs"`
	OpenPRs         int `json:"open_prs"`
	MergedPRs       int `json:"merged_prs"`
	PRsWithReviewers int `json:"prs_with_reviewers"`
	PRsWithoutReviewers int `json:"prs_without_reviewers"`
}

// GetStatistics returns statistics about assignments and PRs
func (s *Service) GetStatistics() (*Statistics, error) {
	// Get user assignment statistics
	rows, err := s.db.Query(`
		SELECT 
			u.user_id,
			u.username,
			COUNT(prr.reviewer_id) as total_assignments,
			COUNT(CASE WHEN pr.status = 'OPEN' THEN 1 END) as open_prs,
			COUNT(CASE WHEN pr.status = 'MERGED' THEN 1 END) as merged_prs
		FROM users u
		LEFT JOIN pr_reviewers prr ON u.user_id = prr.reviewer_id
		LEFT JOIN pull_requests pr ON prr.pull_request_id = pr.pull_request_id
		GROUP BY u.user_id, u.username
		ORDER BY total_assignments DESC, u.username
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userStats []UserAssignmentStats
	for rows.Next() {
		var stat UserAssignmentStats
		if err := rows.Scan(&stat.UserID, &stat.Username, &stat.TotalAssignments, &stat.OpenPRs, &stat.MergedPRs); err != nil {
			return nil, err
		}
		userStats = append(userStats, stat)
	}

	// Get PR statistics
	var prStats PRStatistics
	err = s.db.QueryRow(`
		SELECT 
			COUNT(*) as total_prs,
			COUNT(CASE WHEN status = 'OPEN' THEN 1 END) as open_prs,
			COUNT(CASE WHEN status = 'MERGED' THEN 1 END) as merged_prs,
			COUNT(CASE WHEN EXISTS(SELECT 1 FROM pr_reviewers WHERE pull_request_id = pr.pull_request_id) THEN 1 END) as prs_with_reviewers,
			COUNT(CASE WHEN NOT EXISTS(SELECT 1 FROM pr_reviewers WHERE pull_request_id = pr.pull_request_id) THEN 1 END) as prs_without_reviewers
		FROM pull_requests pr
	`).Scan(
		&prStats.TotalPRs,
		&prStats.OpenPRs,
		&prStats.MergedPRs,
		&prStats.PRsWithReviewers,
		&prStats.PRsWithoutReviewers,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	return &Statistics{
		UserAssignments: userStats,
		PRStats:        prStats,
	}, nil
}

