import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { galleries } from '@/lib/api';
import { formatDate } from '@/lib/utils';
import {
  PageHeader,
  Card,
  Spinner,
  EmptyState,
  Input,
  Pagination,
  Badge,
  Button,
  ConfirmDialog,
} from '@/components/UI';
import { Trash2 } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

export function GalleriesPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const { page, offset, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 50 });
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);

  const { data: galleryList, isLoading } = useQuery({
    queryKey: ['galleries', { search, offset, limit }],
    queryFn: () =>
      galleries.list({
        search: search || undefined,
        limit,
        offset,
      }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => galleries.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['galleries'] });
      setConfirmDeleteId(null);
    },
  });

  return (
    <>
      <PageHeader title="Galleries" description="Browse image galleries" />

      <div className="mb-4">
        <Input
          placeholder="Search galleries by title..."
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            resetPage();
          }}
        />
      </div>

      {isLoading ? (
        <Spinner />
      ) : !galleryList || galleryList.length === 0 ? (
        <EmptyState message="No galleries found." />
      ) : (
        <>
          <div className="space-y-2">
            {galleryList.map((g) => (
              <Card key={g.id} className="hover:border-zinc-600 transition-colors">
                <div className="flex items-center gap-4">
                  <Link to={`/galleries/${g.id}`} className="flex items-center gap-4 min-w-0 flex-1">
                    {g.local_thumbnail_path ? (
                      <img
                        src={`/data/thumbnails/${g.local_thumbnail_path}`}
                        alt=""
                        className="w-16 h-12 object-cover rounded"
                      />
                    ) : (
                      <div className="w-16 h-12 bg-zinc-800 rounded flex items-center justify-center text-zinc-600 text-xs shrink-0">
                        No img
                      </div>
                    )}
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-white truncate">
                        {g.title || `Gallery #${g.id}`}
                      </p>
                      <div className="flex items-center gap-2 mt-1">
                        {g.provider && <Badge>{g.provider}</Badge>}
                        <span className="text-xs text-zinc-500">{formatDate(g.created_at)}</span>
                      </div>
                    </div>
                  </Link>
                  <Button
                    variant="ghost"
                    size="sm"
                    title="Delete gallery"
                    onClick={(e) => {
                      e.preventDefault();
                      setConfirmDeleteId(g.id);
                    }}
                    className="shrink-0"
                  >
                    <Trash2 size={14} className="text-zinc-500 hover:text-red-400" />
                  </Button>
                </div>
              </Card>
            ))}
          </div>

          <Pagination
            page={page}
            hasMore={galleryList.length === limit}
            onPrev={prevPage}
            onNext={nextPage}
          />
        </>
      )}

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete Gallery"
        message="Delete this gallery and all its images? Files will be removed from disk. This cannot be undone."
        confirmLabel="Delete Gallery"
        onConfirm={() => {
          if (confirmDeleteId !== null) {
            deleteMut.mutate(confirmDeleteId);
          }
        }}
        onCancel={() => setConfirmDeleteId(null)}
      />
    </>
  );
}
