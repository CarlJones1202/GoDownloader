import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { videos, images as imagesApi } from '@/lib/api';
import { formatDate } from '@/lib/utils';
import {
  PageHeader,
  Card,
  Spinner,
  EmptyState,
  Badge,
  Button,
  Pagination,
  ConfirmDialog,
} from '@/components/UI';
import { Trash2 } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

export function VideosPage() {
  const queryClient = useQueryClient();
  const { page, offset, limit, prevPage, nextPage } = usePagination({ limit: 50 });
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);

  const { data: videoList, isLoading } = useQuery({
    queryKey: ['videos', { offset, limit }],
    queryFn: () => videos.list({ limit, offset }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => imagesApi.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['videos'] });
      setConfirmDeleteId(null);
    },
  });

  return (
    <>
      <PageHeader title="Videos" description="Browse downloaded videos" />

      {isLoading ? (
        <Spinner />
      ) : !videoList || videoList.length === 0 ? (
        <EmptyState message="No videos found." />
      ) : (
        <>
          <div className="space-y-2">
            {videoList.map((vid) => (
              <Card key={vid.id} className="flex items-center gap-4">
                <div className="w-24 h-16 bg-zinc-800 rounded flex items-center justify-center text-zinc-600 text-xs shrink-0">
                  Video
                </div>
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-white truncate">{vid.filename}</p>
                  <div className="flex items-center gap-2 mt-1">
                    {vid.duration_seconds && (
                      <Badge>{Math.floor(vid.duration_seconds / 60)}:{String(vid.duration_seconds % 60).padStart(2, '0')}</Badge>
                    )}
                    {vid.width && vid.height && (
                      <Badge variant="info">{vid.width}x{vid.height}</Badge>
                    )}
                    {vid.vr_mode !== 'none' && (
                      <Badge variant="warning">VR {vid.vr_mode}</Badge>
                    )}
                    <span className="text-xs text-zinc-500">{formatDate(vid.created_at)}</span>
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  title="Delete video"
                  onClick={() => setConfirmDeleteId(vid.id)}
                  className="shrink-0"
                >
                  <Trash2 size={14} className="text-zinc-500 hover:text-red-400" />
                </Button>
              </Card>
            ))}
          </div>

          <Pagination
            page={page}
            hasMore={videoList.length === limit}
            onPrev={prevPage}
            onNext={nextPage}
          />
        </>
      )}

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete Video"
        message="Delete this video? The file will be removed from disk. This cannot be undone."
        confirmLabel="Delete Video"
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
