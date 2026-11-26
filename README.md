# Firebase Go Middleware

HTTP API sederhana untuk Firebase Realtime Database (RTDB) dan Cloud Firestore.
Server menggunakan Firebase Admin SDK, sehingga membutuhkan kredensial Google
server-side. API key Firebase Expo yang diawali `EXPO_PUBLIC_` adalah kredensial
klien publik dan **bukan** pengganti kredensial Admin SDK.

## Konfigurasi

Salin `.env.example` ke `.env`, lalu ganti `AUTHORIZED_UIDS` dengan satu atau
lebih UID pengguna dari Firebase Authentication, dipisahkan koma. Go tidak
membaca `.env` secara otomatis; ekspor variabelnya sebelum menjalankan service,
atau gunakan tool seperti `direnv`.

Siapkan Google Application Default Credentials:

- Lokal: `gcloud auth application-default login`, atau set
  `GOOGLE_APPLICATION_CREDENTIALS` ke file service account.
- Deployment: gunakan service identity/workload identity dengan izin yang
  diperlukan untuk RTDB dan Firestore.

Jangan commit service account JSON. Admin SDK melewati Firebase Security Rules;
karena itu hanya UID yang diizinkan dapat menggunakan endpoint database.

  ### Instalasi di Termux

  Unduh script lalu jalankan:

  ```sh
  pkg install -y curl
  curl -fsSL https://raw.githubusercontent.com/felinonino500-maker/golang-midleware/main/install-termux.sh -o install-termux.sh
  chmod +x install-termux.sh
  ./install-termux.sh
  ```

  Script memasang Git dan Go, meng-clone repository ke
  `$HOME/golang-midleware`, lalu mengunduh dependency Go termasuk Firebase Admin
  SDK. Kredensial Google dan nilai `AUTHORIZED_UIDS` tetap perlu dikonfigurasi
  sebelum server dapat mengakses Firebase.

```sh
cp .env.example .env
export FIREBASE_PROJECT_ID=bizflow-dashboard-vlo72
export RTDB_DATABASE_URL=https://bizflow-dashboard-vlo72-default-rtdb.asia-southeast1.firebasedatabase.app
export RTDB_PATH=firmware
export AUTHORIZED_UIDS='firebase-auth-uid-anda'
export GOOGLE_APPLICATION_CREDENTIALS=/path/ke/service-account.json
go mod tidy
go run .
```

`PORT` default ke `3000`. Endpoint `/healthz` tidak memerlukan token; semua
endpoint data memerlukan header `Authorization: Bearer <Firebase ID token>`.

## Endpoint

RTDB beroperasi relatif terhadap `RTDB_PATH`:

| Method | Path | Perilaku |
| --- | --- | --- |
| GET | `/api/rtdb[/<path>]` | Membaca nilai |
| PUT | `/api/rtdb[/<path>]` | Mengganti nilai |
| PATCH | `/api/rtdb[/<path>]` | Memperbarui child values dari JSON object |
| POST | `/api/rtdb[/<path>]` | Menambahkan child dengan key otomatis |
| DELETE | `/api/rtdb[/<path>]` | Menghapus nilai |

Firestore memakai path collection/document bergantian:

| Method | Path | Perilaku |
| --- | --- | --- |
| GET | `/api/firestore/<collection>` | Mendaftar dokumen |
| POST | `/api/firestore/<collection>` | Membuat dokumen dengan ID otomatis |
| GET | `/api/firestore/<collection>/<id>` | Membaca dokumen |
| PUT | `/api/firestore/<collection>/<id>` | Mengganti atau membuat dokumen |
| PATCH | `/api/firestore/<collection>/<id>` | Memperbarui field dokumen |
| DELETE | `/api/firestore/<collection>/<id>` | Menghapus dokumen |

Contoh:

```sh
curl -H "Authorization: Bearer $FIREBASE_ID_TOKEN" \
  https://your-api.example.com/api/rtdb

curl -X POST -H "Authorization: Bearer $FIREBASE_ID_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"status":"ready"}' \
  https://your-api.example.com/api/firestore/devices
```

Di Expo, dapatkan ID token pengguna yang sudah login melalui Firebase
Authentication lalu kirim sebagai bearer token. Gunakan URL middleware sebagai
`EXPO_PUBLIC_API_BASE_URL`.
