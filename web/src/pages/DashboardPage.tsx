import { useQuery } from '@tanstack/react-query';
import { admin } from '@/lib/api';
import {
  PageHeader,
  StatCard,
  Card,
  Spinner,
  Badge,
} from '@/components/UI';
import {
  Globe,
  Images,
  Image,
  Film,
  Users,
  Heart,
  ListChecks,
  Download,
} from 'lucide-react';

export function DashboardPage() {
  const { data: stats, isLoading } = useQuery({
    queryKey: ['admin', 'stats'],
    queryFn: admin.stats,
  });

  if (isLoading || !stats) return <Spinner />;

  return (
    <>
      <PageHeader title="Dashboard" description="System overview and statistics" />

      {/* Top stats grid */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <StatCard label="Sources" value={stats.sources} icon={<Globe size={20} />} />
        <StatCard label="Galleries" value={stats.galleries} icon={<Images size={20} />} />
        <StatCard label="Images" value={stats.images} icon={<Image size={20} />} />
        <StatCard label="Videos" value={stats.videos} icon={<Film size={20} />} />
        <StatCard label="People" value={stats.people} icon={<Users size={20} />} />
        <StatCard label="Favorites" value={stats.favorites} icon={<Heart size={20} />} />
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
        {/* Queue status */}
        <Card>
          <div className="flex items-center gap-2 mb-3">
            <ListChecks size={16} className="text-zinc-400" />
            <h3 className="text-sm font-medium text-white">Queue Status</h3>
          </div>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-zinc-400">Pending</span>
              <Badge variant="info">{stats.queue.pending}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Active</span>
              <Badge variant="warning">{stats.queue.active}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Completed</span>
              <Badge variant="success">{stats.queue.completed}</Badge>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Failed</span>
              <Badge variant="danger">{stats.queue.failed}</Badge>
            </div>
          </div>
        </Card>

        {/* Download stats */}
        <Card>
          <div className="flex items-center gap-2 mb-3">
            <Download size={16} className="text-zinc-400" />
            <h3 className="text-sm font-medium text-white">Download Activity</h3>
          </div>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-zinc-400">Completed today</span>
              <span className="text-zinc-200">{stats.downloads.completed_today}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Completed this week</span>
              <span className="text-zinc-200">{stats.downloads.completed_week}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Failed today</span>
              <span className="text-red-400">{stats.downloads.failed_today}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-zinc-400">Failed this week</span>
              <span className="text-red-400">{stats.downloads.failed_week}</span>
            </div>
          </div>
        </Card>
      </div>

      {/* Provider breakdown */}
      {stats.provider_breakdown && stats.provider_breakdown.length > 0 && (
        <Card>
          <h3 className="text-sm font-medium text-white mb-3">Gallery Provider Breakdown</h3>
          <div className="space-y-2">
            {stats.provider_breakdown.map((pb) => (
              <div key={pb.provider} className="flex items-center justify-between text-sm">
                <span className="text-zinc-400">{pb.provider || 'unknown'}</span>
                <span className="text-zinc-200">{pb.count}</span>
              </div>
            ))}
          </div>
        </Card>
      )}
    </>
  );
}
