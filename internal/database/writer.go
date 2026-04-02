package database

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/carlj/godownload/internal/models"
)

type DBWriter struct {
	db   *DB
	wg   sync.WaitGroup
	stop chan struct{}

	// Operation channels - buffered for backpressure
	imageCh         chan ImageOp
	galleryCh       chan GalleryOp
	queueCh         chan QueueOp
	personCh        chan PersonOp
	sourceCh        chan SourceOp
	identifierCh    chan IdentifierOp
	linkGalleryCh   chan LinkGalleryOp
	unlinkGalleryCh chan UnlinkGalleryOp
	clearQueueCh    chan ClearQueueOp
}

type ImageOp struct {
	Ctx             context.Context
	GalleryID       *int64
	Filename        string
	OriginalURL     *string
	Width           *int
	Height          *int
	DurationSeconds *int
	FileHash        *string
	DominantColors  *string
	IsVideo         bool
	VRMode          string
	IsFavorite      bool

	ResultCh chan ImageResult
}

type ImageResult struct {
	ID        int64
	CreatedAt time.Time
	Err       error
}

type GalleryOp struct {
	Ctx                context.Context
	SourceID           *int64
	Provider           *string
	ProviderGalleryID  *string
	Title              *string
	URL                *string
	ThumbnailURL       *string
	LocalThumbnailPath *string

	ResultCh chan GalleryResult
}

type GalleryResult struct {
	ID        int64
	CreatedAt time.Time
	Err       error
}

type QueueOp struct {
	Ctx    context.Context
	ID     int64
	Status models.QueueStatus
	ErrMsg *string

	ResultCh chan QueueResult
}

type QueueResult struct {
	Err error
}

type PersonOp struct {
	Ctx         context.Context
	Name        string
	Aliases     *string
	Nationality *string

	ResultCh chan PersonResult
}

type PersonResult struct {
	ID        int64
	CreatedAt time.Time
	Err       error
}

type SourceOp struct {
	Ctx context.Context
	ID  int64

	ResultCh chan SourceResult
}

type SourceResult struct {
	Err error
}

type IdentifierOp struct {
	Ctx        context.Context
	PersonID   int64
	Provider   string
	ExternalID string

	ResultCh chan IdentifierResult
}

type IdentifierResult struct {
	Err error
}

type LinkGalleryOp struct {
	Ctx       context.Context
	PersonID  int64
	GalleryID int64

	ResultCh chan LinkGalleryResult
}

type LinkGalleryResult struct {
	Err error
}

type UnlinkGalleryOp struct {
	Ctx       context.Context
	PersonID  int64
	GalleryID int64

	ResultCh chan UnlinkGalleryResult
}

type UnlinkGalleryResult struct {
	Err error
}

type ClearQueueOp struct {
	Ctx    context.Context
	Status *string

	ResultCh chan ClearQueueResult
}

type ClearQueueResult struct {
	Count int64
	Err   error
}

func NewWriter(db *DB) *DBWriter {
	w := &DBWriter{
		db:   db,
		stop: make(chan struct{}),
		// Buffer size of 1000 allows significant backpressure before blocking
		imageCh:         make(chan ImageOp, 1000),
		galleryCh:       make(chan GalleryOp, 1000),
		queueCh:         make(chan QueueOp, 1000),
		personCh:        make(chan PersonOp, 500),
		sourceCh:        make(chan SourceOp, 500),
		identifierCh:    make(chan IdentifierOp, 500),
		linkGalleryCh:   make(chan LinkGalleryOp, 500),
		unlinkGalleryCh: make(chan UnlinkGalleryOp, 500),
		clearQueueCh:    make(chan ClearQueueOp, 100),
	}

	w.wg.Add(1)
	go w.run()

	return w
}

func (w *DBWriter) Stop() {
	close(w.stop)
	w.wg.Wait()
}

func (w *DBWriter) run() {
	defer w.wg.Done()

	for {
		select {
		case <-w.stop:
			return

		// Drain remaining operations before exiting
		case op := <-w.imageCh:
			w.doCreateImage(op)
		case op := <-w.galleryCh:
			w.doCreateGallery(op)
		case op := <-w.queueCh:
			w.doUpdateQueueStatus(op)
		case op := <-w.personCh:
			w.doCreatePerson(op)
		case op := <-w.sourceCh:
			w.doTouchSource(op)
		case op := <-w.identifierCh:
			w.doUpsertIdentifier(op)
		case op := <-w.linkGalleryCh:
			w.doLinkGallery(op)
		case op := <-w.unlinkGalleryCh:
			w.doUnlinkGallery(op)
		case op := <-w.clearQueueCh:
			w.doClearQueue(op)
		}
	}
}

func (w *DBWriter) doCreateImage(op ImageOp) {
	result, err := w.db.ExecContext(op.Ctx,
		`INSERT INTO images
		   (gallery_id, filename, original_url, width, height, duration_seconds,
		    file_hash, dominant_colors, is_video, vr_mode, is_favorite)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		op.GalleryID, op.Filename, op.OriginalURL,
		op.Width, op.Height, op.DurationSeconds,
		op.FileHash, op.DominantColors,
		op.IsVideo, op.VRMode, op.IsFavorite,
	)
	if err != nil {
		op.ResultCh <- ImageResult{Err: fmt.Errorf("creating image: %w", err)}
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		op.ResultCh <- ImageResult{Err: fmt.Errorf("getting image id: %w", err)}
		return
	}
	op.ResultCh <- ImageResult{ID: id, CreatedAt: time.Now().UTC()}
}

func (w *DBWriter) doCreateGallery(op GalleryOp) {
	result, err := w.db.ExecContext(op.Ctx,
		`INSERT INTO galleries (source_id, provider, provider_gallery_id, title, url, thumbnail_url, local_thumbnail_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		op.SourceID, op.Provider, op.ProviderGalleryID, op.Title, op.URL, op.ThumbnailURL, op.LocalThumbnailPath,
	)
	if err != nil {
		op.ResultCh <- GalleryResult{Err: fmt.Errorf("creating gallery: %w", err)}
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		op.ResultCh <- GalleryResult{Err: fmt.Errorf("getting gallery id: %w", err)}
		return
	}
	op.ResultCh <- GalleryResult{ID: id, CreatedAt: time.Now().UTC()}
}

func (w *DBWriter) doUpdateQueueStatus(op QueueOp) {
	_, err := w.db.ExecContext(op.Ctx,
		`UPDATE download_queue SET status = ?, error_message = ? WHERE id = ?`,
		op.Status, op.ErrMsg, op.ID,
	)
	op.ResultCh <- QueueResult{Err: err}
}

func (w *DBWriter) doCreatePerson(op PersonOp) {
	result, err := w.db.ExecContext(op.Ctx,
		`INSERT INTO people (name, aliases, nationality) VALUES (?, ?, ?)`,
		op.Name, op.Aliases, op.Nationality,
	)
	if err != nil {
		op.ResultCh <- PersonResult{Err: fmt.Errorf("creating person: %w", err)}
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		op.ResultCh <- PersonResult{Err: fmt.Errorf("getting person id: %w", err)}
		return
	}
	op.ResultCh <- PersonResult{ID: id, CreatedAt: time.Now().UTC()}
}

func (w *DBWriter) doTouchSource(op SourceOp) {
	_, err := w.db.ExecContext(op.Ctx,
		`UPDATE sources SET last_crawled_at = CURRENT_TIMESTAMP WHERE id = ?`,
		op.ID,
	)
	op.ResultCh <- SourceResult{Err: err}
}

func (w *DBWriter) doUpsertIdentifier(op IdentifierOp) {
	_, err := w.db.ExecContext(op.Ctx,
		`INSERT INTO person_identifiers (person_id, provider, external_id) VALUES (?, ?, ?)
		 ON CONFLICT(person_id, provider, external_id) DO NOTHING`,
		op.PersonID, op.Provider, op.ExternalID,
	)
	op.ResultCh <- IdentifierResult{Err: err}
}

func (w *DBWriter) doLinkGallery(op LinkGalleryOp) {
	_, err := w.db.ExecContext(op.Ctx,
		`INSERT OR IGNORE INTO gallery_persons (gallery_id, person_id) VALUES (?, ?)`,
		op.GalleryID, op.PersonID,
	)
	op.ResultCh <- LinkGalleryResult{Err: err}
}

func (w *DBWriter) doUnlinkGallery(op UnlinkGalleryOp) {
	_, err := w.db.ExecContext(op.Ctx,
		`DELETE FROM gallery_persons WHERE gallery_id = ? AND person_id = ?`,
		op.GalleryID, op.PersonID,
	)
	op.ResultCh <- UnlinkGalleryResult{Err: err}
}

func (w *DBWriter) doClearQueue(op ClearQueueOp) {
	query := `DELETE FROM download_queue WHERE 1=1`
	args := []any{}

	if op.Status != nil && *op.Status != "" {
		query += " AND status = ?"
		args = append(args, *op.Status)
	}

	result, err := w.db.ExecContext(op.Ctx, query, args...)
	if err != nil {
		op.ResultCh <- ClearQueueResult{Err: err}
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		op.ResultCh <- ClearQueueResult{Err: err}
		return
	}
	op.ResultCh <- ClearQueueResult{Count: count}
}

// API methods matching database.DB interface

func (w *DBWriter) CreateImage(ctx context.Context, img *models.Image) error {
	op := ImageOp{
		Ctx:             ctx,
		GalleryID:       img.GalleryID,
		Filename:        img.Filename,
		OriginalURL:     img.OriginalURL,
		Width:           img.Width,
		Height:          img.Height,
		DurationSeconds: img.DurationSeconds,
		FileHash:        img.FileHash,
		DominantColors:  img.DominantColors,
		IsVideo:         img.IsVideo,
		VRMode:          img.VRMode,
		IsFavorite:      img.IsFavorite,
		ResultCh:        make(chan ImageResult, 1),
	}

	select {
	case w.imageCh <- op:
		// ok
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		if result.Err != nil {
			return result.Err
		}
		img.ID = result.ID
		img.CreatedAt = result.CreatedAt
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *DBWriter) CreateGallery(ctx context.Context, g *models.Gallery) error {
	op := GalleryOp{
		Ctx:                ctx,
		SourceID:           g.SourceID,
		Provider:           g.Provider,
		ProviderGalleryID:  g.ProviderGalleryID,
		Title:              g.Title,
		URL:                g.URL,
		ThumbnailURL:       g.ThumbnailURL,
		LocalThumbnailPath: g.LocalThumbnailPath,
		ResultCh:           make(chan GalleryResult, 1),
	}

	select {
	case w.galleryCh <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		if result.Err != nil {
			return result.Err
		}
		g.ID = result.ID
		g.CreatedAt = result.CreatedAt
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *DBWriter) UpdateQueueStatus(ctx context.Context, id int64, status models.QueueStatus, errMsg *string) error {
	op := QueueOp{
		Ctx:      ctx,
		ID:       id,
		Status:   status,
		ErrMsg:   errMsg,
		ResultCh: make(chan QueueResult, 1),
	}

	select {
	case w.queueCh <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		return result.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *DBWriter) CreatePerson(ctx context.Context, p *models.Person) error {
	op := PersonOp{
		Ctx:         ctx,
		Name:        p.Name,
		Aliases:     p.Aliases,
		Nationality: p.Nationality,
		ResultCh:    make(chan PersonResult, 1),
	}

	select {
	case w.personCh <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		if result.Err != nil {
			return result.Err
		}
		p.ID = result.ID
		p.CreatedAt = result.CreatedAt
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *DBWriter) TouchSourceCrawledAt(ctx context.Context, id int64) error {
	op := SourceOp{
		Ctx:      ctx,
		ID:       id,
		ResultCh: make(chan SourceResult, 1),
	}

	select {
	case w.sourceCh <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		return result.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *DBWriter) UpsertPersonIdentifier(ctx context.Context, pid *models.PersonIdentifier) error {
	op := IdentifierOp{
		Ctx:        ctx,
		PersonID:   pid.PersonID,
		Provider:   pid.Provider,
		ExternalID: pid.ExternalID,
		ResultCh:   make(chan IdentifierResult, 1),
	}

	select {
	case w.identifierCh <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		return result.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *DBWriter) LinkGallery(ctx context.Context, personID, galleryID int64) error {
	op := LinkGalleryOp{
		Ctx:       ctx,
		PersonID:  personID,
		GalleryID: galleryID,
		ResultCh:  make(chan LinkGalleryResult, 1),
	}

	select {
	case w.linkGalleryCh <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		return result.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *DBWriter) UnlinkGallery(ctx context.Context, personID, galleryID int64) error {
	op := UnlinkGalleryOp{
		Ctx:       ctx,
		PersonID:  personID,
		GalleryID: galleryID,
		ResultCh:  make(chan UnlinkGalleryResult, 1),
	}

	select {
	case w.unlinkGalleryCh <- op:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		return result.Err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *DBWriter) ClearQueue(ctx context.Context, status *string) (int64, error) {
	op := ClearQueueOp{
		Ctx:      ctx,
		Status:   status,
		ResultCh: make(chan ClearQueueResult, 1),
	}

	select {
	case w.clearQueueCh <- op:
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-w.stop:
		return 0, fmt.Errorf("writer stopped")
	}

	select {
	case result := <-op.ResultCh:
		return result.Count, result.Err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// IncrementRetry is also needed - add it
func (w *DBWriter) IncrementRetry(ctx context.Context, id int64) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE download_queue SET retry_count = retry_count + 1, status = ? WHERE id = ?`,
		models.QueueStatusPending, id,
	)
	if err != nil {
		slog.Error("writer: incrementing retry", "id", id, "error", err)
	}
	return err
}

// SetGalleryThumbnail - needed for processors
func (w *DBWriter) SetGalleryThumbnail(ctx context.Context, galleryID int64, thumbPath string) error {
	_, err := w.db.ExecContext(ctx,
		`UPDATE galleries SET local_thumbnail_path = ? WHERE id = ? AND (local_thumbnail_path IS NULL OR local_thumbnail_path = '')`,
		thumbPath, galleryID,
	)
	return err
}

// ResetActiveToPending - needed for manager startup recovery
func (w *DBWriter) ResetActiveToPending(ctx context.Context) (int64, error) {
	result, err := w.db.ExecContext(ctx,
		`UPDATE download_queue SET status = ? WHERE status = ?`,
		models.QueueStatusPending, models.QueueStatusActive,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// EnqueueItem - needed for queue operations
func (w *DBWriter) EnqueueItem(ctx context.Context, item *models.DownloadQueue) error {
	result, err := w.db.ExecContext(ctx,
		`INSERT INTO download_queue (type, url, target_id, status, retry_count) VALUES (?, ?, ?, ?, ?)`,
		item.Type, item.URL, item.TargetID, item.Status, item.RetryCount,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	item.ID = id
	return nil
}
