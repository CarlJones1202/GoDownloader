import { Link } from 'react-router-dom';
import { formatDate } from '@/lib/utils';
import { Trash2, Eye } from 'lucide-react';

interface CoverItem {
  id: number;
  title: string | null | undefined;
  thumbnailPath: string | null | undefined;
  provider: string | null | undefined;
  createdAt: string;
  url?: string;
}

interface CoverGridProps {
  items: CoverItem[];
  onDelete?: (id: number) => void;
}

export function CoverGrid({ items, onDelete }: CoverGridProps) {
  if (items.length === 0) {
    return null;
  }

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
      {items.map((item) => (
        <CoverGridCard key={item.id} item={item} onDelete={onDelete} />
      ))}
    </div>
  );
}

interface CoverGridCardProps {
  item: CoverItem;
  onDelete?: (id: number) => void;
}

function CoverGridCard({ item, onDelete }: CoverGridCardProps) {
  return (
    <Link
      to={`/galleries/${item.id}`}
      className="group relative block aspect-[3/2] rounded-lg overflow-hidden bg-zinc-900 hover:scale-[1.02] transition-transform duration-200"
    >
      {item.thumbnailPath ? (
        <img
          src={`/data/thumbnails/${item.thumbnailPath}`}
          alt={item.title || `Gallery ${item.id}`}
          className="w-full h-full object-cover"
        />
      ) : (
        <div className="w-full h-full bg-zinc-800 flex items-center justify-center">
          <span className="text-zinc-600 text-sm">No preview</span>
        </div>
      )}

      {/* Gradient overlay at bottom */}
      <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 via-black/60 to-transparent pt-16 pb-3 px-3">
        {/* Title */}
        <p className="text-sm font-medium text-white truncate mb-1">
          {item.title || `Gallery #${item.id}`}
        </p>

        {/* Metadata row */}
        <div className="flex items-center justify-between">
          {item.provider && (
            <span className="text-[10px] uppercase tracking-wider text-zinc-300 bg-black/50 px-1.5 py-0.5 rounded">
              {item.provider}
            </span>
          )}
          <span className="text-[10px] text-zinc-400">
            {formatDate(item.createdAt)}
          </span>
        </div>
      </div>

      {/* Hover overlay with actions */}
      <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity duration-200 flex items-center justify-center gap-3">
        <div
          className="p-3 bg-white/10 hover:bg-white/20 rounded-full transition-colors"
        >
          <Eye size={20} className="text-white" />
        </div>
        {onDelete && (
          <button
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              onDelete(item.id);
            }}
            className="p-3 bg-white/10 hover:bg-red-500/80 rounded-full transition-colors"
          >
            <Trash2 size={20} className="text-white" />
          </button>
        )}
      </div>
    </Link>
  );
}
