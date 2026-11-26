package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"cloud.google.com/go/firestore"
	firebaseauth "firebase.google.com/go/v4/auth"
	"firebase.google.com/go/v4/db"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxBodyBytes = 1 << 20

type tokenVerifier interface {
	VerifyIDToken(context.Context, string) (*firebaseauth.Token, error)
}

type api struct {
	cfg        config
	authClient tokenVerifier
	rtdb       *db.Client
	firestore  *firestore.Client
	statusMu   sync.Mutex
	status     firebaseStatus
	checkedAt  time.Time
}

func newAPI(cfg config, authClient tokenVerifier, rtdb *db.Client, firestoreClient *firestore.Client) http.Handler {
	a := &api{cfg: cfg, authClient: authClient, rtdb: rtdb, firestore: firestoreClient}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("GET /{$}", a.statusPage)
	mux.HandleFunc("GET /status", a.firebaseStatus)
	mux.HandleFunc("/api/rtdb", a.handleRTDB)
	mux.HandleFunc("/api/rtdb/", a.handleRTDB)
	mux.HandleFunc("/api/firestore/", a.handleFirestore)
	return a.requireToken(mux)
}

func (a *api) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/healthz" || r.URL.Path == "/status" {
			next.ServeHTTP(w, r)
			return
		}
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		token, err := a.authClient.VerifyIDToken(r.Context(), strings.TrimPrefix(header, prefix))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if _, ok := a.cfg.AuthorizedUIDs[token.UID]; !ok {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *api) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) handleRTDB(w http.ResponseWriter, r *http.Request) {
	suffix := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/rtdb"), "/")
	if suffix != "" {
		for _, segment := range strings.Split(suffix, "/") {
			if !validRTDBSegment(segment) {
				writeError(w, http.StatusBadRequest, "invalid Realtime Database path")
				return
			}
		}
	}
	path := a.cfg.RTDBPath
	if suffix != "" {
		if path != "" {
			path += "/"
		}
		path += suffix
	}
	ref := a.rtdb.NewRef(path)

	switch r.Method {
	case http.MethodGet:
		var value interface{}
		if err := ref.Get(r.Context(), &value); err != nil {
			writeFirebaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, value)
	case http.MethodPut:
		value, err := decodeJSONBody(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := ref.Set(r.Context(), value); err != nil {
			writeFirebaseError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch:
		value, err := decodeJSONObject(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := ref.Update(r.Context(), value); err != nil {
			writeFirebaseError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		value, err := decodeJSONBody(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		child, err := ref.Push(r.Context(), value)
		if err != nil {
			writeFirebaseError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"key": child.Key})
	case http.MethodDelete:
		if err := ref.Delete(r.Context()); err != nil {
			writeFirebaseError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT, PATCH, POST, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *api) handleFirestore(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/firestore/")
	segments := strings.Split(path, "/")
	if path == "" || !validFirestorePath(segments) {
		writeError(w, http.StatusBadRequest, "invalid Firestore path")
		return
	}

	isCollection := len(segments)%2 == 1
	if isCollection {
		a.handleFirestoreCollection(w, r, strings.Join(segments, "/"))
		return
	}
	a.handleFirestoreDocument(w, r, strings.Join(segments, "/"))
}

func (a *api) handleFirestoreCollection(w http.ResponseWriter, r *http.Request, path string) {
	collection := a.firestore.Collection(path)
	switch r.Method {
	case http.MethodGet:
		iter := collection.Documents(r.Context())
		defer iter.Stop()
		documents := make([]map[string]interface{}, 0)
		for {
			doc, err := iter.Next()
			if errors.Is(err, iterator.Done) {
				break
			}
			if err != nil {
				writeFirebaseError(w, err)
				return
			}
			documents = append(documents, map[string]interface{}{
				"id":   doc.Ref.ID,
				"data": doc.Data(),
			})
		}
		writeJSON(w, http.StatusOK, documents)
	case http.MethodPost:
		data, err := decodeJSONObject(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		doc, _, err := collection.Add(r.Context(), data)
		if err != nil {
			writeFirebaseError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": doc.ID})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *api) handleFirestoreDocument(w http.ResponseWriter, r *http.Request, path string) {
	doc := a.firestore.Doc(path)
	switch r.Method {
	case http.MethodGet:
		snapshot, err := doc.Get(r.Context())
		if err != nil {
			if status.Code(err) == codes.NotFound {
				writeError(w, http.StatusNotFound, "document not found")
				return
			}
			writeFirebaseError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":   snapshot.Ref.ID,
			"data": snapshot.Data(),
		})
	case http.MethodPut:
		data, err := decodeJSONObject(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := doc.Set(r.Context(), data); err != nil {
			writeFirebaseError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch:
		data, err := decodeJSONObject(w, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		updates := make([]firestore.Update, 0, len(data))
		for key, value := range data {
			updates = append(updates, firestore.Update{
				FieldPath: firestore.FieldPath{key},
				Value:     value,
			})
		}
		if len(updates) == 0 {
			writeError(w, http.StatusBadRequest, "PATCH body must not be empty")
			return
		}
		if _, err := doc.Update(r.Context(), updates); err != nil {
			writeFirebaseError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if _, err := doc.Delete(r.Context()); err != nil {
			writeFirebaseError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT, PATCH, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request) (interface{}, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("request body must contain valid JSON")
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, errors.New("request body must contain one JSON value")
	}
	return value, nil
}

func decodeJSONObject(w http.ResponseWriter, r *http.Request) (map[string]interface{}, error) {
	value, err := decodeJSONBody(w, r)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, errors.New("request body must be a JSON object")
	}
	return object, nil
}

func validRTDBSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return false
	}
	if strings.ContainsAny(segment, ".$#[]") {
		return false
	}
	for _, char := range segment {
		if !utf8.ValidRune(char) || unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func validFirestorePath(segments []string) bool {
	if len(segments) == 0 {
		return false
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func writeFirebaseError(w http.ResponseWriter, err error) {
	if status.Code(err) == codes.NotFound {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeError(w, http.StatusBadGateway, "Firebase request failed")
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, statusCode int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
