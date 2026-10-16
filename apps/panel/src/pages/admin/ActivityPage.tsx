import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { api, formatWhen, type AuditEvent } from "@/lib/harbor-api";
import { useEffect, useState } from "react";

export default function ActivityPage() {
  const [items, setItems] = useState<AuditEvent[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api<{ items: AuditEvent[] }>("/api/v1/audit?limit=100")
      .then((res) => setItems(res.items))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load activity"));
  }, []);

  if (error) return <p className="text-sm text-error-500">{error}</p>;
  if (!items) return <AdminLoading message="Loading activity…" />;

  return (
    <div className="admin-content space-y-6">
      <AdminPageHeader title="Activity" />
      <div className="admin-card overflow-x-auto">
        <table className="w-full min-w-[640px] text-left text-sm">
          <thead className="text-xs tracking-wide text-gray-500 uppercase">
            <tr>
              <th className="px-4 py-3 font-medium">When</th>
              <th className="px-4 py-3 font-medium">Action</th>
              <th className="px-4 py-3 font-medium">Actor</th>
              <th className="px-4 py-3 font-medium">Detail</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
            {items.map((event) => (
              <tr key={event.id}>
                <td className="px-4 py-3 whitespace-nowrap text-gray-500">{formatWhen(event.created_at)}</td>
                <td className="px-4 py-3 font-medium text-gray-900 dark:text-white">{event.action}</td>
                <td className="px-4 py-3">{event.actor}</td>
                <td className="px-4 py-3 text-gray-500">{event.meta || event.target}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
