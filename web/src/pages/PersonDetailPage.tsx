import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { people } from '@/lib/api';
import { formatDate } from '@/lib/utils';
import type { PersonInfo, Person } from '@/types';
import {
  Card,
  Badge,
  Button,
  Spinner,
  EmptyState,
  Input,
  Select,
  Textarea,
  Pagination,
} from '@/components/UI';
import {
  ArrowLeft,
  Sparkles,
  Search,
  Link2,
  Plus,
  Save,
  X,
  Check,
  User,
  Edit,
  ChevronLeft,
  ChevronRight,
  Info,
  Layers,
  Settings2,
  Calendar,
  MapPin,
  Maximize2,
  Weight,
  Palette,
  Fingerprint,
} from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';
import { CoverGrid } from '@/components/CoverGrid';

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try {
    const parsed = JSON.parse(photos);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function BioCard({ icon: Icon, label, value, color = "blue" }: { icon: any, label: string; value?: string | null, color?: string }) {
  if (!value) return null;
  
  const colors: Record<string, string> = {
    blue: "text-blue-300 bg-blue-500/10 border-blue-500/20",
    pink: "text-pink-300 bg-pink-500/10 border-pink-500/20",
    amber: "text-amber-300 bg-amber-500/10 border-amber-500/20",
    emerald: "text-emerald-300 bg-emerald-500/10 border-emerald-500/20",
    violet: "text-violet-300 bg-violet-500/10 border-violet-500/20",
    zinc: "text-zinc-300 bg-zinc-500/10 border-zinc-500/20",
  };

  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-950/60 p-3">
      <div className="flex items-center gap-3">
      <div className={`p-2 rounded-md border ${colors[color] || colors.zinc}`}>
        <Icon size={18} />
      </div>
      <div>
        <p className="text-[11px] uppercase tracking-wide text-zinc-500 font-medium">{label}</p>
        <p className="text-sm text-zinc-100">{value}</p>
      </div>
      </div>
    </div>
  );
}

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

  const { data: providers } = useQuery({
    queryKey: ['people-providers'],
    queryFn: () => people.providers(),
    enabled: open,
  });

  const { data: searchResponse, isLoading: searching, error: searchError } = useQuery({
    queryKey: ['people-search', personId, provider, query],
    queryFn: () => people.search(personId, provider, query),
    enabled: open && searchTriggered && !!query && !!provider,
    retry: false,
  });

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
    setTimeout(() => setSearchTriggered(true), 0);
  };

  const handleClose = () => {
    setSearchTriggered(false);
    setSelectedResult(null);
    setQuery(personName);
    setProvider('stashdb');
    onClose();
  };

  if (!open) return null;

  const results = searchResponse?.results ?? [];

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 backdrop-blur-md p-4">
      <div className="w-full max-w-4xl max-h-[90vh] flex flex-col overflow-hidden rounded-[2.5rem] border border-white/10 bg-[#0b0b10] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/10 px-8 py-6">
          <div>
            <h2 className="text-xl font-bold text-white">Match Metadata</h2>
            <p className="text-sm text-zinc-400 mt-1">Connect this profile to external data providers</p>
          </div>
          <button onClick={handleClose} className="rounded-full p-2 text-zinc-400 hover:bg-white/5 hover:text-white transition-colors">
            <X size={20} />
          </button>
        </div>

        <div className="px-8 py-6 bg-white/[0.02]">
          <div className="flex flex-col sm:flex-row gap-4">
            <div className="w-full sm:w-48">
              <Select
                label="Provider"
                value={provider}
                onChange={(e) => {
                  setProvider(e.target.value);
                  setSearchTriggered(false);
                }}
                options={(providers ?? ['stashdb']).map((p) => ({ value: p, label: p }))}
              />
            </div>
            <div className="flex-1 flex gap-2 items-end">
              <div className="flex-1">
                <Input
                  label="Name to search"
                  value={query}
                  onChange={(e) => {
                    setQuery(e.target.value);
                    setSearchTriggered(false);
                  }}
                  onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                  placeholder="Enter a name or alias"
                />
              </div>
              <Button size="sm" onClick={handleSearch} disabled={!query || searching} className="mb-0.5">
                {searching ? <Spinner size="sm" /> : <Search size={18} />}
              </Button>
            </div>
          </div>
        </div>

        <div className="flex-1 overflow-y-auto px-8 py-6 space-y-6">
          {searching && (
            <div className="py-20 flex flex-col items-center justify-center gap-4">
              <Spinner size="lg" />
              <p className="text-zinc-500 animate-pulse">Searching through {provider}...</p>
            </div>
          )}

          {searchError && (
            <Card variant="danger" className="flex items-center gap-3">
              <XCircle size={18} />
              <p className="text-sm">Search failed: {searchError instanceof Error ? searchError.message : 'Unknown error'}</p>
            </Card>
          )}

          {searchTriggered && !searching && results.length === 0 && !searchError && (
            <EmptyState 
               icon={<Search size={48} className="text-zinc-700" />}
               message={`No matches found for "${query}"`} 
               description="Try adjusting the name or switching providers."
            />
          )}

          {results.length > 0 && (
            <div className="grid gap-4">
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

        {selectedResult && (
          <div className="flex items-center justify-between border-t border-white/10 px-8 py-6 bg-blue-500/5">
            <div>
              <p className="text-xs font-bold uppercase tracking-wider text-blue-400 mb-1">Target Match</p>
              <h3 className="font-bold text-white text-lg">{selectedResult.name}</h3>
            </div>
            <div className="flex gap-3">
              <Button variant="secondary" onClick={() => setSelectedResult(null)}>Cancel</Button>
              <Button onClick={() => identifyMut.mutate(selectedResult)} disabled={!selectedResult.external_id || identifyMut.isPending}>
                {identifyMut.isPending ? <Spinner size="sm" /> : <><Check size={18} className="mr-2" /> Link Profile</>}
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function SearchResultCard({ result, selected, onSelect }: any) {
  const thumbnailUrl = result.image_urls?.[0] ?? result.image_url;
  
  return (
    <button
      onClick={onSelect}
      className={`group flex items-center gap-6 rounded-3xl border p-4 text-left transition-all duration-300 ${
        selected 
          ? 'border-blue-500/50 bg-blue-500/10 shadow-lg shadow-blue-500/5 scale-[1.02]' 
          : 'border-white/5 bg-white/[0.03] hover:border-white/10 hover:bg-white/[0.05]'
      }`}
    >
      <div className="h-24 w-24 flex-shrink-0 overflow-hidden rounded-2xl bg-zinc-800 ring-1 ring-white/10 shadow-md">
        {thumbnailUrl ? (
          <img src={thumbnailUrl} alt={result.name} className="h-full w-full object-cover group-hover:scale-110 transition-transform duration-500" />
        ) : (
          <div className="flex h-full w-full items-center justify-center bg-zinc-900">
            <User size={32} className="text-zinc-700" />
          </div>
        )}
      </div>

      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <h4 className="text-lg font-bold text-white truncate">{result.name}</h4>
          {selected && <Check size={16} className="text-blue-400" />}
        </div>
        {result.nationality && <p className="text-sm text-zinc-400">{result.nationality} · {result.ethnicity}</p>}
        {result.birth_date && <p className="text-xs text-zinc-500 mt-1">Born {result.birth_date}</p>}
        <p className="text-xs font-mono text-zinc-600 mt-2">{result.external_id}</p>
      </div>
    </button>
  );
}

export function PersonDetailPage() {
  const { id } = useParams<{ id: string }>();
  const personId = Number(id);
  const queryClient = useQueryClient();

  const [editing, setEditing] = useState(false);
  const [showTools, setShowTools] = useState(false);
  const [editForm, setEditForm] = useState<any>({});
  const [photoIndex, setPhotoIndex] = useState(0);
  const { page: galleryPage, offset: galleryOffset, limit: galleryLimit, prevPage: galleryPrev, nextPage: galleryNext } = usePagination({ limit: 12, paramName: 'gpage' });
  const [linkGalleryId, setLinkGalleryId] = useState('');
  const [identifyOpen, setIdentifyOpen] = useState(false);

  const { data: person, isLoading: loadingPerson } = useQuery({
    queryKey: ['person', personId],
    queryFn: () => people.get(personId),
  });

  const { data: galleryList, isLoading: loadingGalleries } = useQuery({
    queryKey: ['person-galleries', personId, { offset: galleryOffset, limit: galleryLimit }],
    queryFn: () => people.galleries(personId, { limit: galleryLimit, offset: galleryOffset }),
  });

  const { data: identifiers } = useQuery({
    queryKey: ['person-identifiers', personId],
    queryFn: () => people.identifiers(personId),
  });

  const updateMut = useMutation({
    mutationFn: () => people.update(personId, editForm),
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

  const startEditing = () => {
    if (!person) return;
    setEditForm({
      name: person.name,
      aliases: person.aliases ?? '',
      nationality: person.nationality ?? '',
      birth_date: person.birth_date ?? '',
      ethnicity: person.ethnicity ?? '',
      hair_color: person.hair_color ?? '',
      eye_color: person.eye_color ?? '',
      height: person.height ?? '',
      weight: person.weight ?? '',
      measurements: person.measurements ?? '',
      tattoos: person.tattoos ?? '',
      piercings: person.piercings ?? '',
      biography: person.biography ?? '',
    });
    setEditing(true);
  };

  if (loadingPerson) return <div className="py-40 flex justify-center"><Spinner size="lg" /></div>;
  if (!person) return <EmptyState message="Profile not found" />;

  const photos = parsePhotos(person.photos);
  const coverPhoto = photos[photoIndex] ?? photos[0];
  const statChips = [
    { label: 'Galleries', value: String(person.gallery_count ?? 0) },
    { label: 'Photos', value: String(photos.length) },
    { label: 'Linked IDs', value: String(identifiers?.length ?? 0) },
  ];

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 pb-16">
      <div className="py-4">
        <Link to="/people" className="inline-flex items-center gap-2 text-sm text-zinc-400 hover:text-zinc-200 transition-colors">
          <ArrowLeft size={16} />
          Back to People
        </Link>
      </div>
          <section className="rounded-xl border border-zinc-800 bg-zinc-900 p-3 md:p-4 mb-4">
            <div className="grid items-start grid-cols-[180px_minmax(0,1fr)] md:grid-cols-[220px_minmax(0,1fr)] gap-4">
              <div className="rounded-lg border border-zinc-700 bg-zinc-800 p-2">
                <div className="relative aspect-[3/4] overflow-hidden rounded-md bg-zinc-800">
                  {coverPhoto ? (
                    <img src={coverPhoto} alt={person.name} className="h-full w-full object-cover" />
                  ) : (
                    <div className="h-full w-full flex items-center justify-center">
                      <User size={44} className="text-zinc-600" />
                    </div>
                  )}
                </div>
                {photos.length > 1 && (
                  <div className="mt-2 flex items-center justify-between">
                    <button
                      onClick={() => setPhotoIndex((i) => (i - 1 + photos.length) % photos.length)}
                      className="inline-flex h-7 w-7 items-center justify-center rounded border border-zinc-600 text-zinc-300 hover:text-white"
                    >
                      <ChevronLeft size={14} />
                    </button>
                    <span className="text-xs text-zinc-400">{photoIndex + 1}/{photos.length}</span>
                    <button
                      onClick={() => setPhotoIndex((i) => (i + 1) % photos.length)}
                      className="inline-flex h-7 w-7 items-center justify-center rounded border border-zinc-600 text-zinc-300 hover:text-white"
                    >
                      <ChevronRight size={14} />
                    </button>
                  </div>
                )}
              </div>

              <div className="rounded-lg border border-zinc-700 bg-zinc-950/50 p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
                    <h1 className="text-2xl md:text-3xl font-semibold text-white">{person.name}</h1>
                    {person.aliases && (
                      <p className="text-sm text-zinc-400 mt-1">
                        {typeof person.aliases === 'string' ? person.aliases.split(',').map((a) => a.trim()).join(' • ') : person.aliases}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <Button variant="secondary" size="sm" onClick={() => setShowTools((v) => !v)}>
                      <Settings2 size={14} /> {showTools ? 'Hide Tools' : 'Tools'}
                    </Button>
                    <Button size="sm" onClick={startEditing}>
                      <Edit size={14} /> Edit
                    </Button>
                  </div>
                </div>

                <div className="mt-4 grid grid-cols-1 sm:grid-cols-3 gap-2">
                  {statChips.map((chip) => (
                    <div key={chip.label} className="rounded-md border border-zinc-800 bg-zinc-900 px-3 py-2">
                      <p className="text-[11px] uppercase tracking-wide text-zinc-500">{chip.label}</p>
                      <p className="text-base text-zinc-100">{chip.value}</p>
                    </div>
                  ))}
                </div>

                <div className="mt-3 flex flex-wrap gap-2">
                  {person.nationality && <Badge variant="info">{person.nationality}</Badge>}
                  {person.ethnicity && <Badge>{person.ethnicity}</Badge>}
                  {identifiers?.map((id: any) => (
                    <Badge key={id.id} variant="success">{id.provider}</Badge>
                  ))}
                </div>

                <div className="mt-4 grid grid-cols-1 lg:grid-cols-2 gap-3">
                  <div className="space-y-2">
                    <BioCard icon={Calendar} label="Birth Date" value={person.birth_date} color="blue" />
                    <BioCard icon={Maximize2} label="Height" value={person.height} color="amber" />
                    <BioCard icon={Weight} label="Weight" value={person.weight} color="emerald" />
                    <BioCard icon={Check} label="Measurements" value={person.measurements} color="blue" />
                  </div>
                  <div className="space-y-2">
                    <BioCard icon={MapPin} label="Nationality" value={person.nationality} color="pink" />
                    <BioCard icon={Fingerprint} label="Ethnicity" value={person.ethnicity} color="violet" />
                    <BioCard icon={Palette} label="Hair Color" value={person.hair_color} color="amber" />
                    <BioCard icon={Palette} label="Eye Color" value={person.eye_color} color="violet" />
                  </div>
                </div>

                {person.biography && (
                  <div className="mt-4 rounded-md border border-zinc-800 bg-zinc-900 p-3">
                    <p className="text-xs uppercase tracking-wide text-zinc-500 mb-1">Biography</p>
                    <p className="text-sm text-zinc-300 whitespace-pre-line">{person.biography}</p>
                  </div>
                )}

                {showTools && (
                  <div className="mt-4 rounded-md border border-zinc-800 bg-zinc-900 p-3">
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                      <Button variant="secondary" size="sm" onClick={() => setIdentifyOpen(true)}>
                        <Search size={14} /> Match Externally
                      </Button>
                      <Button variant="secondary" size="sm" onClick={() => enrichMut.mutate()} disabled={enrichMut.isPending}>
                        <Sparkles size={14} /> {enrichMut.isPending ? 'Syncing...' : 'Sync Full Data'}
                      </Button>
                      <div className="flex gap-2">
                        <Input placeholder="Gallery ID" value={linkGalleryId} onChange={(e) => setLinkGalleryId(e.target.value)} />
                        <Button
                          size="sm"
                          onClick={() => {
                            const gid = parseInt(linkGalleryId, 10);
                            if (gid) linkMut.mutate(gid);
                          }}
                          disabled={!linkGalleryId || linkMut.isPending}
                        >
                          <Link2 size={14} />
                        </Button>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </section>

          <section className="rounded-xl border border-zinc-800 bg-zinc-900 p-4 md:p-5">
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold text-white flex items-center gap-2">
                <Layers size={16} />
                Galleries
              </h2>
              <Badge>{person.gallery_count ?? 0} total</Badge>
            </div>

            {loadingGalleries ? (
              <Spinner />
            ) : !galleryList || galleryList.length === 0 ? (
              <EmptyState message="No galleries linked yet." />
            ) : (
              <>
                <CoverGrid
                  items={galleryList.map((g) => ({
                    id: g.id,
                    title: g.title ?? null,
                    thumbnailPath: g.local_thumbnail_path ? g.local_thumbnail_path.split('/').pop() : undefined,
                    provider: g.provider ?? null,
                    createdAt: g.created_at,
                  }))}
                />
                <div className="mt-4">
                  <Pagination
                    page={galleryPage}
                    hasMore={galleryList.length === galleryLimit}
                    onPrev={galleryPrev}
                    onNext={galleryNext}
                  />
                </div>
              </>
            )}
          </section>
      {/* Edit Modal (Glassy) */}
      {editing && (
        <div className="fixed inset-0 z-[110] flex items-center justify-center bg-black/80 backdrop-blur-xl p-4">
          <div className="w-full max-w-5xl max-h-[90vh] flex flex-col overflow-hidden rounded-[3rem] border border-white/10 bg-[#0b0b10] shadow-2xl">
            <div className="flex items-center justify-between px-10 py-8">
              <h2 className="text-3xl font-black text-white tracking-tight">Edit Profile</h2>
              <button onClick={() => setEditing(false)} className="p-2 rounded-full hover:bg-white/5 text-zinc-400 transition-colors">
                <X size={28} />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto px-10 pb-10 space-y-8">
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                <Input label="Name" value={editForm.name} onChange={e => setEditForm({...editForm, name: e.target.value})} />
                <Input label="Aliases" placeholder="Comma-separated" value={editForm.aliases} onChange={e => setEditForm({...editForm, aliases: e.target.value})} />
                <Input label="Birth Date" placeholder="YYYY-MM-DD" value={editForm.birth_date} onChange={e => setEditForm({...editForm, birth_date: e.target.value})} />
                <Input label="Nationality" value={editForm.nationality} onChange={e => setEditForm({...editForm, nationality: e.target.value})} />
                <Input label="Ethnicity" value={editForm.ethnicity} onChange={e => setEditForm({...editForm, ethnicity: e.target.value})} />
                <Input label="Hair Color" value={editForm.hair_color} onChange={e => setEditForm({...editForm, hair_color: e.target.value})} />
                <Input label="Eye Color" value={editForm.eye_color} onChange={e => setEditForm({...editForm, eye_color: e.target.value})} />
                <Input label="Height" value={editForm.height} onChange={e => setEditForm({...editForm, height: e.target.value})} />
                <Input label="Weight" value={editForm.weight} onChange={e => setEditForm({...editForm, weight: e.target.value})} />
                <Input label="Measurements" value={editForm.measurements} onChange={e => setEditForm({...editForm, measurements: e.target.value})} />
                <Input label="Tattoos" value={editForm.tattoos} onChange={e => setEditForm({...editForm, tattoos: e.target.value})} />
                <Input label="Piercings" value={editForm.piercings} onChange={e => setEditForm({...editForm, piercings: e.target.value})} />
              </div>
              <Textarea label="Biography" rows={6} value={editForm.biography} onChange={e => setEditForm({...editForm, biography: e.target.value})} />
            </div>

            <div className="px-10 py-8 border-t border-white/5 flex justify-end gap-3 bg-white/[0.01]">
              <Button variant="secondary" onClick={() => setEditing(false)}>Discard</Button>
              <Button onClick={() => updateMut.mutate()} disabled={updateMut.isPending}>
                <Save size={18} className="mr-2" /> Save Profile
              </Button>
            </div>
          </div>
        </div>
      )}

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
    </div>
  );
}
