import { useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
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
import { Heart, Download, Search, Palette, Trash2 } from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

export function ImagesPage() {
  const queryClient = useQueryClient();
  const { page, offset, limit, prevPage, nextPage, resetPage } = usePagination({ limit: 50 });
  const [favoritesOnly, setFavoritesOnly] = useState(false);
  const [colorSearch, setColorSearch] = useState('');
  const [activeColorSearch, setActiveColorSearch] = useState('');
  const [confirmDeleteId, setConfirmDeleteId] = useState<number | null>(null);
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);

  // Regular list query
  const { data: imageList, isLoading } = useQuery({
    queryKey: ['images', { offset, limit, is_favorite: favoritesOnly || undefined }],
    queryFn: () =>
      images.list({
        limit,
        offset,
        is_favorite: favoritesOnly || undefined,
        is_video: false,
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
          onClick={() => {
            setFavoritesOnly(!favoritesOnly);
            resetPage();
          }}
        >
          <Heart size={14} /> Favorites
        </Button>
      </PageHeader>

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
