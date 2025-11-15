package service

import (
	"database/sql"
	"fmt"
)

// BulkDeactivateUsers deactivates multiple users in a team
func (s *Service) BulkDeactivateUsers(teamName string, userIDs []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if team exists
	var exists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)", teamName).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("NOT_FOUND: team not found")
	}

	// Deactivate users
	for _, userID := range userIDs {
		// Verify user belongs to team
		var userTeam string
		err = tx.QueryRow("SELECT team_name FROM users WHERE user_id = $1", userID).Scan(&userTeam)
		if err == sql.ErrNoRows {
			continue // Skip non-existent users
		}
		if err != nil {
			return err
		}
		if userTeam != teamName {
			continue // Skip users not in the team
		}

		_, err = tx.Exec(`
			UPDATE users
			SET is_active = false, updated_at = CURRENT_TIMESTAMP
			WHERE user_id = $1
		`, userID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// SafeReassignOpenPRs reassigns reviewers for open PRs when users are deactivated
// This ensures open PRs always have active reviewers
func (s *Service) SafeReassignOpenPRs(deactivatedUserIDs []string) (int, error) {
	if len(deactivatedUserIDs) == 0 {
		return 0, nil
	}

	// Build query to find open PRs with deactivated reviewers
	query := `
		SELECT DISTINCT pr.pull_request_id, prr.reviewer_id, u.team_name
		FROM pull_requests pr
		INNER JOIN pr_reviewers prr ON pr.pull_request_id = prr.pull_request_id
		INNER JOIN users u ON prr.reviewer_id = u.user_id
		WHERE pr.status = 'OPEN' AND prr.reviewer_id = ANY($1) AND u.is_active = false
	`

	rows, err := s.db.Query(query, deactivatedUserIDs)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type reassignInfo struct {
		prID    string
		oldID   string
		teamName string
	}

	var toReassign []reassignInfo
	for rows.Next() {
		var info reassignInfo
		if err := rows.Scan(&info.prID, &info.oldID, &info.teamName); err != nil {
			return 0, err
		}
		toReassign = append(toReassign, info)
	}

	reassignedCount := 0
	for _, info := range toReassign {
		// Try to reassign - if it fails, we continue (no candidate available)
		_, _, err := s.ReassignReviewer(info.prID, info.oldID)
		if err == nil {
			reassignedCount++
		}
		// Ignore errors (NO_CANDIDATE, etc.) - we just continue
	}

	return reassignedCount, nil
}

