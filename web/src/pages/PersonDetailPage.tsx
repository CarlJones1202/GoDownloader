import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { people } from '@/lib/api';
import { formatDate } from '@/lib/utils';
import {
  PageHeader,
  Card,
  Badge,
  Button,
  Spinner,
  EmptyState,
  Input,
  Pagination,
} from '@/components/UI';
import {
  ArrowLeft,
  Sparkles,
  Link2,
  Unlink,
  Plus,
  Save,
  ExternalLink,
} from 'lucide-react';

export function PersonDetailPage() {
  const { id } = useParams<{ id: string }>();
  const personId = Number(id);
  const queryClient = useQueryClient();

  const [editing, setEditing] = useState(false);
  const [editForm, setEditForm] = useState({ name: '', aliases: '', nationality: '' });
  const [galleryOffset, setGalleryOffset] = useState(0);
  const [linkGalleryId, setLinkGalleryId] = useState('');
  const [newIdentifier, setNewIdentifier] = useState({ provider: '', external_id: '' });
  const galleryLimit = 20;

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
            offset={galleryOffset}
            limit={galleryLimit}
            hasMore={galleryList.length === galleryLimit}
            onPrev={() => setGalleryOffset(Math.max(0, galleryOffset - galleryLimit))}
            onNext={() => setGalleryOffset(galleryOffset + galleryLimit)}
          />
        </>
      )}
    </>
  );
}
