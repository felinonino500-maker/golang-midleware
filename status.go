package main

import (
	"context"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type firebaseStatus struct {
	Status    string `json:"status"`
	RTDB      string `json:"rtdb"`
	Firestore string `json:"firestore"`
}

func (a *api) statusPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(statusPageHTML))
}

func (a *api) firebaseStatus(w http.ResponseWriter, r *http.Request) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()

	if time.Since(a.checkedAt) >= 10*time.Second {
		a.status = a.checkFirebase(r.Context())
		a.checkedAt = time.Now()
	}

	statusCode := http.StatusOK
	if a.status.Status != "connected" {
		statusCode = http.StatusServiceUnavailable
	}
	w.Header().Set("Cache-Control", "public, max-age=5")
	writeJSON(w, statusCode, a.status)
}

func (a *api) checkFirebase(parent context.Context) firebaseStatus {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	rtdbResult := make(chan bool, 1)
	go func() {
		path := a.cfg.RTDBPath
		if path != "" {
			path += "/"
		}
		path += "__middleware_health"
		var value interface{}
		rtdbResult <- a.rtdb.NewRef(path).Get(ctx, &value) == nil
	}()
	firestoreResult := make(chan bool, 1)
	go func() {
		_, err := a.firestore.Doc("_middleware_health/check").Get(ctx)
		firestoreResult <- err == nil || status.Code(err) == codes.NotFound
	}()

	rtdbConnected := <-rtdbResult
	firestoreConnected := <-firestoreResult

	result := firebaseStatus{Status: "connected"}
	if rtdbConnected {
		result.RTDB = "connected"
	} else {
		result.RTDB = "disconnected"
		result.Status = "disconnected"
	}
	if firestoreConnected {
		result.Firestore = "connected"
	} else {
		result.Firestore = "disconnected"
		result.Status = "disconnected"
	}
	return result
}

const statusPageHTML = `<!doctype html>
<html lang="id">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Status Firebase Middleware</title>
  <style>
    body { font: 16px system-ui, sans-serif; max-width: 34rem; margin: 3rem auto; padding: 0 1rem; color: #202124; }
    h1 { font-size: 1.5rem; }
    ul { list-style: none; padding: 0; }
    li { display: flex; justify-content: space-between; padding: .8rem 0; border-bottom: 1px solid #ddd; }
    #overall { font-weight: 700; }
  </style>
</head>
<body>
  <h1>Status Firebase Middleware</h1>
  <p id="overall" role="status">Memeriksa koneksi...</p>
  <ul>
    <li><span>Realtime Database</span><strong id="rtdb">Memeriksa...</strong></li>
    <li><span>Firestore</span><strong id="firestore">Memeriksa...</strong></li>
  </ul>
  <script>
    fetch("/status", {cache: "no-store"})
      .then(async response => {
        const data = await response.json();
        document.getElementById("rtdb").textContent = data.rtdb || "tidak diketahui";
        document.getElementById("firestore").textContent = data.firestore || "tidak diketahui";
        document.getElementById("overall").textContent =
          response.ok ? "Firebase terhubung" : "Koneksi Firebase bermasalah";
      })
      .catch(() => {
        document.getElementById("overall").textContent = "Status Firebase tidak dapat diperiksa";
        document.getElementById("rtdb").textContent = "tidak diketahui";
        document.getElementById("firestore").textContent = "tidak diketahui";
      });
  </script>
</body>
</html>`
