#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
mkdir -p "$ROOT_DIR/.tmp"
PILOT_DIR="$(mktemp -d "$ROOT_DIR/.tmp/selfnamed-pilot.XXXXXX")"
PROJECT="euromni-selfnamed-pilot-$$"
COMPOSE_FILE="$ROOT_DIR/pipeline/selfnamed-pilot/compose.yaml"
PLUGIN_DIR="$PILOT_DIR/plugins"
SELFNAMED_VERSION="2.0.19"
STARTED=0

cleanup() {
  local exit_status=$?
  local cleanup_status
  trap - EXIT
  if [[ "$STARTED" -eq 1 ]]; then
    PILOT_DIR="$PILOT_DIR" podman compose -p "$PROJECT" -f "$COMPOSE_FILE" down --timeout 1 --volumes --remove-orphans || {
      cleanup_status=$?
      if [[ "$exit_status" -eq 0 ]]; then
        exit_status=$cleanup_status
      fi
    }
  fi
  rm -r -- "$PILOT_DIR" || {
    cleanup_status=$?
    if [[ "$exit_status" -eq 0 ]]; then
      exit_status=$cleanup_status
    fi
  }
  exit "$exit_status"
}
trap cleanup EXIT

mkdir -p "$PLUGIN_DIR"
SORTING_PLUGIN_SOURCE="$ROOT_DIR/pipeline/fixtures/woocommerce-extra-product-sorting-options/woocommerce-extra-product-sorting-options-bc5829d4d2383ae956869386eb40e772cf4a76a0"
mkdir -p "$PLUGIN_DIR/woocommerce-extra-product-sorting-options"
cp -a "$SORTING_PLUGIN_SOURCE"/. "$PLUGIN_DIR/woocommerce-extra-product-sorting-options/"
SORTING_PLUGIN_ENTRY="$PLUGIN_DIR/woocommerce-extra-product-sorting-options/woocommerce-extra-product-sorting-options.php"
printf '%s  %s\n' \
  aaf1a1a13ec7e0465a4c451b952aba79626be3288104c18122c47bf01c81306f \
  "$SORTING_PLUGIN_ENTRY" | sha256sum --check --status
if ! grep -q "^ \* Version: 2.11.0$" "$SORTING_PLUGIN_ENTRY"; then
  printf 'WooCommerce sorting plugin does not declare the pinned version 2.11.0.\n' >&2
  exit 1
fi
curl --fail --location --silent --show-error \
  https://downloads.wordpress.org/plugin/woocommerce.10.9.0.zip \
  --output "$PILOT_DIR/woocommerce.zip"
curl --fail --location --silent --show-error \
  "https://downloads.wordpress.org/plugin/selfnamed-cosmetics-on-demand.${SELFNAMED_VERSION}.zip" \
  --output "$PILOT_DIR/selfnamed.zip"

printf '%s  %s\n' \
  d19a480cd2d936165c26a487a52723c527091fd0410353f45e06477434c3dccc \
  "$PILOT_DIR/woocommerce.zip" \
  6e464f7130f905d35686c1ded1faeadc010da784c8a18222b354b7f9be87809f \
  "$PILOT_DIR/selfnamed.zip" | sha256sum --check --status

unzip -q "$PILOT_DIR/woocommerce.zip" -d "$PLUGIN_DIR"
unzip -q "$PILOT_DIR/selfnamed.zip" -d "$PLUGIN_DIR"
if ! grep -q "^ \* Version: ${SELFNAMED_VERSION}$" \
  "$PLUGIN_DIR/selfnamed-cosmetics-on-demand/selfnamed-cosmetics-on-demand.php"; then
  printf 'Downloaded Selfnamed plugin does not declare version %s.\n' "$SELFNAMED_VERSION" >&2
  exit 1
fi

export PILOT_DIR
podman compose -p "$PROJECT" -f "$COMPOSE_FILE" pull
STARTED=1
podman compose -p "$PROJECT" -f "$COMPOSE_FILE" up -d database mock-api wordpress cli

network_internal="$(podman network inspect --format '{{.Internal}}' "${PROJECT}_default")"
if [[ "$network_internal" != "true" ]]; then
  printf 'Refusing to run: Podman network is not internal.\n' >&2
  exit 1
fi

for attempt in $(seq 1 30); do
  if podman compose -p "$PROJECT" -f "$COMPOSE_FILE" exec -T cli wp core version >/dev/null 2>&1; then
    break
  fi
  if [[ "$attempt" -eq 30 ]]; then
    printf 'WordPress files did not become ready in the isolated container.\n' >&2
    exit 1
  fi
  sleep 1
done

podman compose -p "$PROJECT" -f "$COMPOSE_FILE" exec -T cli \
  wp core install \
  --url=http://wordpress \
  --title='EUROMNI synthetic Selfnamed pilot' \
  --admin_user=pilot-admin \
  --admin_password=local-pilot-only-password \
  --admin_email=pilot@example.invalid \
  --skip-email

podman compose -p "$PROJECT" -f "$COMPOSE_FILE" exec -T cli wp plugin activate woocommerce
podman compose -p "$PROJECT" -f "$COMPOSE_FILE" exec -T cli wp plugin activate woocommerce-extra-product-sorting-options
podman compose -p "$PROJECT" -f "$COMPOSE_FILE" exec -T cli wp plugin activate selfnamed-cosmetics-on-demand
podman compose -p "$PROJECT" -f "$COMPOSE_FILE" exec -T cli wp eval-file /opt/pilot/order-flow.php
