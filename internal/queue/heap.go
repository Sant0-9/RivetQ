package queue

import (
	"container/heap"
	"time"
)

// jobHeapItem wraps a job for heap operations
type jobHeapItem struct {
	job   *Job
	index int
}

// jobHeap implements heap.Interface for priority queue
// Jobs are ordered by: priority (DESC), ETA (ASC), enqueued time (ASC)
type jobHeap []*jobHeapItem

func (h jobHeap) Len() int { return len(h) }

func (h jobHeap) Less(i, j int) bool {
	// Higher priority comes first
	if h[i].job.Priority != h[j].job.Priority {
		return h[i].job.Priority > h[j].job.Priority
	}

	// Earlier ETA comes first
	if !h[i].job.ETA.Equal(h[j].job.ETA) {
		return h[i].job.ETA.Before(h[j].job.ETA)
	}

	// Earlier enqueue time comes first
	return h[i].job.EnqueuedAt.Before(h[j].job.EnqueuedAt)
}

func (h jobHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *jobHeap) Push(x interface{}) {
	item := x.(*jobHeapItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *jobHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// priorityQueue manages jobs in priority order
type priorityQueue struct {
	heap  jobHeap
	items map[string]*jobHeapItem // jobID -> item
}

// newPriorityQueue creates a new priority queue
func newPriorityQueue() *priorityQueue {
	pq := &priorityQueue{
		heap:  make(jobHeap, 0),
		items: make(map[string]*jobHeapItem),
	}
	heap.Init(&pq.heap)
	return pq
}

// Push adds a job to the queue
func (pq *priorityQueue) Push(job *Job) {
	if _, exists := pq.items[job.ID]; exists {
		return // Already exists
	}

	item := &jobHeapItem{job: job}
	pq.items[job.ID] = item
	heap.Push(&pq.heap, item)
}

// Pop removes and returns the highest priority job
func (pq *priorityQueue) Pop() *Job {
	if pq.heap.Len() == 0 {
		return nil
	}

	item := heap.Pop(&pq.heap).(*jobHeapItem)
	delete(pq.items, item.job.ID)
	return item.job
}

// Peek returns the highest priority job without removing it
func (pq *priorityQueue) Peek() *Job {
	if pq.heap.Len() == 0 {
		return nil
	}
	return pq.heap[0].job
}

// Remove removes a job from the queue
func (pq *priorityQueue) Remove(jobID string) *Job {
	item, exists := pq.items[jobID]
	if !exists {
		return nil
	}

	heap.Remove(&pq.heap, item.index)
	delete(pq.items, jobID)
	return item.job
}

// Len returns the number of jobs in the queue
func (pq *priorityQueue) Len() int {
	return pq.heap.Len()
}

// PeekReady returns the next ready job (ETA has passed) without removing it
func (pq *priorityQueue) PeekReady(now time.Time) *Job {
	if pq.heap.Len() == 0 {
		return nil
	}

	job := pq.heap[0].job
	if job.IsReady(now) {
		return job
	}

	return nil
}

// PopReady removes and returns the next ready job
func (pq *priorityQueue) PopReady(now time.Time) *Job {
	if pq.heap.Len() == 0 {
		return nil
	}

	job := pq.heap[0].job
	if !job.IsReady(now) {
		return nil
	}

	return pq.Pop()
}
