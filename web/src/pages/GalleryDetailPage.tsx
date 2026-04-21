import { useState, useMemo, useCallback } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { galleries, images as imagesApi } from '@/lib/api';
import { formatDate, parseColors, thumbnailUrl } from '@/lib/utils';
import {
  PageHeader,
  Spinner,
  EmptyState,
  Badge,
  Button,
  ConfirmDialog,
  Input,
} from '@/components/UI';
import { JustifiedGrid } from '@/components/JustifiedGrid';
import type { JustifiedItem } from '@/components/JustifiedGrid';
import { Lightbox } from '@/components/Lightbox';
import type { GallerySearchResult } from '@/types';
import {
  Heart,
  ArrowLeft,
  Trash2,
  Edit2,
  Save,
  X,
  Search,
  Star,
  Calendar,
  ExternalLink,
  FileText,
  Loader2,
} from 'lucide-react';
import { cn } from '@/lib/utils';

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try {
    const parsed = JSON.parse(photos);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

export function GalleryDetailPage() {
  const { id } = useParams<{ id: string }>();
  const galleryId = Number(id);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const [confirmDeleteGallery, setConfirmDeleteGallery] = useState(false);
  const [confirmDeleteImageId, setConfirmDeleteImageId] = useState<number | null>(null);
  const [lightboxIndex, setLightboxIndex] = useState<number | null>(null);
  const [isEditingTitle, setIsEditingTitle] = useState(false);
  const [editedTitle, setEditedTitle] = useState('');
  const [sortBy, setSortBy] = useState<'newest' | 'oldest' | 'largest' | 'smallest'>('newest');

  // Metadata search state
  const [showMetadataSearch, setShowMetadataSearch] = useState(false);
  const [metadataQuery, setMetadataQuery] = useState('');
  const [metadataProvider, setMetadataProvider] = useState('');
  const [searchResults, setSearchResults] = useState<GallerySearchResult[]>([]);
  const [isSearching, setIsSearching] = useState(false);
  const [scrapingUrl, setScrapingUrl] = useState<string | null>(null);

  const { data: gallery, isLoading: loadingGallery } = useQuery({
    queryKey: ['gallery', galleryId],
    queryFn: () => galleries.get(galleryId),
  });

  const { data: metadataProviders } = useQuery({
    queryKey: ['metadata-providers'],
    queryFn: () => galleries.metadataProviders(),
    staleTime: Infinity,
  });

  const { data: imageList, isLoading: loadingImages } = useQuery({
    queryKey: ['images', { gallery_id: galleryId, sort_by: sortBy }],
    queryFn: () => imagesApi.list({ gallery_id: galleryId, limit: 200, sort_by: sortBy }),
  });
  const { data: linkedPeople } = useQuery({
    queryKey: ['gallery-people', galleryId],
    queryFn: () => galleries.people(galleryId),
  });

  const favMut = useMutation({
    mutationFn: (imgId: number) => imagesApi.toggleFavorite(imgId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] });
    },
  });

  const deleteGalleryMut = useMutation({
    mutationFn: () => galleries.delete(galleryId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['galleries'] });
      navigate('/galleries');
    },
  });

  const updateTitleMut = useMutation({
    mutationFn: (title: string) => galleries.update(galleryId, { title }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] });
      setIsEditingTitle(false);
    },
  });

  const startEditTitle = () => {
    setEditedTitle(gallery?.title || '');
    setIsEditingTitle(true);
  };

  const cancelEditTitle = () => {
    setIsEditingTitle(false);
    setEditedTitle('');
  };

  const saveTitle = () => {
    updateTitleMut.mutate(editedTitle);
  };

  const deleteImageMut = useMutation({
    mutationFn: (imgId: number) => imagesApi.delete(imgId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] });
      setConfirmDeleteImageId(null);
    },
  });

  // Metadata search
  const openMetadataSearch = useCallback(() => {
    setMetadataQuery(gallery?.title || '');
    setMetadataProvider('');
    setSearchResults([]);
    setShowMetadataSearch(true);
  }, [gallery?.title]);

  const runMetadataSearch = useCallback(async () => {
    if (!metadataQuery.trim()) return;
    setIsSearching(true);
    try {
      const results = await galleries.searchMetadata(
        galleryId,
        metadataQuery.trim(),
        metadataProvider || undefined,
      );
      setSearchResults(results);
    } catch {
      setSearchResults([]);
    } finally {
      setIsSearching(false);
    }
  }, [galleryId, metadataQuery, metadataProvider]);

  const applyMetadata = useCallback(
    async (result: GallerySearchResult) => {
      setScrapingUrl(result.url);
      try {
        await galleries.scrapeMetadata(galleryId, {
          provider: result.provider,
          url: result.url,
          source_id: result.source_id,
        });
        queryClient.invalidateQueries({ queryKey: ['gallery', galleryId] });
        setShowMetadataSearch(false);
      } catch (err) {
        console.error('Failed to apply metadata:', err);
      } finally {
        setScrapingUrl(null);
      }
    },
    [galleryId, queryClient],
  );

  // Build justified grid items from image list.
  const gridItems: JustifiedItem[] = useMemo(() => {
    if (!imageList) return [];
    return imageList.items.map((img) => {
      const colors = parseColors(img.dominant_colors);
      return {
        id: img.id,
        src: `/data/images/${img.filename}`,
        thumbSrc: thumbnailUrl(img.filename),
        width: img.width,
        height: img.height,
        persistentOverlay: img.is_favorite ? (
          <div className="absolute bottom-0 left-0 p-2 pointer-events-auto">
            <button
              onClick={(e) => {
                e.stopPropagation();
                favMut.mutate(img.id);
              }}
              className="p-1"
            >
              <Heart size={16} className="fill-red-500 text-red-500" />
            </button>
          </div>
        ) : undefined,
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
                    setConfirmDeleteImageId(img.id);
                  }}
                  className="p-1"
                  title="Delete image"
                >
                  <Trash2 size={16} className="text-white hover:text-red-400" />
                </button>
              </div>
              <div className="flex items-center gap-1">
                {img.is_video && <Badge variant="info">Video</Badge>}
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
  }, [imageList, favMut]);

  // Build lightbox images (full-size URLs).
  const lightboxImages = useMemo(() => {
    if (!imageList) return [];
    return imageList.items.map((img) => ({
      src: `/data/images/${img.filename}`,
      alt: img.filename,
    }));
  }, [imageList]);

  if (loadingGallery) return <Spinner />;
  if (!gallery) return <EmptyState message="Gallery not found." />;

  const hasMetadata = gallery.description || gallery.rating || gallery.release_date || gallery.source_url;

  return (
    <>
      <div className="mb-4">
        <Link
          to="/galleries"
          className="text-sm text-zinc-400 hover:text-zinc-200 inline-flex items-center gap-1"
        >
          <ArrowLeft size={14} /> Back to galleries
        </Link>
      </div>

      <section className="mb-6 rounded-2xl border border-zinc-800 bg-zinc-900 p-4 md:p-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            {isEditingTitle ? (
              <div className="flex items-center gap-2">
                <Input
                  value={editedTitle}
                  onChange={(e) => setEditedTitle(e.target.value)}
                  className="w-64 h-8"
                  autoFocus
                  onKeyDown={(e) => e.key === 'Enter' && saveTitle()}
                />
                <Button size="sm" onClick={saveTitle} disabled={updateTitleMut.isPending}>
                  <Save size={14} />
                </Button>
                <Button size="sm" variant="ghost" onClick={cancelEditTitle}>
                  <X size={14} />
                </Button>
              </div>
            ) : (
              <h1 className="text-2xl md:text-3xl font-semibold text-white truncate">
                {gallery.title || `Gallery #${gallery.id}`}
              </h1>
            )}
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
              <span className="rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1 text-zinc-400">
                Created {formatDate(gallery.created_at)}
              </span>
              {imageList && (
                <span className="rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1 text-zinc-400">
                  {imageList.total_items} images
                </span>
              )}
              {gallery.provider && <Badge>{gallery.provider}</Badge>}
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            {!isEditingTitle && (
              <Button variant="secondary" size="sm" onClick={startEditTitle}>
                <Edit2 size={14} /> Edit
              </Button>
            )}
            <Button variant="secondary" size="sm" onClick={openMetadataSearch}>
              <Search size={14} /> Metadata
            </Button>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as typeof sortBy)}
              className="bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-1.5 text-sm text-zinc-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
            >
              <option value="newest">Newest first</option>
              <option value="oldest">Oldest first</option>
              <option value="largest">Largest first</option>
              <option value="smallest">Smallest first</option>
            </select>
            <Button
              variant="danger"
              size="sm"
              onClick={() => setConfirmDeleteGallery(true)}
            >
              <Trash2 size={14} /> Delete
            </Button>
          </div>
        </div>

        {(gallery.url || hasMetadata) && (
          <div className="mt-4 grid gap-3 md:grid-cols-[1.2fr,1fr]">
            <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
              <p className="text-xs uppercase tracking-wide text-zinc-500 mb-1">Gallery URL</p>
              {gallery.url ? (
                <a
                  href={gallery.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-sm text-blue-400 hover:underline break-all"
                >
                  {gallery.url}
                </a>
              ) : (
                <p className="text-sm text-zinc-500">No source URL saved.</p>
              )}
            </div>

            {hasMetadata ? (
              <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3 space-y-2">
                <div className="flex items-center gap-2 text-zinc-300 text-sm font-medium">
                  <FileText size={14} />
                  Metadata
                </div>
                {gallery.description && (
                  <p className="text-sm text-zinc-400 line-clamp-3">{gallery.description}</p>
                )}
                <div className="flex flex-wrap items-center gap-3 text-zinc-400 text-xs">
                  {gallery.rating != null && gallery.rating > 0 && (
                    <span className="inline-flex items-center gap-1">
                      <Star size={12} className="text-amber-400" />
                      {gallery.rating.toFixed(1)}
                    </span>
                  )}
                  {gallery.release_date && (
                    <span className="inline-flex items-center gap-1">
                      <Calendar size={12} />
                      {gallery.release_date}
                    </span>
                  )}
                  {gallery.source_url && (
                    <a
                      href={gallery.source_url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 text-blue-400 hover:underline"
                    >
                      <ExternalLink size={12} />
                      Source
                    </a>
                  )}
                </div>
              </div>
            ) : (
              <div className="rounded-lg border border-dashed border-zinc-800 bg-zinc-950/30 p-3 text-sm text-zinc-500">
                No metadata yet. Use Metadata Search to enrich this gallery.
              </div>
            )}
          </div>
        )}

        <div className="mt-4 rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
          <div className="flex items-center justify-between mb-2">
            <p className="text-xs uppercase tracking-wide text-zinc-500">Linked People</p>
            <span className="text-xs text-zinc-500">{linkedPeople?.length ?? 0} linked</span>
          </div>
          {!linkedPeople || linkedPeople.length === 0 ? (
            <p className="text-sm text-zinc-500">No linked people yet.</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {linkedPeople.map((person) => {
                const photo = parsePhotos(person.photos)[0];
                return (
                  <Link
                    key={person.id}
                    to={`/people/${person.id}`}
                    className="inline-flex items-center gap-2 rounded-md border border-zinc-800 bg-zinc-900 px-2 py-1.5 hover:border-zinc-600 transition-colors"
                  >
                    {photo ? (
                      <img src={photo} alt={person.name} className="h-7 w-7 rounded object-cover" />
                    ) : (
                      <div className="h-7 w-7 rounded bg-zinc-800 flex items-center justify-center">
                        <span className="text-[10px] text-zinc-500">N/A</span>
                      </div>
                    )}
                    <span className="text-sm text-zinc-200">{person.name}</span>
                  </Link>
                );
              })}
            </div>
          )}
        </div>
      </section>

      <section className="rounded-2xl border border-zinc-800 bg-zinc-900 p-3 md:p-4">
        {loadingImages ? (
          <Spinner />
        ) : !imageList || imageList.items.length === 0 ? (
          <EmptyState message="No images in this gallery." />
        ) : (
          <JustifiedGrid
            items={gridItems}
            rowHeight={230}
            gap={4}
            onItemClick={(index) => setLightboxIndex(index)}
          />
        )}
      </section>

      {/* Lightbox */}
      {lightboxIndex !== null && (
        <Lightbox
          images={lightboxImages}
          index={lightboxIndex}
          onClose={() => setLightboxIndex(null)}
          onIndexChange={setLightboxIndex}
          imageData={imageList.items}
          onToggleFavorite={(id) => favMut.mutate(id)}
        />
      )}

      {/* Metadata search modal */}
      {showMetadataSearch && (
        <div className="fixed inset-0 z-50 flex items-start justify-center pt-16 bg-black/70">
          <div className="bg-zinc-900 border border-zinc-700 rounded-xl shadow-2xl w-full max-w-3xl max-h-[80vh] flex flex-col">
            {/* Header */}
            <div className="flex items-center justify-between p-4 border-b border-zinc-700">
              <h2 className="text-lg font-semibold text-zinc-100">Search Gallery Metadata</h2>
              <button
                onClick={() => setShowMetadataSearch(false)}
                className="p-1 text-zinc-400 hover:text-zinc-200"
              >
                <X size={18} />
              </button>
            </div>

            {/* Search bar */}
            <div className="p-4 border-b border-zinc-800">
              <div className="flex gap-2">
                <Input
                  value={metadataQuery}
                  onChange={(e) => setMetadataQuery(e.target.value)}
                  placeholder="Search by gallery title..."
                  className="flex-1"
                  autoFocus
                  onKeyDown={(e) => e.key === 'Enter' && runMetadataSearch()}
                />
                <select
                  value={metadataProvider}
                  onChange={(e) => setMetadataProvider(e.target.value)}
                  className="bg-zinc-800 border border-zinc-700 rounded-lg px-2 text-sm text-zinc-200 focus:outline-none focus:ring-1 focus:ring-blue-500"
                >
                  <option value="">All providers</option>
                  {metadataProviders?.map((p) => (
                    <option key={p} value={p}>{p}</option>
                  ))}
                </select>
                <Button onClick={runMetadataSearch} disabled={isSearching || !metadataQuery.trim()}>
                  {isSearching ? <Loader2 size={14} className="animate-spin" /> : <Search size={14} />}
                  Search
                </Button>
              </div>
              <p className="text-xs text-zinc-500 mt-2">
                {metadataProvider
                  ? `Searching: ${metadataProvider}`
                  : `Searches ${metadataProviders?.length ?? 12} providers`}
              </p>
            </div>

            {/* Results */}
            <div className="flex-1 overflow-y-auto p-4">
              {isSearching ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 size={24} className="animate-spin text-zinc-400" />
                  <span className="ml-2 text-zinc-400">Searching providers...</span>
                </div>
              ) : searchResults.length === 0 ? (
                <div className="text-center py-12 text-zinc-500">
                  {metadataQuery ? 'No results. Try a different search term.' : 'Enter a search term to find matching galleries.'}
                </div>
              ) : (
                <div className="space-y-2">
                  <p className="text-xs text-zinc-500 mb-3">{searchResults.length} results found</p>
                  {searchResults.map((result, i) => (
                    <button
                      key={`${result.provider}-${result.url}-${i}`}
                      onClick={() => applyMetadata(result)}
                      disabled={scrapingUrl !== null}
                      className={cn(
                        'w-full flex items-start gap-3 p-3 rounded-lg border border-zinc-700 hover:border-zinc-500 hover:bg-zinc-800/80 transition-colors text-left',
                        scrapingUrl === result.url && 'border-blue-500 bg-blue-500/10',
                      )}
                    >
                      {/* Thumbnail */}
                      {result.thumbnail ? (
                        <img
                          src={result.thumbnail}
                          alt=""
                          className="w-16 h-16 object-cover rounded flex-shrink-0"
                          loading="lazy"
                          onError={(e) => {
                            (e.target as HTMLImageElement).style.display = 'none';
                          }}
                        />
                      ) : (
                        <div className="w-16 h-16 bg-zinc-800 rounded flex-shrink-0 flex items-center justify-center">
                          <FileText size={20} className="text-zinc-600" />
                        </div>
                      )}

                      {/* Info */}
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 mb-1">
                          <Badge>{result.provider}</Badge>
                          <span className="text-sm text-zinc-200 truncate">{result.title}</span>
                        </div>
                        {result.release_date && (
                          <span className="text-xs text-zinc-500 inline-flex items-center gap-1">
                            <Calendar size={10} />
                            {result.release_date}
                          </span>
                        )}
                      </div>

                      {/* Loading indicator for this specific result */}
                      {scrapingUrl === result.url && (
                        <Loader2 size={16} className="animate-spin text-blue-400 flex-shrink-0 mt-1" />
                      )}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* Confirm dialogs */}
      <ConfirmDialog
        open={confirmDeleteGallery}
        title="Delete Gallery"
        message="Delete this gallery and all its images? Files will be removed from disk. This cannot be undone."
        confirmLabel="Delete Gallery"
        onConfirm={() => deleteGalleryMut.mutate()}
        onCancel={() => setConfirmDeleteGallery(false)}
      />

      <ConfirmDialog
        open={confirmDeleteImageId !== null}
        title="Delete Image"
        message="Delete this image? The file will be removed from disk. This cannot be undone."
        confirmLabel="Delete Image"
        onConfirm={() => {
          if (confirmDeleteImageId !== null) {
            deleteImageMut.mutate(confirmDeleteImageId);
          }
        }}
        onCancel={() => setConfirmDeleteImageId(null)}
      />
    </>
  );
}