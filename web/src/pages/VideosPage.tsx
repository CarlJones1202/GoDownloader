import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { videos, images as imagesApi } from '@/lib/api';
import { formatDate, formatDuration, thumbnailUrl } from '@/lib/utils';
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
import { VideoPlayer } from '@/components/VideoPlayer';
import { Heart, Play, Trash2 } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';
import type { Image } from '@/types';

export function VideosPage() {
  const queryClient = useQueryClient();
  const { page, offset, limit, prevPage, nextPage } = usePagination({ limit: 50 });
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);
  const [activeVideo, setActiveVideo] = useState<Image | null>(null);

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

  const favMut = useMutation({
    mutationFn: (id: number) => imagesApi.toggleFavorite(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['videos'] });
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
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {videoList.map((vid) => (
              <div
                key={vid.id}
                className="group cursor-pointer"
                onClick={() => setActiveVideo(vid)}
              >
              <Card
                className="overflow-hidden"
              >
                {/* Thumbnail with play overlay */}
                <div className="relative aspect-video bg-zinc-800 overflow-hidden">
                  <img
                    src={thumbnailUrl(vid.filename)}
                    alt={vid.filename}
                    className="w-full h-full object-cover"
                    loading="lazy"
                    onError={(e) => {
                      // Fall back to grey placeholder if no thumbnail yet.
                      (e.target as HTMLImageElement).style.display = 'none';
                    }}
                  />
                  {/* Play icon overlay */}
                  <div className="absolute inset-0 flex items-center justify-center bg-black/20 opacity-0 group-hover:opacity-100 transition-opacity">
                    <div className="w-12 h-12 rounded-full bg-black/60 flex items-center justify-center">
                      <Play size={24} className="text-white ml-0.5" />
                    </div>
                  </div>
                  {/* Duration badge */}
                  {vid.duration_seconds != null && vid.duration_seconds > 0 && (
                    <div className="absolute bottom-2 right-2 px-1.5 py-0.5 rounded bg-black/70 text-white text-xs font-mono">
                      {formatDuration(vid.duration_seconds)}
                    </div>
                  )}
                </div>
                {/* Info row */}
                <div className="p-2">
                  <p className="text-xs text-white truncate">{vid.filename}</p>
                  <div className="flex items-center gap-2 mt-1">
                    {vid.width != null && vid.height != null && (
                      <Badge variant="info">{vid.width}x{vid.height}</Badge>
                    )}
                    {vid.vr_mode !== 'none' && (
                      <Badge variant="warning">VR {vid.vr_mode}</Badge>
                    )}
                    <span className="text-xs text-zinc-500 ml-auto">{formatDate(vid.created_at)}</span>
                  </div>
                  {/* Action buttons */}
                  <div className="flex items-center gap-1 mt-2 opacity-0 group-hover:opacity-100 transition-opacity"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <Button
                      variant="ghost"
                      size="sm"
                      title={vid.is_favorite ? 'Remove from favorites' : 'Add to favorites'}
                      onClick={() => favMut.mutate(vid.id)}
                    >
                      <Heart
                        size={14}
                        className={vid.is_favorite ? 'text-red-400 fill-red-400' : 'text-zinc-500 hover:text-red-400'}
                      />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      title="Delete video"
                      onClick={() => setConfirmDeleteId(vid.id)}
                    >
                      <Trash2 size={14} className="text-zinc-500 hover:text-red-400" />
                    </Button>
                  </div>
                </div>
              </Card>
              </div>
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

      {/* Video Player overlay */}
      {activeVideo && (
        <VideoPlayer video={activeVideo} onClose={() => setActiveVideo(null)} />
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
