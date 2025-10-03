import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const enqueueRate = new Rate('enqueue_success_rate');
const leaseRate = new Rate('lease_success_rate');
const ackRate = new Rate('ack_success_rate');
const leaseLatency = new Trend('lease_latency');

export const options = {
  stages: [
    { duration: '30s', target: 10 },  // Ramp up to 10 users
    { duration: '1m', target: 10 },   // Stay at 10 users
    { duration: '30s', target: 50 },  // Ramp up to 50 users
    { duration: '1m', target: 50 },   // Stay at 50 users
    { duration: '30s', target: 0 },   // Ramp down to 0 users
  ],
  thresholds: {
    'enqueue_success_rate': ['rate>0.95'],
    'lease_success_rate': ['rate>0.95'],
    'ack_success_rate': ['rate>0.95'],
    'lease_latency': ['p(95)<500', 'p(99)<1000'],
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';
const QUEUE_NAME = 'load-test';

export default function() {
  // Enqueue a job
  const enqueuePayload = JSON.stringify({
    payload: {
      message: `Test message ${Date.now()}`,
      timestamp: new Date().toISOString(),
    },
    priority: Math.floor(Math.random() * 10),
    max_retries: 3,
  });

  const enqueueRes = http.post(
    `${BASE_URL}/v1/queues/${QUEUE_NAME}/enqueue`,
    enqueuePayload,
    { headers: { 'Content-Type': 'application/json' } }
  );

  enqueueRate.add(enqueueRes.status === 200);
  check(enqueueRes, {
    'enqueue status is 200': (r) => r.status === 200,
  });

  sleep(0.1);

  // Lease a job
  const leaseStart = Date.now();
  const leasePayload = JSON.stringify({
    max_jobs: 1,
    visibility_ms: 30000,
  });

  const leaseRes = http.post(
    `${BASE_URL}/v1/queues/${QUEUE_NAME}/lease`,
    leasePayload,
    { headers: { 'Content-Type': 'application/json' } }
  );

  leaseLatency.add(Date.now() - leaseStart);
  leaseRate.add(leaseRes.status === 200);
  
  const leaseSuccess = check(leaseRes, {
    'lease status is 200': (r) => r.status === 200,
  });

  if (leaseSuccess) {
    const leaseData = JSON.parse(leaseRes.body);
    
    if (leaseData.jobs && leaseData.jobs.length > 0) {
      const job = leaseData.jobs[0];
      
      // Simulate work
      sleep(Math.random() * 0.5);

      // Ack the job
      const ackPayload = JSON.stringify({
        job_id: job.id,
        lease_id: job.lease_id,
      });

      const ackRes = http.post(
        `${BASE_URL}/v1/ack`,
        ackPayload,
        { headers: { 'Content-Type': 'application/json' } }
      );

      ackRate.add(ackRes.status === 200);
      check(ackRes, {
        'ack status is 200': (r) => r.status === 200,
      });
    }
  }

  sleep(0.5);
}

export function handleSummary(data) {
  return {
    'summary.json': JSON.stringify(data),
    'stdout': textSummary(data, { indent: ' ', enableColors: true }),
  };
}

function textSummary(data, options) {
  const indent = options.indent || '';
  const enableColors = options.enableColors || false;

  let summary = `\n${indent}Load Test Summary\n`;
  summary += `${indent}${'='.repeat(50)}\n\n`;

  if (data.metrics.enqueue_success_rate) {
    summary += `${indent}Enqueue Success Rate: ${(data.metrics.enqueue_success_rate.values.rate * 100).toFixed(2)}%\n`;
  }
  if (data.metrics.lease_success_rate) {
    summary += `${indent}Lease Success Rate: ${(data.metrics.lease_success_rate.values.rate * 100).toFixed(2)}%\n`;
  }
  if (data.metrics.ack_success_rate) {
    summary += `${indent}Ack Success Rate: ${(data.metrics.ack_success_rate.values.rate * 100).toFixed(2)}%\n`;
  }
  
  if (data.metrics.lease_latency) {
    summary += `\n${indent}Lease Latency:\n`;
    summary += `${indent}  p50: ${data.metrics.lease_latency.values['p(50)'].toFixed(2)}ms\n`;
    summary += `${indent}  p95: ${data.metrics.lease_latency.values['p(95)'].toFixed(2)}ms\n`;
    summary += `${indent}  p99: ${data.metrics.lease_latency.values['p(99)'].toFixed(2)}ms\n`;
  }

  return summary;
}
