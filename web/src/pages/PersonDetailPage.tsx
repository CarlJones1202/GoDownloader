import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { people } from '@/lib/api';
import { formatDate } from '@/lib/utils';
import type { PersonInfo } from '@/types';
import {
  PageHeader,
  Card,
  Badge,
  Button,
  Spinner,
  EmptyState,
  Input,
  Select,
  Pagination,
} from '@/components/UI';
import {
  ArrowLeft,
  Sparkles,
  Search,
  Link2,
  Unlink,
  Plus,
  Save,
  ExternalLink,
  X,
  Check,
  User,
} from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

// ---------------------------------------------------------------------------
// Identify Modal Component
// ---------------------------------------------------------------------------

interface IdentifyModalProps {
  personId: number;
  personName: string;
  open: boolean;
  onClose: () => void;
  onIdentified: () => void;
}

function IdentifyModal({ personId, personName, open, onClose, onIdentified }: IdentifyModalProps) {
  const [provider, setProvider] = useState('stashdb');
  const [query, setQuery] = useState(personName);
  const [searchTriggered, setSearchTriggered] = useState(false);
  const [selectedResult, setSelectedResult] = useState<PersonInfo | null>(null);
  const queryClient = useQueryClient();

  // Fetch available providers
  const { data: providers } = useQuery({
    queryKey: ['people-providers'],
    queryFn: () => people.providers(),
    enabled: open,
  });

  // Search query — only runs when searchTriggered is true
  const {
    data: searchResponse,
    isLoading: searching,
    error: searchError,
  } = useQuery({
    queryKey: ['people-search', personId, provider, query],
    queryFn: () => people.search(personId, provider, query),
    enabled: open && searchTriggered && !!query && !!provider,
    retry: false,
  });

  // Identify mutation
  const identifyMut = useMutation({
    mutationFn: (result: PersonInfo) =>
      people.identify(personId, {
        provider,
        external_id: result.external_id!,
        apply: true,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      queryClient.invalidateQueries({ queryKey: ['person-identifiers', personId] });
      onIdentified();
      handleClose();
    },
  });

  const handleSearch = () => {
    setSearchTriggered(false);
    setSelectedResult(null);
    // Use setTimeout to ensure state reset happens before re-trigger
    setTimeout(() => setSearchTriggered(true), 0);
  };

  const handleClose = () => {
    setSearchTriggered(false);
    setSelectedResult(null);
    setQuery(personName);
    setProvider('stashdb');
    onClose();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleSearch();
    if (e.key === 'Escape') handleClose();
  };

  if (!open) return null;

  const results = searchResponse?.results ?? [];

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/70 pt-16 overflow-y-auto">
      <div className="bg-zinc-900 border border-zinc-700 rounded-lg w-full max-w-3xl mx-4 mb-8">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-800">
          <h2 className="text-lg font-semibold text-white">Identify Person</h2>
          <button onClick={handleClose} className="text-zinc-400 hover:text-white">
            <X size={18} />
          </button>
        </div>

        {/* Search controls */}
        <div className="px-5 py-4 border-b border-zinc-800">
          <div className="flex items-end gap-3">
            <Select
              label="Provider"
              value={provider}
              onChange={(e) => {
                setProvider(e.target.value);
                setSearchTriggered(false);
              }}
              options={(providers ?? ['stashdb']).map((p) => ({ value: p, label: p }))}
              className="w-40"
            />
            <div className="flex-1">
              <Input
                label="Search query"
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setSearchTriggered(false);
                }}
                onKeyDown={handleKeyDown}
                placeholder="Enter name to search..."
              />
            </div>
            <Button
              size="sm"
              onClick={handleSearch}
              disabled={!query || searching}
            >
              <Search size={14} /> {searching ? 'Searching...' : 'Search'}
            </Button>
          </div>
        </div>

        {/* Results */}
        <div className="px-5 py-4 max-h-[60vh] overflow-y-auto">
          {searching && <Spinner />}

          {searchError && (
            <p className="text-sm text-red-400">
              Search failed: {searchError instanceof Error ? searchError.message : 'Unknown error'}
            </p>
          )}

          {searchTriggered && !searching && results.length === 0 && !searchError && (
            <EmptyState message={`No results found for "${query}" on ${provider}.`} />
          )}

          {results.length > 0 && (
            <div className="space-y-3">
              <p className="text-xs text-zinc-500">
                {results.length} result{results.length !== 1 ? 's' : ''} from {provider}
              </p>
              {results.map((result, idx) => (
                <SearchResultCard
                  key={result.external_id ?? idx}
                  result={result}
                  selected={selectedResult?.external_id === result.external_id}
                  onSelect={() => setSelectedResult(result)}
                />
              ))}
            </div>
          )}
        </div>

        {/* Footer with apply action */}
        {selectedResult && (
          <div className="px-5 py-4 border-t border-zinc-800 flex items-center justify-between">
            <div className="text-sm text-zinc-300">
              Selected: <span className="font-medium text-white">{selectedResult.name}</span>
              {selectedResult.external_id && (
                <span className="text-zinc-500 ml-2 font-mono text-xs">
                  {selectedResult.external_id}
                </span>
              )}
            </div>
            <div className="flex gap-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setSelectedResult(null)}
              >
                Deselect
              </Button>
              <Button
                size="sm"
                onClick={() => identifyMut.mutate(selectedResult)}
                disabled={!selectedResult.external_id || identifyMut.isPending}
              >
                <Check size={14} /> {identifyMut.isPending ? 'Applying...' : 'Apply & Link'}
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Search Result Card
// ---------------------------------------------------------------------------

interface SearchResultCardProps {
  result: PersonInfo;
  selected: boolean;
  onSelect: () => void;
}

function SearchResultCard({ result, selected, onSelect }: SearchResultCardProps) {
  const details: string[] = [];
  if (result.nationality) details.push(result.nationality);
  if (result.birth_date) details.push(`Born: ${result.birth_date}`);
  if (result.ethnicity) details.push(result.ethnicity);
  if (result.height) details.push(result.height);
  if (result.measurements) details.push(result.measurements);
  if (result.hair_color) details.push(`Hair: ${result.hair_color}`);
  if (result.eye_color) details.push(`Eyes: ${result.eye_color}`);

  return (
    <div
      onClick={onSelect}
      className={`flex gap-4 p-3 rounded-lg border cursor-pointer transition-colors ${
        selected
          ? 'border-blue-500 bg-blue-950/30'
          : 'border-zinc-800 bg-zinc-800/50 hover:border-zinc-600'
      }`}
    >
      {/* Thumbnail */}
      <div className="flex-shrink-0 w-16 h-20 rounded overflow-hidden bg-zinc-800">
        {result.image_url ? (
          <img
            src={result.image_url}
            alt={result.name}
            className="w-full h-full object-cover"
            onError={(e) => {
              (e.target as HTMLImageElement).style.display = 'none';
              (e.target as HTMLImageElement).parentElement!.classList.add('flex', 'items-center', 'justify-center');
              const icon = document.createElement('div');
              icon.innerHTML = '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-zinc-600"><path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"></path><circle cx="12" cy="7" r="4"></circle></svg>';
              (e.target as HTMLImageElement).parentElement!.appendChild(icon);
            }}
          />
        ) : (
          <div className="w-full h-full flex items-center justify-center">
            <User size={24} className="text-zinc-600" />
          </div>
        )}
      </div>

      {/* Info */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-white">{result.name || 'Unknown'}</span>
          {selected && <Badge variant="info">Selected</Badge>}
        </div>

        {result.aliases && result.aliases.length > 0 && (
          <p className="text-xs text-zinc-500 mt-0.5 truncate">
            aka {result.aliases.slice(0, 3).join(', ')}
            {result.aliases.length > 3 && ` +${result.aliases.length - 3} more`}
          </p>
        )}

        {details.length > 0 && (
          <div className="flex flex-wrap gap-x-3 gap-y-1 mt-1.5">
            {details.map((d, i) => (
              <span key={i} className="text-xs text-zinc-400">{d}</span>
            ))}
          </div>
        )}

        {result.external_id && (
          <p className="text-xs text-zinc-600 font-mono mt-1">{result.external_id}</p>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Person Detail Page
// ---------------------------------------------------------------------------

export function PersonDetailPage() {
  const { id } = useParams<{ id: string }>();
  const personId = Number(id);
  const queryClient = useQueryClient();

  const [editing, setEditing] = useState(false);
  const [editForm, setEditForm] = useState({ name: '', aliases: '', nationality: '' });
  const { page: galleryPage, offset: galleryOffset, limit: galleryLimit, prevPage: galleryPrev, nextPage: galleryNext } = usePagination({ limit: 20, paramName: 'gpage' });
  const [linkGalleryId, setLinkGalleryId] = useState('');
  const [newIdentifier, setNewIdentifier] = useState({ provider: '', external_id: '' });
  const [identifyOpen, setIdentifyOpen] = useState(false);

  // --- Queries ---

  const { data: person, isLoading: loadingPerson } = useQuery({
    queryKey: ['person', personId],
    queryFn: () => people.get(personId),
  });

  const { data: galleryList, isLoading: loadingGalleries } = useQuery({
    queryKey: ['person-galleries', personId, { offset: galleryOffset, limit: galleryLimit }],
    queryFn: () => people.galleries(personId, { limit: galleryLimit, offset: galleryOffset }),
  });

  const { data: identifiers, isLoading: loadingIds } = useQuery({
    queryKey: ['person-identifiers', personId],
    queryFn: () => people.identifiers(personId),
  });

  // --- Mutations ---

  const updateMut = useMutation({
    mutationFn: () =>
      people.update(personId, {
        name: editForm.name,
        aliases: editForm.aliases || undefined,
        nationality: editForm.nationality || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      setEditing(false);
    },
  });

  const enrichMut = useMutation({
    mutationFn: () => people.enrich(personId, undefined, true),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person', personId] });
      queryClient.invalidateQueries({ queryKey: ['person-identifiers', personId] });
    },
  });

  const linkMut = useMutation({
    mutationFn: (galleryId: number) => people.linkGallery(personId, galleryId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person-galleries', personId] });
      setLinkGalleryId('');
    },
  });

  const unlinkMut = useMutation({
    mutationFn: (galleryId: number) => people.unlinkGallery(personId, galleryId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person-galleries', personId] });
    },
  });

  const upsertIdMut = useMutation({
    mutationFn: () => people.upsertIdentifier(personId, newIdentifier),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['person-identifiers', personId] });
      setNewIdentifier({ provider: '', external_id: '' });
    },
  });

  // --- Helpers ---

  const startEditing = () => {
    if (!person) return;
    setEditForm({
      name: person.name,
      aliases: person.aliases ?? '',
      nationality: person.nationality ?? '',
    });
    setEditing(true);
  };

  if (loadingPerson) return <Spinner />;
  if (!person) return <EmptyState message="Person not found." />;

  return (
    <>
      {/* Back link */}
      <div className="mb-4">
        <Link
          to="/people"
          className="text-sm text-zinc-400 hover:text-zinc-200 inline-flex items-center gap-1"
        >
          <ArrowLeft size={14} /> Back to people
        </Link>
      </div>

      {/* Header */}
      <PageHeader title={person.name}>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => (editing ? setEditing(false) : startEditing())}
        >
          {editing ? 'Cancel' : 'Edit'}
        </Button>
        <Button
          variant="secondary"
          size="sm"
          onClick={() => setIdentifyOpen(true)}
        >
          <Search size={14} /> Identify
        </Button>
        <Button
          size="sm"
          onClick={() => enrichMut.mutate()}
          disabled={enrichMut.isPending}
        >
          <Sparkles size={14} /> {enrichMut.isPending ? 'Enriching...' : 'Enrich'}
        </Button>
      </PageHeader>

      {/* Edit form */}
      {editing ? (
        <Card className="mb-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            <Input
              label="Name"
              value={editForm.name}
              onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
            />
            <Input
              label="Aliases"
              placeholder="Comma-separated"
              value={editForm.aliases}
              onChange={(e) => setEditForm({ ...editForm, aliases: e.target.value })}
            />
            <Input
              label="Nationality"
              value={editForm.nationality}
              onChange={(e) => setEditForm({ ...editForm, nationality: e.target.value })}
            />
          </div>
          <div className="flex justify-end mt-3">
            <Button
              size="sm"
              onClick={() => updateMut.mutate()}
              disabled={!editForm.name || updateMut.isPending}
            >
              <Save size={14} /> Save
            </Button>
          </div>
        </Card>
      ) : (
        <Card className="mb-6">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
            <div>
              <span className="text-zinc-500">Aliases</span>
              <p className="text-zinc-200 mt-0.5">{person.aliases || '—'}</p>
            </div>
            <div>
              <span className="text-zinc-500">Birth Date</span>
              <p className="text-zinc-200 mt-0.5">{person.birth_date || '—'}</p>
            </div>
            <div>
              <span className="text-zinc-500">Nationality</span>
              <p className="text-zinc-200 mt-0.5">{person.nationality || '—'}</p>
            </div>
            <div>
              <span className="text-zinc-500">Added</span>
              <p className="text-zinc-200 mt-0.5">{formatDate(person.created_at)}</p>
            </div>
          </div>
        </Card>
      )}

      {/* Identifiers */}
      <h2 className="text-lg font-semibold text-white mb-3">External Identifiers</h2>
      <Card className="mb-6">
        {loadingIds ? (
          <Spinner />
        ) : !identifiers || identifiers.length === 0 ? (
          <p className="text-sm text-zinc-500">No external identifiers linked.</p>
        ) : (
          <div className="space-y-2 mb-3">
            {identifiers.map((ident) => (
              <div key={ident.id} className="flex items-center gap-2">
                <Badge variant="info">{ident.provider}</Badge>
                <span className="text-sm text-zinc-300 font-mono">{ident.external_id}</span>
              </div>
            ))}
          </div>
        )}

        {/* Add identifier form */}
        <div className="flex items-end gap-2 mt-3 pt-3 border-t border-zinc-800">
          <Input
            label="Provider"
            placeholder="e.g. stashdb"
            value={newIdentifier.provider}
            onChange={(e) => setNewIdentifier({ ...newIdentifier, provider: e.target.value })}
            className="w-32"
          />
          <Input
            label="External ID"
            placeholder="e.g. abc-123"
            value={newIdentifier.external_id}
            onChange={(e) => setNewIdentifier({ ...newIdentifier, external_id: e.target.value })}
          />
          <Button
            size="sm"
            onClick={() => upsertIdMut.mutate()}
            disabled={!newIdentifier.provider || !newIdentifier.external_id || upsertIdMut.isPending}
          >
            <Plus size={14} /> Add
          </Button>
        </div>
      </Card>

      {/* Linked Galleries */}
      <h2 className="text-lg font-semibold text-white mb-3">Linked Galleries</h2>

      {/* Link gallery form */}
      <div className="flex items-end gap-2 mb-4">
        <Input
          placeholder="Gallery ID to link"
          value={linkGalleryId}
          onChange={(e) => setLinkGalleryId(e.target.value)}
          className="w-40"
        />
        <Button
          size="sm"
          onClick={() => {
            const gid = parseInt(linkGalleryId);
            if (gid) linkMut.mutate(gid);
          }}
          disabled={!linkGalleryId || linkMut.isPending}
        >
          <Link2 size={14} /> Link
        </Button>
      </div>

      {loadingGalleries ? (
        <Spinner />
      ) : !galleryList || galleryList.length === 0 ? (
        <EmptyState message="No galleries linked to this person." />
      ) : (
        <>
          <div className="space-y-2">
            {galleryList.map((g) => (
              <Card key={g.id} className="flex items-center justify-between">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <Link
                      to={`/galleries/${g.id}`}
                      className="text-sm font-medium text-white hover:text-blue-400 truncate"
                    >
                      {g.title || `Gallery #${g.id}`}
                    </Link>
                    {g.provider && <Badge>{g.provider}</Badge>}
                  </div>
                  {g.url && (
                    <a
                      href={g.url}
                      target="_blank"
                      className="text-xs text-zinc-500 hover:text-blue-400 flex items-center gap-1 mt-0.5"
                    >
                      <ExternalLink size={10} /> {g.url}
                    </a>
                  )}
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  title="Unlink gallery"
                  onClick={() => unlinkMut.mutate(g.id)}
                  disabled={unlinkMut.isPending}
                >
                  <Unlink size={14} />
                </Button>
              </Card>
            ))}
          </div>

          <Pagination
            page={galleryPage}
            hasMore={galleryList.length === galleryLimit}
            onPrev={galleryPrev}
            onNext={galleryNext}
          />
        </>
      )}

      {/* Identify Modal */}
      <IdentifyModal
        personId={personId}
        personName={person.name}
        open={identifyOpen}
        onClose={() => setIdentifyOpen(false)}
        onIdentified={() => {
          queryClient.invalidateQueries({ queryKey: ['person', personId] });
          queryClient.invalidateQueries({ queryKey: ['person-identifiers', personId] });
        }}
      />
    </>
  );
}
