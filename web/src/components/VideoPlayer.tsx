import { useEffect, useRef, useCallback, useState } from 'react';
import { X } from 'lucide-react';
import type { Image } from '@/types';
import { videoUrl, trickplayVttUrl, formatDuration } from '@/lib/utils';

interface VideoPlayerProps {
  video: Image;
  onClose: () => void;
}

/**
 * Full-screen video player overlay with:
 * - HTML5 <video> playback
 * - Trickplay VTT track for timeline scrub thumbnails
 * - Keyboard controls (Escape to close, Space to play/pause, arrow keys to seek)
 */
export function VideoPlayer({ video, onClose }: VideoPlayerProps) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(video.duration_seconds ?? 0);

  // Prevent body scroll while player is open.
  useEffect(() => {
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = ''; };
  }, []);

  const togglePlay = useCallback(() => {
    const el = videoRef.current;
    if (!el) return;
    if (el.paused) {
      el.play();
    } else {
      el.pause();
    }
  }, []);

  const seek = useCallback((delta: number) => {
    const el = videoRef.current;
    if (!el) return;
    el.currentTime = Math.max(0, Math.min(el.duration, el.currentTime + delta));
  }, []);

  // Keyboard controls.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      switch (e.key) {
        case 'Escape':
          onClose();
          break;
        case ' ':
          e.preventDefault();
          togglePlay();
          break;
        case 'ArrowLeft':
          seek(-5);
          break;
        case 'ArrowRight':
          seek(5);
          break;
        case 'f':
          videoRef.current?.requestFullscreen?.();
          break;
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [onClose, togglePlay, seek]);

  const handleBackgroundClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) onClose();
  };

  const src = videoUrl(video.filename);
  const vttSrc = trickplayVttUrl(video.filename);

  return (
    <div
      className="fixed inset-0 z-50 bg-black/95 flex flex-col items-center justify-center"
      onClick={handleBackgroundClick}
    >
      {/* Close button */}
      <button
        onClick={onClose}
        className="absolute top-4 right-4 z-10 p-2 text-white/70 hover:text-white transition-colors"
        aria-label="Close"
      >
        <X size={24} />
      </button>

      {/* Title + metadata bar */}
      <div className="absolute top-4 left-4 z-10 text-white/70 text-sm max-w-[70%] truncate">
        <span className="text-white/50">{video.filename}</span>
        {video.width && video.height && (
          <span className="ml-3 text-white/40">{video.width}x{video.height}</span>
        )}
        {duration > 0 && (
          <span className="ml-3 text-white/40">{formatDuration(duration)}</span>
        )}
      </div>

      {/* Video element */}
      <video
        ref={videoRef}
        src={src}
        controls
        autoPlay
        className="max-w-[95vw] max-h-[90vh] outline-none"
        onPlay={() => setIsPlaying(true)}
        onPause={() => setIsPlaying(false)}
        onTimeUpdate={() => {
          if (videoRef.current) setCurrentTime(videoRef.current.currentTime);
        }}
        onLoadedMetadata={() => {
          if (videoRef.current) setDuration(Math.floor(videoRef.current.duration));
        }}
        onClick={(e) => e.stopPropagation()}
      >
        {/* Trickplay VTT for browser-native timeline thumbnails (Chromium supports metadata tracks) */}
        <track kind="metadata" src={vttSrc} default />
      </video>

      {/* Bottom status bar */}
      <div className="absolute bottom-4 left-0 right-0 flex items-center justify-center gap-4 text-white/50 text-xs">
        <span>{isPlaying ? 'Playing' : 'Paused'}</span>
        <span>{formatDuration(Math.floor(currentTime))} / {formatDuration(duration)}</span>
        <span className="text-white/30">Space: play/pause | Arrows: seek | F: fullscreen | Esc: close</span>
      </div>
    </div>
  );
}
