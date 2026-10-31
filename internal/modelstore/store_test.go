package modelstore

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Meidorislav/c4-forge/internal/db"
	"github.com/Meidorislav/c4-forge/internal/db/dbtest"
	"github.com/Meidorislav/c4-forge/internal/model"
)

const (
	workspaceID model.ID = "019a2f00-0000-7000-8000-000000000001"
	actorID     model.ID = "019a2f00-0000-7000-8000-000000000002"
	systemID    model.ID = "019a2f00-0000-7000-8000-000000000003"
)

type fixtureModel struct {
	Elements      []model.Element      `json:"elements"`
	Relationships []model.Relationship `json:"relationships"`
	Views         []model.View         `json:"views"`
	Placements    []model.Placement    `json:"placements"`
	Shapes        []model.Shape        `json:"shapes"`
}

type fixtureSuite struct {
	WorkspaceID model.ID                `json:"workspaceId"`
	Models      map[string]fixtureModel `json:"models"`
	Cases       []struct {
		Name       string            `json:"name"`
		Initial    string            `json:"initial"`
		Operations []model.Operation `json:"operations"`
		Expected   string            `json:"expected"`
		Expanded   []model.Operation `json:"expanded"`
		Error      *struct {
			Code  model.ErrorCode `json:"code"`
			Field model.FieldName `json:"field"`
		} `json:"error"`
	} `json:"cases"`
}

func newPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testStore(t *testing.T) (*Store, *pgxpool.Pool, string) {
	t.Helper()
	url := dbtest.NewDatabase(t)
	pool := newPool(t, url)
	if err := db.Migrate(t.Context(), pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	return New(pool), pool, url
}

func newID(t *testing.T) model.ID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return model.ID(id.String())
}

func batch(t *testing.T, operations ...model.Operation) Batch {
	t.Helper()
	return Batch{ID: newID(t), ActorID: actorID, Operations: operations}
}

func patch(t *testing.T, id model.ID, fields map[model.FieldName]any) model.Operation {
	t.Helper()
	result := model.Operation{Op: model.OperationSet, Kind: model.EntityElement, ID: id, Fields: make(map[model.FieldName]json.RawMessage)}
	for key, value := range fields {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		result.Fields[key] = data
	}
	return result
}

func createSystem(t *testing.T) model.Operation {
	t.Helper()
	return patch(t, systemID, map[model.FieldName]any{model.FieldType: "softwareSystem", model.FieldNameValue: "System", model.FieldDSLID: "system"})
}

func openEmpty(t *testing.T, store *Store, id model.ID) *Workspace {
	t.Helper()
	if err := store.Create(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	workspace, err := store.Open(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func readSuite(t *testing.T) fixtureSuite {
	t.Helper()
	data, err := os.ReadFile("../../testdata/model-operations/suite.json")
	if err != nil {
		t.Fatal(err)
	}
	var suite fixtureSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		t.Fatal(err)
	}
	if len(suite.Cases) == 0 {
		t.Fatal("empty fixture suite")
	}
	return suite
}

func snapshot(t *testing.T, id model.ID, data fixtureModel) *model.Model {
	t.Helper()
	result, err := model.NewWithPresentation(id, data.Elements, data.Relationships, data.Views, data.Placements, data.Shapes)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func seedOperations(t *testing.T, data fixtureModel) []model.Operation {
	t.Helper()
	result := make([]model.Operation, 0, len(data.Elements)+len(data.Relationships))
	add := func(kind model.EntityKind, id model.ID, entity any) {
		raw, err := json.Marshal(entity)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[model.FieldName]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		delete(fields, model.FieldID)
		result = append(result, model.Operation{Op: model.OperationSet, Kind: kind, ID: id, Fields: fields})
	}
	for _, element := range data.Elements {
		add(model.EntityElement, element.ID, element)
	}
	for _, relationship := range data.Relationships {
		add(model.EntityRelationship, relationship.ID, relationship)
	}
	for _, view := range data.Views {
		add(model.EntityView, view.ID, view)
	}
	for _, placement := range data.Placements {
		add(model.EntityPlacement, placement.ID, placement)
	}
	for _, shape := range data.Shapes {
		add(model.EntityShape, shape.ID, shape)
	}
	return result
}

func assertSnapshot(t *testing.T, workspace *Workspace, want *model.Model, seq int64) {
	t.Helper()
	got, gotSeq := workspace.Snapshot()
	if gotSeq != seq || !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot seq=%d, want %d\nelements=%+v\nrelationships=%+v", gotSeq, seq, got.Elements(), got.Relationships())
	}
}

func TestSharedFixturesPersistAndReload(t *testing.T) {
	store, pool, _ := testStore(t)
	suite := readSuite(t)
	for _, fixture := range suite.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			workspace := openEmpty(t, store, suite.WorkspaceID)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if _, err := pool.Exec(ctx, "DELETE FROM workspaces WHERE id=$1", uid(suite.WorkspaceID)); err != nil {
					t.Error(err)
				}
			})
			initial := snapshot(t, suite.WorkspaceID, suite.Models[fixture.Initial])
			if _, err := workspace.Apply(t.Context(), batch(t, seedOperations(t, suite.Models[fixture.Initial])...)); err != nil {
				t.Fatalf("seed: %v", err)
			}
			request := batch(t, fixture.Operations...)
			committed, err := workspace.Apply(t.Context(), request)
			expected, seq := initial, int64(1)
			if fixture.Error != nil {
				var validation *model.ValidationError
				if !errors.As(err, &validation) || validation.Code != fixture.Error.Code || validation.Field != fixture.Error.Field {
					t.Fatalf("error = %v, want (%s, %s)", err, fixture.Error.Code, fixture.Error.Field)
				}
				if committed.Sequence != 0 || committed.Changes != nil {
					t.Fatal("failed write returned commit")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				expected, seq = snapshot(t, suite.WorkspaceID, suite.Models[fixture.Expected]), 2
				expanded := make([]model.Operation, 0, len(committed.Changes))
				for _, change := range committed.Changes {
					expanded = append(expanded, change.Operation)
				}
				assertJSON(t, expanded, fixture.Expanded)
				var logged []byte
				var loggedActor uuid.UUID
				var timestampPresent bool
				if err := pool.QueryRow(t.Context(), "SELECT ops,actor_id,created_at IS NOT NULL FROM operations WHERE workspace_id=$1 AND batch_id=$2", uid(suite.WorkspaceID), uid(request.ID)).Scan(&logged, &loggedActor, &timestampPresent); err != nil {
					t.Fatal(err)
				}
				var changes []model.Change
				if err := json.Unmarshal(logged, &changes); err != nil {
					t.Fatal(err)
				}
				assertJSON(t, changes, committed.Changes)
				if loggedActor != uid(actorID) || !timestampPresent {
					t.Fatal("history lost actor or timestamp")
				}
			}
			assertSnapshot(t, workspace, expected, seq)
			reloaded, err := New(pool).Open(t.Context(), suite.WorkspaceID)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			assertSnapshot(t, reloaded, expected, seq)
			var count int64
			if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM operations WHERE workspace_id=$1", uid(suite.WorkspaceID)).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != seq {
				t.Fatalf("history has %d batches, want %d", count, seq)
			}
		})
	}
}

func assertJSON(t *testing.T, got, want any) {
	t.Helper()
	normalized := func(value any) any {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var result any
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if !reflect.DeepEqual(normalized(got), normalized(want)) {
		t.Fatalf("JSON mismatch: got %+v, want %+v", got, want)
	}
}

func TestRestartLoadsCommittedModel(t *testing.T) {
	store, pool, url := testStore(t)
	workspace := openEmpty(t, store, workspaceID)
	if _, err := workspace.Apply(t.Context(), batch(t, createSystem(t))); err != nil {
		t.Fatal(err)
	}
	original, seq := workspace.Snapshot()
	pool.Close()
	// Recreate both dependencies, retaining no working set or connections.
	restarted := New(newPool(t, url))
	loaded, err := restarted.Open(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, loaded, original, seq)
}

func TestCommitFailurePreservesMemoryAndDatabase(t *testing.T) {
	store, pool, _ := testStore(t)
	workspace := openEmpty(t, store, workspaceID)
	original, _ := workspace.Snapshot()
	// The deferred trigger fires only when PostgreSQL processes COMMIT, after
	// the entity rows, log and sequence have all been written in the transaction.
	_, err := pool.Exec(t.Context(), `
 CREATE FUNCTION reject_commit() RETURNS trigger LANGUAGE plpgsql AS $$
 BEGIN RAISE EXCEPTION 'commit failure probe'; END; $$;
 CREATE CONSTRAINT TRIGGER reject_commit AFTER INSERT ON operations
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_commit();`)
	if err != nil {
		t.Fatal(err)
	}
	request := batch(t, createSystem(t))
	result, err := workspace.Apply(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), "commit batch") || !strings.Contains(err.Error(), "commit failure probe") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	if result.Sequence != 0 || result.Changes != nil {
		t.Fatal("failed commit returned success data")
	}
	assertSnapshot(t, workspace, original, 0)
	loaded, err := store.Open(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, loaded, original, 0)
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM operations").Scan(&count); err != nil || count != 0 {
		t.Fatalf("history rollback: count=%d, err=%v", count, err)
	}
	if _, err := pool.Exec(t.Context(), "DROP TRIGGER reject_commit ON operations"); err != nil {
		t.Fatal(err)
	}
	if result, err := workspace.Apply(t.Context(), request); err != nil || result.Sequence != 1 {
		t.Fatalf("retry after rollback: %+v, %v", result, err)
	}
}

func TestConcurrentHandlesRejectStaleWriter(t *testing.T) {
	store, _, _ := testStore(t)
	first := openEmpty(t, store, workspaceID)
	second, err := store.Open(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	requests := []Batch{batch(t, createSystem(t)), batch(t, patch(t, systemID, map[model.FieldName]any{model.FieldType: "softwareSystem", model.FieldNameValue: "Other writer", model.FieldDSLID: "other"}))}
	for i, workspace := range []*Workspace{first, second} {
		wg.Go(func() { _, err := workspace.Apply(t.Context(), requests[i]); errs <- err })
	}
	wg.Wait()
	close(errs)
	successes, stale := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrStaleSnapshot):
			stale++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("success=%d stale=%d", successes, stale)
	}
	loaded, err := store.Open(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	current, seq := loaded.Snapshot()
	if seq != 1 || len(current.Elements()) != 1 {
		t.Fatalf("unexpected persisted sequence %d", seq)
	}
	// A fresh handle can retry as a new action against current state.
	if _, err := loaded.Apply(t.Context(), batch(t, patch(t, systemID, map[model.FieldName]any{model.FieldDescription: "next"}))); err != nil {
		t.Fatal(err)
	}
}

func TestSameHandleSerializesConcurrentBatches(t *testing.T) {
	store, pool, _ := testStore(t)
	workspace := openEmpty(t, store, workspaceID)
	if _, err := workspace.Apply(t.Context(), batch(t, createSystem(t))); err != nil {
		t.Fatal(err)
	}
	requests := []Batch{batch(t, patch(t, systemID, map[model.FieldName]any{model.FieldDescription: "Description"})), batch(t, patch(t, systemID, map[model.FieldName]any{model.FieldExternal: true}))}
	var wg sync.WaitGroup
	errs := make(chan error, len(requests))
	for _, request := range requests {
		wg.Go(func() { _, err := workspace.Apply(t.Context(), request); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	current, seq := workspace.Snapshot()
	element, _ := current.Element(systemID)
	if seq != 3 || element.Description != "Description" || !element.External {
		t.Fatal("concurrent fields or sequence lost")
	}
	loaded, err := New(pool).Open(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, loaded, current, 3)
}

func TestDuplicateBatchRollsBack(t *testing.T) {
	store, pool, _ := testStore(t)
	workspace := openEmpty(t, store, workspaceID)
	request := batch(t, createSystem(t))
	if _, err := workspace.Apply(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	current, _ := workspace.Snapshot()
	if _, err := workspace.Apply(t.Context(), request); err == nil {
		t.Fatal("duplicate batch accepted")
	}
	assertSnapshot(t, workspace, current, 1)
	loaded, err := New(pool).Open(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, loaded, current, 1)
}

func TestWorkspaceErrors(t *testing.T) {
	store, _, _ := testStore(t)
	if _, err := store.Open(t.Context(), workspaceID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing workspace: %v", err)
	}
	if err := store.Create(t.Context(), "invalid"); err == nil {
		t.Fatal("invalid workspace ID accepted")
	}
	if _, err := store.Open(t.Context(), "invalid"); err == nil {
		t.Fatal("invalid workspace ID accepted")
	}
	workspace := openEmpty(t, store, workspaceID)
	for _, request := range []Batch{{ID: "invalid", ActorID: actorID}, {ID: newID(t), ActorID: "invalid"}} {
		if _, err := workspace.Apply(t.Context(), request); err == nil {
			t.Fatal("invalid batch identity accepted")
		}
	}
}

func TestDatabaseCascadesMatchEngine(t *testing.T) {
	store, pool, _ := testStore(t)
	suite := readSuite(t)
	for _, fixture := range suite.Cases {
		// Single delete cases cover relationship, leaf, subtree and sibling roots.
		if fixture.Error != nil || len(fixture.Operations) != 1 || fixture.Operations[0].Op != model.OperationDelete {
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) {
			workspace := openEmpty(t, store, workspaceID)
			if _, err := workspace.Apply(t.Context(), batch(t, seedOperations(t, suite.Models[fixture.Initial])...)); err != nil {
				t.Fatal(err)
			}
			operation := fixture.Operations[0]
			query := "DELETE FROM elements WHERE workspace_id=$1 AND id=$2"
			switch operation.Kind {
			case model.EntityRelationship:
				query = "DELETE FROM relationships WHERE workspace_id=$1 AND id=$2"
			case model.EntityView:
				query = "DELETE FROM views WHERE workspace_id=$1 AND id=$2"
			case model.EntityPlacement:
				query = "DELETE FROM placements WHERE workspace_id=$1 AND id=$2"
			case model.EntityShape:
				query = "DELETE FROM shapes WHERE workspace_id=$1 AND id=$2"
			}
			if _, err := pool.Exec(t.Context(), query, uid(workspaceID), uid(operation.ID)); err != nil {
				t.Fatal(err)
			}
			// Direct SQL deliberately bypasses the operation engine and its expanded
			// deletes: this proves the foreign-key cascades independently.
			loaded, err := store.Open(t.Context(), workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			assertSnapshot(t, loaded, snapshot(t, workspaceID, suite.Models[fixture.Expected]), 1)
			if _, err := pool.Exec(t.Context(), "DELETE FROM workspaces WHERE id=$1", uid(workspaceID)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDatabaseConstraintsAndWorkspaceIsolation(t *testing.T) {
	store, pool, _ := testStore(t)
	first := openEmpty(t, store, workspaceID)
	otherID := newID(t)
	second := openEmpty(t, store, otherID)
	for _, workspace := range []*Workspace{first, second} {
		if _, err := workspace.Apply(t.Context(), batch(t, createSystem(t))); err != nil {
			t.Fatal(err)
		}
	}
	// IDs and DSL identifiers can repeat in distinct workspaces; writes and
	// cascades must still be restricted to the owning workspace.
	if _, err := first.Apply(t.Context(), batch(t, model.Operation{Op: model.OperationDelete, Kind: model.EntityElement, ID: systemID})); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Open(t.Context(), otherID)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := second.Snapshot()
	assertSnapshot(t, reloaded, want, 1)
	cases := []struct {
		name, query string
		args        []any
	}{
		{"unknown type", "INSERT INTO elements(workspace_id,id,type,name,dsl_id) VALUES($1,$2,'database','Bad','bad')", []any{uid(otherID), uid(newID(t))}},
		{"cross workspace parent", "INSERT INTO elements(workspace_id,id,type,parent_id,name,dsl_id) VALUES($1,$2,'container',$3,'Bad','bad')", []any{uid(workspaceID), uid(newID(t)), uid(systemID)}},
		{"cross workspace endpoint", "INSERT INTO relationships(workspace_id,id,source_id,destination_id) VALUES($1,$2,$3,$3)", []any{uid(workspaceID), uid(newID(t)), uid(systemID)}},
		{"duplicate DSL ID", "INSERT INTO elements(workspace_id,id,type,name,dsl_id) VALUES($1,$2,'softwareSystem','Duplicate','system')", []any{uid(otherID), uid(newID(t))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(t.Context(), tc.query, tc.args...); err == nil {
				t.Fatal("database accepted invalid row")
			}
		})
	}
	if _, err := pool.Exec(t.Context(), "DELETE FROM workspaces WHERE id=$1", uid(otherID)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM elements WHERE workspace_id=$1)+(SELECT count(*) FROM relationships WHERE workspace_id=$1)+(SELECT count(*) FROM operations WHERE workspace_id=$1)", uid(otherID)).Scan(&count); err != nil || count != 0 {
		t.Fatalf("workspace cascade left %d rows: %v", count, err)
	}
}

func TestRowWriteFailureRollsBackHistoryAndSequence(t *testing.T) {
	store, pool, _ := testStore(t)
	workspace := openEmpty(t, store, workspaceID)
	initial, _ := workspace.Snapshot()
	if _, err := pool.Exec(t.Context(), `ALTER TABLE elements ADD CONSTRAINT reject_probe CHECK (name <> 'System')`); err != nil {
		t.Fatal(err)
	}
	_, err := workspace.Apply(t.Context(), batch(t, createSystem(t)))
	if err == nil || !strings.Contains(err.Error(), "write model") {
		t.Fatalf("expected row write failure: %v", err)
	}
	assertSnapshot(t, workspace, initial, 0)
	loaded, err := store.Open(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, loaded, initial, 0)
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM operations").Scan(&count); err != nil || count != 0 {
		t.Fatalf("history rollback: %d, %v", count, err)
	}
}

func TestPresentationDatabaseConstraints(t *testing.T) {
	store, pool, _ := testStore(t)
	suite := readSuite(t)
	workspace := openEmpty(t, store, workspaceID)
	if _, err := workspace.Apply(t.Context(), batch(t, seedOperations(t, suite.Models["scene"])...)); err != nil {
		t.Fatal(err)
	}
	other := newID(t)
	openEmpty(t, store, other)
	viewID := model.ID("019a2f00-0000-7000-8000-000000000020")
	placementElement := model.ID("019a2f00-0000-7000-8000-000000000003")
	tests := []struct {
		name, query string
		args        []any
	}{
		{"duplicate view key", `INSERT INTO views(workspace_id,id,type,key) VALUES ($1,$2,'systemLandscape','landscape')`, []any{uid(workspaceID), uid(newID(t))}},
		{"duplicate placement", `INSERT INTO placements(workspace_id,id,view_id,element_id,x,y) VALUES ($1,$2,$3,$4,0,0)`, []any{uid(workspaceID), uid(newID(t)), uid(viewID), uid(placementElement)}},
		{"cross workspace view", `INSERT INTO shapes(workspace_id,id,view_id,type,attrs) VALUES ($1,$2,$3,'text','{"geometry":{}}')`, []any{uid(other), uid(newID(t)), uid(viewID)}},
		{"missing geometry", `INSERT INTO shapes(workspace_id,id,view_id,type,attrs) VALUES ($1,$2,$3,'text','{}')`, []any{uid(workspaceID), uid(newID(t)), uid(viewID)}},
		{"invalid geometry", `INSERT INTO shapes(workspace_id,id,view_id,type,attrs) VALUES ($1,$2,$3,'text','{"geometry":[] }')`, []any{uid(workspaceID), uid(newID(t)), uid(viewID)}},
		{"nonfinite coordinate", `INSERT INTO placements(workspace_id,id,view_id,element_id,x,y) VALUES ($1,$2,$3,$4,'NaN',0)`, []any{uid(workspaceID), uid(newID(t)), uid(viewID), uid(placementElement)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(t.Context(), tc.query, tc.args...); err == nil {
				t.Fatal("database accepted invalid presentation row")
			}
		})
	}
}

func TestShapeGeometryPreservesDecimalPrecision(t *testing.T) {
	store, _, _ := testStore(t)
	workspace := openEmpty(t, store, workspaceID)
	viewID := newID(t)
	shapeID := newID(t)
	ops := []model.Operation{
		{Op: model.OperationSet, Kind: model.EntityView, ID: viewID, Fields: map[model.FieldName]json.RawMessage{model.FieldType: json.RawMessage(`"systemLandscape"`), model.FieldKey: json.RawMessage(`"landscape"`)}},
		{Op: model.OperationSet, Kind: model.EntityShape, ID: shapeID, Fields: map[model.FieldName]json.RawMessage{model.FieldViewID: json.RawMessage(`"` + string(viewID) + `"`), model.FieldType: json.RawMessage(`"rectangle"`), model.FieldGeometry: json.RawMessage(`{"large":9007199254740993,"small":1e-7}`)}},
	}
	if _, err := workspace.Apply(t.Context(), batch(t, ops...)); err != nil {
		t.Fatal(err)
	}
	before, _ := workspace.Snapshot()
	shape, _ := before.Shape(shapeID)
	if !strings.Contains(string(shape.Geometry), "9007199254740993") {
		t.Fatalf("geometry lost precision: %s", shape.Geometry)
	}
	reopened, err := store.Open(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, reopened, before, 1)
}
