package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	firebaseauth "firebase.google.com/go/v4/auth"
)

type fakeTokenVerifier struct {
	uid string
	err error
}

func (v fakeTokenVerifier) VerifyIDToken(context.Context, string) (*firebaseauth.Token, error) {
	if v.err != nil {
		return nil, v.err
	}
	return &firebaseauth.Token{UID: v.uid}, nil
}

func TestRequireToken(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		authorization string
		uid           string
		verifyErr     error
		allowedUIDs   map[string]struct{}
		wantStatus    int
	}{
		{name: "request without token is rejected", path: "/api/rtdb", wantStatus: http.StatusUnauthorized},
		{name: "invalid Firebase token is rejected", path: "/api/rtdb", authorization: "Bearer invalid", verifyErr: errors.New("invalid"), wantStatus: http.StatusUnauthorized},
		{name: "valid token for other UID is forbidden", path: "/api/rtdb", authorization: "Bearer valid", uid: "other", allowedUIDs: map[string]struct{}{"allowed": {}}, wantStatus: http.StatusForbidden},
		{name: "valid authorized token passes", path: "/api/firestore/items", authorization: "Bearer valid", uid: "allowed", allowedUIDs: map[string]struct{}{"allowed": {}}, wantStatus: http.StatusNoContent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			apiHandler := (&api{
				cfg:        config{AuthorizedUIDs: test.allowedUIDs},
				authClient: fakeTokenVerifier{uid: test.uid, err: test.verifyErr},
			}).requireToken(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			response := httptest.NewRecorder()
			apiHandler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestHealthIsPublic(t *testing.T) {
	handler := newAPI(config{}, nil, nil, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestValidRTDBSegment(t *testing.T) {
	for _, segment := range []string{"", ".", "..", "a.b", "a#b", "a$b", "a[b", "a]b", "a\nb"} {
		if validRTDBSegment(segment) {
			t.Errorf("validRTDBSegment(%q) = true, want false", segment)
		}
	}
	for _, segment := range []string{"firmware", "device-1", "sensor_2"} {
		if !validRTDBSegment(segment) {
			t.Errorf("validRTDBSegment(%q) = false, want true", segment)
		}
	}
}

func TestValidFirestorePath(t *testing.T) {
	for _, segments := range [][]string{{}, {""}, {"items", ""}, {"items", ".", "docs"}} {
		if validFirestorePath(segments) {
			t.Errorf("validFirestorePath(%q) = true, want false", segments)
		}
	}
	for _, segments := range [][]string{{"items"}, {"items", "doc-1"}, {"accounts", "a1", "settings"}} {
		if !validFirestorePath(segments) {
			t.Errorf("validFirestorePath(%q) = false, want true", segments)
		}
	}
}
