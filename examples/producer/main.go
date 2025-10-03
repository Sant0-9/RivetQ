package main

import (
	"context"
	"fmt"
	"log"
	"time"

	rivetq "github.com/rivetq/rivetq/clients/go"
)

func main() {
	client := rivetq.NewClient("http://localhost:8080")

	// Enqueue jobs continuously
	for i := 0; i < 100; i++ {
		jobID, err := client.Enqueue(
			context.Background(),
			"example-queue",
			map[string]interface{}{
				"message": fmt.Sprintf("Job %d", i),
				"timestamp": time.Now().Unix(),
			},
			&rivetq.EnqueueOptions{
				Priority:   uint8(i % 10),
				MaxRetries: 3,
			},
		)
		if err != nil {
			log.Printf("Failed to enqueue job: %v", err)
			continue
		}

		fmt.Printf("Enqueued job %d: %s\n", i, jobID)
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("All jobs enqueued!")
}
