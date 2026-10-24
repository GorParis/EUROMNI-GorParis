<?php
declare(strict_types=1);

require_once '/var/www/html/config/config.inc.php';

function pilot_assert(bool $condition, string $message): void
{
    if (!$condition) {
        fwrite(STDERR, "PrestaShop pilot assertion failed: {$message}\n");
        exit(1);
    }
}

function pilot_product_id(string $slug, int $languageId, int $shopId): int
{
    $id = (int) Db::getInstance()->getValue(
        'SELECT id_product FROM `' . _DB_PREFIX_ . 'product_lang`'
        . ' WHERE link_rewrite = \'' . pSQL($slug) . '\''
        . ' AND id_lang = ' . $languageId . ' AND id_shop = ' . $shopId
    );
    if ($id > 0) {
        return $id;
    }

    $product = new Product();
    $product->name = [$languageId => 'EUROMNI test ' . $slug];
    $product->link_rewrite = [$languageId => $slug];
    $product->price = '12.34';
    $product->active = true;
    $product->available_for_order = true;
    $product->show_price = true;
    $product->visibility = 'both';
    $product->id_category_default = (int) Configuration::get('PS_HOME_CATEGORY');
    $product->id_shop_default = $shopId;
    $product->minimal_quantity = 1;
    $product->condition = 'new';
    $product->indexed = true;
    $product->out_of_stock = 2;
    pilot_assert($product->add(), "could not create synthetic product {$slug}");
    $product->addToCategories([(int) Configuration::get('PS_HOME_CATEGORY')]);
    return (int) $product->id;
}

$context = Context::getContext();
$context->controller = new FrontController();
$shopId = (int) $context->shop->id;
$languageId = (int) $context->language->id;
$now = new DateTimeImmutable('now', new DateTimeZone('UTC'));

Configuration::updateValue('PS_NB_DAYS_NEW_PRODUCT', 30);
Configuration::updateValue('NEW_PRODUCTS_NBR', 8);

$synthetic = [
    'euromni-recent-01d' => 1,
    'euromni-recent-05d' => 5,
    'euromni-recent-29d' => 29,
    'euromni-old-31d' => 31,
];
Db::getInstance()->execute(
    'UPDATE `' . _DB_PREFIX_ . 'product` SET date_add = DATE_SUB(UTC_TIMESTAMP(), INTERVAL 90 DAY)'
);
Db::getInstance()->execute(
    'UPDATE `' . _DB_PREFIX_ . 'product_shop` SET date_add = DATE_SUB(UTC_TIMESTAMP(), INTERVAL 90 DAY)'
);
foreach ($synthetic as $slug => $ageDays) {
    $productId = pilot_product_id($slug, $languageId, $shopId);
    $createdAt = $now->sub(new DateInterval('P' . $ageDays . 'D'))->format('Y-m-d H:i:s');
    Db::getInstance()->update(
        'product',
        ['date_add' => pSQL($createdAt)],
        'id_product = ' . $productId
    );
    Db::getInstance()->update(
        'product_shop',
        ['date_add' => pSQL($createdAt)],
        'id_product = ' . $productId . ' AND id_shop = ' . $shopId
    );
}

$module = Module::getInstanceByName('ps_newproducts');
pilot_assert($module instanceof Ps_NewProducts, 'pinned ps_newproducts source is not loadable');
if (!Module::isInstalled('ps_newproducts')) {
    pilot_assert((bool) $module->install(), 'native ps_newproducts install failed');
}
pilot_assert(
    (int) Configuration::get('NEW_PRODUCTS_NBR') === 8,
    'native module product-count setting was not set to 8'
);

$native = Product::getNewProducts($languageId, 0, 8);
$actualSlugs = array_map(
    static fn (array $product): string => (string) $product['link_rewrite'],
    $native
);
$expectedSlugs = [
    'euromni-recent-01d',
    'euromni-recent-05d',
    'euromni-recent-29d',
];
pilot_assert(
    $actualSlugs === $expectedSlugs,
    'global 30-day native query returned ' . json_encode($actualSlugs)
);

Configuration::updateValue('NEW_PRODUCTS_NBR', 2);
$limited = Product::getNewProducts($languageId, 0, (int) Configuration::get('NEW_PRODUCTS_NBR'));
pilot_assert(count($limited) === 2, 'native module product-count setting did not limit results');
pilot_assert(
    (string) $limited[0]['link_rewrite'] === 'euromni-recent-01d'
        && (string) $limited[1]['link_rewrite'] === 'euromni-recent-05d',
    'native module limit did not preserve newest-first order'
);

Configuration::updateValue('PS_NB_DAYS_NEW_PRODUCT', 0);
pilot_assert(
    empty(Product::getNewProducts($languageId, 0, 8)),
    'zero global PrestaShop window did not exclude all synthetic products'
);

Configuration::updateValue('PS_NB_DAYS_NEW_PRODUCT', 30);
Configuration::updateValue('NEW_PRODUCTS_NBR', 8);

printf(
    "PASS: pinned PrestaShop %s / ps_newproducts %s; global window=30 days; "
        . "native limit=8; returned %d products (1, 5, 29 days old); 31-day product excluded; "
        . "configured limit=2 and global disable verified.\n",
    _PS_VERSION_,
    $module->version,
    count($native)
);
