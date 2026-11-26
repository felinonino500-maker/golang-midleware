package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// Ganti sesuai Project ID & Web API Key Anda (atau isi via environment variable)
	projectID = "YOUR_PROJECT_ID"
	apiKey    = "YOUR_WEB_API_KEY"
	deviceID  = "esp32-01"
	listenAddr = ":8080"
)

type FirestoreDoc struct {
	Name   string `json:"name"`
	Fields struct {
		Suhu struct {
			DoubleValue float64 `json:"doubleValue"`
		} `json:"suhu"`
		Relay struct {
			IntegerValue string `json:"integerValue"`
		} `json:"relay"`
		Level struct {
			StringValue string `json:"stringValue"`
		} `json:"level"`
		Message struct {
			StringValue string `json:"stringValue"`
		} `json:"message"`
		LastSeen struct {
			TimestampValue string `json:"timestampValue"`
		} `json:"last_seen"`
	} `json:"fields"`
}

type LogEntry struct {
	ID        string  `json:"id"`
	Timestamp string  `json:"timestamp"`
	Level     string  `json:"level"`
	Suhu      float64 `json:"suhu"`
	Relay     string  `json:"relay"`
	Message   string  `json:"message"`
}

type MonitorResponse struct {
	DeviceID string     `json:"device_id"`
	Status   string     `json:"status"`
	LastSeen string     `json:"last_seen"`
	Logs     []LogEntry `json:"logs"`
}

func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func fetchLatestDevice(client *http.Client, proj, key, dev string) (*LogEntry, error) {
	// Contoh mengambil 1 dokumen status terbaru di: devices/<deviceID>
	url := fmt.Sprintf(
		"https://firestore.googleapis.com/v1/projects/%s/databases/(default)/documents/devices/%s?key=%s",
		proj, dev, key,
	)

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("firestore HTTP %d", resp.StatusCode)
	}

	var doc FirestoreDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, err
	}

	level := doc.Fields.Level.StringValue
	if level == "" {
		level = "INFO"
	}
	msg := doc.Fields.Message.StringValue
	if msg == "" {
		msg = "Telemetry OK"
	}
	ts := doc.Fields.LastSeen.TimestampValue
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}

	parts := strings.Split(doc.Name, "/")
	docID := dev
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		docID = parts[len(parts)-1]
	}

	return &LogEntry{
		ID:        docID,
		Timestamp: ts,
		Level:     level,
		Suhu:      doc.Fields.Suhu.DoubleValue,
		Relay:     doc.Fields.Relay.IntegerValue,
		Message:   msg,
	}, nil
}

func computeStatus(lastSeenStr string) string {
	t, err := time.Parse(time.RFC3339, lastSeenStr)
	if err != nil {
		return "offline"
	}
	// Anggap online jika update terakhir kurang dari 20 detik yang lalu
	if time.Since(t) <= 20*time.Second {
		return "online"
	}
	return "offline"
}

func main() {
	proj := getEnvOrDefault("FIRESTORE_PROJECT_ID", projectID)
	key := getEnvOrDefault("FIRESTORE_API_KEY", apiKey)
	dev := getEnvOrDefault("ESP32_DEVICE_ID", deviceID)

	httpClient := &http.Client{Timeout: 8 * time.Second}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(loggerHTML))
	})

	http.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		entry, err := fetchLatestDevice(httpClient, proj, key, dev)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		res := MonitorResponse{
			DeviceID: dev,
			Status:   computeStatus(entry.Timestamp),
			LastSeen: entry.Timestamp,
			Logs:     []LogEntry{*entry},
		}
		_ = json.NewEncoder(w).Encode(res)
	})

	log.Printf("Logger berjalan di http://localhost%s (device: %s)", listenAddr, dev)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}

const loggerHTML = `<!DOCTYPE html>
<html lang="id">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>ESP32 Logger (Go)</title>
  <style>
    body { font-family: system-ui, sans-serif; margin: 24px; background: #0f172a; color: #e2e8f0; }
    .top { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 16px; }
    .card { background: #1e293b; border: 1px solid #334155; border-radius: 10px; padding: 14px 16px; margin-bottom: 16px; display: flex; flex-wrap: wrap; gap: 24px; }
    .online { color: #4ade80; font-weight: 700; }
    .offline { color: #f87171; font-weight: 700; }
    .controls { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
    select, input, button { background: #1e293b; color: #e2e8f0; border: 1px solid #334155; border-radius: 8px; padding: 8px 12px; font-size: 14px; }
    button { background: #2563eb; border: none; font-weight: 600; cursor: pointer; }
    table { width: 100%; border-collapse: collapse; background: #1e293b; border: 1px solid #334155; border-radius: 10px; overflow: hidden; }
    th, td { padding: 10px 12px; text-align: left; border-bottom: 1px solid #334155; font-size: 14px; }
    th { background: #0b1220; color: #94a3b8; }
    .err { background: #450a0a; border: 1px solid #dc2626; color: #fca5a5; padding: 10px 14px; border-radius: 8px; margin-bottom: 12px; display: none; }
  </style>
</head>
<body>
  <div class="top">
    <h2 style="margin:0;">ESP32 Logger Monitor</h2>
    <div>
      <button onclick="loadData()">Refresh</button>
      <label style="margin-left:8px; font-size:14px;">
        <input type="checkbox" id="autoChk" checked onchange="toggleAuto()" /> Auto (3s)
      </label>
    </div>
  </div>

  <div id="errBox" class="err"></div>

  <div class="card">
    <div>Device: <strong id="devId">-</strong></div>
    <div>Status: <span id="devStatus" class="offline">OFFLINE</span></div>
    <div>Last Seen: <strong id="lastSeen">-</strong></div>
  </div>

  <div class="controls">
    <select id="lvlFilter" onchange="renderRows()">
      <option value="ALL">Semua Level</option>
      <option value="INFO">INFO</option>
      <option value="WARN">WARN</option>
      <option value="ERROR">ERROR</option>
    </select>
    <input id="qInput" type="text" placeholder="Cari pesan atau waktu..." oninput="renderRows()" style="flex:1; min-width:200px;" />
  </div>

  <table>
    <thead>
      <tr>
        <th>Waktu</th>
        <th>Level</th>
        <th>Suhu (°C)</th>
        <th>Relay</th>
        <th>Pesan</th>
      </tr>
    </thead>
    <tbody id="tbody"></tbody>
  </table>

  <script>
    let latestLogs = [];
    let timer = null;

    function showError(msg) {
      const el = document.getElementById("errBox");
      if (!msg) { el.style.display = "none"; el.textContent = ""; return; }
      el.style.display = "block";
      el.textContent = msg;
    }

    function renderRows() {
      const lvl = document.getElementById("lvlFilter").value;
      const q = document.getElementById("qInput").value.trim().toLowerCase();
      const rows = latestLogs.filter(x => {
        const okLvl = lvl === "ALL" || x.level === lvl;
        const okQ = !q || String(x.message || "").toLowerCase().includes(q) || String(x.timestamp || "").toLowerCase().includes(q);
        return okLvl && okQ;
      });

      const tbody = document.getElementById("tbody");
      if (!rows.length) {
        tbody.innerHTML = '<tr><td colspan="5" style="text-align:center; color:#94a3b8; padding:20px;">Tidak ada data</td></tr>';
        return;
      }

      tbody.innerHTML = rows.map(x => `
        <tr>
          <td>${x.timestamp}</td>
          <td><strong>${x.level || "INFO"}</strong></td>
          <td>${Number(x.suhu || 0).toFixed(2)}</td>
          <td>${x.relay ?? "-"}</td>
          <td>${x.message || "-"}</td>
        </tr>
      `).join("");
    }

    async function loadData() {
      try {
        const r = await fetch("/api/logs");
        const j = await r.json();
        if (!r.ok) throw new Error(j.error || ("HTTP " + r.status));

        showError("");
        document.getElementById("devId").textContent = j.device_id || "-";
        document.getElementById("lastSeen").textContent = j.last_seen || "-";

        const st = document.getElementById("devStatus");
        const isOnline = j.status === "online";
        st.textContent = isOnline ? "ONLINE" : "OFFLINE";
        st.className = isOnline ? "online" : "offline";

        latestLogs = Array.isArray(j.logs) ? j.logs : [];
        renderRows();
      } catch (e) {
        showError("Gagal memuat data: " + e.message);
      }
    }

    function toggleAuto() {
      if (timer) { clearInterval(timer); timer = null; }
      if (document.getElementById("autoChk").checked) {
        timer = setInterval(loadData, 3000);
      }
    }

    loadData();
    toggleAuto();
  </script>
</body>
</html>
`
