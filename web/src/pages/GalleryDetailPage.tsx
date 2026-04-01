import { useState } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { galleries, images as imagesApi } from '@/lib/api';
import { formatDate, parseColors } from '@/lib/utils';
import {
  PageHeader,
  Card,
  Spinner,
  EmptyState,
  Badge,
  Button,
  ConfirmDialog,
} from '@/components/UI';
import { Heart, ArrowLeft, Trash2 } from 'lucide-react';
import { cn } from '@/lib/utils';

export function GalleryDetailPage() {
  const { id } = useParams<{ id: string }>();
  const galleryId = Number(id);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const [confirmDeleteGallery, setConfirmDeleteGallery] = useState(false);
  const [confirmDeleteImageId, setConfirmDeleteImageId] = useState<number | null>(null);

  const { data: gallery, isLoading: loadingGallery } = useQuery({
    queryKey: ['gallery', galleryId],
    queryFn: () => galleries.get(galleryId),
  });

  const { data: imageList, isLoading: loadingImages } = useQuery({
    queryKey: ['images', { gallery_id: galleryId }],
    queryFn: () => imagesApi.list({ gallery_id: galleryId, limit: 200 }),
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

  const deleteImageMut = useMutation({
    mutationFn: (imgId: number) => imagesApi.delete(imgId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images', { gallery_id: galleryId }] });
      setConfirmDeleteImageId(null);
    },
  });

  if (loadingGallery) return <Spinner />;
  if (!gallery) return <EmptyState message="Gallery not found." />;

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

      <PageHeader title={gallery.title || `Gallery #${gallery.id}`}>
        {gallery.provider && <Badge>{gallery.provider}</Badge>}
        <Button
          variant="danger"
          size="sm"
          onClick={() => setConfirmDeleteGallery(true)}
        >
          <Trash2 size={14} /> Delete Gallery
        </Button>
      </PageHeader>

      <div className="text-xs text-zinc-500 mb-6 space-y-1">
        {gallery.url && (
          <p>
            URL:{' '}
            <a href={gallery.url} target="_blank" className="text-blue-400 hover:underline">
              {gallery.url}
            </a>
          </p>
        )}
        <p>Created: {formatDate(gallery.created_at)}</p>
      </div>

      {/* Images grid */}
      {loadingImages ? (
        <Spinner />
      ) : !imageList || imageList.length === 0 ? (
        <EmptyState message="No images in this gallery." />
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
          {imageList.map((img) => {
            const colors = parseColors(img.dominant_colors);
            return (
              <Card key={img.id} className="p-0 overflow-hidden group relative">
                <div className="aspect-[4/3] bg-zinc-800 flex items-center justify-center">
                  <img
                    src={`/data/images/${img.filename}`}
                    alt={img.filename}
                    className="w-full h-full object-cover"
                    loading="lazy"
                  />
                </div>
                {/* Overlay controls */}
                <div className="absolute inset-0 bg-gradient-to-t from-black/60 to-transparent opacity-0 group-hover:opacity-100 transition-opacity flex items-end p-2">
                  <div className="flex items-center justify-between w-full">
                    <div className="flex items-center gap-1">
                      <button
                        onClick={(e) => {
                          e.preventDefault();
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
                          e.preventDefault();
                          setConfirmDeleteImageId(img.id);
                        }}
                        className="p-1"
                        title="Delete image"
                      >
                        <Trash2 size={16} className="text-white hover:text-red-400" />
                      </button>
                    </div>
                    {img.is_video && <Badge variant="info">Video</Badge>}
                  </div>
                </div>
                {/* Color palette */}
                {colors.length > 0 && (
                  <div className="flex h-1">
                    {colors.map((c, i) => (
                      <div key={i} className="flex-1" style={{ backgroundColor: c }} />
                    ))}
                  </div>
                )}
              </Card>
            );
          })}
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
