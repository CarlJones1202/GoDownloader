import { useState, useMemo, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { images } from '@/lib/api';
import { cn, parseColors, thumbnailUrl } from '@/lib/utils';
import {
  PageHeader,
  Card,
  Spinner,
  EmptyState,
  Input,
  Button,
  Pagination,
  ConfirmDialog,
} from '@/components/UI';
import { JustifiedGrid } from '@/components/JustifiedGrid';
import type { JustifiedItem } from '@/components/JustifiedGrid';
import { Lightbox } from '@/components/Lightbox';
import { Heart, Download, Search, Palette, Trash2, Shuffle, HardDrive } from 'lucide-react';
import { Select } from '@/components/UI';
import { usePagination } from '@/hooks/usePagination';

export function ImagesPage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const { page, offset, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 50 });
  
  const sortBy = (searchParams.get('sort') as any) || 'newest';
  const randomSeed = Number(searchParams.get('seed')) || 0;
  const onDiskOnly = searchParams.get('on_disk') === 'true';
  const favoritesOnly = searchParams.get('favorites') === 'true';

  const [colorSearch, setColorSearch] = useState('');
  const [activeColorSearch, setActiveColorSearch] = useState('');
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);

  const updateFilter = useCallback((key: string, value: any) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (value === null || value === false || value === '' || (key === 'sort' && value === 'newest') || (key === 'seed' && value === 0)) {
        next.delete(key);
      } else {
        next.set(key, String(value));
      }
      next.delete('page');
      return next;
    }, { replace: true });
  }, [setSearchParams]);

  // Regular list query
  const { data: imageList, isLoading } = useQuery({
    queryKey: ['images', { offset, limit, is_favorite: favoritesOnly || undefined, sort_by: sortBy, random_seed: sortBy === 'random' ? randomSeed : undefined, on_disk: onDiskOnly || undefined }],
    queryFn: () =>
      images.list({
        limit,
        offset,
        is_favorite: favoritesOnly || undefined,
        is_video: false,
        sort_by: sortBy,
        random_seed: sortBy === 'random' ? randomSeed : undefined,
        on_disk: onDiskOnly || undefined,
      }),
    enabled: !activeColorSearch,
  });

  // Color search query
  const { data: colorResults, isLoading: isColorLoading } = useQuery({
    queryKey: ['images', 'color', activeColorSearch],
    queryFn: () => images.searchByColor(activeColorSearch, 50),
    enabled: !!activeColorSearch,
  });

  const favMut = useMutation({
    mutationFn: (id: number) => images.toggleFavorite(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images'] });
    },
  });

  const redownloadMut = useMutation({
    mutationFn: (id: number) => images.redownload(id),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => images.delete(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images'] });
      setConfirmDeleteId(null);
    },
  });

  const handleColorSearch = () => {
    if (colorSearch.trim()) {
      setActiveColorSearch(colorSearch.trim());
    }
  };

  const clearColorSearch = () => {
    setColorSearch('');
    setActiveColorSearch('');
  };

  const displayImages = activeColorSearch
    ? (colorResults ?? []).map((r) => r.image)
    : (imageList?.items ?? []);

  const loading = activeColorSearch ? isColorLoading : isLoading;

  // Build justified grid items.
  const gridItems: JustifiedItem[] = useMemo(() => {
    return displayImages.map((img) => {
      const colors = parseColors(img.dominant_colors);
      return {
        id: img.id,
        src: `/data/images/${img.filename}`,
        thumbSrc: thumbnailUrl(img.filename),
        width: img.width,
        height: img.height,
        overlay: (
          <div className="flex flex-col justify-end h-full bg-gradient-to-t from-black/60 to-transparent p-2">
            <div className="flex items-center justify-between w-full">
              <div className="flex items-center gap-1">
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    favMut.mutate(img.id);
                  }}
                  className="p-1"
                >
                  <Heart
                    size={16}
                    className={cn(
                      img.is_favorite ? 'fill-red-500 text-red-500' : 'text-white',
                    )}
                  />
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    redownloadMut.mutate(img.id);
                  }}
                  className="p-1"
                  title="Re-download"
                >
                  <Download size={16} className="text-white" />
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    setConfirmDeleteId(img.id);
                  }}
                  className="p-1"
                  title="Delete image"
                >
                  <Trash2 size={16} className="text-white hover:text-red-400" />
                </button>
              </div>
              <div className="flex items-center gap-1">
                {img.width && img.height && (
                  <span className="text-[10px] text-white/70">
                    {img.width}x{img.height}
                  </span>
                )}
                {colors.length > 0 && (
                  <div className="flex h-2 rounded overflow-hidden">
                    {colors.map((c, i) => (
                      <div key={i} className="w-2" style={{ backgroundColor: c }} />
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        ),
      };
    });
  }, [displayImages, favMut, redownloadMut]);

  // Build lightbox images.
  const lightboxImages = useMemo(() => {
    return displayImages.map((img) => ({
      src: `/data/images/${img.filename}`,
      alt: img.filename,
    }));
  }, [displayImages]);

  return (
    <>
      <PageHeader title="Images" description="Browse and manage images">
        <Button
          variant={favoritesOnly ? 'primary' : 'secondary'}
          size="sm"
          onClick={() => updateFilter('favorites', !favoritesOnly)}
        >
          <Heart size={14} /> Favorites
        </Button>
      </PageHeader>

      {/* Filters and Sorting */}
      <Card className="mb-4">
        <div className="flex flex-wrap items-end gap-4">
          <div className="w-48">
            <Select
              label="Sort By"
              value={sortBy}
              onChange={(e) => updateFilter('sort', e.target.value)}
              options={[
                { value: 'newest', label: 'Newest first' },
                { value: 'oldest', label: 'Oldest first' },
                { value: 'largest', label: 'Largest first' },
                { value: 'smallest', label: 'Smallest first' },
                { value: 'random', label: 'Random' },
              ]}
            />
          </div>

          {sortBy === 'random' && (
            <>
              <div className="w-32">
                <Input
                  label="Seed"
                  type="number"
                  value={randomSeed}
                  onChange={(e) => updateFilter('seed', e.target.value)}
                />
              </div>
              <Button
                variant="secondary"
                size="md"
                className="mb-0.5 h-10"
                onClick={() => updateFilter('seed', Math.floor(Math.random() * 1000000))}
              >
                <Shuffle size={14} className="mr-1" /> Shuffle
              </Button>
            </>
          )}

          <div className="flex items-center gap-2 h-10 mb-0.5">
            <input
              id="onDiskOnly"
              type="checkbox"
              checked={onDiskOnly}
              onChange={(e) => updateFilter('on_disk', e.target.checked)}
              className="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-blue-600 focus:ring-blue-500 focus:ring-offset-zinc-900"
            />
            <label htmlFor="onDiskOnly" className="text-sm font-medium text-zinc-300 cursor-pointer flex items-center gap-1.5">
              <HardDrive size={14} /> Only images on disk
            </label>
          </div>
        </div>
      </Card>

      {/* Color search */}
      <Card className="mb-4">
        <div className="flex items-center gap-2">
          <Palette size={16} className="text-zinc-400" />
          <span className="text-sm text-zinc-400">Color Search</span>
        </div>
        <div className="flex gap-2 mt-2">
          <div className="flex-1 flex gap-2">
            <Input
              placeholder="#ff0000 or ff0000"
              value={colorSearch}
              onChange={(e) => setColorSearch(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleColorSearch()}
            />
            {colorSearch && (
              <div
                className="w-10 h-10 rounded border border-zinc-700 shrink-0"
                style={{ backgroundColor: colorSearch.startsWith('#') ? colorSearch : `#${colorSearch}` }}
              />
            )}
          </div>
          <Button size="sm" onClick={handleColorSearch} disabled={!colorSearch.trim()}>
            <Search size={14} /> Search
          </Button>
          {activeColorSearch && (
            <Button variant="secondary" size="sm" onClick={clearColorSearch}>
              Clear
            </Button>
          )}
        </div>
      </Card>

      {loading ? (
        <Spinner />
      ) : displayImages.length === 0 ? (
        <EmptyState message="No images found." />
      ) : (
        <>
          <JustifiedGrid
            items={gridItems}
            rowHeight={220}
            gap={4}
            onItemClick={(index) => setLightboxIndex(index)}
          />

          {!activeColorSearch && imageList && (
            <Pagination
              page={imageList.current_page}
              totalPages={imageList.total_pages}
              onPrev={prevPage}
              onNext={nextPage}
              hasMore={imageList.current_page < imageList.total_pages}
            />
          )}
        </>
      )}

      {/* Lightbox */}
      {lightboxIndex !== null && (
        <Lightbox
          images={lightboxImages}
          index={lightboxIndex}
          onClose={() => setLightboxIndex(null)}
          onIndexChange={setLightboxIndex}
          imageData={displayImages}
          onToggleFavorite={(id) => favMut.mutate(id)}
        />
      )}

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete Image"
        message="Delete this image? The file will be removed from disk. This cannot be undone."
        confirmLabel="Delete Image"
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
