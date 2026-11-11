package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
)

const legacyNodeID = "yandex-backend-nodejs"
const nodeOutcomeQuote = "Научитесь разрабатывать серверные приложения на Node.js и Nest.js"

func legacyNodeCourse(c catalog.Course) bool {
	return c.ID == legacyNodeID && c.Source == practicumOrigin+"/backend-nodejs" && c.Title == "Бэкенд на Node.js для фронтенд-разработчиков" && c.Language == "javascript" && c.Status == "published" && !c.Demo
}

// RepairNodeDirection corrects one known legacy title-classification error using
// today's independently bound official outcome. It cannot bulk-reclassify other
// records or override an existing evidence-backed direction. No model is called.
func RepairNodeDirection(ctx context.Context, db *store.DB, client *http.Client, w io.Writer) error {
	courses, err := db.Load(ctx)
	if err != nil {
		return err
	}
	var before *catalog.Course
	for i := range courses {
		if courses[i].ID == legacyNodeID {
			before = &courses[i]
		}
	}
	if before == nil || !legacyNodeCourse(*before) {
		return ErrSource
	}
	if before.Direction == "backend" && before.Profile != nil && before.Profile.Direction != nil && before.Profile.Direction.Value == "backend" {
		return json.NewEncoder(w).Encode(map[string]any{"course_id": legacyNodeID, "status": "already_correct", "model_requests": 0, "writes": 0})
	}
	if before.Direction != "frontend" || before.Profile != nil && before.Profile.Direction != nil {
		return ErrSource
	}
	if client == nil {
		client = NewClient()
		defer client.CloseIdleConnections()
	}
	candidate, documents, err := aiFetchDocuments(ctx, db, client, *before)
	if err != nil {
		return err
	}
	var document *AIDocument
	for i := range documents {
		if documents[i].Scope == "course" && strings.Contains(aiNormalize(documents[i].Text), nodeOutcomeQuote) {
			document = &documents[i]
		}
	}
	if document == nil || document.BindingDigest == "" {
		return ErrSource
	}
	proof := catalog.LearningEvidence{Source: document.Source, Locator: "source-repair:declared-outcome;quote_sha256=" + fingerprint([]byte(nodeOutcomeQuote)), Digest: document.Digest, CheckedAt: time.Now().UTC()}
	if !proof.Valid() {
		return ErrSource
	}
	n, err := db.UpdateVerified(ctx, []string{"practicum.yandex.ru"}, "verified-node-direction-repair", func(ctx context.Context, tx pgx.Tx, current []catalog.Course) ([]catalog.Course, error) {
		// Publication may have changed this identity while HTTP was in progress.
		valid, err := aiPracticumEventIdentityValid(ctx, tx, AIEvent{CourseID: before.ID, Candidate: candidate, Documents: documents})
		if err != nil {
			return nil, err
		}
		if !valid {
			return nil, ErrSource
		}
		for _, c := range current {
			if c.ID != legacyNodeID {
				continue
			}
			if !legacyNodeCourse(c) || aiIdentityDigest(c) != aiIdentityDigest(*before) || c.Direction != "frontend" || c.Profile != nil && c.Profile.Direction != nil {
				return nil, ErrSource
			}
			p := catalog.CourseProfile{}
			if c.Profile != nil {
				p = *c.Profile
			}
			p.Direction = &catalog.ProfileValue{Value: "backend", Evidence: proof}
			c.Direction, c.Profile = "backend", &p
			c.ProfileOwnership &^= profileDirectionBit | (profileDirectionBit << aiOwnershipShift)
			if !p.Valid() {
				return nil, ErrSource
			}
			return []catalog.Course{c}, nil
		}
		return nil, ErrSource
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(map[string]any{"course_id": legacyNodeID, "status": "corrected", "direction": "backend", "model_requests": 0, "writes": n, "quote": nodeOutcomeQuote, "evidence": proof})
}
