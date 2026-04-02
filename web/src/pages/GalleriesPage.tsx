import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { galleries } from '@/lib/api';
import {
  PageHeader,
  Spinner,
  EmptyState,
  Input,
  Pagination,
  ConfirmDialog,
} from '@/components/UI';
import { CoverGrid } from '@/components/CoverGrid';
import { Search, Grid3X3 } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

export function GalleriesPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const { page, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 50 });
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);

  const { data: galleryList, isLoading } = useQuery({
    queryKey: ['galleries', { search, page, limit }],
    queryFn: () =>
      galleries.list({
        search: search || undefined,
        limit,
      }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => galleries.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['galleries'] });
      setConfirmDeleteId(null);
    },
  });

  const handleDelete = (id: number) => {
    setConfirmDeleteId(id);
  };

  const coverItems = galleryList?.map((g) => ({
    id: g.id,
    title: g.title ?? null,
    thumbnailPath: g.local_thumbnail_path,
    provider: g.provider,
    createdAt: g.created_at,
    url: g.url,
  })) ?? [];

  return (
    <>
      <PageHeader title="Galleries" description="Your image gallery collection">
        <div className="flex items-center gap-2 text-zinc-400">
          <Grid3X3 size={18} />
        </div>
      </PageHeader>

      {/* Search bar */}
      <div className="mb-6 relative">
        <Search
          size={16}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500"
        />
        <Input
          placeholder="Search galleries..."
          value={search}
          onChange={(e) => {
            setSearch(e.target.value);
            resetPage();
          }}
          className="pl-9 bg-zinc-900 border-zinc-700"
        />
      </div>

      {isLoading ? (
        <Spinner />
      ) : !galleryList || galleryList.length === 0 ? (
        <EmptyState message="No galleries found." />
      ) : (
        <>
          <CoverGrid items={coverItems} onDelete={handleDelete} />

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
