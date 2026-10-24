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
16. publication de build : Wasm écrit d’abord dans le FileStore content-addressed ; les métadonnées sont ensuite enregistrées transactionnellement avec le build courant
17. réconciliation : hashes référencés lus dans PostgreSQL ; seuls les objets absents du registre et plus anciens que le délai de grâce peuvent être supprimés
18. ordonnanceur autonome ajouté sous `cmd/artifact-reconciler` : passage immédiat puis périodique, arrêt SIGINT/SIGTERM, retry après erreurs transitoires
19. politique provisoire : grâce 7 jours, cadence normale 24 heures, retry 5 minutes ; valeurs configurables et à réviser avant production
20. contrat de package distribuable : validation stricte du manifeste, de l’IR JSON canonique et du Wasm, avec vérification des hashes, versions approuvées, capabilities, signatures ABI et imports/exports ; l’intégrité ne prouve pas l’authenticité du producteur
21. publication sécurisée par le même validateur de package avant écriture dans le FileStore ; un Wasm avec import supplémentaire est rejeté avant tout stockage
22. primitives expérimentales Ed25519 : enveloppe détachée liée aux digests des trois blobs ; clé de confiance fournie hors package ; vérification avant validation structurelle ; aucune intégration à un service, à l'import, à la publication ni à la promotion

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

## Pilote Wasm WooCommerce `woocommerce-extra-product-sorting-options`

Le build est limité à l’archive officielle épinglée au commit
`bc5829d4d2383ae956869386eb40e772cf4a76a0` (version `2.11.0`, GPL-3.0,
archive SHA-256
`65a239244a526bd98fb512a2ac5c5157379a9fcf8ffd2148c6efec007070144e`,
source SHA-256
`aaf1a1a13ec7e0465a4c451b952aba79626be3288104c18122c47bf01c81306f`). Its
header declares WooCommerce 3.9.4 minimum and 9.7.1 tested-up-to; the latter is
upstream metadata, not a test of a live WooCommerce instance.
L’importeur refuse tout autre hash, borne et valide la structure ZIP, lit
l’entrée PHP sans extraction et vérifie l’identité/version du module.

La règle de lowering ne traduit que le hook
`woocommerce_get_catalog_ordering_args` pour les options `alphabetical` et
`reverse_alpha`. Le guest appelle uniquement `catalog.product.sort` sous la
capacité `catalog`; le host trie le nom traduit pour la locale reçue
(`fr-FR` ou `en-GB`) avec des critères déterministes de départage. Les options
`by_stock`, `review_count`, `on_sale_first`, les hooks de configuration et les
autres versions source restent explicitement non prises en charge. Aucun code
PHP n’est exécuté.

`TestBuildWooCommerceSortingArchiveUsesPinnedSource` vérifie le hash, la
provenance, le hook, l’IR dynamique et l’import Wasm limité à `euromni.call`.
`TestCatalogProductSortMatchesSupportedWooCommerceOptions` valide l’oracle de
tri. Le test d’intégration
`TestRealWooCommerceSortingArchiveRunsAsPublishedWasm` importe l’archive,
publie Wasm et manifeste dans PostgreSQL jetable, puis exécute les deux options
dans Wasmtime et compare les produits ordonnés à l’oracle français.

Cette preuve QP-08 n’est ni une installation du plugin dans WordPress ni une
compatibilité générale WooCommerce ; elle n’active pas de gestion de cycle de
vie WooCommerce dans l’admin EUROMNI.

Les rôles `owner` et `admin` peuvent lire et modifier `days` et `limit` depuis
l’administration. Les endpoints sont authentifiés, limités à `ps_newproducts`
installé dans la boutique de la session, valident les mêmes bornes que le
runtime et ne remplacent pas les autres clés de `shop_modules.config`. L’écran
affiche un état explicite si le module n’est pas installé.

Validation ciblée :

```sh
go test -count=1 ./internal/pipeline ./internal/store ./internal/wasm
go test -tags=integration -count=1 ./internal/pipeline -run '^TestRealPrestaShopNewProductsWasmRunsAsInstalledModule$'
go test -tags=integration -count=1 ./internal/pipeline -run '^TestRealWooCommerceSortingArchiveRunsAsPublishedWasm$'
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

## Rendu du widget nouveautés dans la vitrine EUROMNI

La gestion bornée du registre et de l’installation est décrite dans
[module-management.md](./module-management.md).

La vitrine dispose maintenant d’un endpoint public `GET /v1/widgets/new-products`
qui exécute le module installé et renvoie les produits issus de sa réponse ABI.
Le CMS configure ce runner uniquement avec un store SQL et
`EUROMNI_ARTIFACT_DIR`; le profil Compose `cms` partage le volume privé
d’artefacts avec le profil d’import.

Le Web Component affiche les produits avec les cartes, images, traductions et
prix déjà utilisés par le catalogue. Une réponse vide masque la section ;
une erreur du service est indiquée sans interrompre le catalogue. La section
est également actualisée lors d’un changement de langue et masquée sur une fiche
produit. Les tests vérifient l’endpoint et la réponse du runner sur le ZIP
publié.

La validation visuelle Playwright sur le CMS de test a vérifié les cinq cartes,
images et prix, la navigation vers une fiche et son retour, le filtre de
catégorie, le changement de langue, la sélection de variante et l’ouverture puis
la fermeture du panier. À 390 px, les cinq images chargent et la page ne déborde
pas horizontalement. Des réponses simulées confirment que la liste vide masque
le widget et qu’une erreur 503 affiche son message sans masquer le catalogue.
Cette vérification a révélé puis corrigé un sélecteur de statut partagé qui
masquait le message d’erreur ; aucun panier n’a été soumis et aucune commande
n’a été créée.

## Pilote natif isolé transport/commande : Selfnamed WooCommerce

Le plugin officiel « Selfnamed: Cosmetics on demand » est publié en version
`2.1.0`, sous GPLv3 ou ultérieure. Pour que le pilote corresponde à la boutique
locale, le test utilise la version `2.0.19` (archive SHA-256
`6e464f7130f905d35686c1ded1faeadc010da784c8a18222b354b7f9be87809f`;
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
Le pilote couvre maintenant aussi les notifications `PRODUCT`, `ORDER`
(`SHIPPED`, `CANCELLED`) et `STORE`, le rejet d’un token de callback invalide,
la séparation d’un panier mixte et les calculs de livraison en WordPress /
WooCommerce pour six zones synthétiques : FR/BE, DE, GB et US, avec des options
standard et express et des tranches de poids jusqu’à 15 kg. Les assertions
couvrent huit tranches françaises, des pays avec différentes offres, deux
options simultanées aux États-Unis, la limite supérieure d’une offre express,
un poids hors barème et un pays non desservi. La matrice du mock est
[`shipping-zones.json`](../pipeline/selfnamed-pilot/shipping-zones.json) ;
elle reprend l’organisation par pays, service et poids de l’[article d’aide
Selfnamed](https://help.selfnamed.com/en/articles/10009962-what-is-selfnamed-s-shipping-time-and-costs)
comme exemple de scénarios, mais les montants sont fictifs et ne représentent
pas les prix publics ou une boutique réelle. Le token du profil retourné par le
mock correspond au token synthétique utilisé pour le callback.
Le scénario à 1 001 g n’obtient pas de tarif avec le calcul flottant kg→g du
plugin 2.0.19 ; le test confirme 1 000 g et 1 002 g, et cette limite reste à
traiter.
Le scénario de pagination fait répondre le mock catalogue sur deux pages de
deux produits, et l’endpoint des zones sur deux pages de trois entrées. Dans
WooCommerce, la page 1 importe les deux premiers produits ; la page 2 est
requise pour créer le troisième, puis les deux pages sont rejouées pour vérifier
l’idempotence. `create_all_products(page, size)` ne suit pas les pages à lui
seul. À l’inverse, `get_all_shipping_zones()` parcourt et agrège les deux pages ;
le test vérifie les six identifiants en ordre et confirme les appels aux deux
pages pour chaque récupération non mise en cache.
C’est une preuve d’intégration native WordPress/WooCommerce, pas une importation
ou traduction EUROMNI, pas un test Wasm et pas une véritable commande,
expédition ou synchronisation Selfnamed. Toute expérience ultérieure avec des
credentials réels est reportée à une décision distincte et devra utiliser des
credentials de test, des données synthétiques et une sortie réseau explicitement
autorisée et limitée.

### Caractérisation du mapping catalogue Selfnamed

La matrice de fixtures synthétiques [`catalog-products.json`](../pipeline/selfnamed-pilot/catalog-products.json)
alimente maintenant aussi le mock catalogue du pilote. Elle couvre le cas
nominal, le hors stock avec poids de 1 001 g et tags, puis un prix décimal
minimal avec description vide. Le test oracle Go
`TestSelfnamedCatalogProductMappingCharacterization` fige les champs observables
depuis la source du plugin : création en brouillon, identifiant fournisseur,
nom, description, prix recopié, gestion de stock désactivée, statut en stock,
volume et conversion des poids en kilogrammes. Le prix `17.40` ressort comme
`17.4` et `0.10` comme `0.1` : le mock JSON et WooCommerce éliminent les zéros
décimaux terminaux. Le pilote WordPress/WooCommerce contrôle les résultats,
les tags, l’absence de doublon après réimport et le refus HTTP 401. Le flux
commande/livraison du pilote existant passe également.

Cette tranche caractérise le comportement ; elle n’est ni un mapper de production
EUROMNI ni un build Wasm. Le pilote épingle désormais la même version Selfnamed
`2.0.19` que la copie persistante, et vérifie que l’archive téléchargée déclare
bien cette version. Trois cas sont couverts : cas nominal, rupture de stock
avec poids non entier en kg et tags, et prix décimal minimal avec description
vide. Un refus HTTP 401 est aussi simulé ; aucune image n’est fournie, donc
aucune requête de téléchargement ne peut partir. Les tests vérifiés sont
`go test ./internal/pipeline -count=1` et
`./pipeline/selfnamed-pilot/run.sh`.

### Première prévisualisation Wasm catalogue Selfnamed

Le guest Rust `no_std` reçoit le JSON produit par le trigger IR et calcule le
DTO de prévisualisation. Il appelle ensuite
`catalog.supplier.product.preview.result` : l’hôte valide le DTO et le renvoie,
sans lecture ni écriture de table. La capability `catalog` est allowlistée,
mais ces appels n’ont aucun accès réseau, filesystem ou catalogue persistant.
L’entrée fournisseur est bornée à 8 KiB ; les images et champs hors périmètre
sont ignorés. Le prix est normalisé comme dans la caractérisation WooCommerce
(`17.40` → `17.4`, sans supprimer les zéros d’un entier). La réponse présente
le produit comme brouillon et le poids en grammes ; devise, locale et unité du
volume sont encore inconnues et apparaissent dans `unresolved`. Le mapper Go
est conservé comme oracle natif ; le test Wasmtime compare le résultat du guest
aux mêmes fixtures.

`TestSelfnamedProductPreviewWasmRunsWithoutPersistence` compile le guest Rust
épinglé avec Cargo `--frozen`, vérifie que l’import Wasm est limité à
`euromni.call`, publie puis charge le module installé dans Wasmtime et compare
la sortie attendue pour les trois fixtures partagées avec le pilote natif :
nominal, rupture de stock avec poids
de 1 001 g et tags, et prix `0.10` avec description vide. Le nombre de produits
en base reste identique après les trois appels. Le même module Wasmtime rejette
aussi un JSON produit malformé et une entrée produit dépassant 8 KiB avec
l’erreur ABI `invalid` ; le nombre de produits persistant reste inchangé après
les appels valides et invalides. Les tests d’hôte couvrent aussi l’absence de
capability. `CompileSelfnamedPreviewIRToWasm` n’accepte que la forme IR prévue,
construit le guest depuis le crate verrouillé, ajoute les sections custom du
hash exact de l’IR et de l’identité du compilateur approuvé, puis vérifie
imports/exports Wasm. `TestCompileSelfnamedPreviewIRToWasmIsReproducibleAndBoundToIR`
compare deux builds indépendants octet pour octet et vérifie le manifeste ainsi
que le refus d’un IR différent.

Cette tranche Wasm reste limitée à la prévisualisation catalogue : aucun accès
réseau, pagination, commande, livraison ou modification de produit n’entre dans
ce call. La restriction d’import à `euromni.call` et les trois sorties oracle
restent vérifiées ; devise, locale et unité de volume restent non résolues. Le
travail ne vise pas la traduction générale du plugin PHP.

Cette preuve valide un mapping de prévisualisation exécuté dans le guest Rust,
une validation ABI hôte et un aperçu non persistant ; elle ne valide pas la
traduction de PHP ni l’équivalence complète du plugin Selfnamed. La
caractérisation du plugin natif, le mapper hôte oracle et le guest Rust sont des
implémentations distinctes. Le compilateur IR générique délègue explicitement
les IR Selfnamed à la fonction dédiée ; les autres IR conservent leur chemin
WAT actuel.
Devise, locale et unité du volume doivent encore être décidées ;
images/téléchargements, catégories, mise à jour de produits existants et
publication active restent hors périmètre.

### Suite recommandée pour le chantier Wasm

L’objectif immédiat est une preuve verticale limitée, pas la traduction
générale du plugin ni une décision sur son mode de distribution.

1. Terminé : définir le contrat métier pur de prévisualisation. Les trois
   fixtures Selfnamed et leur oracle restent la référence.
2. Terminé : déplacer le mapping dans un guest Rust `no_std` spécialisé. Le
   guest fait un seul appel `catalog.supplier.product.preview.result`, sans
   accès réseau, pagination, commande, livraison ou écriture CMS.
3. Terminé pour le prototype : comparer les trois sorties, rejeter les entrées
   malformées ou supérieures à 8 KiB, vérifier l’unique import `euromni.call`
   et l’absence de mutation persistante.
4. Terminé : garder l’hôte comme frontière CMS ; il valide le DTO guest et
   reste seul responsable de l’affichage ou de toute future action d’import.
5. Terminé pour le prototype : compiler le guest Rust avec toolchain et
   dépendances verrouillées, lier le hash exact de l’IR et l’identité du
   compilateur au Wasm, choisir le compilateur du manifeste selon l’artefact,
   puis démontrer deux builds identiques et l’exécution de l’artefact publié.
6. Contrat du build distribuable cadré : trois blobs transport-indépendants
   (`manifest.json`, `module.ir.json`, `module.wasm`), validateur strict et
   validation pré-publication. Le prototype d'authenticité Ed25519 est
   implémenté : signature détachée sur les trois digests, clé publique
   identifiée par empreinte et approuvée hors package. Le périmètre a été
   approuvé pour ce prototype ; provisionnement/rotation/révocation, limites
   de transfert et politique de production restent ouverts. Aucun service de
   signature ou flux de réception n'est connecté. Le choix entre catalogue
   central et traitement local est différé.

Devise, locale et unité de volume restent non résolues : ne pas les inventer
pour faire passer l’oracle. La cible est une transformation métier contrôlée et
reproductible, pas une promesse de performance ni l’exécution/traduction du PHP
source.

## Copie locale WooCommerce de référence — 23 octobre 2026

Pour compléter le pilote synthétique, une copie exportée d’une boutique
WooCommerce de l’utilisateur a été restaurée dans une instance WordPress locale
persistante via Duplicator Lite 5.0.4. Le ZIP de 439 684 531 octets a passé
`unzip -tq`, et `installer.php` a passé `php -l` dans le conteneur avant
l’installation. Duplicator a terminé l’extraction et la migration sur
WordPress 6.9.2 / PHP 8.3 et MariaDB 11.8.8 ; les résultats affichés pour les
fichiers, la base, le search/replace et les plugins sont au vert. L’URL source
`https://comte-bastian.fr/shop` a été remplacée par `http://127.0.0.1:18083`.

La sandbox `euromni-woo-sandbox` est séparée du CMS EUROMNI, placée sur un réseau
Podman `internal`, avec accès web uniquement via `127.0.0.1:18083`, volumes
persistants et redémarrage rootless configuré. Les connexions TCP sortantes ont
été testées comme bloquées ; `WP_HTTP_BLOCK_EXTERNAL`, WP-Cron et les mises à
jour automatiques sont désactivés. L’utilisateur a déclaré l’export assaini,
sans audit indépendant de son contenu ; garder les données confidentielles et
ne pas ajouter de credentials au dépôt ou aux notes.

**Nettoyage de l’installateur :** après l’écran « Step 2 of 2: Test Site » et les
validations au vert, les fichiers générés `installer.php`, `dup-installer/`,
le journal de démarrage et l’archive copiée dans le document-root ont été
retirés manuellement de la sandbox ; l’export source reste intact. L’accès à
`/wp-admin/` est maintenant ouvert dans une session authentifiée avec le compte
administrateur local temporaire `sandbox-review`, créé car les identifiants du
compte restauré ne sont pas disponibles. L’utilisateur doit remplacer son mot
de passe depuis son profil ; aucun mot de passe n’est consigné. Maintenir
l’isolement réseau, les paiements, e-mails, cron et intégrations externes
désactivés.

### Valeur pour la validation des modules

Cette boutique devient un oracle comportemental local pour confronter le plugin
Selfnamed original au pilote mock précédent et aux hypothèses de traduction :
inventaire réel des versions/plugins, flux de synchronisation produits, données
de livraison et export de commande, hooks, appels API, idempotence et effets de
bord. La suite recommandée est une inspection statique/lecture seule, puis des
tests avec données synthétiques et mock local, en observant les requêtes sans
autoriser de sortie réseau. Prioriser une capacité non financière et bornée
(par exemple la lecture/synchronisation catalogue) avant de considérer
l’export de commandes.

La copie locale ne prouve pas l’import ou l’exécution EUROMNI : le pilote natif
Selfnamed avec mock, le pilote Wasm WooCommerce de tri et la traduction du ZIP
PrestaShop `ps_newproducts` sont trois voies distinctes. La traduction du plugin
Selfnamed en Wasm reste à concevoir et ne doit pas être supposée générique.

Un décompte local, sans lecture de titres ni de credentials, a trouvé 17 produits
WooCommerce dont 16 portent le marqueur `_selfnamed_product_id`. Le plugin copié
est en version `2.0.19` et requiert une clé API Bearer pour lire
`GET https://ds-api.selfnamed.com/products`; il crée les nouveaux produits en
brouillon et tente de télécharger séparément leurs images. Aucun appel distant
n’a été fait : l’API n’est pas nécessaire pour commencer l’inspection des
produits déjà restaurés, et l’isolement réseau doit rester actif pendant ces
premiers essais.

Les fixtures PrestaShop sont des snapshots des dépôts officiels GitHub, épinglés aux commits `359cd5c86c498964d9c38c97db9fcd13d73be3fe`, `d1bac20a8b5a4664759db5c6775be9b426ad4b62` et `72e9476406319cbb91ac383121ca9967dc61f277`. Elles valident l'analyse statique des fichiers PHP réels. Le ZIP source `ps_newproducts` passe maintenant par l’ingestion bornée décrite plus haut ; cela ne couvre pas le paquet de distribution PrestaShop Addons.

La référence de compatibilité est la documentation développeur PrestaShop [`9.x`](https://devdocs.prestashop-project.org/9/modules/concepts/hooks/), qui décrit l’enregistrement, les familles `action` / `display` et les méthodes handler. Les fixtures montrent une exception (`paymentOptions`) qui reste sans sémantique déduite. Aucune documentation développeur publique `10.x` n’a été trouvée au 13 octobre 2026 ; PS10 n’est donc pas déclaré compatible.

Les références Shopify et Magento sont épinglées et conservées dans [pipeline/fixtures/real-modules](../pipeline/fixtures/real-modules) avec leurs licences. `TestAnalyzeRealPlatformFixtures` les vérifie depuis le dépôt, sans réseau ni dépendance à `/tmp`. Les tests n’exécutent pas les apps, extensions ou modules ; ils valident uniquement la lecture de leurs manifests et configurations.

La tranche IR v3 compile les entrées statiques et sait résoudre des références dynamiques UUID, entières et chaînes déclarées par le trigger. Sa provenance comprend les hashes PHP et archive, plus versions plateforme/adaptateur. `PublishModuleBuild` stocke le Wasm puis enregistre hash et manifeste JSONB dans une mise à jour SQL atomique. Le binaire autonome `cmd/artifact-reconciler` lance la réconciliation immédiatement puis à chaque intervalle configuré ; ses défauts provisoires sont 7 jours de grâce et 24 heures de cadence, à revoir avant production. Son test de scheduler couvre le cycle/réessai. Les pilotes Wasm validés couvrent WooCommerce `woocommerce_product_query` (synthétique), WooCommerce `woocommerce_get_catalog_ordering_args` (archive réelle épinglée, deux tris publiés) et PrestaShop `ps_newproducts`; pour ce dernier, les réglages admin owner/admin, l’ingestion du ZIP source, la publication et l’exécution dans le CMS de test sont démontrées.

## Versionnement et pinning des builds — QP-02

Les migrations PostgreSQL `0016_immutable_module_builds.sql` et MariaDB
`0010_immutable_module_builds.sql` conservent les builds historiques dans
`module_builds`, initialisent le pointeur courant depuis les métadonnées
existantes et associent les installations aux builds disponibles. Une
publication ultérieure avance le pointeur du module sans modifier les
installations déjà épinglées. La première publication lie les installations
historiques non versionnées à un build afin de préserver leur capacité d’exécution.

`RegisterModuleBuild` enregistre le build et met à jour le pointeur courant dans
une transaction ; `RegisterModuleBuildOnly` publie un candidat dans le registre
sans modifier les pins des installations, y compris celles encore sans pin.
Un hash déjà enregistré avec une provenance différente est refusé. La
validation du manifeste exige les hashes source/IR/Wasm, versions et
identifiants requis, ABI, capacités triées sans doublons et refuse les champs
JSON inconnus. L’IR v3, le manifeste v1, les versions d’adaptateur PrestaShop
et WooCommerce 1.0.0, et les identifiants de compilateur Wasmtime approuvés
sont centralisés dans `internal/buildmeta`. Le nouveau compilateur ajoute
l’empreinte SHA-256 de l’IR dans une section Wasm personnalisée ; cette
empreinte rend distincts les artefacts de provenance différente sans modifier
le code invité. `PromoteInstalledModuleBuild` fournit une promotion explicite et
transactionnelle pour une installation donnée. L’inventaire et le runtime lisent
le build de l’installation, tandis que les nouvelles installations choisissent
le build courant ou le hash explicitement sélectionné. Le réconciliateur conserve
les artefacts des builds courants et des installations qui les épinglent. Le
test PostgreSQL vérifie la liaison initiale des installations, la conservation
des pins de deux boutiques après une publication, la promotion isolée d’une
boutique et l’absence de mutation après un conflit
de provenance. `TestMariaDBMigrationAndCommerceFlow` a également appliqué
successivement la baseline puis la migration incrémentale et validé le parcours
commerce MariaDB. Les versions approuvées sont désormais verrouillées. Dans
le CMS local `euromni-cms-test`, le mode `--publish-only` a rendu un build
candidat visible et promotable sans changer le pin installé ; l’API admin l’a
ensuite promu pour la boutique de démonstration et l’endpoint du widget a
exécuté ce build avec succès.

## Prochaines étapes prévisibles

1. comparer plus largement le widget avec les données et la configuration PrestaShop d’origine ;
2. examiner le paquet PrestaShop Addons et définir une preuve distincte de compatibilité avec une instance PrestaShop réelle ;
3. tester le plugin de tri WooCommerce sur une instance réelle et décider si d’autres options limitées méritent un lowering ;
4. définir des règles explicites de lowering pour d’autres triggers et vérifier leur sémantique ;
5. revoir les valeurs provisoires en vue de la production et planifier la réconciliation dans le cycle du CMS ;
6. ajouter conditions et plusieurs opérations seulement après définition de leur contrat.

## Remarque

Le module minimal n’est pas un plugin métier complet ; c’est une preuve de sécurité et de compatibilité. Les analyseurs des quatre plateformes sont des preuves de lecture statique ciblée, pas des modules traduits ni exécutables dans le runtime.
