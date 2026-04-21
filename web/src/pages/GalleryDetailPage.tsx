import { useState, useMemo, useCallback } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { galleries, images as imagesApi, people } from '@/lib/api';
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

  const unlinkPersonMut = useMutation({
    mutationFn: (personId: number) => people.unlinkGallery(personId, galleryId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['gallery-people', galleryId] });
    },
  });

  const handleDeleteImage = (imgId: number) => {
    setConfirmDeleteImageId(imgId);
  };

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
    <div className="relative">
      {/* Immersive Background Layer with Masked Fade */}
      <div className="absolute inset-x-0 -top-6 -mx-6 h-[800px] pointer-events-none select-none overflow-hidden">
        {gallery.local_thumbnail_path ? (
          <div 
            className="h-full w-full"
            style={{ 
              maskImage: 'linear-gradient(to bottom, black 0%, transparent 100%)',
              WebkitMaskImage: 'linear-gradient(to bottom, black 0%, transparent 100%)'
            }}
          >
            <img
              src={`/data/thumbnails/${gallery.local_thumbnail_path.split('/').pop()}`}
              alt=""
              className="h-full w-full object-cover scale-150 blur-[120px] opacity-60"
            />
          </div>
        ) : (
          <div className="h-full w-full bg-zinc-900" />
        )}
      </div>

      <div className="relative z-10">
        <Link
          to="/galleries"
          className="group text-sm text-zinc-400 hover:text-white transition-colors inline-flex items-center gap-1.5 mb-8"
        >
          <div className="p-1.5 rounded-full bg-white/5 group-hover:bg-white/10 transition-colors">
            <ArrowLeft size={16} />
          </div>
          Back to galleries
        </Link>

        <div className="flex flex-col md:flex-row gap-8 items-start md:items-end mb-12">
          {/* Main Cover Image */}
          <div className="relative group shrink-0">
            <div className="absolute -inset-1 bg-gradient-to-r from-blue-500 to-purple-600 rounded-2xl blur opacity-25 group-hover:opacity-50 transition duration-1000 group-hover:duration-200" />
            <div className="relative w-48 h-64 md:w-56 md:h-80 rounded-xl overflow-hidden bg-zinc-800 shadow-2xl ring-1 ring-white/10">
              {gallery.local_thumbnail_path ? (
                <img
                  src={`/data/thumbnails/${gallery.local_thumbnail_path.split('/').pop()}`}
                  alt={gallery.title}
                  className="w-full h-full object-cover"
                />
              ) : (
                <div className="w-full h-full flex items-center justify-center text-zinc-600">
                  <FileText size={48} />
                </div>
              )}
            </div>
          </div>

          <div className="flex-1 min-w-0 animate-fade-in-up">
            <div className="flex flex-wrap items-center gap-2 mb-3">
              {gallery.provider && (
                <Badge variant="info" className="px-2.5 py-1 text-[10px] uppercase tracking-wider font-bold">
                  {gallery.provider}
                </Badge>
              )}
              {imageList && (
                <Badge variant="default" className="bg-white/5 text-zinc-300 border border-white/10 px-2.5 py-1">
                  {imageList.total_items} Images
                </Badge>
              )}
            </div>

            {isEditingTitle ? (
              <div className="flex items-center gap-2 mb-4 max-w-xl">
                <Input
                  value={editedTitle}
                  onChange={(e) => setEditedTitle(e.target.value)}
                  className="text-xl md:text-2xl font-bold bg-white/5 border-white/10 h-12"
                  autoFocus
                  onKeyDown={(e) => e.key === 'Enter' && saveTitle()}
                />
                <Button size="md" onClick={saveTitle} disabled={updateTitleMut.isPending} className="h-12 w-12 shrink-0">
                  <Save size={20} />
                </Button>
                <Button size="md" variant="ghost" onClick={cancelEditTitle} className="h-12 w-12 shrink-0">
                  <X size={20} />
                </Button>
              </div>
            ) : (
              <h1 className="text-3xl md:text-5xl font-bold text-white mb-4 tracking-tight drop-shadow-lg break-words">
                {gallery.title || `Gallery #${gallery.id}`}
              </h1>
            )}

            <div className="flex flex-wrap items-center gap-3">
              {!isEditingTitle && (
                <button
                  onClick={startEditTitle}
                  className="p-2 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-400 hover:text-white transition-all"
                  title="Edit title"
                >
                  <Edit2 size={18} />
                </button>
              )}
              <div className="h-4 w-[1px] bg-white/10 mx-1" />
              <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/5 border border-white/5 text-xs text-zinc-400">
                <Calendar size={14} />
                <span>Added {formatDate(gallery.created_at)}</span>
              </div>
              {gallery.release_date && (
                <div className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-white/5 border border-white/5 text-xs text-zinc-400">
                  <Star size={14} className="text-amber-400" />
                  <span>Released {gallery.release_date}</span>
                </div>
              )}
            </div>
          </div>

          {/* Floating Action Bar */}
          <div className="flex items-center gap-2 glass p-1.5 rounded-xl shadow-xl ring-1 ring-white/10">
            <Button
              variant="secondary"
              size="sm"
              onClick={openMetadataSearch}
              className="bg-white/5 border-transparent hover:bg-white/10"
            >
              <Search size={16} /> Metadata
            </Button>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as typeof sortBy)}
              className="bg-white/5 border border-transparent hover:border-white/10 rounded-lg px-3 py-1.5 text-xs text-zinc-200 focus:outline-none transition-all cursor-pointer"
            >
              <option value="newest">Newest first</option>
              <option value="oldest">Oldest first</option>
              <option value="largest">Largest first</option>
              <option value="smallest">Smallest first</option>
            </select>
            <div className="w-[1px] h-6 bg-white/10 mx-1" />
            <Button
              variant="danger"
              size="sm"
              onClick={() => setConfirmDeleteGallery(true)}
              className="bg-red-500/10 text-red-400 border-transparent hover:bg-red-500/20"
            >
              <Trash2 size={16} />
            </Button>
          </div>
        </div>
      </div>

    {/* Info Cards Grid - Moved outside hero container for cleaner separation */}
    <div className="grid grid-cols-1 md:grid-cols-3 gap-6 px-6 mb-12">
        {/* Source Card */}
        <div className="glass-card p-5 rounded-2xl flex flex-col gap-3">
          <div className="flex items-center gap-2 text-zinc-400 text-xs font-bold uppercase tracking-wider">
            <ExternalLink size={14} /> Source Information
          </div>
          {gallery.url ? (
            <div className="space-y-2">
              <a
                href={gallery.url}
                target="_blank"
                rel="noopener noreferrer"
                className="group flex items-center gap-2 p-2.5 rounded-xl bg-white/5 border border-white/5 hover:bg-blue-500/10 hover:border-blue-500/30 transition-all"
              >
                <div className="flex-1 min-w-0">
                  <p className="text-xs text-zinc-500 mb-0.5">Gallery URL</p>
                  <p className="text-sm text-blue-400 truncate">{gallery.url}</p>
                </div>
                <ExternalLink size={14} className="text-blue-500 opacity-0 group-hover:opacity-100 transition-opacity" />
              </a>
              {gallery.source_url && gallery.source_url !== gallery.url && (
                <a
                  href={gallery.source_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="group flex items-center gap-2 p-2.5 rounded-xl bg-white/5 border border-white/5 hover:bg-blue-500/10 hover:border-blue-500/30 transition-all"
                >
                  <div className="flex-1 min-w-0">
                    <p className="text-xs text-zinc-500 mb-0.5">Source Website</p>
                    <p className="text-sm text-blue-400 truncate font-medium">View Original</p>
                  </div>
                  <ExternalLink size={14} className="text-blue-500 opacity-0 group-hover:opacity-100 transition-opacity" />
                </a>
              )}
            </div>
          ) : (
            <p className="text-sm text-zinc-500 italic py-4">No source URL available.</p>
          )}
        </div>

        {/* Metadata Card */}
        <div className="md:col-span-2 glass-card p-5 rounded-2xl flex flex-col gap-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2 text-zinc-400 text-xs font-bold uppercase tracking-wider">
              <FileText size={14} /> Description & Details
            </div>
            {gallery.rating != null && gallery.rating > 0 && (
              <div className="flex items-center gap-1 bg-amber-400/10 text-amber-400 px-2 py-0.5 rounded text-xs font-bold ring-1 ring-amber-400/20">
                <Star size={12} className="fill-amber-400" />
                {gallery.rating.toFixed(1)}
              </div>
            )}
          </div>
          {gallery.description ? (
            <div className="relative">
              <p className="text-sm text-zinc-300 leading-relaxed max-h-[120px] overflow-y-auto pr-2 custom-scrollbar">
                {gallery.description}
              </p>
            </div>
          ) : (
            <div className="flex-1 flex items-center justify-center border border-dashed border-white/5 rounded-xl py-6">
              <p className="text-sm text-zinc-500">No description provided for this gallery.</p>
            </div>
          )}
        </div>

        {/* People Card */}
        <div className="md:col-span-3 glass-card p-5 rounded-2xl">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2 text-zinc-400 text-xs font-bold uppercase tracking-wider">
              <Heart size={14} /> Linked People
            </div>
            <span className="text-[10px] bg-white/5 text-zinc-500 px-2 py-0.5 rounded-full border border-white/5">
              {linkedPeople?.length ?? 0} total
            </span>
          </div>
          {!linkedPeople || linkedPeople.length === 0 ? (
            <div className="py-4 border border-dashed border-white/5 rounded-xl text-center">
              <p className="text-sm text-zinc-500 italic">No performers linked to this gallery.</p>
            </div>
          ) : (
            <div className="flex flex-wrap gap-3">
              {linkedPeople.map((person) => {
                const photo = parsePhotos(person.photos)[0];
                return (
                  <Link
                    key={person.id}
                    to={`/people/${person.id}`}
                    className="group relative flex items-center gap-3 rounded-full bg-white/5 border border-white/5 pr-4 pl-1 py-1 hover:bg-white/10 hover:border-white/20 transition-all duration-300"
                  >
                    <div className="relative h-8 w-8 rounded-full overflow-hidden ring-1 ring-white/10 group-hover:ring-blue-500/50 transition-all">
                      {photo ? (
                        <img src={photo} alt={person.name} className="h-full w-full object-cover group-hover:scale-110 transition-transform duration-500" />
                      ) : (
                        <div className="h-full w-full bg-zinc-800 flex items-center justify-center text-[10px] text-zinc-500 font-bold uppercase">
                          {person.name.charAt(0)}
                        </div>
                      )}
                    </div>
                    <span className="text-sm font-medium text-zinc-300 group-hover:text-white transition-colors">
                      {person.name}
                    </span>
                    <button
                      onClick={(e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        unlinkPersonMut.mutate(person.id);
                      }}
                      disabled={unlinkPersonMut.isPending}
                      className="ml-1 p-1 rounded-full text-zinc-500 hover:text-red-400 hover:bg-red-400/10 opacity-0 group-hover:opacity-100 transition-all"
                      title={`Unlink ${person.name}`}
                    >
                      <X size={12} />
                    </button>
                  </Link>
                );
              })}
            </div>
          )}
        </div>
      </div>

      <section className="p-1">
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
      </div>
    </>
  );
}