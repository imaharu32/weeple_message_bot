package main

import (
	"context"
	"errors"
	"net/url"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type calendarSyncRecord struct {
	GoogleEventID    string    `firestore:"googleEventId"`
	DiscordEventID   string    `firestore:"discordEventId"`
	CalendarID       string    `firestore:"calendarId"`
	Title            string    `firestore:"title"`
	StartAt          time.Time `firestore:"startAt"`
	EndAt            time.Time `firestore:"endAt"`
	Location         string    `firestore:"location"`
	Description      string    `firestore:"description"`
	GoogleUpdatedAt  time.Time `firestore:"googleUpdatedAt"`
	GoogleEtag       string    `firestore:"googleEtag"`
	Announced5d      bool      `firestore:"announced5d"`
	Announced5dAt    time.Time `firestore:"announced5dAt,omitempty"`
	LastSyncedAt     time.Time `firestore:"lastSyncedAt"`
	Status           string    `firestore:"status"`
	GoogleEventLink  string    `firestore:"googleEventLink"`
	DiscordEventLink string    `firestore:"discordEventLink"`
}

type firestoreCalendarStore struct {
	client         *firestore.Client
	collectionName string
}

func newFirestoreCalendarStore(client *firestore.Client, collectionName string) *firestoreCalendarStore {
	return &firestoreCalendarStore{client: client, collectionName: collectionName}
}

func (s *firestoreCalendarStore) Close() error {
	return s.client.Close()
}

func (s *firestoreCalendarStore) doc(calendarID string, googleEventID string) *firestore.DocumentRef {
	return s.client.Collection(s.collectionName).Doc(composeCalendarRecordID(calendarID, googleEventID))
}

func (s *firestoreCalendarStore) Get(ctx context.Context, calendarID string, googleEventID string) (*calendarSyncRecord, error) {
	doc, err := s.doc(calendarID, googleEventID).Get(ctx)
	if err != nil {
		if isFirestoreNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	var rec calendarSyncRecord
	if err := doc.DataTo(&rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *firestoreCalendarStore) Upsert(ctx context.Context, rec calendarSyncRecord) error {
	if rec.CalendarID == "" || rec.GoogleEventID == "" {
		return errors.New("calendarId and googleEventId are required")
	}
	_, err := s.doc(rec.CalendarID, rec.GoogleEventID).Set(ctx, rec)
	return err
}

func (s *firestoreCalendarStore) Delete(ctx context.Context, calendarID string, googleEventID string) error {
	_, err := s.doc(calendarID, googleEventID).Delete(ctx)
	if isFirestoreNotFound(err) {
		return nil
	}
	return err
}

func (s *firestoreCalendarStore) TryMarkAnnounced5d(ctx context.Context, calendarID string, googleEventID string) (bool, error) {
	now := time.Now().UTC()
	ref := s.doc(calendarID, googleEventID)
	shouldAnnounce := false

	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc, err := tx.Get(ref)
		if err != nil {
			if isFirestoreNotFound(err) {
				return nil
			}
			return err
		}

		announced, _ := doc.Data()["announced5d"].(bool)
		if announced {
			return nil
		}

		shouldAnnounce = true
		return tx.Set(ref, map[string]interface{}{
			"announced5d":   true,
			"announced5dAt": now,
			"lastSyncedAt":  now,
		}, firestore.MergeAll)
	})
	if err != nil {
		return false, err
	}

	return shouldAnnounce, nil
}

func (s *firestoreCalendarStore) ListActiveDiscordEventIDs(ctx context.Context, calendarID string) (map[string]string, error) {
	result := make(map[string]string)
	iter := s.client.Collection(s.collectionName).Where("status", "==", "active").Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		recCalendarID, _ := doc.Data()["calendarId"].(string)
		if recCalendarID != calendarID {
			continue
		}

		googleID, _ := doc.Data()["googleEventId"].(string)
		discordID, _ := doc.Data()["discordEventId"].(string)
		if googleID != "" && discordID != "" {
			result[googleID] = discordID
		}
	}

	return result, nil
}

func composeCalendarRecordID(calendarID string, googleEventID string) string {
	return url.QueryEscape(calendarID) + "::" + googleEventID
}

func isFirestoreNotFound(err error) bool {
	if err == nil {
		return false
	}
	return status.Code(err) == codes.NotFound
}
