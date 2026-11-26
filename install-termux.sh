#!/data/data/com.termux/files/usr/bin/bash
set -euo pipefail

REPO_URL="https://github.com/felinonino500-maker/golang-midleware.git"
REPO_DIR="${HOME}/golang-midleware"

pkg update -y
pkg install -y git golang

if [[ -d "${REPO_DIR}/.git" ]]; then
  printf 'Repository sudah ada: %s\n' "${REPO_DIR}"
elif [[ -e "${REPO_DIR}" ]]; then
  printf 'Direktori tujuan sudah ada dan bukan repository Git: %s\n' "${REPO_DIR}" >&2
  exit 1
else
  git clone "${REPO_URL}" "${REPO_DIR}"
fi

cd "${REPO_DIR}"
go mod download

printf '\nGo dan module Firebase sudah siap.\n'
printf 'Repository: %s\n' "${REPO_DIR}"
printf 'Sebelum menjalankan server, atur FIREBASE_PROJECT_ID, RTDB_DATABASE_URL,\n'
printf 'AUTHORIZED_UIDS, serta GOOGLE_APPLICATION_CREDENTIALS.\n'
