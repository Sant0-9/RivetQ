package rest

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rivetq/rivetq/internal/queue"
	"github.com/rs/zerolog/log"
)

// Server provides REST API
type Server struct {
	manager *queue.Manager
	router  *chi.Mux
}

// NewServer creates a new REST server
func NewServer(manager *queue.Manager) *Server {
	s := &Server{
		manager: manager,
		router:  chi.NewRouter(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.RequestID)
	s.router.Use(corsMiddleware)

	// API routes
	s.router.Route("/v1/queues", func(r chi.Router) {
		r.Get("/", s.listQueues)
		
		r.Route("/{queue}", func(r chi.Router) {
			r.Post("/enqueue", s.enqueue)
			r.Post("/lease", s.lease)
			r.Get("/stats", s.stats)
			r.Post("/rate_limit", s.setRateLimit)
			r.Get("/rate_limit", s.getRateLimit)
		})
	})

	s.router.Post("/v1/ack", s.ack)
	s.router.Post("/v1/nack", s.nack)

	// Health check
	s.router.Get("/healthz", s.health)
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.router
}

// Request/Response types
type EnqueueRequest struct {
	Payload        json.RawMessage   `json:"payload"`
	Headers        map[string]string `json:"headers,omitempty"`
	Priority       uint8             `json:"priority,omitempty"`
	DelayMs        int64             `json:"delay_ms,omitempty"`
	MaxRetries     uint32            `json:"max_retries,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

type EnqueueResponse struct {
	JobID string `json:"job_id"`
}

type LeaseRequest struct {
	MaxJobs      int   `json:"max_jobs,omitempty"`
	VisibilityMs int64 `json:"visibility_ms,omitempty"`
}

type LeaseResponse struct {
	Jobs []JobResponse `json:"jobs"`
}

type JobResponse struct {
	ID       string            `json:"id"`
	Queue    string            `json:"queue"`
	Payload  json.RawMessage   `json:"payload"`
	Headers  map[string]string `json:"headers,omitempty"`
	Priority uint8             `json:"priority"`
	Tries    uint32            `json:"tries"`
	LeaseID  string            `json:"lease_id"`
}

type AckRequest struct {
	JobID   string `json:"job_id"`
	LeaseID string `json:"lease_id"`
}

type AckResponse struct {
	Success bool `json:"success"`
}

type NackRequest struct {
	JobID   string `json:"job_id"`
	LeaseID string `json:"lease_id"`
	Reason  string `json:"reason,omitempty"`
}

type NackResponse struct {
	Success bool `json:"success"`
}

type StatsResponse struct {
	Ready    int `json:"ready"`
	Inflight int `json:"inflight"`
	DLQ      int `json:"dlq"`
}

type RateLimitRequest struct {
	Capacity   float64 `json:"capacity"`
	RefillRate float64 `json:"refill_rate"`
}

type RateLimitResponse struct {
	Capacity   float64 `json:"capacity"`
	RefillRate float64 `json:"refill_rate"`
	Exists     bool    `json:"exists"`
}

// Handlers
func (s *Server) enqueue(w http.ResponseWriter, r *http.Request) {
	queueName := chi.URLParam(r, "queue")

	var req EnqueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	retryPolicy := queue.DefaultRetryPolicy()
	if req.MaxRetries > 0 {
		retryPolicy.MaxRetries = req.MaxRetries
	}

	jobID, err := s.manager.Enqueue(
		queueName,
		[]byte(req.Payload),
		req.Headers,
		req.Priority,
		req.DelayMs,
		retryPolicy,
		req.IdempotencyKey,
	)
	if err != nil {
		log.Error().Err(err).Msg("failed to enqueue job")
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, EnqueueResponse{JobID: jobID})
}

func (s *Server) lease(w http.ResponseWriter, r *http.Request) {
	queueName := chi.URLParam(r, "queue")

	var req LeaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.MaxJobs = 1
		req.VisibilityMs = 30000 // Default 30 seconds
	}

	if req.MaxJobs == 0 {
		req.MaxJobs = 1
	}
	if req.VisibilityMs == 0 {
		req.VisibilityMs = 30000
	}

	jobs, err := s.manager.Lease(queueName, req.MaxJobs, req.VisibilityMs)
	if err != nil {
		log.Error().Err(err).Msg("failed to lease jobs")
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	jobResponses := make([]JobResponse, len(jobs))
	for i, job := range jobs {
		jobResponses[i] = JobResponse{
			ID:       job.ID,
			Queue:    job.Queue,
			Payload:  json.RawMessage(job.Payload),
			Headers:  job.Headers,
			Priority: job.Priority,
			Tries:    job.Tries,
			LeaseID:  job.LeaseID,
		}
	}

	respondJSON(w, http.StatusOK, LeaseResponse{Jobs: jobResponses})
}

func (s *Server) ack(w http.ResponseWriter, r *http.Request) {
	var req AckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := s.manager.Ack(req.JobID, req.LeaseID)
	if err != nil {
		log.Error().Err(err).Msg("failed to ack job")
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, AckResponse{Success: true})
}

func (s *Server) nack(w http.ResponseWriter, r *http.Request) {
	var req NackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	err := s.manager.Nack(req.JobID, req.LeaseID, req.Reason)
	if err != nil {
		log.Error().Err(err).Msg("failed to nack job")
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, NackResponse{Success: true})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	queueName := chi.URLParam(r, "queue")

	ready, inflight, dlq, err := s.manager.Stats(queueName)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, StatsResponse{
		Ready:    ready,
		Inflight: inflight,
		DLQ:      dlq,
	})
}

func (s *Server) listQueues(w http.ResponseWriter, r *http.Request) {
	queues := s.manager.ListQueues()
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"queues": queues,
	})
}

func (s *Server) setRateLimit(w http.ResponseWriter, r *http.Request) {
	queueName := chi.URLParam(r, "queue")

	var req RateLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.manager.SetRateLimit(queueName, req.Capacity, req.RefillRate)
	respondJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (s *Server) getRateLimit(w http.ResponseWriter, r *http.Request) {
	queueName := chi.URLParam(r, "queue")

	capacity, refillRate, exists := s.manager.GetRateLimit(queueName)
	respondJSON(w, http.StatusOK, RateLimitResponse{
		Capacity:   capacity,
		RefillRate: refillRate,
		Exists:     exists,
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

// Helper functions
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
