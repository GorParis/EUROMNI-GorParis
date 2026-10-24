#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
PILOT_DIR="$ROOT_DIR/pipeline/prestashop-pilot"
STATE_DIR="$ROOT_DIR/.tmp/prestashop-pilot"
ENV_FILE="$STATE_DIR/instance.env"
PROJECT="euromni-prestashop-pilot"
COMPOSE_FILE="$PILOT_DIR/compose.yaml"
PRESTASHOP_IMAGE="docker.io/prestashop/prestashop@sha256:bd05527bc324b43bf70458413be60181614fad62f71a63af2525362985207b3d"
MODULE_SOURCE="$ROOT_DIR/pipeline/fixtures/prestashop-ps_newproducts/ps_newproducts-72e9476406319cbb91ac383121ca9967dc61f277"
STAGED_MODULE_SOURCE="$STATE_DIR/module-source"
LANGUAGE_PACK="$STATE_DIR/prestashop-9.0.3-en-US.zip"
LANGUAGE_PACK_SHA256="588021e28e222c5d170b02893dd755060b2d93dae20046e59407d3d346ef0371"
ACTION="${1:-status}"

usage() {
  cat <<'EOF'
Usage: pipeline/prestashop-pilot/manage.sh {start|test|status|stop|credentials|destroy --confirm}

The instance persists in Podman named volumes. `stop` preserves all data;
`destroy --confirm` permanently removes only this pilot project and its volumes.
EOF
}

create_environment() {
  if [[ -f "$ENV_FILE" ]]; then
    return
  fi
  mkdir -p "$STATE_DIR"
  chmod 700 "$STATE_DIR"
  umask 077
  python3 - "$ENV_FILE" <<'PY'
import secrets
import sys

path = sys.argv[1]
with open(path, "x", encoding="ascii") as output:
    output.write(f"PRESTASHOP_DB_PASSWORD={secrets.token_urlsafe(32)}\n")
    output.write(f"PRESTASHOP_DB_ROOT_PASSWORD={secrets.token_urlsafe(32)}\n")
    output.write(f"PRESTASHOP_ADMIN_EMAIL=pilot-admin@example.invalid\n")
    output.write(f"PRESTASHOP_ADMIN_PASSWORD={secrets.token_urlsafe(32)}\n")
    output.write("PRESTASHOP_ERASE_DB=0\n")
PY
  chmod 600 "$ENV_FILE"
}

load_environment() {
  if [[ ! -f "$ENV_FILE" ]]; then
    printf 'Pilot credentials are missing; run start first.\n' >&2
    exit 1
  fi
  set -a
  # This file is generated locally by create_environment and is excluded by .gitignore.
  . "$ENV_FILE"
  set +a
  export PRESTASHOP_MODULE_SOURCE="$STAGED_MODULE_SOURCE"
}

stage_module_source() {
  mkdir -p "$STAGED_MODULE_SOURCE"
  cp -R "$MODULE_SOURCE"/. "$STAGED_MODULE_SOURCE"/
  printf '%s  %s\n' \
    a1d5460bee7754f296fd780c864a2a04cdfd988e54d56784dd887ecf2ecdb001 \
    "$STAGED_MODULE_SOURCE/ps_newproducts.php" | sha256sum --check --status || {
      printf 'Staged ps_newproducts source does not match the pinned fixture.\n' >&2
      exit 1
    }
}

ensure_language_pack() {
  local temporary_file="${LANGUAGE_PACK}.part"
  if [[ ! -f "$LANGUAGE_PACK" ]]; then
    curl --silent --show-error --location --fail --max-time 60 \
      https://i18n.prestashop-project.org/translations/9.0.3/en-US/en-US.zip \
      --output "$temporary_file"
    if ! printf '%s  %s\n' "$LANGUAGE_PACK_SHA256" "$temporary_file" |
      sha256sum --check --status; then
      rm -f -- "$temporary_file"
      printf 'Downloaded PrestaShop language pack does not match the pinned SHA-256.\n' >&2
      exit 1
    fi
    chmod 600 "$temporary_file"
    mv -- "$temporary_file" "$LANGUAGE_PACK"
  fi
  if ! printf '%s  %s\n' "$LANGUAGE_PACK_SHA256" "$LANGUAGE_PACK" |
    sha256sum --check --status; then
    printf 'Cached PrestaShop language pack does not match the pinned SHA-256.\n' >&2
    exit 1
  fi
}

seed_language_pack() {
  local volume="${PROJECT}_prestashop-files"
  local cached_hash
  # PrestaShop 9 checks this legacy marker before using its XLF archive cache.
  if ! podman volume exists "$volume"; then
    podman volume create "$volume" >/dev/null
  fi
  cached_hash="$(podman run --rm --network none \
    --volume "$volume:/var/www/html" \
    --entrypoint sh "$PRESTASHOP_IMAGE" \
    -c 'if [ -f /var/www/html/translations/sf-en-US.zip ]; then
          sha256sum /var/www/html/translations/sf-en-US.zip | cut -d " " -f 1
        fi')"
  if [[ "$cached_hash" == "$LANGUAGE_PACK_SHA256" ]]; then
    podman run --rm --network none \
      --volume "$volume:/var/www/html" \
      --entrypoint sh "$PRESTASHOP_IMAGE" \
      -c 'chmod 644 /var/www/html/translations/sf-en-US.zip &&
          touch /var/www/html/translations/en.gzip'
    return
  fi
  podman run --rm --network none \
    --volume "$volume:/var/www/html" \
    --volume "$LANGUAGE_PACK:/tmp/en-US.zip:ro" \
    --entrypoint sh "$PRESTASHOP_IMAGE" \
    -c 'mkdir -p /var/www/html/translations &&
        cp /tmp/en-US.zip /var/www/html/translations/sf-en-US.zip &&
        chmod 644 /var/www/html/translations/sf-en-US.zip &&
        touch /var/www/html/translations/en.gzip'
}

compose() {
  podman compose --env-file "$ENV_FILE" -p "$PROJECT" -f "$COMPOSE_FILE" "$@"
}

wait_for_shop() {
  local attempt response_code
  for attempt in $(seq 1 300); do
    response_code="$(curl --silent --output /dev/null --write-out '%{http_code}' \
      --max-time 5 http://127.0.0.1:18084/ || true)"
    if [[ "$response_code" =~ ^[23][0-9][0-9]$ ]]; then
      return
    fi
    if [[ "$response_code" =~ ^5[0-9][0-9]$ ]]; then
      compose logs --tail=40 prestashop
      printf 'PrestaShop returned HTTP %s during startup.\n' "$response_code" >&2
      exit 1
    fi
    if ! podman ps --filter "name=${PROJECT}_prestashop_1" --filter status=running --format '{{.Names}}' | grep -q .; then
      compose logs --tail=80 prestashop
      printf 'PrestaShop container stopped during startup.\n' >&2
      exit 1
    fi
    sleep 2
  done
  compose logs --tail=80 prestashop
  printf 'Timed out waiting for PrestaShop at http://127.0.0.1:18084/.\n' >&2
  exit 1
}

run_native_test() {
  local storefront slug
  compose exec -T prestashop chown www-data:www-data \
    /var/www/html/themes/classic/assets/cache
  compose exec -T prestashop php /opt/euromni-pilot/seed-and-test.php
  if ! storefront="$(curl --silent --show-error --location --fail --max-time 20 \
    http://127.0.0.1:18084/)"; then
    compose logs --tail=40 prestashop
    printf 'Could not load the PrestaShop storefront for native module checks.\n' >&2
    exit 1
  fi
  for slug in euromni-recent-01d euromni-recent-05d euromni-recent-29d; do
    if ! grep -Fq "$slug" <<<"$storefront"; then
      printf 'Native storefront did not render expected product %s.\n' "$slug" >&2
      exit 1
    fi
  done
  if grep -Fq 'euromni-old-31d' <<<"$storefront"; then
    printf 'Native storefront rendered the product outside the 30-day window.\n' >&2
    exit 1
  fi

  compose exec -T prestashop php -r \
    'require "/var/www/html/config/config.inc.php"; Configuration::updateValue("NEW_PRODUCTS_NBR", 2);'
  if ! storefront="$(curl --silent --show-error --location --fail --max-time 20 \
    http://127.0.0.1:18084/)"; then
    compose exec -T prestashop php -r \
      'require "/var/www/html/config/config.inc.php"; Configuration::updateValue("NEW_PRODUCTS_NBR", 8);'
    printf 'Could not verify the native module product limit on the storefront.\n' >&2
    exit 1
  fi
  if ! grep -Fq 'euromni-recent-01d' <<<"$storefront" ||
    ! grep -Fq 'euromni-recent-05d' <<<"$storefront" ||
    grep -Fq 'euromni-recent-29d' <<<"$storefront"; then
    compose exec -T prestashop php -r \
      'require "/var/www/html/config/config.inc.php"; Configuration::updateValue("NEW_PRODUCTS_NBR", 8);'
    printf 'Native storefront did not honor the configured limit of two products.\n' >&2
    exit 1
  fi

  compose exec -T prestashop php -r \
    'require "/var/www/html/config/config.inc.php"; Configuration::updateValue("NEW_PRODUCTS_NBR", 8); Configuration::updateValue("PS_NB_DAYS_NEW_PRODUCT", 0);'
  if ! storefront="$(curl --silent --show-error --location --fail --max-time 20 \
    http://127.0.0.1:18084/)"; then
    compose exec -T prestashop php -r \
      'require "/var/www/html/config/config.inc.php"; Configuration::updateValue("PS_NB_DAYS_NEW_PRODUCT", 30);'
    printf 'Could not verify the native global-window disable on the storefront.\n' >&2
    exit 1
  fi
  if grep -Fq 'euromni-recent-' <<<"$storefront"; then
    compose exec -T prestashop php -r \
      'require "/var/www/html/config/config.inc.php"; Configuration::updateValue("PS_NB_DAYS_NEW_PRODUCT", 30);'
    printf 'Native storefront rendered new products with the global window disabled.\n' >&2
    exit 1
  fi
  compose exec -T prestashop php -r \
    'require "/var/www/html/config/config.inc.php"; Configuration::updateValue("PS_NB_DAYS_NEW_PRODUCT", 30); Configuration::updateValue("NEW_PRODUCTS_NBR", 8);'
}

set_database_erase() {
  python3 - "$ENV_FILE" "$1" <<'PY'
import sys

path, value = sys.argv[1:]
with open(path, encoding="ascii") as source:
    lines = source.readlines()
with open(path, "w", encoding="ascii") as output:
    found = False
    for line in lines:
        if line.startswith("PRESTASHOP_ERASE_DB="):
            output.write(f"PRESTASHOP_ERASE_DB={value}\n")
            found = True
        else:
            output.write(line)
    if not found:
        output.write(f"PRESTASHOP_ERASE_DB={value}\n")
PY
  chmod 600 "$ENV_FILE"
}

prepare_install() {
  local volume="${PROJECT}_prestashop-files"
  if ! podman volume exists "$volume"; then
    set_database_erase 1
  elif ! podman run --rm --network none \
    --volume "$volume:/var/www/html" \
    --entrypoint sh "$PRESTASHOP_IMAGE" \
    -c '[ -f /var/www/html/config/settings.inc.php ] ||
        [ -f /var/www/html/app/config/parameters.php ]'; then
    set_database_erase 1
  fi
  load_environment
}

recover_interrupted_install() {
  local volume="${PROJECT}_prestashop-files"
  if podman volume exists "$volume" && podman run --rm --network none \
      --volume "$volume:/var/www/html" \
      --entrypoint sh "$PRESTASHOP_IMAGE" \
      -c '[ -f /var/www/html/install.lock ]'; then
    if podman ps --filter "name=${PROJECT}_prestashop_1" --filter status=running --format '{{.Names}}' | grep -q .; then
      podman stop "${PROJECT}_prestashop_1"
    fi
    podman run --rm --network none \
      --volume "$volume:/var/www/html" \
      --entrypoint sh "$PRESTASHOP_IMAGE" \
      -c 'rm -f -- /var/www/html/install.lock \
          /var/www/html/config/settings.inc.php \
          /var/www/html/app/config/parameters.php &&
          if [ -d /var/www/html/admin-dev ]; then
            rm -r -- /var/www/html/admin-dev
          fi'
    set_database_erase 1
  fi
}

case "$ACTION" in
  start)
    create_environment
    load_environment
    stage_module_source
    ensure_language_pack
    compose pull
    recover_interrupted_install
    prepare_install
    seed_language_pack
    compose up -d
    if [[ "$(podman network inspect --format '{{.Internal}}' "${PROJECT}_default")" != "true" ]]; then
      printf 'Refusing to continue: pilot network is not internal.\n' >&2
      exit 1
    fi
    wait_for_shop
    run_native_test
    set_database_erase 0
    printf 'PrestaShop pilot is ready at http://127.0.0.1:18084/ (admin: /admin-dev/).\n'
    printf 'Local login details are stored in %s.\n' "$ENV_FILE"
    ;;
  test)
    load_environment
    stage_module_source
    ensure_language_pack
    recover_interrupted_install
    prepare_install
    seed_language_pack
    compose up -d
    wait_for_shop
    run_native_test
    set_database_erase 0
    ;;
  status)
    if [[ ! -f "$ENV_FILE" ]]; then
      printf 'PrestaShop pilot has not been initialized.\n'
      exit 0
    fi
    load_environment
    compose ps
    printf 'URL: http://127.0.0.1:18084/ (admin: /admin-dev/)\n'
    ;;
  stop)
    load_environment
    compose stop
    ;;
  credentials)
    load_environment
    printf 'Login details (local test instance only):\n'
    sed -n '/^PRESTASHOP_ADMIN_EMAIL=/p; /^PRESTASHOP_ADMIN_PASSWORD=/p' "$ENV_FILE"
    ;;
  destroy)
    if [[ "${2:-}" != "--confirm" ]]; then
      usage >&2
      printf 'Pass --confirm to remove the persistent PrestaShop pilot data.\n' >&2
      exit 2
    fi
    load_environment
    compose down --volumes --remove-orphans
    rm -f -- "$ENV_FILE"
    rmdir -- "$STATE_DIR" 2>/dev/null || true
    printf 'PrestaShop pilot containers, network, and named volumes removed.\n'
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
