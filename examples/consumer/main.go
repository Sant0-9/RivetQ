package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"time"

	rivetq "github.com/rivetq/rivetq/clients/go"
)

func main() {
	client := rivetq.NewClient("http://localhost:8080")

	fmt.Println("Starting consumer...")

	// Consume jobs continuously
	for {
		jobs, err := client.Lease(
			context.Background(),
			"example-queue",
			5,     // Lease up to 5 jobs
			30000, // 30 second visibility timeout
		)
		if err != nil {
			log.Printf("Failed to lease jobs: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		if len(jobs) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		for _, job := range jobs {
			fmt.Printf("Processing job %s (priority %d, tries %d)\n",
				job.ID, job.Priority, job.Tries)

			// Parse payload
			var payload map[string]interface{}
			if err := json.Unmarshal(job.Payload, &payload); err != nil {
				log.Printf("Failed to parse payload: %v", err)
				continue
			}

			fmt.Printf("  Payload: %v\n", payload)

			// Simulate work
			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)

			// Randomly succeed or fail (90% success rate)
			if rand.Float64() < 0.9 {
				// Success - ack
				if err := client.Ack(context.Background(), job.ID, job.LeaseID); err != nil {
					log.Printf("Failed to ack job: %v", err)
				} else {
					fmt.Printf("  ✓ Job completed successfully\n")
				}
			} else {
				// Failure - nack
				if err := client.Nack(context.Background(), job.ID, job.LeaseID, "simulated failure"); err != nil {
					log.Printf("Failed to nack job: %v", err)
				} else {
					fmt.Printf("  ✗ Job failed, will retry\n")
				}
			}
		}
	}
}
