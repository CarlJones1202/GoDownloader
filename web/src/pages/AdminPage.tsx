import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { admin } from '@/lib/api';
import { formatDateTime } from '@/lib/utils';
import type { DownloadQueue } from '@/types';
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
    <>
      <PageHeader title="Admin" description="Queue management and system tools">
        {isPaused ? (
          <Button
            size="sm"
            onClick={() => resumeMut.mutate()}
            disabled={resumeMut.isPending}
          >
            <Play size={14} /> Resume Queue
          </Button>
        ) : (
          <Button
            variant="secondary"
            size="sm"
            onClick={() => pauseMut.mutate()}
            disabled={pauseMut.isPending}
          >
            <Pause size={14} /> Pause Queue
          </Button>
        )}
      </PageHeader>

      {/* Queue stats */}
      {stats && (
        <div className="grid grid-cols-2 sm:grid-cols-5 gap-3 mb-6">
          <StatCard label="Pending" value={stats.pending} icon={<Clock size={20} />} />
          <StatCard label="Active" value={stats.active} icon={<Activity size={20} />} />
          <StatCard label="Completed" value={stats.completed} icon={<CheckCircle size={20} />} />
          <StatCard label="Failed" value={stats.failed} icon={<XCircle size={20} />} />
          <StatCard
            label="Status"
            value={isPaused ? 'Paused' : 'Running'}
            icon={isPaused ? <Pause size={20} /> : <Play size={20} />}
          />
        </div>
      )}

      {/* Queue filters and clear */}
      <Card className="mb-4">
        <div className="flex items-end gap-3 flex-wrap">
          <Select
            label="Status"
            options={STATUS_OPTIONS}
            value={statusFilter}
            onChange={(e) => {
              setStatusFilter(e.target.value);
              resetPage();
            }}
            className="w-36"
          />
          <Select
            label="Type"
            options={TYPE_OPTIONS}
            value={typeFilter}
            onChange={(e) => {
              setTypeFilter(e.target.value);
              resetPage();
            }}
            className="w-36"
          />
          <div className="flex-1" />
          <div className="flex items-end gap-2">
            <Select
              label="Clear queue"
              options={[
                { value: '', label: 'All items' },
                { value: 'pending', label: 'Pending only' },
                { value: 'completed', label: 'Completed only' },
                { value: 'failed', label: 'Failed only' },
              ]}
              value={clearStatus}
              onChange={(e) => setClearStatus(e.target.value)}
              className="w-40"
            />
            <Button
              variant="secondary"
              size="sm"
              onClick={() => retryFailedMut.mutate()}
              disabled={retryFailedMut.isPending || !stats || stats.failed === 0}
            >
              <RotateCcw size={14} /> Retry Failed
            </Button>
            <Button
              variant="danger"
              size="sm"
              onClick={() => setConfirmClear(true)}
            >
              <Trash2 size={14} /> Clear
            </Button>
          </div>
        </div>
      </Card>

      {/* Queue items list */}
      {loadingQueue ? (
        <Spinner />
      ) : !queueItems || queueItems.length === 0 ? (
        <EmptyState message="No queue items found." />
      ) : (
        <>
          <div className="space-y-2">
            {queueItems.map((item: DownloadQueue) => (
              <Card key={item.id} className="flex items-center gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <Badge variant={statusVariant(item.status)}>
                      {item.status}
                    </Badge>
                    <Badge>{item.type}</Badge>
                    {item.retry_count > 0 && (
                      <span className="text-xs text-zinc-500">
                        retries: {item.retry_count}
                      </span>
                    )}
                  </div>
                  <p className="text-xs text-zinc-400 truncate mt-1">{item.url}</p>
                  {item.error_message && (
                    <p className="text-xs text-red-400 flex items-center gap-1 mt-0.5">
                      <AlertTriangle size={10} /> {item.error_message}
                    </p>
                  )}
                  <p className="text-xs text-zinc-600 mt-0.5">
                    {formatDateTime(item.created_at)}
                    {item.source_name && ` · source: ${item.source_name}`}
                    {item.gallery_title && ` · gallery: ${item.gallery_title}`}
                    {!item.source_name && item.target_id && ` · target: ${item.target_id}`}
                  </p>
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  {item.status === 'failed' && (
                    <Button
                      variant="ghost"
                      size="sm"
                      title="Retry"
                      onClick={() => retryMut.mutate(item.id)}
                      disabled={retryMut.isPending}
                    >
                      <RotateCcw size={14} />
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="sm"
                    title="Delete"
                    onClick={() => deleteItemMut.mutate(item.id)}
                    disabled={deleteItemMut.isPending}
                  >
                    <Trash2 size={14} />
                  </Button>
                </div>
              </Card>
            ))}
          </div>

          <Pagination
            page={page}
            hasMore={queueItems.length === limit}
            onPrev={prevPage}
            onNext={nextPage}
          />
        </>
      )}

      {/* Gallery Cleanup */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mt-8 mb-3">
        <div>
          <h2 className="text-lg font-semibold text-white mb-3">Gallery Cleanup</h2>
          <Card>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-zinc-300">
                  Find and remove orphaned images not linked to any gallery.
                </p>
                {cleanupPreview && (
                  <p className="text-sm text-zinc-400 mt-1">
                    <Layers size={14} className="inline mr-1" />
                    {cleanupPreview.count ?? 0} orphaned images found (dry run).
                  </p>
                )}
              </div>
              <Button
                variant="danger"
                size="sm"
                onClick={() => setConfirmCleanup(true)}
                disabled={!cleanupPreview || (cleanupPreview.count ?? 0) === 0}
              >
                <Trash2 size={14} /> Clean Up
              </Button>
            </div>
          </Card>
        </div>

        <div>
          <h2 className="text-lg font-semibold text-white mb-3">Linker Tools</h2>
          <Card>
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-zinc-300">
                  Scan all galleries and link them to people based on name/alias matches.
                </p>
                <p className="text-xs text-zinc-500 mt-1">
                  This runs the same process that happens automatically during crawling.
                </p>
              </div>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => autolinkMut.mutate()}
                disabled={autolinkMut.isPending}
              >
                {autolinkMut.isPending ? (
                  <Spinner size="sm" className="mr-1" />
                ) : (
                  <Sparkles size={14} className="mr-1" />
                )}
                Run Autolink
              </Button>
            </div>
          </Card>
        </div>
      </div>

      <div className="mb-3">
        <h2 className="text-lg font-semibold text-white mb-3">Server Control</h2>
        <Card>
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-zinc-300">
                Gracefully stop the API server. Active work is allowed to finish and queue processing is paused first.
              </p>
              <p className="text-xs text-zinc-500 mt-1">
                Use this only when an external process manager will restart the service.
              </p>
            </div>
            <Button
              variant="danger"
              size="sm"
              onClick={() => setConfirmStopServer(true)}
              disabled={stopServerMut.isPending}
            >
              <Pause size={14} /> Stop Server
            </Button>
          </div>
        </Card>
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
    </>
  );
}
