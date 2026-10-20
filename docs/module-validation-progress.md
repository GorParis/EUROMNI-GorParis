# Progression – validation du module minimal

## État actuel

- le runtime Wasmtime est déjà validé avec des fixtures internes
- le projet a documenté l’incompatibilité Go `wasip1` avec le linker actuel
- un module minimal de référence a été ajouté pour valider la couche `guest -> runtime -> host`

## Ce qui a été créé

- [pipeline/testdata/minimal-catalog/minimal_catalog.wat](../pipeline/testdata/minimal-catalog/minimal_catalog.wat) : module WAT de validation minimal
- [pipeline/testdata/minimal-catalog/README.md](../pipeline/testdata/minimal-catalog/README.md) : explication du motif et des étapes suivantes
- test de validation dans [internal/wasm/toolchain_spike_test.go](../internal/wasm/toolchain_spike_test.go)

## Objectif de la preuve

Le module minimal doit démontrer :

- imports autorisés strictement limités à `euromni.call`
- absence de WASI implicite
- exécution possible sur le runtime avec capability `catalog`
- blocage sans capability
- blocage si hash ou ABI est modifié

## Étapes validées

1. test d’imports du module Go WASI : confirmé incompatible (imports WASI supplémentaires)
2. test d’imports du module minimal : confirmé conforme (`euromni.call` seulement)
3. exécution runtime du module minimal : confirmée avec capability `catalog`, puis réponse ABI refusée sans capability
4. validateur d’archive PHP : confirmé pour refuser les chemins dangereux, fichiers non autorisés et paquets trop volumineux
5. parse du manifest PHP : confirmé pour les champs minimaux requis (`name`, `version`, `platform`, `shop`)
6. analyse statique de hooks PHP WooCommerce : confirmée pour repérer `add_action` / `add_filter` et dériver des capabilities attendues (`cart`, `catalog`)
7. analyse statique PrestaShop : confirmée sur `ps_emailalerts`, `ps_cashondelivery` et `ps_newproducts`, incluant `registerHook('...')`, la constante `HOOKS`, la classification `action` / `display` / `other` et l’association aux méthodes `hook…` réelles ou à `renderWidget` pour `WidgetInterface`
8. analyse Shopify : types et cibles de Discount Function et extension checkout lus depuis leurs manifests TOML réels
9. analyse Magento : nom du module, événements, plugins et arguments DI lus depuis les fichiers XML réels de Japan Tax et gRPC
10. IR v3 → Wasm : appel catalogue unique validé contre l’allowlist, capability dérivée de `wasm.Mapping`, provenance versionnée et génération WAT/Wasm reproductible
11. lowering explicite : `woocommerce_product_query` vers `catalog.product.list` et `ps_newproducts`/`displayHome` vers `catalog.product.newest`; les triggers sans règle sont refusés
12. exécution installée : Wasm généré chargé par `Runtime.LoadInstalled`, puis exécuté par `Runtime.Run` avec hash et capability `catalog` provenant de PostgreSQL
13. entrée IR dynamique : champ trigger `product_id` typé UUID validé, résolu en enveloppe ABI et transmis à `run(ptr,len)` par `Runtime.RunWithInput`
14. provenance IR : hash SHA-256 calculé sur les octets PHP analysés ; le lowering rejette un hash source différent, une version plateforme manquante ou un adaptateur qui ne correspond pas à la plateforme
15. manifeste de build v1 : lie provenance source, hash IR, ABI, capabilities dérivées et hash/taille Wasm ; sérialisation et vérification testées
16. publication de build : Wasm écrit d’abord dans le FileStore content-addressed, puis hash Wasm et manifeste JSONB enregistrés ensemble dans `modules`
17. réconciliation : hashes référencés lus dans PostgreSQL ; seuls les objets absents du registre et plus anciens que le délai de grâce peuvent être supprimés
18. ordonnanceur autonome ajouté sous `cmd/artifact-reconciler` : passage immédiat puis périodique, arrêt SIGINT/SIGTERM, retry après erreurs transitoires
19. politique provisoire : grâce 7 jours, cadence normale 24 heures, retry 5 minutes ; valeurs configurables et à réviser avant production

## Campagne non financière — 19 octobre 2026

Les essais ont été limités aux modules et fonctionnalités hors paiement :

- `go test -count=1 ./internal/pipeline -run '^(TestAnalyzeRealPlatformFixtures|TestAnalyzeRealPrestaShopFixtures/ps_emailalerts)$'` : réussi. Analyse statique des manifests Shopify Discount et checkout UI, des déclarations Magento Japan Tax et gRPC, et des hooks PrestaShop `ps_emailalerts`.
- `go test -count=1 ./internal/pipeline -run '^(TestAnalyzeRealNonPaymentWooCommerceFixture|TestAnalyzeRealPrestaShopFixtures)$'` : réussi. Analyse statique de `woocommerce-extra-product-sorting-options` (tri catalogue, GPL-3.0) et `ps_newproducts` (bloc catalogue, AFL-3.0), en plus des fixtures PrestaShop déjà présentes. La forme PrestaShop `WidgetInterface` associe `displayHome` à `renderWidget`.
- `go test -tags=integration -count=1 ./internal/pipeline -run '^TestIRGeneratedWasmRunsAsInstalledModule$'` : réussi. Le pilote WooCommerce `woocommerce_product_query` traverse analyse, lowering IR, compilation Wasm, publication en base PostgreSQL et exécution installée avec capability catalogue. La source PHP du test est une fixture synthétique, pas un plugin WooCommerce réel.

Ces tests de pipeline valident l’analyse déclarative de plusieurs références
réelles et un seul chemin complet de pilote catalogue à partir d’une source
synthétique. Les nouveaux modules réels sont épinglés dans `pipeline/fixtures/`,
mais ne passent pas encore par l’importateur EUROMNI : ils n’ont pas de
manifeste EUROMNI et le validateur d’archive actuel n’accepte pas directement
leurs paquets. QP-08 reste en cours : il faut définir des lowerings et oracles
fidèles pour le tri WooCommerce et le bloc des seuls nouveaux produits
PrestaShop, puis tester leur publication/exécution avec la même ABI. Les
fixtures WooCommerce Stripe et PrestaShop `ps_cashondelivery` ne sont pas
incluses dans cette campagne.

## Pilote Wasm PrestaShop `ps_newproducts` — 20 octobre 2026

Le lowering reconnaît le hook `displayHome`, son handler widget et les accès
aux nouveaux produits et à leurs deux réglages. Il produit un appel statique
`catalog.product.newest` ; l’hôte lit `days` (fenêtre de nouveauté PrestaShop)
et `limit` (nombre de produits configuré, défaut du module : 8) depuis le
document `shop_modules.config` associé au module et à la boutique. La
configuration n’est jamais transmise par le guest.
L’hôte rejette les fenêtres supérieures à 3 650 jours et les limites hors de
1–100 ; une fenêtre nulle produit une liste vide, comme le widget désactivé.
La requête ne retourne que les produits actifs créés dans la fenêtre, triés du
plus récent au plus ancien.

`TestLowerRealPrestaShopNewProductsFixture` vérifie l’analyse et le lowering
sur l’entrée PHP réelle épinglée et la compilation. Les tests hôte vérifient
les bornes, le type entier et le refus d’une configuration manquante ou invalide.
Le test d’intégration
`TestRealPrestaShopNewProductsWasmRunsAsInstalledModule` attribue des dates
synthétiques à des produits seedés, publie le Wasm et son manifeste dans une
base PostgreSQL d’intégration jetable, charge le module installé dans Wasmtime,
puis vérifie la réponse pour les réglages persistés et leur prise en compte
immédiate après modification.

Cette preuve ne prétend pas exécuter le PHP tiers : l’IR est une règle de
traduction explicite et bornée. Elle ne couvre ni le paquet PrestaShop Addons
amont, ni le rendu Smarty, ni l’installation du module PHP dans une instance
PrestaShop.

Les rôles `owner` et `admin` peuvent lire et modifier `days` et `limit` depuis
l’administration. Les endpoints sont authentifiés, limités à `ps_newproducts`
installé dans la boutique de la session, valident les mêmes bornes que le
runtime et ne remplacent pas les autres clés de `shop_modules.config`. L’écran
affiche un état explicite si le module n’est pas installé.

Validation ciblée :

```sh
go test -count=1 ./internal/pipeline ./internal/store ./internal/wasm
go test -tags=integration -count=1 ./internal/pipeline -run '^TestRealPrestaShopNewProductsWasmRunsAsInstalledModule$'
```

## Ingestion du ZIP amont et publication dans le CMS de test — 20 octobre 2026

Le ZIP épinglé du dépôt officiel `PrestaShop/ps_newproducts` (commit
`72e9476406319cbb91ac383121ca9967dc61f277`, SHA-256
`0f0750d8c2a1e6e12fc67169224f597f166b5c80b99cb306f53e9b339b17af77`, version
2.0.1) passe maintenant par un parcours d’ingestion spécialisé. Le validateur
refuse les chemins dangereux, liens, entrées dupliquées ou archives dépassant
les limites ; il lit seulement l’entrée PHP et `config.xml` après validation
de l’ensemble. Il ne décompresse pas sur disque et n’exécute jamais le PHP. Le
lowering strict refuse toute source qui ne correspond pas au seul comportement
`displayHome`/nouveaux produits déjà pris en charge.

`cmd/prestashop-import` peut faire un dry-run ou, avec `--install`, publier
l’artefact Wasm content-addressed, enregistrer son manifeste, installer
`ps_newproducts` et ses réglages dans la boutique `atelier-demo`, puis charger
et exécuter le module installé sous Wasmtime. Le profil Compose `module-import`,
lancé dans le projet `euromni-cms-test`, isole l’accès à la base PostgreSQL
locale et conserve les artefacts dans un volume privé séparé. La commande exige
aussi l’environnement `development` et rejette les hôtes de base non locaux.

Le parcours a été exécuté avec succès dans le CMS de test : version amont 2.0.1,
`days=30`, `limit=8`, manifeste et hachage publiés, puis appel catalogue Wasm
exécuté. Le test `TestUpstreamPrestaShopArchivePublishesAndRunsInIsolatedCMS`
reproduit ce chemin dans une base PostgreSQL jetable. La valeur
`euromni-test-adapter` de provenance n’affirme pas qu’une version réelle de
PrestaShop est installée : il s’agit d’un ZIP source de dépôt officiel, et non
du paquet PrestaShop Addons. Le rendu Smarty, la compatibilité sur une instance
PrestaShop et l’activation de la vitrine restent hors de cette preuve.

## Pilote natif isolé transport/commande : Selfnamed WooCommerce

Le plugin officiel « Selfnamed: Cosmetics on demand » est publié en version
`2.1.0`, sous GPLv3 ou ultérieure (archive SHA-256
`f6bc806ec3f33e67edc085b46703e02666e431fe3ff351056928691bd91e7e5c`;
[fiche WordPress.org](https://wordpress.org/plugins/selfnamed-cosmetics-on-demand/),
[readme](https://plugins.svn.wordpress.org/selfnamed-cosmetics-on-demand/trunk/readme.txt)).
Son API est configurée par une option WordPress ; l’essai garde le code du
plugin inchangé et dirige cette URL vers un mock exclusivement local.

`./pipeline/selfnamed-pilot/run.sh` télécharge les archives WooCommerce `10.9.0`
(SHA-256 `d19a480cd2d936165c26a487a52723c527091fd0410353f45e06477434c3dccc`)
et Selfnamed depuis WordPress.org et valide leurs SHA-256 avant extraction. La
stack utilise WordPress `6.9.2`, PHP `8.3`, WooCommerce `10.9.0`,
MariaDB `11.8.3` et des images OCI épinglées par digest. Elle tourne sous
Podman sur un réseau `internal`, sans port publié ; plugin et WooCommerce sont
montés en lecture seule, la base est éphémère et l’ensemble est supprimé à la
fin. Elle ne contient aucun credential Selfnamed : un bearer token fictif est
accepté seulement par le mock. Les commandes et coordonnées sont synthétiques,
les emails WooCommerce désactivés et aucune connexion au service externe n’est
permise.

**Résultat :** le plugin réel s’active dans WordPress, récupère les données
synthétiques d’un produit par le mock et soumet une commande avec groupe de
livraison ; le test confirme les champs attendus, l’identifiant et le statut
retournés, ainsi que l’absence de doublon lors d’une deuxième synchronisation.
C’est une preuve d’intégration native WordPress/WooCommerce, pas une importation
ou traduction EUROMNI, pas un test Wasm et pas une véritable commande,
expédition ou synchronisation Selfnamed. Toute expérience ultérieure avec des
credentials réels est reportée à une décision distincte et devra utiliser des
credentials de test, des données synthétiques et une sortie réseau explicitement
autorisée et limitée.

Les fixtures PrestaShop sont des snapshots des dépôts officiels GitHub, épinglés aux commits `359cd5c86c498964d9c38c97db9fcd13d73be3fe`, `d1bac20a8b5a4664759db5c6775be9b426ad4b62` et `72e9476406319cbb91ac383121ca9967dc61f277`. Elles valident l'analyse statique des fichiers PHP réels. Le ZIP source `ps_newproducts` passe maintenant par l’ingestion bornée décrite plus haut ; cela ne couvre pas le paquet de distribution PrestaShop Addons.

La référence de compatibilité est la documentation développeur PrestaShop [`9.x`](https://devdocs.prestashop-project.org/9/modules/concepts/hooks/), qui décrit l’enregistrement, les familles `action` / `display` et les méthodes handler. Les fixtures montrent une exception (`paymentOptions`) qui reste sans sémantique déduite. Aucune documentation développeur publique `10.x` n’a été trouvée au 13 octobre 2026 ; PS10 n’est donc pas déclaré compatible.

Les références Shopify et Magento sont épinglées et conservées dans [pipeline/fixtures/real-modules](../pipeline/fixtures/real-modules) avec leurs licences. `TestAnalyzeRealPlatformFixtures` les vérifie depuis le dépôt, sans réseau ni dépendance à `/tmp`. Les tests n’exécutent pas les apps, extensions ou modules ; ils valident uniquement la lecture de leurs manifests et configurations.

La tranche IR v3 compile les entrées statiques et sait résoudre des références dynamiques UUID et entières. Sa provenance comprend les hashes PHP et archive, plus versions plateforme/adaptateur. `PublishModuleBuild` stocke le Wasm puis enregistre hash et manifeste JSONB dans une mise à jour SQL atomique. Le binaire autonome `cmd/artifact-reconciler` lance la réconciliation immédiatement puis à chaque intervalle configuré ; ses défauts provisoires sont 7 jours de grâce et 24 heures de cadence, à revoir avant production. Son test de scheduler couvre le cycle/réessai. Deux règles de lowering catalogue sont validées, WooCommerce `woocommerce_product_query` et PrestaShop `ps_newproducts`; pour ce dernier, les réglages admin owner/admin, l’ingestion du ZIP source, la publication et l’exécution dans le CMS de test sont démontrées.

## Prochaines étapes prévisibles

1. comparer le comportement du widget rendu avec les données et la configuration PrestaShop d’origine ;
2. examiner le paquet PrestaShop Addons et définir une preuve distincte de compatibilité avec une instance PrestaShop réelle ;
3. décider et implémenter le rendu de widget EUROMNI, sans interpréter ou exécuter de template Smarty arbitraire.
5. définir des règles explicites de lowering pour d’autres triggers et vérifier leur sémantique
6. revoir les valeurs provisoires en vue de la production et planifier la réconciliation dans le cycle du CMS
7. ajouter conditions et plusieurs opérations seulement après définition de leur contrat

## Remarque

Le module minimal n’est pas un plugin métier complet ; c’est une preuve de sécurité et de compatibilité. Les analyseurs des quatre plateformes sont des preuves de lecture statique ciblée, pas des modules traduits ni exécutables dans le runtime.
