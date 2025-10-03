import json
from typing import Any, Dict, List, Optional
import requests


class RivetQError(Exception):
    """Base exception for RivetQ client errors."""
    pass


class Client:
    """RivetQ HTTP client."""

    def __init__(self, base_url: str = "http://localhost:8080", timeout: int = 30):
        """
        Initialize RivetQ client.
        
        Args:
            base_url: Base URL of RivetQ server
            timeout: Request timeout in seconds
        """
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.session = requests.Session()

    def enqueue(
        self,
        queue: str,
        payload: Any,
        priority: int = 5,
        delay_ms: int = 0,
        max_retries: int = 3,
        idempotency_key: Optional[str] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> str:
        """
        Enqueue a job.
        
        Args:
            queue: Queue name
            payload: Job payload (will be JSON serialized)
            priority: Priority 0-9 (higher is more important)
            delay_ms: Delay before job becomes available
            max_retries: Maximum retry attempts
            idempotency_key: Optional idempotency key
            headers: Optional job headers
            
        Returns:
            Job ID
        """
        data = {
            "payload": payload,
            "priority": priority,
            "delay_ms": delay_ms,
            "max_retries": max_retries,
        }
        
        if idempotency_key:
            data["idempotency_key"] = idempotency_key
        if headers:
            data["headers"] = headers

        resp = self._request("POST", f"/v1/queues/{queue}/enqueue", data)
        return resp["job_id"]

    def lease(
        self, queue: str, max_jobs: int = 1, visibility_ms: int = 30000
    ) -> List[Dict[str, Any]]:
        """
        Lease jobs from queue.
        
        Args:
            queue: Queue name
            max_jobs: Maximum number of jobs to lease
            visibility_ms: Visibility timeout in milliseconds
            
        Returns:
            List of job dictionaries
        """
        data = {"max_jobs": max_jobs, "visibility_ms": visibility_ms}
        resp = self._request("POST", f"/v1/queues/{queue}/lease", data)
        return resp.get("jobs", [])

    def ack(self, job_id: str, lease_id: str) -> None:
        """
        Acknowledge job completion.
        
        Args:
            job_id: Job ID
            lease_id: Lease ID from leased job
        """
        data = {"job_id": job_id, "lease_id": lease_id}
        self._request("POST", "/v1/ack", data)

    def nack(self, job_id: str, lease_id: str, reason: str = "") -> None:
        """
        Negatively acknowledge job (requeue with backoff).
        
        Args:
            job_id: Job ID
            lease_id: Lease ID from leased job
            reason: Optional reason for nack
        """
        data = {"job_id": job_id, "lease_id": lease_id, "reason": reason}
        self._request("POST", "/v1/nack", data)

    def stats(self, queue: str) -> Dict[str, int]:
        """
        Get queue statistics.
        
        Args:
            queue: Queue name
            
        Returns:
            Dictionary with ready, inflight, and dlq counts
        """
        return self._request("GET", f"/v1/queues/{queue}/stats")

    def list_queues(self) -> List[str]:
        """
        List all queues.
        
        Returns:
            List of queue names
        """
        resp = self._request("GET", "/v1/queues/")
        return resp.get("queues", [])

    def set_rate_limit(self, queue: str, capacity: float, refill_rate: float) -> None:
        """
        Set rate limit for queue.
        
        Args:
            queue: Queue name
            capacity: Token bucket capacity
            refill_rate: Tokens per second
        """
        data = {"capacity": capacity, "refill_rate": refill_rate}
        self._request("POST", f"/v1/queues/{queue}/rate_limit", data)

    def get_rate_limit(self, queue: str) -> Dict[str, Any]:
        """
        Get rate limit for queue.
        
        Args:
            queue: Queue name
            
        Returns:
            Dictionary with capacity, refill_rate, and exists fields
        """
        return self._request("GET", f"/v1/queues/{queue}/rate_limit")

    def _request(
        self, method: str, path: str, data: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Make HTTP request to RivetQ server."""
        url = self.base_url + path
        
        try:
            if method == "GET":
                resp = self.session.get(url, timeout=self.timeout)
            else:
                resp = self.session.request(
                    method,
                    url,
                    json=data,
                    headers={"Content-Type": "application/json"},
                    timeout=self.timeout,
                )
            
            resp.raise_for_status()
            
            if resp.content:
                return resp.json()
            return {}
            
        except requests.exceptions.HTTPError as e:
            try:
                error_data = e.response.json()
                error_msg = error_data.get("error", str(e))
            except:
                error_msg = str(e)
            raise RivetQError(f"HTTP {e.response.status_code}: {error_msg}")
        except requests.exceptions.RequestException as e:
            raise RivetQError(f"Request failed: {str(e)}")
