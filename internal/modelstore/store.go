// Package modelstore persists workspace models and their operation history.
package modelstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Meidorislav/c4-forge/internal/db/sqlc"
	"github.com/Meidorislav/c4-forge/internal/model"
)

// ErrStaleSnapshot means another handle committed after this one was loaded.
// Reopen the workspace before retrying; never overwrite that writer's changes.
var ErrStaleSnapshot = errors.New("workspace snapshot is stale; reopen before writing")

// Store uses an already connected and migrated PostgreSQL pool.
// It does not own the pool or manage workspace membership (roadmap M3/M4).
type Store struct{ pool *pgxpool.Pool }

// New creates a store. The caller retains responsibility for closing the pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Create inserts an empty workspace storage root. User-facing workspace
// metadata, membership and authorization belong to roadmap 4.1.
func (s *Store) Create(ctx context.Context, id model.ID) error {
	if err := id.Validate(); err != nil {
		return fmt.Errorf("workspace ID: %w", err)
	}
	if err := sqlc.New(s.pool).CreateWorkspace(ctx, uid(id)); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	return nil
}

// Workspace owns one loaded working set. Snapshot and Apply are safe to call
// concurrently. Separate handles are guarded by PostgreSQL's workspace row lock
// and a sequence comparison, even if they belong to different processes.
type Workspace struct {
	mu       sync.Mutex
	store    *Store
	snapshot *model.Model
	seq      int64
}

// Open loads the sequence and entities from one consistent database snapshot.
// A missing workspace returns an error wrapping pgx.ErrNoRows.
func (s *Store) Open(ctx context.Context, id model.ID) (*Workspace, error) {
	if err := id.Validate(); err != nil {
		return nil, fmt.Errorf("workspace ID: %w", err)
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin workspace load: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := sqlc.New(tx)
	seq, err := q.WorkspaceSequence(ctx, uid(id))
	if err != nil {
		return nil, fmt.Errorf("load workspace sequence: %w", err)
	}
	elements, err := q.ListElements(ctx, uid(id))
	if err != nil {
		return nil, fmt.Errorf("load elements: %w", err)
	}
	relationships, err := q.ListRelationships(ctx, uid(id))
	if err != nil {
		return nil, fmt.Errorf("load relationships: %w", err)
	}
	views, err := q.ListViews(ctx, uid(id))
	if err != nil {
		return nil, fmt.Errorf("load views: %w", err)
	}
	placements, err := q.ListPlacements(ctx, uid(id))
	if err != nil {
		return nil, fmt.Errorf("load placements: %w", err)
	}
	shapes, err := q.ListShapes(ctx, uid(id))
	if err != nil {
		return nil, fmt.Errorf("load shapes: %w", err)
	}
	snapshot, err := decodeModel(id, elements, relationships, views, placements, shapes)
	if err != nil {
		return nil, fmt.Errorf("decode model: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit workspace load: %w", err)
	}
	return &Workspace{store: s, snapshot: snapshot, seq: seq}, nil
}

// Snapshot returns an immutable model and its committed workspace sequence.
func (w *Workspace) Snapshot() (*model.Model, int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.snapshot, w.seq
}

// Batch carries the caller-supplied identity and actor for the operation log.
// ActorID must come from the authenticated server context once M3/M4 is wired.
// The unique database batch key reserves retry identity; retry handling and
// transport acknowledgements are part of roadmap 4.3.
type Batch struct {
	ID         model.ID
	ActorID    model.ID
	Operations []model.Operation
}

// Commit is returned only after PostgreSQL has committed the complete batch.
type Commit struct {
	Sequence int64
	Changes  []model.Change
}

// Apply validates, logs and writes one batch in a single transaction (ADR-0003).
// Only a successful commit publishes the candidate model to this working set.
// On error, the old in-memory snapshot and sequence are retained. A connection
// failure during commit can have an unknown DB outcome; reopen before retrying.
func (w *Workspace) Apply(ctx context.Context, batch Batch) (Commit, error) {
	if err := batch.ID.Validate(); err != nil {
		return Commit{}, fmt.Errorf("batch ID: %w", err)
	}
	if err := batch.ActorID.Validate(); err != nil {
		return Commit{}, fmt.Errorf("actor ID: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	tx, err := w.store.pool.Begin(ctx)
	if err != nil {
		return Commit{}, fmt.Errorf("begin batch: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := sqlc.New(tx)
	workspaceID := uid(w.snapshot.WorkspaceID())
	seq, err := q.LockWorkspace(ctx, workspaceID)
	if err != nil {
		return Commit{}, fmt.Errorf("lock workspace: %w", err)
	}
	if seq != w.seq {
		return Commit{}, ErrStaleSnapshot
	}
	candidate, changes, err := w.snapshot.ApplyWithChanges(batch.Operations)
	if err != nil {
		return Commit{}, err
	}
	seq, err = q.AdvanceSequence(ctx, workspaceID)
	if err != nil {
		return Commit{}, fmt.Errorf("advance sequence: %w", err)
	}
	data, err := json.Marshal(changes)
	if err != nil {
		return Commit{}, fmt.Errorf("encode operation history: %w", err)
	}
	if err := q.InsertOperation(ctx, sqlc.InsertOperationParams{
		WorkspaceID: workspaceID, Seq: seq,
		BatchID: uid(batch.ID), ActorID: uid(batch.ActorID), Ops: data,
	}); err != nil {
		return Commit{}, fmt.Errorf("write operation history: %w", err)
	}
	if err := writeModel(ctx, q, w.snapshot, candidate); err != nil {
		return Commit{}, fmt.Errorf("write model: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Commit{}, fmt.Errorf("commit batch: %w", err)
	}
	w.snapshot, w.seq = candidate, seq
	return Commit{Sequence: seq, Changes: changes}, nil
}

func writeModel(ctx context.Context, q *sqlc.Queries, previous, next *model.Model) error {
	workspaceID := uid(next.WorkspaceID())
	// Write the final candidate, not transient operations. Deferred constraints
	// permit forward references and DSL identifier swaps. Retarget surviving
	// references before deleting roots, so ON DELETE CASCADE cannot remove them.
	for _, element := range next.Elements() {
		old, exists := previous.Element(element.ID)
		if exists && reflect.DeepEqual(old, element) {
			continue
		}
		attrs, err := entityAttrs(model.EntityElement, element)
		if err != nil {
			return err
		}
		parentID := pgtype.UUID{}
		if element.ParentID != "" {
			parentID = pgtype.UUID{Bytes: uid(element.ParentID), Valid: true}
		}
		if err := q.PutElement(ctx, sqlc.PutElementParams{
			WorkspaceID: workspaceID, ID: uid(element.ID),
			Type: string(element.Type), ParentID: parentID,
			Name: element.Name, DslID: element.DSLID, Attrs: attrs,
		}); err != nil {
			return err
		}
	}
	for _, relationship := range next.Relationships() {
		old, exists := previous.Relationship(relationship.ID)
		if exists && reflect.DeepEqual(old, relationship) {
			continue
		}
		attrs, err := entityAttrs(model.EntityRelationship, relationship)
		if err != nil {
			return err
		}
		if err := q.PutRelationship(ctx, sqlc.PutRelationshipParams{
			WorkspaceID: workspaceID, ID: uid(relationship.ID),
			SourceID: uid(relationship.SourceID), DestinationID: uid(relationship.DestinationID),
			Attrs: attrs,
		}); err != nil {
			return err
		}
	}
	for _, view := range next.Views() {
		old, exists := previous.View(view.ID)
		if exists && reflect.DeepEqual(old, view) {
			continue
		}
		attrs, err := entityAttrs(model.EntityView, view)
		if err != nil {
			return err
		}
		scope := pgtype.UUID{}
		if view.ScopeID != "" {
			scope = pgtype.UUID{Bytes: uid(view.ScopeID), Valid: true}
		}
		if err := q.PutView(ctx, sqlc.PutViewParams{WorkspaceID: workspaceID, ID: uid(view.ID), Type: view.Type, Key: view.Key, ScopeID: scope, Attrs: attrs}); err != nil {
			return err
		}
	}
	for _, placement := range next.Placements() {
		old, exists := previous.Placement(placement.ID)
		if exists && reflect.DeepEqual(old, placement) {
			continue
		}
		width, height := pgtype.Float8{}, pgtype.Float8{}
		if placement.Width != nil {
			width = pgtype.Float8{Float64: *placement.Width, Valid: true}
		}
		if placement.Height != nil {
			height = pgtype.Float8{Float64: *placement.Height, Valid: true}
		}
		if err := q.PutPlacement(ctx, sqlc.PutPlacementParams{WorkspaceID: workspaceID, ID: uid(placement.ID), ViewID: uid(placement.ViewID), ElementID: uid(placement.ElementID), X: *placement.X, Y: *placement.Y, Width: width, Height: height}); err != nil {
			return err
		}
	}
	for _, shape := range next.Shapes() {
		old, exists := previous.Shape(shape.ID)
		if exists && reflect.DeepEqual(old, shape) {
			continue
		}
		attrs, err := entityAttrs(model.EntityShape, shape)
		if err != nil {
			return err
		}
		if err := q.PutShape(ctx, sqlc.PutShapeParams{WorkspaceID: workspaceID, ID: uid(shape.ID), ViewID: uid(shape.ViewID), Type: shape.Type, Attrs: attrs}); err != nil {
			return err
		}
	}
	for _, shape := range previous.Shapes() {
		if _, exists := next.Shape(shape.ID); !exists {
			if err := q.DeleteShape(ctx, sqlc.DeleteShapeParams{WorkspaceID: workspaceID, ID: uid(shape.ID)}); err != nil {
				return err
			}
		}
	}
	for _, placement := range previous.Placements() {
		if _, exists := next.Placement(placement.ID); !exists {
			if err := q.DeletePlacement(ctx, sqlc.DeletePlacementParams{WorkspaceID: workspaceID, ID: uid(placement.ID)}); err != nil {
				return err
			}
		}
	}
	for _, view := range previous.Views() {
		if _, exists := next.View(view.ID); !exists {
			if err := q.DeleteView(ctx, sqlc.DeleteViewParams{WorkspaceID: workspaceID, ID: uid(view.ID)}); err != nil {
				return err
			}
		}
	}

	for _, relationship := range previous.Relationships() {
		if _, exists := next.Relationship(relationship.ID); !exists {
			if err := q.DeleteRelationship(ctx, sqlc.DeleteRelationshipParams{WorkspaceID: workspaceID, ID: uid(relationship.ID)}); err != nil {
				return err
			}
		}
	}
	for _, element := range previous.Elements() {
		if _, exists := next.Element(element.ID); !exists {
			if err := q.DeleteElement(ctx, sqlc.DeleteElementParams{WorkspaceID: workspaceID, ID: uid(element.ID)}); err != nil {
				return err
			}
		}
	}
	return nil
}

func entityAttrs(kind model.EntityKind, entity any) ([]byte, error) {
	data, err := json.Marshal(entity)
	if err != nil {
		return nil, err
	}
	var values map[model.FieldName]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	attrs := make(map[model.FieldName]json.RawMessage)
	for _, field := range model.DefaultRegistry().Fields(kind) {
		if field.Storage != model.StorageAttrs {
			continue
		}
		if value, ok := values[field.Name]; ok {
			attrs[field.Name] = value
		}
	}
	return json.Marshal(attrs)
}

func decodeModel(id model.ID, elementRows []sqlc.Element, relationshipRows []sqlc.Relationship, viewRows []sqlc.View, placementRows []sqlc.Placement, shapeRows []sqlc.Shape) (*model.Model, error) {
	elements := make([]model.Element, 0, len(elementRows))
	for _, row := range elementRows {
		var element model.Element
		if err := json.Unmarshal(row.Attrs, &element); err != nil {
			return nil, err
		}
		element.ID, element.Type = model.ID(row.ID.String()), model.ElementType(row.Type)
		element.ParentID = ""
		if row.ParentID.Valid {
			element.ParentID = model.ID(uuid.UUID(row.ParentID.Bytes).String())
		}
		element.Name, element.DSLID = row.Name, row.DslID
		elements = append(elements, element)
	}
	relationships := make([]model.Relationship, 0, len(relationshipRows))
	for _, row := range relationshipRows {
		var relationship model.Relationship
		if err := json.Unmarshal(row.Attrs, &relationship); err != nil {
			return nil, err
		}
		relationship.ID = model.ID(row.ID.String())
		relationship.SourceID, relationship.DestinationID = model.ID(row.SourceID.String()), model.ID(row.DestinationID.String())
		relationships = append(relationships, relationship)
	}
	views := make([]model.View, 0, len(viewRows))
	for _, row := range viewRows {
		var view model.View
		if err := json.Unmarshal(row.Attrs, &view); err != nil {
			return nil, err
		}
		view.ID = model.ID(row.ID.String())
		view.Type = row.Type
		view.Key = row.Key
		if row.ScopeID.Valid {
			view.ScopeID = model.ID(uuid.UUID(row.ScopeID.Bytes).String())
		}
		views = append(views, view)
	}
	placements := make([]model.Placement, 0, len(placementRows))
	for _, row := range placementRows {
		p := model.Placement{ID: model.ID(row.ID.String()), ViewID: model.ID(row.ViewID.String()), ElementID: model.ID(row.ElementID.String()), X: &row.X, Y: &row.Y}
		if row.Width.Valid {
			p.Width = &row.Width.Float64
		}
		if row.Height.Valid {
			p.Height = &row.Height.Float64
		}
		placements = append(placements, p)
	}
	shapes := make([]model.Shape, 0, len(shapeRows))
	for _, row := range shapeRows {
		var shape model.Shape
		if err := json.Unmarshal(row.Attrs, &shape); err != nil {
			return nil, err
		}
		shape.ID = model.ID(row.ID.String())
		shape.ViewID = model.ID(row.ViewID.String())
		shape.Type = row.Type
		shapes = append(shapes, shape)
	}
	return model.NewWithPresentation(id, elements, relationships, views, placements, shapes)
}

// Callers validate IDs before conversion, including model.New on load.
func uid(id model.ID) uuid.UUID { return uuid.MustParse(string(id)) }
