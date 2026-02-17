#!/bin/sh
set -eu

APP_DIR="/CLIProxyAPI"
DATA_DIR="${CLI_PROXY_DATA_DIR:-${APP_DIR}/data}"
CONFIG_PATH="${CLI_PROXY_CONFIG_PATH:-${DATA_DIR}/config.yaml}"
AUTH_DIR="${CLI_PROXY_AUTH_DIR:-${DATA_DIR}/auths}"
LOG_DIR="${CLI_PROXY_LOG_DIR:-${DATA_DIR}/logs}"
EXAMPLE_CONFIG="${APP_DIR}/config.example.yaml"

mkdir -p "${DATA_DIR}" "${AUTH_DIR}" "${LOG_DIR}"

set_auth_dir_in_config() {
  AUTH_LINE="auth-dir: \"${AUTH_DIR}\""
  TMP_CONFIG="${CONFIG_PATH}.tmp"
  if grep -q '^auth-dir:' "${CONFIG_PATH}"; then
    awk -v auth_line="${AUTH_LINE}" '
      /^auth-dir:/ { print auth_line; next }
      { print }
    ' "${CONFIG_PATH}" > "${TMP_CONFIG}"
    mv "${TMP_CONFIG}" "${CONFIG_PATH}"
  else
    printf "\n%s\n" "${AUTH_LINE}" >> "${CONFIG_PATH}"
  fi
}

if [ ! -s "${CONFIG_PATH}" ]; then
  if [ ! -f "${EXAMPLE_CONFIG}" ]; then
    echo "Error: template config not found: ${EXAMPLE_CONFIG}" >&2
    exit 1
  fi

  cp "${EXAMPLE_CONFIG}" "${CONFIG_PATH}"
  set_auth_dir_in_config

  echo "Initialized config: ${CONFIG_PATH}"
fi

CURRENT_AUTH_DIR="$(
  sed -n 's/^[[:space:]]*auth-dir:[[:space:]]*//p' "${CONFIG_PATH}" \
    | head -n 1 \
    | sed 's/^"//; s/"$//; s/^[[:space:]]*//; s/[[:space:]]*$//' \
    || true
)"

case "${CURRENT_AUTH_DIR}" in
  ""|"~/.cli-proxy-api"|"/root/.cli-proxy-api")
    set_auth_dir_in_config
    ;;
esac

if [ -z "${WRITABLE_PATH:-}" ] && [ -z "${writable_path:-}" ]; then
  export WRITABLE_PATH="${DATA_DIR}"
fi

exec "${APP_DIR}/CLIProxyAPI" -config "${CONFIG_PATH}"
