import { useCallback, useEffect, useState } from 'react';
import { ChevronLeft, ChevronRight, X, Info, Heart, Link as LinkIcon, Play, Pause, Maximize, Minimize, Settings2, Timer } from 'lucide-react';
import type { Image } from '@/types';
import { formatDate, parseColors, cn } from '@/lib/utils';
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
  const [isPlaying, setIsPlaying] = useState(false);
  const [speed, setSpeed] = useState(3000);
  const [isFullScreen, setIsFullScreen] = useState(false);

  const toggleFullscreen = useCallback(async () => {
    try {
      if (!document.fullscreenElement) {
        await document.documentElement.requestFullscreen();
      } else {
        await document.exitFullscreen();
      }
    } catch (err) {
      console.error('Fullscreen error:', err);
    }
  }, []);

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
    let timer: number | undefined;
    if (isPlaying) {
      timer = window.setInterval(() => {
        onIndexChange((index + 1) % images.length);
      }, speed);
    }
    return () => {
      if (timer) clearInterval(timer);
    };
  }, [isPlaying, index, images.length, onIndexChange, speed]);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
      else if (e.key === 'ArrowLeft') goPrev();
      else if (e.key === 'ArrowRight') goNext();
      else if (e.key === 'i' || e.key === 'I') setShowInfo((s) => !s);
      else if (e.key === 'f' || e.key === 'F') handleToggleFavorite();
      else if (e.key === ' ') {
        e.preventDefault();
        setIsPlaying((p) => !p);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose, goPrev, goNext, handleToggleFavorite, isPlaying]);

  useEffect(() => {
    const handleFsChange = () => {
      setIsFullScreen(!!document.fullscreenElement);
    };
    document.addEventListener('fullscreenchange', handleFsChange);
    return () => document.removeEventListener('fullscreenchange', handleFsChange);
  }, []);

  useEffect(() => {
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = '';
      if (document.fullscreenElement) {
        document.exitFullscreen().catch(() => {});
      }
    };
  }, []);

  const colors = currentImage?.dominant_colors ? parseColors(currentImage.dominant_colors) : [];

  return (
    <div className={`fixed inset-0 z-50 bg-black/98 flex flex-col transition-all ${isFullScreen ? 'cursor-none' : ''}`}>
      {/* Top bar */}
      {!isFullScreen && (
        <div className="flex items-center justify-between px-6 py-4 bg-gradient-to-b from-black/60 to-transparent">
          <div className="flex items-center gap-6">
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
              title="Toggle info (I)"
            >
              <Info size={20} />
            </button>

            <div className="h-4 w-px bg-white/10" />

            {/* Slideshow Controls */}
            <div className="flex items-center gap-2">
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  setIsPlaying(!isPlaying);
                }}
                className={`p-2 rounded-lg transition-all ${
                  isPlaying 
                    ? 'bg-blue-500 text-white shadow-lg shadow-blue-500/20' 
                    : 'text-white/60 hover:text-white hover:bg-white/10'
                }`}
                title={isPlaying ? "Pause (Space)" : "Play (Space)"}
              >
                {isPlaying ? <Pause size={20} /> : <Play size={20} />}
              </button>

              <div className="flex items-center gap-1 bg-white/10 hover:bg-white/20 rounded-lg px-2 py-1 transition-colors">
                <Timer size={14} className="text-white/40" />
                <select
                  value={speed}
                  onChange={(e) => setSpeed(Number(e.target.value))}
                  onClick={(e) => e.stopPropagation()}
                  className="bg-transparent text-white/90 text-xs focus:outline-none cursor-pointer font-medium [color-scheme:dark]"
                >
                  <option value={1000} className="bg-zinc-900 text-white">1s</option>
                  <option value={2000} className="bg-zinc-900 text-white">2s</option>
                  <option value={3000} className="bg-zinc-900 text-white">3s</option>
                  <option value={5000} className="bg-zinc-900 text-white">5s</option>
                  <option value={10000} className="bg-zinc-900 text-white">10s</option>
                </select>
              </div>
            </div>

            <button
              onClick={(e) => {
                e.stopPropagation();
                toggleFullscreen();
              }}
              className="p-2 text-white/60 hover:text-white hover:bg-white/10 rounded-lg transition-all"
              title="Full screen (F11)"
            >
              <Maximize size={20} />
            </button>
          </div>

          <div className="flex items-center gap-4">
            <div className="text-white/60 text-sm font-medium tracking-wider">
              {index + 1} <span className="text-white/20 mx-1">/</span> {images.length}
            </div>
            <button
              onClick={onClose}
              className="p-2 text-white/40 hover:text-white transition-colors"
              aria-label="Close"
            >
              <X size={28} />
            </button>
          </div>
        </div>
      )}

      {/* Image area */}
      <div 
        className="flex-1 flex items-center justify-center relative group" 
        onClick={(e) => {
          if (isFullScreen) {
            toggleFullscreen();
          } else {
            onClose();
          }
        }}
      >
        {isFullScreen && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              toggleFullscreen();
            }}
            className="absolute top-6 right-6 p-3 text-white/20 hover:text-white opacity-0 group-hover:opacity-100 transition-all z-10"
            title="Exit Full Screen (Esc)"
          >
            <Minimize size={32} />
          </button>
        )}

        {!isFullScreen && hasPrev && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              goPrev();
            }}
            className="absolute left-6 top-1/2 -translate-y-1/2 p-4 text-white/20 hover:text-white hover:bg-white/5 rounded-full transition-all z-10"
            aria-label="Previous"
          >
            <ChevronLeft size={48} />
          </button>
        )}

        <img
          src={images[index].src}
          alt={images[index].alt ?? ''}
          className={cn(
            "transition-all duration-500 ease-in-out select-none object-contain pointer-events-auto",
            isFullScreen 
              ? "w-full h-full" 
              : "max-w-[90vw] max-h-[75vh] md:max-h-[82vh] shadow-2xl rounded-sm"
          )}
          draggable={false}
          onClick={(e) => e.stopPropagation()}
        />

        {!isFullScreen && hasNext && (
          <button
            onClick={(e) => {
              e.stopPropagation();
              goNext();
            }}
            className="absolute right-6 top-1/2 -translate-y-1/2 p-4 text-white/20 hover:text-white hover:bg-white/5 rounded-full transition-all z-10"
            aria-label="Next"
          >
            <ChevronRight size={48} />
          </button>
        )}
      </div>

      {/* Bottom info bar */}
      {!isFullScreen && (
        <div className="px-8 py-6 bg-gradient-to-t from-black/80 to-transparent">
          {currentImage ? (
            <div className="flex items-center justify-between text-white/90 text-sm">
              <div className="flex items-center gap-8">
                {currentImage.width && currentImage.height && (
                  <div className="flex flex-col">
                    <span className="text-white/30 text-[10px] uppercase tracking-widest mb-0.5">Resolution</span>
                    <span className="font-mono text-white/70">{currentImage.width} × {currentImage.height}</span>
                  </div>
                )}
                <div className="h-8 w-px bg-white/10" />
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    handleToggleFavorite();
                  }}
                  className="flex flex-col items-center gap-1 transition-colors group"
                  title="Toggle favorite (F)"
                >
                  <Heart 
                    size={20} 
                    className={currentImage.is_favorite 
                      ? 'text-red-500 fill-red-500 scale-110' 
                      : 'text-white/30 group-hover:text-white/60 transition-transform group-hover:scale-110'} 
                  />
                  <span className="text-[10px] uppercase tracking-widest text-white/30 group-hover:text-white/60">
                    {currentImage.is_favorite ? 'Favorited' : 'Favorite'}
                  </span>
                </button>
                {colors.length > 0 && (
                  <div className="flex flex-col gap-1.5">
                    <span className="text-white/30 text-[10px] uppercase tracking-widest">Palette</span>
                    <div className="flex gap-1">
                      {colors.map((c, i) => (
                        <div key={i} className="w-4 h-4 rounded-full border border-white/10" style={{ backgroundColor: c }} />
                      ))}
                    </div>
                  </div>
                )}
              </div>
              <div className="flex items-center gap-8">
                {currentImage.gallery_id && (
                  <Link 
                    to={`/galleries/${currentImage.gallery_id}`} 
                    className="flex flex-col items-end gap-1 text-blue-400 hover:text-blue-300 transition-colors"
                    onClick={(e) => e.stopPropagation()}
                  >
                    <span className="text-white/30 text-[10px] uppercase tracking-widest">Source Gallery</span>
                    <div className="flex items-center gap-1.5">
                      <LinkIcon size={12} />
                      <span className="font-medium">View Collection</span>
                    </div>
                  </Link>
                )}
                <div className="h-8 w-px bg-white/10" />
                <div className="flex flex-col items-end">
                  <span className="text-white/30 text-[10px] uppercase tracking-widest mb-0.5">Added On</span>
                  <span className="text-white/60">{formatDate(currentImage.created_at)}</span>
                </div>
              </div>
            </div>
          ) : (
            <div className="text-white/40 text-sm flex items-center justify-center font-mono">
              {index + 1} / {images.length}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
