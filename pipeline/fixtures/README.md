# Modules PHP de référence

Les sources et archives ci-dessous servent de fixtures réelles pour le pipeline EUROMNI. Aucun module n'est installé ni exécuté.

## WooCommerce

1. `woocommerce`
   - Plateforme: WooCommerce
   - Rôle: base de référence pour les hooks produits, panier, checkout, commandes.
   - Source: `https://downloads.wordpress.org/plugin/woocommerce.latest-stable.zip`

2. `woocommerce-gateway-stripe`
   - Plateforme: WooCommerce
   - Rôle: module de paiement réel, utile pour tester les hooks `woocommerce_gateway_description`, `woocommerce_stripe_payment_method_icons` et les flux de checkout.
   - Source: `https://downloads.wordpress.org/plugin/woocommerce-gateway-stripe.latest-stable.zip`

## PrestaShop

1. `prestashop-ps_emailalerts`
   - Plateforme: PrestaShop
   - Rôle: hooks d'ordre, produit et compte client, enregistrés par appels `registerHook()` littéraux.
   - Source: `https://github.com/PrestaShop/ps_emailalerts`
   - Commit: `359cd5c86c498964d9c38c97db9fcd13d73be3fe`

2. `prestashop-ps_cashondelivery`
   - Plateforme: PrestaShop
   - Rôle: module de paiement avec hooks `paymentOptions` et `displayOrderConfirmation` déclarés dans la constante `HOOKS`.
   - Source: `https://github.com/PrestaShop/ps_cashondelivery`
   - Commit: `d1bac20a8b5a4664759db5c6775be9b426ad4b62`

## Utilisation

```sh
./pipeline/fetch_real_module_fixtures.sh
```

La commande télécharge les archives dans `pipeline/fixtures/` et les décompresse dans des dossiers séparés. Les sources PrestaShop sont épinglées à des commits ; les archives WooCommerce suivent la dernière version stable publiée.

Les ZIP de sources GitHub PrestaShop ne sont pas les packages de distribution Addons et ne contiennent pas de `manifest.json` EUROMNI. Ils servent ici à valider l'analyse statique des vrais fichiers PHP, pas la validation du format d'archive EUROMNI ni un usage en production.

## Shopify et Magento

Les quatre snapshots suivants sont intégrés sous `real-modules/` afin que les tests soient autonomes. Les sources et leurs licences sont conservées ; les modules ne sont jamais installés ni exécutés.

1. Shopify `function-examples`, commit `19ccafceda1d0052c2c90c0a1e4db3fe37b16c27` : Discount Function JavaScript, licence MIT.
2. Shopify `checkout-ui-extensions-po-box-validator`, commit `f1d35d27575b8024113096c3c6f7892ab63a482e` : extension UI de checkout, licence MIT.
3. Magento `magento-japan-tax`, commit `c0e781438f8f46b65f37d30882199c6717a08d12` : événements et plugins de taxe, licence OSL-3.0.
4. Magento `module-grpc`, commit `6fe9d46de08dc5eda06d72053c560cd9207f1707` : déclarations DI d’injection de commandes, licences OSL-3.0/AFL-3.0.

Récupération et extraction dans le dépôt :

```sh
./pipeline/fetch_platform_reference_modules.sh
go test ./internal/pipeline -run TestAnalyzeRealPlatformFixtures
```

Les snapshots sont des données d’analyse uniquement. Le dépôt `Shopify/discounts-reference-app` n’a pas été repris car aucune licence de redistribution n’y est déclarée ; `magento/marketplace-subscriptions` a été remplacé car son manifeste Composer le déclare propriétaire.
