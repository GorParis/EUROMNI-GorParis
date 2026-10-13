#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
FIXTURE_DIR="$ROOT_DIR/fixtures"
mkdir -p "$FIXTURE_DIR"

cd "$FIXTURE_DIR"

for name in \
  "woocommerce:https://downloads.wordpress.org/plugin/woocommerce.latest-stable.zip" \
  "woocommerce-gateway-stripe:https://downloads.wordpress.org/plugin/woocommerce-gateway-stripe.latest-stable.zip" \
  "prestashop-ps_emailalerts:https://codeload.github.com/PrestaShop/ps_emailalerts/zip/359cd5c86c498964d9c38c97db9fcd13d73be3fe" \
  "prestashop-ps_cashondelivery:https://codeload.github.com/PrestaShop/ps_cashondelivery/zip/d1bac20a8b5a4664759db5c6775be9b426ad4b62"
do
  target="${name%%:*}"
  url="${name#*:}"
  zip_path="$FIXTURE_DIR/${target}.zip"

  if [[ -f "$zip_path" ]]; then
    echo "already present: $zip_path"
    continue
  fi

  echo "downloading $target from $url"
  curl -L --fail --silent --show-error "$url" -o "$zip_path"
done

for zip_path in "$FIXTURE_DIR"/*.zip; do
  if [[ ! -f "$zip_path" ]]; then
    continue
  fi
  unzip -q -o "$zip_path" -d "$(basename "$zip_path" .zip)"
done

echo "fixtures ready in $FIXTURE_DIR"
