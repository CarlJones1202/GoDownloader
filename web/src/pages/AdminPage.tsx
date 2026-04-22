import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { admin } from '@/lib/api';
import { formatDateTime } from '@/lib/utils';
import type { ActiveDownload, DownloadQueue } from '@/types';
import {
  PageHeader,
  Card,
  Badge,
  Button,
  Spinner,
  EmptyState,
  Select,
  Pagination,
  StatCard,
  ConfirmDialog,
} from '@/components/UI';
import {
  Pause,
  Play,
  Trash2,
  RotateCcw,
  AlertTriangle,
  Activity,
  CheckCircle,
  XCircle,
  Clock,
  Layers,
  Sparkles,
  Download,
  Globe,
  Image as ImageIcon,
  Video as VideoIcon,
} from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

const STATUS_OPTIONS = [
  { value: '', label: 'All statuses' },
  { value: 'pending', label: 'Pending' },
  { value: 'active', label: 'Active' },
  { value: 'completed', label: 'Completed' },
  { value: 'failed', label: 'Failed' },
];

const TYPE_OPTIONS = [
  { value: '', label: 'All types' },
  { value: 'image', label: 'Image' },
  { value: 'video', label: 'Video' },
  { value: 'gallery', label: 'Gallery' },
];

function statusVariant(status: string): 'default' | 'success' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'completed': return 'success';
    case 'active': return 'info';
    case 'pending': return 'warning';
    case 'failed': return 'danger';
    default: return 'default';
  }
}

export function AdminPage() {
  const queryClient = useQueryClient();
  const { page, offset, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 50 });
  const [statusFilter, setStatusFilter] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const [clearStatus, setClearStatus] = useState('');
  const [confirmClear, setConfirmClear] = useState(false);
  const [confirmCleanup, setConfirmCleanup] = useState(false);
  const [confirmStopServer, setConfirmStopServer] = useState(false);

  // --- Queries ---

  const { data: queueStatus } = useQuery({
    queryKey: ['queue-status'],
    queryFn: admin.queue.status,
    refetchInterval: 5000,
  });

  const { data: activeDownloads } = useQuery({
    queryKey: ['queue-active'],
    queryFn: admin.queue.activeDownloads,
    refetchInterval: 2000,
  });

  const { data: queueItems, isLoading: loadingQueue } = useQuery({
    queryKey: ['queue-items', { offset, limit, status: statusFilter || undefined, type: typeFilter || undefined }],
    queryFn: () =>
      admin.queue.list({
        limit,
        offset,
        status: statusFilter || undefined,
        type: typeFilter || undefined,
      }),
  });

  const { data: cleanupPreview } = useQuery({
    queryKey: ['gallery-cleanup-preview'],
    queryFn: () => admin.galleryCleanup(true),
  });

  // --- Mutations ---

  const pauseMut = useMutation({
    mutationFn: admin.queue.pause,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['queue-status'] }),
  });

  const resumeMut = useMutation({
    mutationFn: admin.queue.resume,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['queue-status'] }),
  });

  const clearMut = useMutation({
    mutationFn: () => admin.queue.clear(clearStatus || undefined),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['queue-items'] });
      queryClient.invalidateQueries({ queryKey: ['queue-status'] });
      setConfirmClear(false);
    },
  });

  const retryMut = useMutation({
    mutationFn: (id: number) => admin.queue.retry(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['queue-items'] });
      queryClient.invalidateQueries({ queryKey: ['queue-status'] });
    },
  });

  const retryFailedMut = useMutation({
    mutationFn: admin.queue.retryFailed,
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['queue-items'] });
      queryClient.invalidateQueries({ queryKey: ['queue-status'] });
      alert(`Re-queued ${data.retried} failed items.`);
    },
  });

  const deleteItemMut = useMutation({
    mutationFn: (id: number) => admin.queue.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['queue-items'] });
      queryClient.invalidateQueries({ queryKey: ['queue-status'] });
    },
  });

  const cleanupMut = useMutation({
    mutationFn: () => admin.galleryCleanup(false),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gallery-cleanup-preview'] });
      setConfirmCleanup(false);
    },
  });
  
  const autolinkMut = useMutation({
    mutationFn: admin.autolinkGalleries,
    onSuccess: (data) => {
      alert(data.message || 'Autolink scan started in background. Check server logs for progress.');
    },
  });

  const stopServerMut = useMutation({
    mutationFn: admin.stopServer,
    onSuccess: () => {
      setConfirmStopServer(false);
      alert('Server shutdown requested. The API should stop within a few seconds.');
    },
  });

  const stats = queueStatus?.stats;
  const isPaused = queueStatus?.paused ?? false;

  return (
    <div className="max-w-[1600px] mx-auto px-4 py-8">
      <div className="flex flex-col md:flex-row md:items-center justify-between mb-8 gap-4 border-b border-zinc-800 pb-6">
        <div>
          <h1 className="text-3xl font-bold bg-gradient-to-r from-white to-zinc-500 bg-clip-text text-transparent">
            Admin Dashboard
          </h1>
          <p className="text-zinc-400 mt-1 text-sm">Control center for queue processing and maintenance</p>
        </div>
        <div className="flex items-center gap-3">
          {isPaused ? (
            <Button
              className="bg-emerald-600 hover:bg-emerald-500 text-white border-none shadow-lg shadow-emerald-900/20"
              onClick={() => resumeMut.mutate()}
              disabled={resumeMut.isPending}
            >
              <Play size={16} fill="currentColor" /> Resume Queue
            </Button>
          ) : (
            <Button
              variant="secondary"
              className="bg-zinc-800 hover:bg-zinc-700 text-white border-zinc-700"
              onClick={() => pauseMut.mutate()}
              disabled={pauseMut.isPending}
            >
              <Pause size={16} fill="currentColor" /> Pause Queue
            </Button>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-8">
        {/* Main Content: Stats & List */}
        <div className="lg:col-span-8 space-y-6">
          {/* Stats Bar */}
          {stats && (
            <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-6 gap-4">
              <StatCard label="Total Pending" value={stats.pending} icon={<Clock className="text-amber-400" size={20} />} />
              <StatCard label="Images Queued" value={stats.pending_images || 0} icon={<ImageIcon className="text-purple-400" size={20} />} />
              <StatCard label="Videos Queued" value={stats.pending_videos || 0} icon={<VideoIcon className="text-indigo-400" size={20} />} />
              <StatCard label="Active" value={stats.active} icon={<Activity className="text-blue-400" size={20} />} />
              <StatCard label="Completed" value={stats.completed} icon={<CheckCircle className="text-emerald-400" size={20} />} />
              <StatCard label="Failed" value={stats.failed} icon={<XCircle className="text-rose-400" size={20} />} />
            </div>
          )}

          {/* Queue Filter Bar */}
          <Card className="bg-zinc-900/40 backdrop-blur-sm border-zinc-800/50 p-4">
            <div className="flex flex-wrap items-center justify-between gap-4">
              <div className="flex items-center gap-1 bg-zinc-950 p-1 rounded-lg border border-zinc-800">
                {TYPE_OPTIONS.map((opt) => (
                  <button
                    key={opt.value}
                    onClick={() => {
                      setTypeFilter(opt.value);
                      resetPage();
                    }}
                    className={`px-4 py-1.5 rounded-md text-xs font-medium transition-all ${
                      typeFilter === opt.value
                        ? 'bg-zinc-800 text-white shadow-lg'
                        : 'text-zinc-500 hover:text-zinc-300'
                    }`}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
              <div className="flex items-center gap-3">
                <Select
                  options={STATUS_OPTIONS}
                  value={statusFilter}
                  onChange={(e) => {
                    setStatusFilter(e.target.value);
                    resetPage();
                  }}
                  className="w-40 bg-zinc-950 border-zinc-800 h-9"
                />
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-zinc-400 hover:text-white"
                  onClick={() => retryFailedMut.mutate()}
                  disabled={retryFailedMut.isPending || !stats || stats.failed === 0}
                >
                  <RotateCcw size={14} className="mr-1.5" /> Retry Failed
                </Button>
                <div className="h-6 w-[1px] bg-zinc-800 mx-2" />
                <Select
                  options={[
                    { value: '', label: 'All items' },
                    { value: 'pending', label: 'Pending' },
                    { value: 'completed', label: 'Completed' },
                    { value: 'failed', label: 'Failed' },
                  ]}
                  value={clearStatus}
                  onChange={(e) => setClearStatus(e.target.value)}
                  className="w-36 bg-zinc-950 border-zinc-800 text-sm h-9"
                />
                <Button
                  variant="danger"
                  size="sm"
                  className="bg-rose-950/30 text-rose-400 hover:bg-rose-900/50 border border-rose-500/20"
                  onClick={() => setConfirmClear(true)}
                >
                  <Trash2 size={14} />
                </Button>
              </div>
            </div>
          </Card>

          {/* Queue List */}
          <div className="space-y-3">
            <div className="flex items-center justify-between px-2 mb-1">
              <h3 className="text-sm font-medium text-zinc-500 uppercase tracking-wider">Queue Records</h3>
              {loadingQueue && <Spinner size="sm" />}
            </div>

            {loadingQueue && !queueItems ? (
              <div className="py-20 flex justify-center"><Spinner /></div>
            ) : !queueItems || queueItems.length === 0 ? (
              <EmptyState message="No queue items match your filters." />
            ) : (
              <>
                <div className="space-y-2">
                  {queueItems.map((item: DownloadQueue) => (
                    <Card key={item.id} className="group relative bg-zinc-900/30 hover:bg-zinc-900/60 border-zinc-800/40 transition-all overflow-hidden">
                      <div className="flex items-start gap-4 p-4">
                        <div className={`mt-1 h-2 w-2 rounded-full shrink-0 ${
                          item.status === 'active' ? 'bg-blue-500 animate-pulse' :
                          item.status === 'completed' ? 'bg-emerald-500' :
                          item.status === 'failed' ? 'bg-rose-500' : 'bg-zinc-600'
                        }`} />
                        
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2 mb-1">
                            <Badge variant={statusVariant(item.status)} className="uppercase text-[10px] px-1.5 py-0">
                              {item.status}
                            </Badge>
                            <Badge variant="default" className="text-[10px] px-1.5 py-0 bg-zinc-800 border-zinc-700">
                              {item.type}
                            </Badge>
                            {item.retry_count > 0 && (
                              <span className="text-[10px] font-medium text-amber-500/80">
                                RETRY {item.retry_count}
                              </span>
                            )}
                          </div>
                          
                          <p className="text-sm text-zinc-300 font-mono truncate">{item.url}</p>
                          
                          {item.error_message && (
                            <p className="text-xs text-rose-400/90 flex items-center gap-1.5 mt-1 bg-rose-500/5 p-1.5 rounded border border-rose-500/10">
                              <AlertTriangle size={12} /> {item.error_message}
                            </p>
                          )}
                          
                          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-2 text-[11px] text-zinc-500">
                            <span className="flex items-center gap-1"><Clock size={10} /> {formatDateTime(item.created_at)}</span>
                            {item.source_name && (
                              <span className="flex items-center gap-1">
                                <span className="h-1 w-1 rounded-full bg-zinc-700" />
                                source: <span className="text-zinc-400">{item.source_name}</span>
                              </span>
                            )}
                            {item.gallery_title && (
                              <span className="flex items-center gap-1 max-w-[200px] truncate">
                                <span className="h-1 w-1 rounded-full bg-zinc-700" />
                                gallery: <span className="text-zinc-400">{item.gallery_title}</span>
                              </span>
                            )}
                          </div>
                        </div>

                        <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                          {item.status === 'failed' && (
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-8 w-8 p-0 text-zinc-400 hover:text-white"
                              onClick={() => retryMut.mutate(item.id)}
                            >
                              <RotateCcw size={14} />
                            </Button>
                          )}
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-8 w-8 p-0 text-zinc-400 hover:text-rose-400"
                            onClick={() => deleteItemMut.mutate(item.id)}
                          >
                            <Trash2 size={14} />
                          </Button>
                        </div>
                      </div>
                    </Card>
                  ))}
                </div>

                <div className="pt-4">
                  <Pagination
                    page={page}
                    hasMore={queueItems.length === limit}
                    onPrev={prevPage}
                    onNext={nextPage}
                  />
                </div>
              </>
            )}
          </div>
        </div>

        {/* Sidebar: Monitoring & Tools */}
        <div className="lg:col-span-4 space-y-6">
          {/* Active Downloads Monitor */}
          <div className="space-y-4">
            <h2 className="text-xs font-bold text-zinc-500 uppercase tracking-widest flex items-center gap-2">
              <Download size={14} className="text-blue-500" />
              Live Monitor
            </h2>
            
            <Card className="bg-zinc-900/40 backdrop-blur-md border-zinc-800/60 overflow-hidden shadow-2xl shadow-blue-900/10">
              <div className="p-4 border-b border-zinc-800/60 bg-blue-500/5 flex items-center justify-between">
                <span className="text-sm font-medium text-blue-400">Activity</span>
                <Badge variant="info" className="bg-blue-500/20 text-blue-300 border-blue-500/30 text-[10px]">
                  {activeDownloads?.length || 0} In Flight
                </Badge>
              </div>
              
              <div className="p-4 space-y-4">
                {/* Provider breakdown summary */}
                {queueStatus?.active_by_provider && Object.keys(queueStatus.active_by_provider).length > 0 ? (
                  <div className="flex flex-wrap gap-2 pb-2">
                    {Object.entries(queueStatus.active_by_provider)
                      .sort(([, a], [, b]) => b - a)
                      .map(([provider, count]) => (
                        <div
                          key={provider}
                          className="flex items-center gap-1.5 px-2 py-1 rounded bg-zinc-950 border border-zinc-800/80 text-[10px]"
                          title={`${count} items from ${provider}`}
                        >
                          <Globe size={10} className="text-blue-500/70" />
                          <span className="text-zinc-400">{provider}</span>
                          <span className="text-blue-400 font-bold ml-0.5">{count}</span>
                        </div>
                      ))}
                  </div>
                ) : !activeDownloads || activeDownloads.length === 0 ? (
                  <div className="py-6 text-center">
                    <Activity size={24} className="mx-auto text-zinc-800 mb-2" />
                    <p className="text-xs text-zinc-600 italic">No active downloads</p>
                  </div>
                ) : null}

                {/* Individual active items */}
                {activeDownloads && activeDownloads.length > 0 && (
                  <div className="space-y-2 max-h-[400px] overflow-y-auto pr-1 custom-scrollbar">
                    {activeDownloads.map((dl: ActiveDownload) => {
                      const elapsedSec = Math.floor((Date.now() - dl.started_at) / 1000);
                      const elapsed = elapsedSec >= 60
                        ? `${Math.floor(elapsedSec / 60)}m ${elapsedSec % 60}s`
                        : `${elapsedSec}s`;
                      const displayURL = dl.url.includes('|') ? dl.url.split('|')[0] : dl.url;
                      
                      return (
                        <div key={dl.id} className="p-2.5 rounded bg-zinc-950/50 border border-zinc-800/30 flex items-center gap-3 hover:bg-zinc-950 hover:border-zinc-700 transition-colors group">
                          <div className="h-1.5 w-1.5 rounded-full bg-blue-500 animate-pulse shadow-[0_0_8px_rgba(59,130,246,0.5)]" />
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center justify-between mb-0.5">
                              <div className="flex items-center gap-1.5 truncate mr-2">
                                <span className="text-[10px] font-bold text-zinc-500 group-hover:text-zinc-400 transition-colors uppercase tracking-tighter">
                                  {dl.provider}
                                </span>
                                {dl.source_name && (
                                  <>
                                    <span className="h-0.5 w-0.5 rounded-full bg-zinc-700" />
                                    <span className="text-[10px] text-zinc-600 truncate italic">{dl.source_name}</span>
                                  </>
                                )}
                              </div>
                              <span className="text-[10px] text-zinc-600 whitespace-nowrap flex items-center gap-1">
                                <Clock size={9} /> {elapsed}
                              </span>
                            </div>
                            <p className="text-[11px] text-zinc-400 truncate font-mono">{displayURL.split('/').pop()}</p>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            </Card>
          </div>

          {/* System Tools */}
          <div className="space-y-4">
            <h2 className="text-xs font-bold text-zinc-500 uppercase tracking-widest flex items-center gap-2">
              <Layers size={14} className="text-emerald-500" />
              Maintenance
            </h2>

            <div className="space-y-3">
              {/* Autolink Tool */}
              <Card className="p-4 bg-zinc-900/40 border-zinc-800/60 hover:border-emerald-500/30 transition-colors">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <h4 className="text-sm font-semibold text-zinc-200">Person Linker</h4>
                    <p className="text-xs text-zinc-500 mt-1">Scan galleries and link to people profiles</p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-emerald-500 hover:bg-emerald-500/10 h-8 w-8 p-0"
                    onClick={() => autolinkMut.mutate()}
                    disabled={autolinkMut.isPending}
                  >
                    {autolinkMut.isPending ? <Spinner size="sm" /> : <Sparkles size={16} />}
                  </Button>
                </div>
              </Card>

              {/* Cleanup Tool */}
              <Card className="p-4 bg-zinc-900/40 border-zinc-800/60 hover:border-rose-500/30 transition-colors">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <h4 className="text-sm font-semibold text-zinc-200">Gallery Cleanup</h4>
                    <p className="text-xs text-zinc-500 mt-1">Remove orphaned image files</p>
                    {cleanupPreview && (cleanupPreview.count ?? 0) > 0 && (
                      <Badge variant="danger" className="mt-2 text-[10px] bg-rose-950/20 text-rose-400 border-rose-500/20">
                        {cleanupPreview.count} orphans detected
                      </Badge>
                    )}
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-rose-500 hover:bg-rose-500/10 h-8 w-8 p-0"
                    onClick={() => setConfirmCleanup(true)}
                    disabled={!cleanupPreview || (cleanupPreview.count ?? 0) === 0}
                  >
                    <Trash2 size={16} />
                  </Button>
                </div>
              </Card>

              {/* Server Control */}
              <Card className="p-4 bg-zinc-900/40 border-zinc-800/60 border-dashed">
                <div className="flex items-center justify-between gap-4">
                  <div>
                    <h4 className="text-sm font-semibold text-zinc-400 italic">Server Control</h4>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-zinc-600 hover:text-rose-500 hover:bg-rose-500/5 h-8 px-2"
                    onClick={() => setConfirmStopServer(true)}
                  >
                    <XCircle size={14} className="mr-1.5" /> Stop API
                  </Button>
                </div>
              </Card>
            </div>
          </div>
        </div>
      </div>

      {/* Confirm dialogs */}
      <ConfirmDialog
        open={confirmClear}
        title="Clear Queue"
        message={`Delete ${clearStatus || 'all'} queue items? This cannot be undone.`}
        confirmLabel="Clear"
        onConfirm={() => clearMut.mutate()}
        onCancel={() => setConfirmClear(false)}
      />

      <ConfirmDialog
        open={confirmCleanup}
        title="Gallery Cleanup"
        message={`Delete ${cleanupPreview?.count ?? 0} orphaned images? This cannot be undone.`}
        confirmLabel="Delete Orphans"
        onConfirm={() => cleanupMut.mutate()}
        onCancel={() => setConfirmCleanup(false)}
      />

      <ConfirmDialog
        open={confirmStopServer}
        title="Stop Server"
        message="Request a graceful server shutdown? The API will stop accepting requests shortly."
        confirmLabel="Stop Server"
        onConfirm={() => stopServerMut.mutate()}
        onCancel={() => setConfirmStopServer(false)}
      />
    </div>
  );
}
