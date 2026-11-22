# mongo

MongoDB as a small document store. It does not hide behind `*sql.DB`: MongoDB is not SQL, so it has its own
API of five calls on a named collection, with documents as `map[string]any`. Keep the surface this small and a move
to or from the SQL backends stays a weekend, not a rewrite.

Depends on: nothing in this repo (only the official driver). Used by: no template yet.

## Interface

    mongo.Open(uri, name) (*Store, error)     // connects and pings, so a wrong URI fails here
    s.Insert(coll, doc) (id, error)           // "_id" generated as a string when absent
    s.Get(coll, id) (Doc, error)              // ErrNotFound when missing
    s.Find(coll, filter, limit) ([]Doc, error)
    s.Update(coll, id, set) error             // merges fields
    s.Delete(coll, id) error
    s.Unique(coll, field) error               // unique index
    s.Close() error

## Files

    mongo.go        the whole module
    mongo_test.go   skipped unless FWK_TEST_MONGO is set

## Tested

Ran green on MongoDB 8 (podman) before the split:

    podman run -d --rm --name fwk-mongo -p 127.0.0.1:57017:27017 mongo:8
    FWK_TEST_MONGO=mongodb://127.0.0.1:57017 go test ./kit/mongo

## Not done

- Not re-run against a real server after the move into this folder; without `FWK_TEST_MONGO` the tests skip.
- No transactions, no aggregation, no change streams.

## Test

    go test ./kit/mongo
