package main

import (
	"fmt"
	"os"
	"strings"
)

type config struct {
	FirebaseProjectID string
	RTDBDatabaseURL   string
	RTDBPath          string
	Port              string
	AuthorizedUIDs    map[string]struct{}
}

func loadConfig() (config, error) {
	cfg := config{
		FirebaseProjectID: strings.TrimSpace(os.Getenv("FIREBASE_PROJECT_ID")),
		RTDBDatabaseURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("RTDB_DATABASE_URL")), "/"),
		RTDBPath:          strings.Trim(os.Getenv("RTDB_PATH"), "/"),
		Port:              strings.TrimSpace(os.Getenv("PORT")),
		AuthorizedUIDs:    make(map[string]struct{}),
	}
	for _, uid := range strings.Split(os.Getenv("AUTHORIZED_UIDS"), ",") {
		if uid = strings.TrimSpace(uid); uid != "" {
			cfg.AuthorizedUIDs[uid] = struct{}{}
		}
	}
	if cfg.Port == "" {
		cfg.Port = "3000"
	}
	if cfg.FirebaseProjectID == "" {
		return config{}, fmt.Errorf("FIREBASE_PROJECT_ID is required")
	}
	if cfg.RTDBDatabaseURL == "" {
		return config{}, fmt.Errorf("RTDB_DATABASE_URL is required")
	}
	if len(cfg.AuthorizedUIDs) == 0 {
		return config{}, fmt.Errorf("AUTHORIZED_UIDS must contain at least one Firebase Auth UID")
	}
	if cfg.RTDBPath != "" {
		for _, segment := range strings.Split(cfg.RTDBPath, "/") {
			if !validRTDBSegment(segment) {
				return config{}, fmt.Errorf("RTDB_PATH contains an invalid path segment")
			}
		}
	}
	return cfg, nil
}
