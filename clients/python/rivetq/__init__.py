"""
RivetQ Python Client

Simple HTTP-based client for RivetQ task queue.

Example:
    from rivetq import Client

    client = Client("http://localhost:8080")
    
    # Enqueue a job
    job_id = client.enqueue("emails", {"to": "user@example.com"}, priority=7)
    
    # Lease jobs
    jobs = client.lease("emails", max_jobs=5, visibility_ms=30000)
    
    # Process and ack
    for job in jobs:
        print(f"Processing {job['id']}")
        # ... do work ...
        client.ack(job['id'], job['lease_id'])
"""

__version__ = "0.1.0"

from .client import Client

__all__ = ["Client"]
