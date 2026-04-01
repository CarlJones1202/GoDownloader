import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { videos } from '@/lib/api';
import { formatDate } from '@/lib/utils';
import {
  PageHeader,
  Card,
  Spinner,
  EmptyState,
  Badge,
  Pagination,
} from '@/components/UI';

export function VideosPage() {
  const [offset, setOffset] = useState(0);
  const limit = 50;

  const { data: videoList, isLoading } = useQuery({
    queryKey: ['videos', { offset, limit }],
    queryFn: () => videos.list({ limit, offset }),
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
              </Card>
            ))}
          </div>

          <Pagination
            offset={offset}
            limit={limit}
            hasMore={videoList.length === limit}
            onPrev={() => setOffset(Math.max(0, offset - limit))}
            onNext={() => setOffset(offset + limit)}
          />
        </>
      )}
    </>
  );
}
