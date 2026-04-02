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
  Unlink,
  Plus,
  Save,
  ExternalLink,
  X,
  Check,
  User,
  ChevronLeft,
  ChevronRight,
} from 'lucide-react';
import { usePagination } from '@/hooks/usePagination';

function parsePhotos(photos?: string): string[] {
  if (!photos) return [];
  try {
    const parsed = JSON.parse(photos);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function DetailItem({ label, value }: { label: string; value?: string | null }) {
  if (!value) return null;
  return (
    <div className="rounded-2xl bg-white/5 p-3 ring-1 ring-white/5">
      <span className="text-[11px] uppercase tracking-wide text-zinc-500">{label}</span>
      <p className="mt-1 text-sm text-zinc-100 leading-snug">{value}</p>
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
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/75 px-4 pt-10">
      <div className="mb-10 w-full max-w-5xl overflow-hidden rounded-[2rem] border border-white/10 bg-[#0b0b10] shadow-2xl shadow-black/40">
        <div className="flex items-center justify-between border-b border-white/10 px-6 py-4">
          <div>
            <h2 className="text-lg font-semibold text-white">Identify Person</h2>
            <p className="text-sm text-zinc-400">Search providers and apply a match</p>
          </div>
          <button onClick={handleClose} className="rounded-full p-2 text-zinc-400 hover:bg-white/5 hover:text-white">
            <X size={18} />
          </button>
        </div>

        <div className="px-6 py-5">
          <div className="grid gap-3 md:grid-cols-[180px,1fr,auto] md:items-end">
            <Select
              label="Provider"
              value={provider}
              onChange={(e) => {
                setProvider(e.target.value);
                setSearchTriggered(false);
              }}
              options={(providers ?? ['stashdb']).map((p) => ({ value: p, label: p }))}
            />
            <Input
              label="Search query"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setSearchTriggered(false);
              }}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              placeholder="Enter a name or alias"
            />
            <Button size="sm" onClick={handleSearch} disabled={!query || searching}>
              <Search size={14} /> {searching ? 'Searching...' : 'Search'}
            </Button>
          </div>
        </div>

        <div className="max-h-[58vh] overflow-y-auto px-6 pb-6">
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
              <p className="text-xs uppercase tracking-wide text-zinc-500">
                {results.length} result{results.length !== 1 ? 's' : ''}
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

        {selectedResult && (
          <div className="flex items-center justify-between border-t border-white/10 px-6 py-4">
            <div className="text-sm text-zinc-300">
              Selected <span className="font-medium text-white">{selectedResult.name}</span>
            </div>
            <div className="flex gap-2">
              <Button variant="secondary" size="sm" onClick={() => setSelectedResult(null)}>
                Deselect
              </Button>
              <Button size="sm" onClick={() => identifyMut.mutate(selectedResult)} disabled={!selectedResult.external_id || identifyMut.isPending}>
                <Check size={14} /> {identifyMut.isPending ? 'Applying...' : 'Apply & Link'}
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

interface SearchResultCardProps {
  result: PersonInfo;
  selected: boolean;
  onSelect: () => void;
}

function SearchResultCard({ result, selected, onSelect }: SearchResultCardProps) {
  const details: string[] = [];
  if (result.nationality) details.push(result.nationality);
  if (result.birth_date) details.push(`Born ${result.birth_date}`);
  if (result.ethnicity) details.push(result.ethnicity);
  if (result.height) details.push(result.height);
  if (result.weight) details.push(result.weight);
  if (result.measurements) details.push(result.measurements);
  if (result.hair_color) details.push(`Hair ${result.hair_color}`);
  if (result.eye_color) details.push(`Eyes ${result.eye_color}`);
  if (result.tattoos) details.push(`Tattoos ${result.tattoos}`);
  if (result.piercings) details.push(`Piercings ${result.piercings}`);

  const thumbnailUrl = result.image_urls?.[0] ?? result.image_url;

  return (
    <button
      onClick={onSelect}
      className={`group flex w-full gap-4 rounded-[1.5rem] border p-3 text-left transition-all ${
        selected ? 'border-blue-500/70 bg-blue-500/10' : 'border-white/8 bg-white/5 hover:border-white/15 hover:bg-white/7'
      }`}
    >
      <div className="h-28 w-20 flex-shrink-0 overflow-hidden rounded-[1rem] bg-zinc-800 ring-1 ring-white/5">
        {thumbnailUrl ? (
          <img src={thumbnailUrl} alt={result.name} className="h-full w-full object-cover" />
        ) : (
          <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-zinc-800 to-zinc-900">
            <User size={24} className="text-zinc-600" />
          </div>
        )}
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate text-base font-semibold text-white">{result.name || 'Unknown'}</span>
              {selected && <Badge variant="info">Selected</Badge>}
            </div>
            {result.aliases && result.aliases.length > 0 && (
              <p className="mt-1 text-sm text-zinc-400">aka {result.aliases.slice(0, 3).join(', ')}</p>
            )}
          </div>
          {result.external_id && <span className="font-mono text-[11px] text-zinc-600">{result.external_id}</span>}
        </div>

        {details.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-2">
            {details.slice(0, 6).map((d) => (
              <span key={d} className="rounded-full bg-white/5 px-2.5 py-1 text-xs text-zinc-300 ring-1 ring-white/5">
                {d}
              </span>
            ))}
          </div>
        )}

        {result.biography && <p className="mt-3 line-clamp-2 text-sm leading-relaxed text-zinc-500">{result.biography}</p>}
      </div>
    </button>
  );
}

interface EditForm {
  name: string;
  aliases: string;
  nationality: string;
  birth_date: string;
  ethnicity: string;
  hair_color: string;
  eye_color: string;
  height: string;
  weight: string;
  measurements: string;
  tattoos: string;
  piercings: string;
  biography: string;
}

const emptyForm: EditForm = {
  name: '',
  aliases: '',
  nationality: '',
  birth_date: '',
  ethnicity: '',
  hair_color: '',
  eye_color: '',
  height: '',
  weight: '',
  measurements: '',
  tattoos: '',
  piercings: '',
  biography: '',
};

function personToForm(p: Person): EditForm {
  return {
    name: p.name,
    aliases: p.aliases ?? '',
    nationality: p.nationality ?? '',
    birth_date: p.birth_date ?? '',
    ethnicity: p.ethnicity ?? '',
    hair_color: p.hair_color ?? '',
    eye_color: p.eye_color ?? '',
    height: p.height ?? '',
    weight: p.weight ?? '',
    measurements: p.measurements ?? '',
    tattoos: p.tattoos ?? '',
    piercings: p.piercings ?? '',
    biography: p.biography ?? '',
  };
}

export function PersonDetailPage() {
  const { id } = useParams<{ id: string }>();
  const personId = Number(id);
  const queryClient = useQueryClient();

  const [editing, setEditing] = useState(false);
  const [editForm, setEditForm] = useState<EditForm>(emptyForm);
  const [photoIndex, setPhotoIndex] = useState(0);
  const { page: galleryPage, offset: galleryOffset, limit: galleryLimit, prevPage: galleryPrev, nextPage: galleryNext } = usePagination({ limit: 20, paramName: 'gpage' });
  const [linkGalleryId, setLinkGalleryId] = useState('');
  const [newIdentifier, setNewIdentifier] = useState({ provider: '', external_id: '' });
  const [identifyOpen, setIdentifyOpen] = useState(false);

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

  const updateMut = useMutation({
    mutationFn: () => {
      const data: Partial<Person> = { name: editForm.name };
      if (editForm.aliases) data.aliases = editForm.aliases;
      if (editForm.nationality) data.nationality = editForm.nationality;
      if (editForm.birth_date) data.birth_date = editForm.birth_date;
      if (editForm.ethnicity) data.ethnicity = editForm.ethnicity;
      if (editForm.hair_color) data.hair_color = editForm.hair_color;
      if (editForm.eye_color) data.eye_color = editForm.eye_color;
      if (editForm.height) data.height = editForm.height;
      if (editForm.weight) data.weight = editForm.weight;
      if (editForm.measurements) data.measurements = editForm.measurements;
      if (editForm.tattoos) data.tattoos = editForm.tattoos;
      if (editForm.piercings) data.piercings = editForm.piercings;
      if (editForm.biography) data.biography = editForm.biography;
      return people.update(personId, data);
    },
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

  const startEditing = () => {
    if (!person) return;
    setEditForm(personToForm(person));
    setEditing(true);
  };

  const setField = (field: keyof EditForm, value: string) => setEditForm((prev) => ({ ...prev, [field]: value }));

  if (loadingPerson) return <Spinner />;
  if (!person) return <EmptyState message="Person not found." />;

  const photos = parsePhotos(person.photos);
  const coverPhoto = photos[photoIndex] ?? photos[0];

  return (
    <>
      <div className="mb-4">
        <Link to="/people" className="text-sm text-zinc-400 hover:text-zinc-200 inline-flex items-center gap-1">
          <ArrowLeft size={14} /> Back to people
        </Link>
      </div>

      <div className="grid gap-6 xl:grid-cols-[360px,1fr]">
        <aside className="space-y-4 xl:sticky xl:top-6 self-start">
          <div className="overflow-hidden rounded-[2rem] border border-white/8 bg-white/5">
            <div className="relative aspect-[4/5] bg-zinc-900">
              {coverPhoto ? (
                <img src={coverPhoto} alt={person.name} className="h-full w-full object-cover" />
              ) : (
                <div className="flex h-full w-full items-center justify-center bg-gradient-to-br from-zinc-800 to-zinc-950">
                  <User size={72} className="text-zinc-600" />
                </div>
              )}
            </div>

            {photos.length > 1 && (
              <div className="flex items-center justify-between border-t border-white/8 px-4 py-3 text-xs text-zinc-300">
                <button onClick={() => setPhotoIndex((i) => (i - 1 + photos.length) % photos.length)} className="rounded-full bg-white/5 p-2 hover:bg-white/10">
                  <ChevronLeft size={14} />
                </button>
                <span>{photoIndex + 1} / {photos.length}</span>
                <button onClick={() => setPhotoIndex((i) => (i + 1) % photos.length)} className="rounded-full bg-white/5 p-2 hover:bg-white/10">
                  <ChevronRight size={14} />
                </button>
              </div>
            )}
          </div>

          <Card className="rounded-[2rem] border-white/8 bg-white/5">
            <div className="flex flex-wrap gap-2">
              {person.nationality && <Badge>{person.nationality}</Badge>}
              {person.ethnicity && <Badge variant="info">{person.ethnicity}</Badge>}
              {person.height && <Badge variant="default">{person.height}</Badge>}
              {person.measurements && <Badge variant="warning">{person.measurements}</Badge>}
            </div>
            <p className="mt-4 text-sm text-zinc-300 whitespace-pre-line">
              {person.biography || 'No biography yet.'}
            </p>
            <div className="mt-4 grid gap-3">
              <Button variant="secondary" size="sm" onClick={() => (editing ? setEditing(false) : startEditing())}>
                {editing ? 'Cancel edit' : 'Edit profile'}
              </Button>
              <Button variant="secondary" size="sm" onClick={() => setIdentifyOpen(true)}>
                <Search size={14} /> Identify
              </Button>
              <Button size="sm" onClick={() => enrichMut.mutate()} disabled={enrichMut.isPending}>
                <Sparkles size={14} /> {enrichMut.isPending ? 'Enriching...' : 'Enrich'}
              </Button>
            </div>
          </Card>
        </aside>

        <main className="space-y-6">
          <div className="rounded-[2rem] border border-white/8 bg-white/5 p-6">
            <p className="text-xs uppercase tracking-[0.28em] text-zinc-500">Talent profile</p>
            <h1 className="mt-2 text-4xl font-semibold text-white">{person.name}</h1>
            <div className="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <DetailItem label="Aliases" value={person.aliases} />
              <DetailItem label="Birth date" value={person.birth_date} />
              <DetailItem label="Hair color" value={person.hair_color} />
              <DetailItem label="Eye color" value={person.eye_color} />
              <DetailItem label="Weight" value={person.weight} />
              <DetailItem label="Tattoos" value={person.tattoos} />
              <DetailItem label="Piercings" value={person.piercings} />
              <DetailItem label="Added" value={formatDate(person.created_at)} />
            </div>
          </div>

          {editing && (
            <Card className="rounded-[2rem] border-white/8 bg-white/5">
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
                <Input label="Name" value={editForm.name} onChange={(e) => setField('name', e.target.value)} />
                <Input label="Aliases" placeholder="Comma-separated" value={editForm.aliases} onChange={(e) => setField('aliases', e.target.value)} />
                <Input label="Birth Date" placeholder="YYYY-MM-DD" value={editForm.birth_date} onChange={(e) => setField('birth_date', e.target.value)} />
                <Input label="Nationality" value={editForm.nationality} onChange={(e) => setField('nationality', e.target.value)} />
                <Input label="Ethnicity" value={editForm.ethnicity} onChange={(e) => setField('ethnicity', e.target.value)} />
                <Input label="Hair Color" value={editForm.hair_color} onChange={(e) => setField('hair_color', e.target.value)} />
                <Input label="Eye Color" value={editForm.eye_color} onChange={(e) => setField('eye_color', e.target.value)} />
                <Input label="Height" placeholder={'e.g. 5\'6" (168cm)'} value={editForm.height} onChange={(e) => setField('height', e.target.value)} />
                <Input label="Weight" placeholder="e.g. 115 lbs (52kg)" value={editForm.weight} onChange={(e) => setField('weight', e.target.value)} />
                <Input label="Measurements" placeholder="e.g. 34C-24-35" value={editForm.measurements} onChange={(e) => setField('measurements', e.target.value)} />
                <Input label="Tattoos" value={editForm.tattoos} onChange={(e) => setField('tattoos', e.target.value)} />
                <Input label="Piercings" value={editForm.piercings} onChange={(e) => setField('piercings', e.target.value)} />
              </div>
              <div className="mt-3">
                <Textarea label="Biography" rows={5} value={editForm.biography} onChange={(e) => setField('biography', e.target.value)} />
              </div>
              <div className="mt-4 flex justify-end">
                <Button size="sm" onClick={() => updateMut.mutate()} disabled={!editForm.name || updateMut.isPending}>
                  <Save size={14} /> Save changes
                </Button>
              </div>
            </Card>
          )}

          <section>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-white">Identifiers</h2>
              <span className="text-sm text-zinc-500">External matches and references</span>
            </div>
            <Card className="rounded-[2rem] border-white/8 bg-white/5">
              {loadingIds ? (
                <Spinner />
              ) : !identifiers || identifiers.length === 0 ? (
                <EmptyState message="No external identifiers linked." />
              ) : (
                <div className="space-y-3">
                  {identifiers.map((ident) => (
                    <div key={ident.id} className="flex items-center justify-between rounded-2xl bg-black/20 px-4 py-3 ring-1 ring-white/5">
                      <div className="flex items-center gap-2">
                        <Badge variant="info">{ident.provider}</Badge>
                        <span className="font-mono text-sm text-zinc-200">{ident.external_id}</span>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              <div className="mt-4 grid gap-2 sm:grid-cols-[160px,1fr,auto] sm:items-end">
                <Input label="Provider" placeholder="e.g. stashdb" value={newIdentifier.provider} onChange={(e) => setNewIdentifier({ ...newIdentifier, provider: e.target.value })} />
                <Input label="External ID" placeholder="e.g. abc-123" value={newIdentifier.external_id} onChange={(e) => setNewIdentifier({ ...newIdentifier, external_id: e.target.value })} />
                <Button size="sm" onClick={() => upsertIdMut.mutate()} disabled={!newIdentifier.provider || !newIdentifier.external_id || upsertIdMut.isPending}>
                  <Plus size={14} /> Add
                </Button>
              </div>
            </Card>
          </section>

          <section>
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-lg font-semibold text-white">Linked galleries</h2>
              <span className="text-sm text-zinc-500">Collections associated with this profile</span>
            </div>

            <div className="mb-4 flex items-end gap-2">
              <Input placeholder="Gallery ID to link" value={linkGalleryId} onChange={(e) => setLinkGalleryId(e.target.value)} className="w-44" />
              <Button size="sm" onClick={() => {
                const gid = parseInt(linkGalleryId);
                if (gid) linkMut.mutate(gid);
              }} disabled={!linkGalleryId || linkMut.isPending}>
                <Link2 size={14} /> Link
              </Button>
            </div>

            {loadingGalleries ? (
              <Spinner />
            ) : !galleryList || galleryList.length === 0 ? (
              <EmptyState message="No galleries linked to this person." />
            ) : (
              <div className="space-y-3">
                {galleryList.map((g) => (
                  <Card key={g.id} className="rounded-[1.5rem] border-white/8 bg-white/5 p-4">
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <Link to={`/galleries/${g.id}`} className="block text-sm font-medium text-white hover:text-blue-400 line-clamp-1">
                          {g.title || `Gallery #${g.id}`}
                        </Link>
                        {g.provider && <Badge className="mt-2">{g.provider}</Badge>}
                        {g.url && (
                          <a href={g.url} target="_blank" className="mt-2 inline-flex items-center gap-1 text-xs text-zinc-500 hover:text-blue-400">
                            <ExternalLink size={10} /> {g.url}
                          </a>
                        )}
                      </div>
                      <Button variant="ghost" size="sm" title="Unlink gallery" onClick={() => unlinkMut.mutate(g.id)} disabled={unlinkMut.isPending}>
                        <Unlink size={14} />
                      </Button>
                    </div>
                  </Card>
                ))}
              </div>
            )}

            {galleryList && galleryList.length > 0 && (
              <Pagination page={galleryPage} hasMore={galleryList.length === galleryLimit} onPrev={galleryPrev} onNext={galleryNext} />
            )}
          </section>
        </main>
      </div>

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
