"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

interface ClusterMember {
  id: string;
  addr: string;
  raft_addr: string;
  status: string;
  is_leader: boolean;
  last_seen_unix: number;
  joined_at_unix: number;
}

interface ClusterInfo {
  local_id: string;
  leader: string;
  member_count: number;
  members: ClusterMember[];
}

export default function ClusterPage() {
  const [info, setInfo] = useState<ClusterInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchClusterInfo();
    const interval = setInterval(fetchClusterInfo, 3000);
    return () => clearInterval(interval);
  }, []);

  const fetchClusterInfo = async () => {
    try {
      const res = await fetch(`${API_BASE}/v1/cluster/info`);
      if (!res.ok) {
        throw new Error("Cluster not available");
      }
      const data = await res.json();
      setInfo(data);
      setError(null);
    } catch (error: any) {
      setError(error.message);
    } finally {
      setLoading(false);
    }
  };

  const formatTimestamp = (unix: number) => {
    return new Date(unix * 1000).toLocaleString();
  };

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <div className="text-gray-600">Loading cluster information...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gray-50">
        <div className="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
          <div className="mb-8">
            <Link
              href="/"
              className="text-sm text-blue-600 hover:text-blue-700 mb-4 inline-block"
            >
              Back to Queues
            </Link>
            <h1 className="text-3xl font-bold text-gray-900">Cluster</h1>
          </div>
          <div className="bg-yellow-50 border border-yellow-200 text-yellow-800 px-4 py-3 rounded-md">
            Clustering is not enabled or unavailable: {error}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
        <div className="mb-8">
          <Link
            href="/"
            className="text-sm text-blue-600 hover:text-blue-700 mb-4 inline-block"
          >
            Back to Queues
          </Link>
          <h1 className="text-3xl font-bold text-gray-900">Cluster Status</h1>
          <p className="mt-2 text-sm text-gray-600">
            Monitor cluster nodes and leadership
          </p>
        </div>

        <div className="grid gap-6 md:grid-cols-3 mb-8">
          <div className="bg-white rounded-lg shadow p-6">
            <div className="text-sm font-medium text-gray-600">Total Nodes</div>
            <div className="text-3xl font-bold text-gray-900 mt-2">
              {info?.member_count || 0}
            </div>
          </div>

          <div className="bg-white rounded-lg shadow p-6">
            <div className="text-sm font-medium text-gray-600">Local Node</div>
            <div className="text-lg font-semibold text-gray-900 mt-2 truncate">
              {info?.local_id || "N/A"}
            </div>
          </div>

          <div className="bg-white rounded-lg shadow p-6">
            <div className="text-sm font-medium text-gray-600">Leader</div>
            <div className="text-lg font-semibold text-gray-900 mt-2 truncate">
              {info?.leader || "No leader"}
            </div>
          </div>
        </div>

        <div className="bg-white shadow rounded-lg overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200">
            <h2 className="text-lg font-semibold text-gray-900">
              Cluster Members
            </h2>
          </div>
          <div className="overflow-x-auto">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Node ID
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Address
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Raft Address
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Status
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Role
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">
                    Last Seen
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {info?.members.map((member) => (
                  <tr key={member.id}>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                      {member.id}
                      {member.id === info.local_id && (
                        <span className="ml-2 text-xs text-blue-600">(local)</span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {member.addr}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {member.raft_addr}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span
                        className={`px-2 py-1 text-xs font-semibold rounded-full ${
                          member.status === "alive"
                            ? "bg-green-100 text-green-800"
                            : member.status === "suspect"
                            ? "bg-yellow-100 text-yellow-800"
                            : "bg-red-100 text-red-800"
                        }`}
                      >
                        {member.status}
                      </span>
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {member.is_leader ? (
                        <span className="px-2 py-1 text-xs font-semibold rounded-full bg-blue-100 text-blue-800">
                          Leader
                        </span>
                      ) : (
                        <span className="text-gray-400">Follower</span>
                      )}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatTimestamp(member.last_seen_unix)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}
