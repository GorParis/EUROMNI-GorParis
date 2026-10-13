#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
FIXTURE_DIR="$ROOT_DIR/fixtures/real-modules"
mkdir -p "$FIXTURE_DIR"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

while read -r name repository commit; do
  archive="$WORK_DIR/$name.zip"
  destination="$FIXTURE_DIR/$name"
  mkdir -p "$destination"

  url="https://codeload.github.com/$repository/zip/$commit"
  echo "downloading $repository at $commit"
  curl --fail --location --silent --show-error "$url" -o "$archive"
  unzip -q -o "$archive" -d "$destination"
done <<'MODULES'
shopify-function-examples Shopify/function-examples 19ccafceda1d0052c2c90c0a1e4db3fe37b16c27
shopify-checkout-ui-extensions-po-box-validator Shopify/checkout-ui-extensions-po-box-validator f1d35d27575b8024113096c3c6f7892ab63a482e
magento-japan-tax magento/magento-japan-tax c0e781438f8f46b65f37d30882199c6717a08d12
magento-module-grpc magento/module-grpc 6fe9d46de08dc5eda06d72053c560cd9207f1707
MODULES

echo "fixtures ready in $FIXTURE_DIR"