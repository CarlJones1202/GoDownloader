import { useCallback, useEffect, useState } from 'react';
import { ChevronLeft, ChevronRight, X, Info, Heart, Link as LinkIcon } from 'lucide-react';
import type { Image } from '@/types';
import { formatDate, parseColors } from '@/lib/utils';
import { Link } from 'react-router-dom';

interface LightboxProps {
  images: { src: string; alt?: string }[];
  index: number;
  onClose: () => void;
  onIndexChange: (index: number) => void;
  imageData?: Image[];
  onToggleFavorite?: (id: number) => void;
}

export function Lightbox({ images, index, onClose, onIndexChange, imageData, onToggleFavorite }: LightboxProps) {
  const [showInfo, setShowInfo] = useState(false);
  const hasPrev = index > 0;
  const hasNext = index < images.length - 1;

  const currentImage = imageData?.[index];

  const goPrev = useCallback(() => {
    if (hasPrev) onIndexChange(index - 1);
  }, [hasPrev, index, onIndexChange]);

  const goNext = useCallback(() => {
    if (hasNext) onIndexChange(index + 1);
  }, [hasNext, index, onIndexChange]);

  const handleToggleFavorite = useCallback(() => {
    if (currentImage && onToggleFavorite) {
      onToggleFavorite(currentImage.id);
    }
  }, [currentImage, onToggleFavorite]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
      else if (e.key === 'ArrowLeft') goPrev();
      else if (e.key === 'ArrowRight') goNext();
      else if (e.key === 'i' || e.key === 'I') setShowInfo((s) => !s);
      else if (e.key === 'f' || e.key === 'F') handleToggleFavorite();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose, goPrev, goNext, handleToggleFavorite]);

  useEffect(() => {
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = '';
    };
  }, []);

  const colors = currentImage?.dominant_colors ? parseColors(currentImage.dominant_colors) : [];

  return (
    <div className="fixed inset-0 z-50 bg-black/95 flex flex-col">
      {/* Top bar */}
      <div className="flex items-center justify-between px-6 py-4">
        <div className="flex items-center gap-4">
          <button
            onClick={(e) => {
              e.stopPropagation();
              setShowInfo(!showInfo);
            }}
            className={`p-2 rounded-lg transition-all ${
              showInfo 
                ? 'bg-white text-black' 
                : 'text-white/60 hover:text-white hover:bg-white/10'
            }`}
            aria-label="Toggle image info"
          >
            <Info size={20} />
          </button>
          <div className="text-white/60 text-sm font-mono">
            {index + 1} / {images.length}
          </div>
        </div>
        <button
          onClick={onClose}
          className="p-2 text-white/40 hover:text-white transition-colors"
          aria-label="Close"
        >
          <X size={28} />
        </button>
      </div>

      {/* Image area - clicking background closes */}
      <div className="flex-1 flex items-center justify-center relative" onClick={onClose}>
        {hasPrev && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              goPrev();
            }}
            className="absolute left-4 top-1/2 -translate-y-1/2 p-3 text-white/40 hover:text-white transition-colors"
            aria-label="Previous"
          >
            <ChevronLeft size={36} />
          </button>
        )}

        <img
          src={images[index].src}
          alt={images[index].alt ?? ''}
          className="max-w-[90vw] max-h-[85vh] object-contain select-none"
          draggable={false}
          onClick={(e) => e.stopPropagation()}
        />

        {hasNext && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              goNext();
            }}
            className="absolute right-4 top-1/2 -translate-y-1/2 p-3 text-white/40 hover:text-white transition-colors"
            aria-label="Next"
          >
            <ChevronRight size={36} />
          </button>
        )}
      </div>

      {/* Bottom info bar */}
      <div className="px-6 py-4 bg-black/60">
        {currentImage ? (
          <div className="flex items-center justify-between text-white/80 text-sm">
            <div className="flex items-center gap-6">
              {currentImage.width && currentImage.height && (
                <span className="text-white/50">{currentImage.width} × {currentImage.height}</span>
              )}
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  handleToggleFavorite();
                }}
                className="flex items-center gap-1.5 transition-colors hover:text-white"
                title="Toggle favorite (F)"
              >
                <Heart 
                  size={16} 
                  className={currentImage.is_favorite ? 'text-red-500 fill-red-500' : 'text-white/50'} 
                />
                <span className="text-xs">{currentImage.is_favorite ? 'Favorited' : 'Favorite'}</span>
              </button>
              {colors.length > 0 && (
                <div className="flex gap-1.5">
                  {colors.map((c, i) => (
                    <div key={i} className="w-5 h-5 rounded-sm" style={{ backgroundColor: c }} />
                  ))}
                </div>
              )}
            </div>
            <div className="flex items-center gap-6">
              {currentImage.gallery_id && (
                <Link 
                  to={`/galleries/${currentImage.gallery_id}`} 
                  className="flex items-center gap-2 text-blue-400 hover:text-blue-300 transition-colors"
                  onClick={(e) => e.stopPropagation()}
                >
                  <LinkIcon size={14} />
                  Gallery
                </Link>
              )}
              <span className="text-white/40">{formatDate(currentImage.created_at)}</span>
            </div>
          </div>
        ) : (
          <div className="text-white/40 text-sm">
            {index + 1} / {images.length}
          </div>
        )}
      </div>
    </div>
  );
}
