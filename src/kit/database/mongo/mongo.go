// mongo is the MongoDB module: ONE file, standalone (imports nothing else from this repo).
// MongoDB is a document store, not SQL, so it does not hide behind *sql.DB; it exposes a small
// document API instead: Insert / Get / Find / Update / Delete on a named collection, documents
// as map[string]any. Keep the surface this small and a move to/from the SQL backends stays a
// weekend, not a rewrite.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mdb "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Doc is one document. "_id" is the primary key; Insert generates a string id when absent.
type Doc = map[string]any

var ErrNotFound = errors.New("mongo: not found")

type Store struct {
	client *mdb.Client
	db     *mdb.Database
}

// Open connects to uri ("mongodb://host:27017") and uses database name. It pings, so a wrong
// host fails here and not at the first request.
func Open(uri, name string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := mdb.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(5 * time.Second))
	if err != nil {
		return nil, fmt.Errorf("mongo: connect: %w", err)
	}
	if err := c.Ping(ctx, nil); err != nil {
		c.Disconnect(ctx)
		return nil, fmt.Errorf("mongo: ping: %w", err)
	}
	return &Store{client: c, db: c.Database(name)}, nil
}

func (s *Store) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.client.Disconnect(ctx)
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

// Insert stores d and returns its id. A duplicate "_id" is an error, never an overwrite.
func (s *Store) Insert(coll string, d Doc) (string, error) {
	c, cancel := ctx()
	defer cancel()
	if d["_id"] == nil {
		d["_id"] = bson.NewObjectID().Hex()
	}
	if _, err := s.db.Collection(coll).InsertOne(c, d); err != nil {
		return "", fmt.Errorf("mongo: insert %s: %w", coll, err)
	}
	return fmt.Sprint(d["_id"]), nil
}

// Get returns the document with that id, or ErrNotFound.
func (s *Store) Get(coll, id string) (Doc, error) {
	c, cancel := ctx()
	defer cancel()
	var d Doc
	err := s.db.Collection(coll).FindOne(c, bson.M{"_id": id}).Decode(&d)
	if errors.Is(err, mdb.ErrNoDocuments) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mongo: get %s: %w", coll, err)
	}
	return d, nil
}

// Find returns up to limit documents matching the equality filter (nil = all), newest-id last.
func (s *Store) Find(coll string, filter Doc, limit int64) ([]Doc, error) {
	c, cancel := ctx()
	defer cancel()
	if filter == nil {
		filter = Doc{}
	}
	cur, err := s.db.Collection(coll).Find(c, filter, options.Find().SetLimit(limit).SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("mongo: find %s: %w", coll, err)
	}
	var out []Doc
	if err := cur.All(c, &out); err != nil {
		return nil, fmt.Errorf("mongo: find %s: %w", coll, err)
	}
	return out, nil
}

// Update sets the given fields on the document (others untouched). ErrNotFound if absent.
func (s *Store) Update(coll, id string, set Doc) error {
	c, cancel := ctx()
	defer cancel()
	r, err := s.db.Collection(coll).UpdateOne(c, bson.M{"_id": id}, bson.M{"$set": set})
	if err != nil {
		return fmt.Errorf("mongo: update %s: %w", coll, err)
	}
	if r.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the document; ErrNotFound if absent.
func (s *Store) Delete(coll, id string) error {
	c, cancel := ctx()
	defer cancel()
	r, err := s.db.Collection(coll).DeleteOne(c, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("mongo: delete %s: %w", coll, err)
	}
	if r.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Unique makes field unique in coll (idempotent) - the document-store counterpart of UNIQUE.
func (s *Store) Unique(coll, field string) error {
	c, cancel := ctx()
	defer cancel()
	_, err := s.db.Collection(coll).Indexes().CreateOne(c, mdb.IndexModel{
		Keys: bson.D{{Key: field, Value: 1}}, Options: options.Index().SetUnique(true),
	})
	return err
}
