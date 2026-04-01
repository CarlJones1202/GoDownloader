import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
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
} from '@/components/UI';

export function GalleriesPage() {
  const [search, setSearch] = useState('');
  const [offset, setOffset] = useState(0);
  const limit = 50;

  const { data: galleryList, isLoading } = useQuery({
    queryKey: ['galleries', { search, offset, limit }],
    queryFn: () =>
      galleries.list({
        search: search || undefined,
        limit,
        offset,
      }),
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
            setOffset(0);
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
              <Link key={g.id} to={`/galleries/${g.id}`}>
                <Card className="hover:border-zinc-600 transition-colors cursor-pointer">
                  <div className="flex items-center gap-4">
                    {g.local_thumbnail_path ? (
                      <img
                        src={`/data/thumbnails/${g.local_thumbnail_path}`}
                        alt=""
                        className="w-16 h-12 object-cover rounded"
                      />
                    ) : (
                      <div className="w-16 h-12 bg-zinc-800 rounded flex items-center justify-center text-zinc-600 text-xs">
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
                  </div>
                </Card>
              </Link>
            ))}
          </div>

          <Pagination
            offset={offset}
            limit={limit}
            hasMore={galleryList.length === limit}
            onPrev={() => setOffset(Math.max(0, offset - limit))}
            onNext={() => setOffset(offset + limit)}
          />
        </>
      )}
    </>
  );
}
