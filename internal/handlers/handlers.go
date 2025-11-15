package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/avito-tech/pr-reviewer-service/internal/models"
	"github.com/avito-tech/pr-reviewer-service/internal/service"
	"github.com/gorilla/mux"
)

type Handlers struct {
	service *service.Service
}

func NewHandlers(svc *service.Service) *Handlers {
	return &Handlers{service: svc}
}

func (h *Handlers) CreateTeam(w http.ResponseWriter, r *http.Request) {
	var team models.Team
	if err := json.NewDecoder(r.Body).Decode(&team); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if err := h.service.CreateTeam(team); err != nil {
		code := service.GetErrorCode(err)
		if code == "TEAM_EXISTS" {
			h.writeError(w, http.StatusBadRequest, code, service.GetErrorMessage(err))
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	// Return the created team
	createdTeam, err := h.service.GetTeam(team.TeamName)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"team": createdTeam,
	})
}

func (h *Handlers) GetTeam(w http.ResponseWriter, r *http.Request) {
	teamName := r.URL.Query().Get("team_name")
	if teamName == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "team_name is required")
		return
	}

	team, err := h.service.GetTeam(teamName)
	if err != nil {
		code := service.GetErrorCode(err)
		if code == "NOT_FOUND" {
			h.writeError(w, http.StatusNotFound, code, service.GetErrorMessage(err))
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(team)
}

func (h *Handlers) SetUserActive(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID   string `json:"user_id"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	user, err := h.service.SetUserActive(req.UserID, req.IsActive)
	if err != nil {
		code := service.GetErrorCode(err)
		if code == "NOT_FOUND" {
			h.writeError(w, http.StatusNotFound, code, service.GetErrorMessage(err))
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": user,
	})
}

func (h *Handlers) CreatePullRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PullRequestID   string `json:"pull_request_id"`
		PullRequestName string `json:"pull_request_name"`
		AuthorID        string `json:"author_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	pr, err := h.service.CreatePullRequest(req.PullRequestID, req.PullRequestName, req.AuthorID)
	if err != nil {
		code := service.GetErrorCode(err)
		if code == "PR_EXISTS" {
			h.writeError(w, http.StatusConflict, code, service.GetErrorMessage(err))
			return
		}
		if code == "NOT_FOUND" {
			h.writeError(w, http.StatusNotFound, code, service.GetErrorMessage(err))
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pr": pr,
	})
}

func (h *Handlers) MergePullRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PullRequestID string `json:"pull_request_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	pr, err := h.service.MergePullRequest(req.PullRequestID)
	if err != nil {
		code := service.GetErrorCode(err)
		if code == "NOT_FOUND" {
			h.writeError(w, http.StatusNotFound, code, service.GetErrorMessage(err))
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pr": pr,
	})
}

func (h *Handlers) ReassignReviewer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PullRequestID string `json:"pull_request_id"`
		OldUserID     string `json:"old_user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	pr, replacedBy, err := h.service.ReassignReviewer(req.PullRequestID, req.OldUserID)
	if err != nil {
		code := service.GetErrorCode(err)
		if code == "NOT_FOUND" || code == "PR_MERGED" || code == "NOT_ASSIGNED" || code == "NO_CANDIDATE" {
			statusCode := http.StatusNotFound
			if code == "PR_MERGED" || code == "NOT_ASSIGNED" || code == "NO_CANDIDATE" {
				statusCode = http.StatusConflict
			}
			h.writeError(w, statusCode, code, service.GetErrorMessage(err))
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pr":         pr,
		"replaced_by": replacedBy,
	})
}

func (h *Handlers) GetUserReviewPRs(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "user_id is required")
		return
	}

	prs, err := h.service.GetUserReviewPRs(userID)
	if err != nil {
		code := service.GetErrorCode(err)
		if code == "NOT_FOUND" {
			h.writeError(w, http.StatusNotFound, code, service.GetErrorMessage(err))
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":       userID,
		"pull_requests": prs,
	})
}

func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (h *Handlers) writeError(w http.ResponseWriter, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(models.ErrorResponse{
		Error: models.ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

func (h *Handlers) GetStatistics(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.GetStatistics()
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (h *Handlers) BulkDeactivateUsers(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TeamName string   `json:"team_name"`
		UserIDs  []string `json:"user_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if err := h.service.BulkDeactivateUsers(req.TeamName, req.UserIDs); err != nil {
		code := service.GetErrorCode(err)
		if code == "NOT_FOUND" {
			h.writeError(w, http.StatusNotFound, code, service.GetErrorMessage(err))
			return
		}
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	// Safe reassign open PRs
	reassignedCount, err := h.service.SafeReassignOpenPRs(req.UserIDs)
	if err != nil {
		// Log but don't fail the request
		log.Printf("Warning: failed to safely reassign PRs: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deactivated_count": len(req.UserIDs),
		"reassigned_prs":    reassignedCount,
	})
}

func (h *Handlers) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/team/add", h.CreateTeam).Methods("POST")
	router.HandleFunc("/team/get", h.GetTeam).Methods("GET")
	router.HandleFunc("/users/setIsActive", h.SetUserActive).Methods("POST")
	router.HandleFunc("/pullRequest/create", h.CreatePullRequest).Methods("POST")
	router.HandleFunc("/pullRequest/merge", h.MergePullRequest).Methods("POST")
	router.HandleFunc("/pullRequest/reassign", h.ReassignReviewer).Methods("POST")
	router.HandleFunc("/users/getReview", h.GetUserReviewPRs).Methods("GET")
	router.HandleFunc("/health", h.HealthCheck).Methods("GET")
	router.HandleFunc("/stats", h.GetStatistics).Methods("GET")
	router.HandleFunc("/users/bulkDeactivate", h.BulkDeactivateUsers).Methods("POST")
}

