package api

import (
	"context"

	pb "github.com/rivetq/rivetq/api/gen"
	"github.com/rivetq/rivetq/internal/queue"
)

// GRPCServer implements the gRPC QueueService
type GRPCServer struct {
	pb.UnimplementedQueueServiceServer
	manager *queue.Manager
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(manager *queue.Manager) *GRPCServer {
	return &GRPCServer{
		manager: manager,
	}
}

// Enqueue implements QueueService.Enqueue
func (s *GRPCServer) Enqueue(ctx context.Context, req *pb.EnqueueRequest) (*pb.EnqueueResponse, error) {
	retryPolicy := queue.DefaultRetryPolicy()
	if req.RetryPolicy != nil {
		retryPolicy.MaxRetries = req.RetryPolicy.MaxRetries
	}

	jobID, err := s.manager.Enqueue(
		req.QueueName,
		req.Payload,
		req.Headers,
		uint8(req.Priority),
		req.DelayMs,
		retryPolicy,
		req.IdempotencyKey,
	)
	if err != nil {
		return nil, err
	}

	return &pb.EnqueueResponse{JobId: jobID}, nil
}

// Lease implements QueueService.Lease
func (s *GRPCServer) Lease(ctx context.Context, req *pb.LeaseRequest) (*pb.LeaseResponse, error) {
	jobs, err := s.manager.Lease(req.QueueName, int(req.MaxJobs), req.VisibilityMs)
	if err != nil {
		return nil, err
	}

	pbJobs := make([]*pb.Job, len(jobs))
	for i, job := range jobs {
		pbJobs[i] = &pb.Job{
			Id:       job.ID,
			Queue:    job.Queue,
			Payload:  job.Payload,
			Headers:  job.Headers,
			Priority: uint32(job.Priority),
			Tries:    job.Tries,
			LeaseId:  job.LeaseID,
		}
	}

	return &pb.LeaseResponse{Jobs: pbJobs}, nil
}

// Ack implements QueueService.Ack
func (s *GRPCServer) Ack(ctx context.Context, req *pb.AckRequest) (*pb.AckResponse, error) {
	err := s.manager.Ack(req.JobId, req.LeaseId)
	return &pb.AckResponse{Success: err == nil}, err
}

// Nack implements QueueService.Nack
func (s *GRPCServer) Nack(ctx context.Context, req *pb.NackRequest) (*pb.NackResponse, error) {
	err := s.manager.Nack(req.JobId, req.LeaseId, req.Reason)
	return &pb.NackResponse{Success: err == nil}, err
}

// Stats implements QueueService.Stats
func (s *GRPCServer) Stats(ctx context.Context, req *pb.StatsRequest) (*pb.StatsResponse, error) {
	ready, inflight, dlq, err := s.manager.Stats(req.QueueName)
	if err != nil {
		return nil, err
	}

	return &pb.StatsResponse{
		Ready:    int32(ready),
		Inflight: int32(inflight),
		Dlq:      int32(dlq),
	}, nil
}

// ListQueues implements QueueService.ListQueues
func (s *GRPCServer) ListQueues(ctx context.Context, req *pb.ListQueuesRequest) (*pb.ListQueuesResponse, error) {
	queues := s.manager.ListQueues()
	return &pb.ListQueuesResponse{Queues: queues}, nil
}

// SetRateLimit implements QueueService.SetRateLimit
func (s *GRPCServer) SetRateLimit(ctx context.Context, req *pb.SetRateLimitRequest) (*pb.SetRateLimitResponse, error) {
	s.manager.SetRateLimit(req.QueueName, req.Capacity, req.RefillRate)
	return &pb.SetRateLimitResponse{Success: true}, nil
}

// GetRateLimit implements QueueService.GetRateLimit
func (s *GRPCServer) GetRateLimit(ctx context.Context, req *pb.GetRateLimitRequest) (*pb.GetRateLimitResponse, error) {
	capacity, refillRate, exists := s.manager.GetRateLimit(req.QueueName)
	return &pb.GetRateLimitResponse{
		Capacity:   capacity,
		RefillRate: refillRate,
		Exists:     exists,
	}, nil
}
