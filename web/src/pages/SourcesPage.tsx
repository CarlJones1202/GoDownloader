import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { sources, admin } from '@/lib/api';
import { formatDateTime } from '@/lib/utils';
import {
  PageHeader,
  Button,
  Card,
  Badge,
  Spinner,
  EmptyState,
  Input,
  ConfirmDialog,
} from '@/components/UI';
import { Plus, Play, RotateCcw, Trash2, ArrowUpCircle } from 'lucide-react';

export function SourcesPage() {
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [newSource, setNewSource] = useState({ url: '', name: '', priority: 0 });

  const { data: sourceList, isLoading } = useQuery({
    queryKey: ['sources'],
    queryFn: sources.list,
  });

  const createMut = useMutation({
    mutationFn: () => sources.create(newSource),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      setShowCreate(false);
      setNewSource({ url: '', name: '', priority: 0 });
    },
  });

  const crawlMut = useMutation({
    mutationFn: (id: number) => sources.crawl(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sources'] }),
  });

  const recrawlMut = useMutation({
    mutationFn: (id: number) => sources.recrawl(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['sources'] }),
  });

  const prioritizeMut = useMutation({
    mutationFn: (id: number) => admin.prioritizeSource(id),
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      alert(data.message || 'Source prioritized.');
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => sources.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      setDeleteId(null);
    },
  });

  if (isLoading) return <Spinner />;

  return (
    <>
      <PageHeader title="Sources" description="Manage crawlable content sources">
        <Button onClick={() => setShowCreate(!showCreate)}>
          <Plus size={14} /> Add Source
        </Button>
      </PageHeader>

      {/* Create form */}
      {showCreate && (
        <Card className="mb-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <Input
              label="URL"
              placeholder="https://forum.example.com/thread/123"
              value={newSource.url}
              onChange={(e) => setNewSource({ ...newSource, url: e.target.value })}
            />
            <Input
              label="Name"
              placeholder="Source name"
              value={newSource.name}
              onChange={(e) => setNewSource({ ...newSource, name: e.target.value })}
            />
            <Input
              label="Priority"
              type="number"
              value={newSource.priority}
              onChange={(e) => setNewSource({ ...newSource, priority: parseInt(e.target.value) || 0 })}
            />
          </div>
          <div className="flex justify-end gap-2 mt-3">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={() => createMut.mutate()}
              disabled={!newSource.url || !newSource.name || createMut.isPending}
            >
              Create
            </Button>
          </div>
        </Card>
      )}

      {/* Source list */}
      {!sourceList || sourceList.length === 0 ? (
        <EmptyState message="No sources yet. Add one to get started." />
      ) : (
        <div className="space-y-2">
          {sourceList.map((src) => (
            <Card key={src.id} className="flex items-center justify-between">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-white truncate">{src.name}</span>
                  <Badge variant={src.enabled ? 'success' : 'default'}>
                    {src.enabled ? 'enabled' : 'disabled'}
                  </Badge>
                  {src.priority > 0 && <Badge variant="info">P{src.priority}</Badge>}
                </div>
                <p className="text-xs text-zinc-500 truncate mt-0.5">{src.url}</p>
                <p className="text-xs text-zinc-600 mt-0.5">
                  Last crawled: {formatDateTime(src.last_crawled_at)}
                </p>
              </div>
              <div className="flex items-center gap-1 ml-4">
                <Button
                  variant="ghost"
                  size="sm"
                  title="Crawl"
                  onClick={() => crawlMut.mutate(src.id)}
                  disabled={crawlMut.isPending}
                >
                  <Play size={14} />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  title="Prioritize (move to top of queue)"
                  onClick={() => prioritizeMut.mutate(src.id)}
                  disabled={prioritizeMut.isPending}
                  className="text-amber-500 hover:text-amber-400 hover:bg-amber-500/10"
                >
                  <ArrowUpCircle size={14} />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  title="Full recrawl"
                  onClick={() => recrawlMut.mutate(src.id)}
                  disabled={recrawlMut.isPending}
                >
                  <RotateCcw size={14} />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  title="Delete"
                  onClick={() => setDeleteId(src.id)}
                >
                  <Trash2 size={14} />
                </Button>
              </div>
            </Card>
          ))}
        </div>
      )}

      <ConfirmDialog
        open={deleteId !== null}
        title="Delete Source"
        message="Are you sure? This will remove the source. Galleries and images will not be deleted."
        onConfirm={() => deleteId && deleteMut.mutate(deleteId)}
        onCancel={() => setDeleteId(null)}
      />
    </>
  );
}
