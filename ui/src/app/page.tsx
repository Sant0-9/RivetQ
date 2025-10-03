"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

interface QueueStats {
  ready: number;
  inflight: number;
  dlq: number;
}

export default function Home() {
  const [queues, setQueues] = useState<string[]>([]);
  const [queueStats, setQueueStats] = useState<Record<string, QueueStats>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchQueues();
    const interval = setInterval(fetchQueues, 2000);
    return () => clearInterval(interval);
  }, []);

  const fetchQueues = async () => {
    try {
      const res = await fetch(`${API_BASE}/v1/queues/`);
      const data = await res.json();
      setQueues(data.queues || []);

      // Fetch stats for each queue
      const statsPromises = (data.queues || []).map(async (queue: string) => {
        const statsRes = await fetch(`${API_BASE}/v1/queues/${queue}/stats`);
        const stats = await statsRes.json();
        return [queue, stats];
      });

      const statsResults = await Promise.all(statsPromises);
      const statsMap = Object.fromEntries(statsResults);
      setQueueStats(statsMap);
    } catch (error) {
      console.error("Failed to fetch queues:", error);
    } finally {
      setLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-gray-600">Loading...</div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold text-gray-900">RivetQ Admin</h1>
          <p className="mt-2 text-sm text-gray-600">
            Manage your task queues and monitor job processing
          </p>
        </div>

        <div className="mb-6 flex gap-4">
          <Link
            href="/enqueue"
            className="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-blue-600 hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-blue-500"
          >
            Enqueue Job
          </Link>
        </div>

        {queues.length === 0 ? (
          <div className="text-center py-12">
            <p className="text-gray-500">No queues found. Enqueue a job to create a queue.</p>
          </div>
        ) : (
          <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
            {queues.map((queue) => {
              const stats = queueStats[queue] || { ready: 0, inflight: 0, dlq: 0 };
              return (
                <Link
                  key={queue}
                  href={`/queue/${queue}`}
                  className="block bg-white rounded-lg shadow hover:shadow-md transition-shadow"
                >
                  <div className="p-6">
                    <h3 className="text-lg font-semibold text-gray-900 mb-4">
                      {queue}
                    </h3>
                    <div className="space-y-2">
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-gray-600">Ready</span>
                        <span className="text-sm font-medium text-green-600">
                          {stats.ready}
                        </span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-gray-600">Inflight</span>
                        <span className="text-sm font-medium text-blue-600">
                          {stats.inflight}
                        </span>
                      </div>
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-gray-600">DLQ</span>
                        <span className="text-sm font-medium text-red-600">
                          {stats.dlq}
                        </span>
                      </div>
                    </div>
                  </div>
                </Link>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
