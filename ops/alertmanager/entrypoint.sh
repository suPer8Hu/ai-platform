#!/usr/bin/env sh
set -eu

: "${SMTP_HOST:?missing}"
: "${SMTP_PORT:?missing}"
: "${SMTP_USER:?missing}"
: "${SMTP_PASS:?missing}"
: "${SMTP_FROM:?missing}"
: "${SMTP_TO:?missing}"

TEMPLATE="/etc/alertmanager/alertmanager.yml.tmpl"
OUT="/tmp/alertmanager.yml"

# Basic escaping for sed replacement
esc() {
  printf '%s' "$1" | sed -e 's/[\/&]/\\&/g'
}

SMTP_HOST_ESC="$(esc "$SMTP_HOST")"
SMTP_PORT_ESC="$(esc "$SMTP_PORT")"
SMTP_USER_ESC="$(esc "$SMTP_USER")"
SMTP_PASS_ESC="$(esc "$SMTP_PASS")"
SMTP_FROM_ESC="$(esc "$SMTP_FROM")"
SMTP_TO_ESC="$(esc "$SMTP_TO")"

sed \
  -e "s/\${SMTP_HOST}/${SMTP_HOST_ESC}/g" \
  -e "s/\${SMTP_PORT}/${SMTP_PORT_ESC}/g" \
  -e "s/\${SMTP_USER}/${SMTP_USER_ESC}/g" \
  -e "s/\${SMTP_PASS}/${SMTP_PASS_ESC}/g" \
  -e "s/\${SMTP_FROM}/${SMTP_FROM_ESC}/g" \
  -e "s/\${SMTP_TO}/${SMTP_TO_ESC}/g" \
  "$TEMPLATE" > "$OUT"

exec /bin/alertmanager \
  --config.file="$OUT" \
  --storage.path=/alertmanager