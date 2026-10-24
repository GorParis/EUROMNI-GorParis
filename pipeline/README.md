# Pipeline de modules EUROMNI

Le pilotage, les critères de passage et les preuves attendues sont suivis dans
le [plan directeur qualité](../docs/plan-directeur-qualite.md).

## Documentation plateforme

Ces références sont les sources de contrat des analyseurs et des tests ; la
version déclarée par chaque package reste prioritaire sur une page `latest`.

- **WooCommerce** : [documentation développeur](https://developer.woocommerce.com/docs/) et [référence du code](https://woocommerce.github.io/code-reference/). Pour les hooks génériques WordPress, voir aussi le [guide des hooks](https://developer.wordpress.org/plugins/hooks/).
- **PrestaShop** : [modules 9.x](https://devdocs.prestashop-project.org/9/modules/) et [hooks 9.x](https://devdocs.prestashop-project.org/9/modules/concepts/hooks/). Au 13 octobre 2026, aucune documentation développeur publique `10.x` n’est disponible ; ne pas inférer sa compatibilité depuis 9.x.
- **Shopify** : [Shopify Functions](https://shopify.dev/docs/apps/build/functions), [référence des Functions et versions d’API](https://shopify.dev/docs/api/functions/latest) et [configuration des app extensions](https://shopify.dev/docs/apps/build/app-extensions/configure-app-extensions). Les Functions définissent leurs entrées par cible et requête GraphQL ; leur sortie est propre à l’API de Function.
- **Magento / Adobe Commerce** : [composants](https://developer.adobe.com/commerce/php/development/components/), [événements et observers](https://developer.adobe.com/commerce/php/development/components/events-and-observers/), [plugins/intercepteurs](https://developer.adobe.com/commerce/php/development/components/plugins) et [service contracts](https://developer.adobe.com/commerce/php/development/components/service-contracts/).

Les adaptateurs de plateforme sont des composants du pipeline Go EUROMNI : ces
références ne requièrent ni quatre services ni quatre conteneurs. Seul le guest
Wasm est exécuté dans le runtime sandboxé ; le code source externe n’est jamais
chargé comme PHP dans le processus CMS.

## Objectif

Les références couvrent maintenant quatre plateformes :

- WooCommerce, WooCommerce Stripe Gateway et le module non financier `woocommerce-extra-product-sorting-options`
- PrestaShop `ps_emailalerts`, `ps_cashondelivery` et le module non financier `ps_newproducts`
- Shopify Discount Function et checkout UI extension
- Magento Japan Tax et gRPC

Ils peuvent être téléchargés avec :

```sh
./pipeline/fetch_real_module_fixtures.sh
```

Les archives et sources sont stockées dans `pipeline/fixtures/` pour des validations de source, de hooks et de capabilities, sans les exécuter dans le runtime EUROMNI. Les ZIP PrestaShop sont des snapshots de dépôts officiels épinglés, pas des packages de distribution Addons.

Les références Shopify et Magento sont intégrées dans `pipeline/fixtures/real-modules/`
avec leurs fichiers de licence :

```sh
./pipeline/fetch_platform_reference_modules.sh
go test ./internal/pipeline -run TestAnalyzeRealPlatformFixtures
```

L’analyse Shopify lit les types et cibles déclarés en TOML ; celle de Magento lit `module.xml`, les événements et les déclarations DI (plugins, préférences, arguments). Ces preuves de configuration ne signifient ni analyse complète du code, ni traduction en Wasm, ni exécution des modules.

L’adaptateur PrestaShop prend la documentation développeur `9.x` comme référence
actuelle : elle distingue les hooks `action…` et `display…`, enregistrés puis
traités par des méthodes publiques `hook…`. Les fixtures réelles confirment aussi
des hooks hors de ces deux familles, comme `paymentOptions` ; ils restent classés
`other` plutôt que de recevoir une sémantique supposée. Au 13 octobre 2026, aucune
documentation développeur publique `10.x` n’est publiée ; cette compatibilité
reste à vérifier quand les contrats 10.x seront disponibles.

Transformer des extensions WooCommerce, PrestaShop et autres en modules Wasm
isolés, reliés au cœur uniquement par l’ABI versionnée `euromni.abi.v1`.
L’entrée PHP est une source non fiable : elle n’est jamais exécutée dans le
processus EUROMNI ni en production.

La traduction générale n’est pas encore implémentée. Le dépôt fournit déjà
le runtime Wasmtime, le contrat d’appel hôte, un stockage local d’artefacts
content-addressed (`internal/artifact`) et une première analyse statique
expérimentale pour WooCommerce, PrestaShop, Shopify et Magento. L’upload HTTP,
l’analyse générale et la traduction des autres comportements restent à construire.

## Première tranche IR → Wasm

`internal/pipeline/ir.go` définit `euromni.ir.v3` : un module source, un
déclencheur déclaré, un appel hôte et sa provenance. La provenance contient le
SHA-256 des octets PHP analysés et celui de l’archive complète, la plateforme et
sa version cible, la version du module si connue, l’identifiant/version
d’adaptateur et une référence source facultative. `AnalyzePHPSource` calcule le
hash PHP ; `NewPackageProvenance` calcule et vérifie les deux digests.
`NewBuildManifest` lie provenance, hash de l’IR, ABI, capabilities dérivées et
hash/taille du Wasm.

`PublishModuleBuild` écrit d’abord le Wasm dans le stockage content-addressed,
puis met à jour en une seule instruction SQL `modules.wasm_sha256` et
`modules.build_manifest` (`jsonb`). Si l’écriture DB échoue, le Wasm reste
immuable mais non référencé. `ReconcileModuleArtifacts` récupère les hashes de
tous les modules en base et ne supprime que les fichiers sans référence et plus
vieux que le délai de grâce fourni ; les `Put` répétés rafraîchissent leur âge.
Valeurs provisoires à réviser avant production : `DefaultArtifactOrphanGracePeriod`
= 7 jours et `DefaultArtifactReconciliationInterval` = 24 heures. La cadence est
un objectif d’exploitation ; l’API n’est pas encore planifiée dans le cycle du CMS.

Le processus autonome correspondant est `cmd/artifact-reconciler` ; son lancement
et sa configuration sont décrits dans le README racine. Il ne crée pas de
conteneur et doit partager le même répertoire FileStore que le processus qui publie
les builds.

Une valeur d’entrée peut être un littéral
ou une référence typée à un champ du payload trigger (`string`, `uuid` ou
`integer`).
`ValidateIR` vérifie le déclencheur, l’appel allowlisté, son schéma et ses
identifiants ; `BuildIRCallEnvelope` valide le payload, refuse champs inconnus
ou absents et construit l’enveloppe ABI. Les capabilities sont dérivées de
`wasm.Mapping`, jamais fournies par l’IR.

`CompileIRToWAT` puis `CompileIRToWasm` génèrent un guest déterministe avec le
seul import `euromni.call`. Les appels statiques exportent `run()` ; les appels
dynamiques exportent `run(ptr,len)`, alimenté par `Runtime.RunWithInput` après
validation de l’enveloppe et copie bornée en mémoire linéaire.

Un spike séparé compile une fixture Rust `no_std` pour
`wasm32-unknown-unknown` avec Rust `1.99.1`, puis inspecte tous les imports et
exécute l'appel ABI dans Wasmtime. Il s'agit d'une candidate expérimentale, pas
d'une toolchain de production ni d'une traduction générique de code métier.
Voir la [matrice de portabilité](../docs/portability.md).

Deux règles explicites sont actuellement disponibles : le hook WooCommerce
`woocommerce_product_query` vers `catalog.product.list`, et le widget
PrestaShop `ps_newproducts` sur `displayHome` vers
`catalog.product.newest`. Cette dernière ne s’applique qu’à l’analyse qui
reconnaît l’appel de nouveaux produits et ses réglages de fenêtre et de nombre.
À l’exécution, l’hôte lit `days` et `limit` depuis la configuration persistée
de `shop_modules` ; ils ne sont pas fournis par le guest. `days: 0` désactive
le widget, et l’hôte borne les requêtes à 3 650 jours et 100 produits. L’appel
filtre les produits actifs sur `created_at`, du plus récent au plus ancien.

Le prototype `catalog.supplier.product.preview` exerce aussi un trigger IR
dynamique avec les trois fixtures Selfnamed synthétiques, communes au pilote
natif et au test oracle. Le test d’intégration compile, publie et charge le
module Wasm dans Wasmtime, puis vérifie chaque projection de prévisualisation
et l’absence d’écriture catalogue. Ce n’est pas une traduction du PHP
Selfnamed : aucune image ni requête réseau n’est impliquée. Les limites et champs encore indéterminés sont
documentés dans [le contrat ABI](../docs/abi.md#prévisualisation-catalogue-selfnamed-prototype).

### Ingestion bornée du ZIP amont `ps_newproducts`

`cmd/prestashop-import` accepte uniquement le module PrestaShop `ps_newproducts`
dans son ZIP amont : il vérifie les chemins, liens, doublons, nombre et tailles
des entrées, puis lit `config.xml` et `ps_newproducts.php` sans extraire ni
exécuter de PHP. Le build ne réussit que si le package correspond au comportement
catalogue explicitement reconnu par l’adaptateur. Le SHA-256 de l’archive et
celui du point d’entrée sont liés au manifeste de build.

Essai local sans effet sur la base :

```sh
podman compose -p euromni-cms-test --profile cms --profile module-import run --rm -T \
  prestashop-import \
  --archive pipeline/fixtures/prestashop-ps_newproducts.zip \
  --platform-version euromni-test-adapter \
  --days 30 --limit 8
```

Pour publier et lancer le Wasm généré dans la base de démonstration isolée,
ajouter `--install`. Ce mode exige `EUROMNI_ENVIRONMENT=development`, la base
PostgreSQL locale nommée `euromni` et la boutique seedée `atelier-demo`. Le
profil Compose `module-import` écrit les artefacts dans son volume privé
`module-artifacts`, distinct du volume PostgreSQL mais conservé par le nom de
projet `euromni-cms-test`. L’import enregistre le manifeste et l’installation
dans PostgreSQL, puis charge et exécute le Wasm via Wasmtime ; aucun PHP tiers
n’est exécuté.

Pour publier un build candidat sans changer la version de Wasm installée dans
aucune boutique, utiliser `--publish-only` avec les mêmes garde-fous de
développement. Le pointeur courant du module et l’inventaire des builds sont
mis à jour ; les pins `shop_modules` restent inchangés. Le candidat apparaît
alors dans l’admin pour une promotion explicite et peut être vérifié avant
activation. Le test d'intégration vérifie la relecture et le manifeste de
l'artefact, confirme que le pin installé est inchangé, puis promeut le digest
et exécute ce build via Wasmtime :

```sh
podman compose -p euromni-cms-test --profile cms --profile module-import run --rm -T \
  prestashop-import \
  --archive pipeline/fixtures/prestashop-ps_newproducts.zip \
  --platform-version 9.0.0-test-build-2 \
  --publish-only
```

Le point d'entrée de dépôt en quarantaine est
`POST /v1/admin/modules/imports` avec un corps brut `application/zip` et une
session Bearer de rôle `owner` ou `admin`. Il est activé uniquement si
`EUROMNI_QUARANTINE_DIR` désigne un répertoire privé ; sinon l'API répond
`503`. Les archives sont validées en mémoire, stockées séparément des builds,
scopées par boutique et indexées par SHA-256, sans extraction, compilation ni
exécution PHP. La limite par requête est 16 MiB et le quota local est de
256 MiB par boutique. Cette première tranche ne fournit ni endpoint de revue,
ni promotion depuis la quarantaine, ni politique de purge ; ne pas l'activer
en production avant d'avoir configuré la rétention et les contrôles
opérationnels. Le répertoire de quarantaine est local à une instance CMS et
ne doit pas être partagé entre plusieurs processus/instances ; le quota n'est
pas un mécanisme de coordination distribué.

Le ZIP versionné est un snapshot épinglé du dépôt officiel PrestaShop
(`ps_newproducts` 2.0.1, commit amont
`72e9476406319cbb91ac383121ca9967dc61f277`, SHA-256
`0f0750d8c2a1e6e12fc67169224f597f166b5c80b99cb306f53e9b339b17af77`), pas une
archive de distribution Addons. L'importeur vérifie cette empreinte avant de
lire le ZIP et refuse toute autre archive. `euromni-test-adapter` est une
étiquette de provenance de test, pas une version PrestaShop effectivement
installée. Cette preuve couvre l’ingestion bornée, la publication dans EUROMNI
et l’exécution du Wasm dans le CMS de test ; elle ne couvre ni l’installation
du PHP amont dans PrestaShop, ni le rendu Smarty, ni une parité fonctionnelle
complète avec le module d’origine.

L’IR accepte un appel par module et prend en charge `catalog.product.get`,
`catalog.product.list`, `catalog.product.newest` et `catalog.category.list`.
Conditions, chaînes d’opérations et traduction générale du code métier restent
hors périmètre. Le lowering WooCommerce utilise encore une entrée statique ;
le widget PrestaShop utilise un appel statique avec configuration lue côté hôte.

Le test PrestaShop compile l’entrée PHP réelle épinglée, puis publie et exécute
le Wasm sur une base PostgreSQL d’intégration jetable. Le test
`TestUpstreamPrestaShopArchivePublishesAndRunsInIsolatedCMS` reproduit aussi
l’ingestion du ZIP amont et l’installation transactionnelle dans une base
jetable. La commande `cmd/prestashop-import` permet le même parcours dans le
volume de test local après validation explicite.

Le chemin lowering → Wasm → module installé → `Runtime.Run` est couvert par un
test PostgreSQL sous le tag `integration` :

```sh
go test -tags=integration ./internal/pipeline -run TestIRGeneratedWasmRunsAsInstalledModule
go test -tags=integration ./internal/pipeline -run TestRealPrestaShopNewProductsWasmRunsAsInstalledModule
```

## Chaîne cible

1. **Ingestion authentifiée** : un opérateur envoie une archive et choisit une
   plateforme/version. Pas d’endpoint anonyme ; l’accès devra être protégé par
   une identité admin et une autorisation de boutique.
2. **Quarantaine source** : borner la taille et le nombre d’entrées, refuser les
   chemins absolus, `..`, liens symboliques et fichiers inattendus. Enregistrer
   le SHA-256 de l’archive et sa provenance. Ne jamais lancer PHP.
3. **Analyse statique par adaptateur** : détecter la plateforme/version, parser
   le PHP et extraire hooks, appels de données, effets de bord et dépendances.
   WooCommerce et PrestaShop ont des adaptateurs séparés ; leurs concepts sont
   convertis vers un modèle intermédiaire commun.
4. **Modèle intermédiaire (IR)** : première version à un appel et capabilities
   dérivées de l’allowlist ; restent à représenter les opérations métier,
   expressions et schémas de déclenchement. Tout code non pris en charge produit
   un diagnostic ou une demande de revue, jamais une capability supplémentaire
   implicite.
5. **Traduction** : produire du code invité depuis l’IR. Un LLM peut proposer
   une extraction structurée, mais sa sortie n’est jamais exécutée ni publiée
   sans validation déterministe et revue des capabilities.
6. **Compilation reproductible** : compiler avec une toolchain et une version
   verrouillées ; consigner versions du compilateur, des adaptateurs, de l’IR,
   de l’ABI et le hash source. À source et configuration identiques, viser un
   artefact Wasm identique.
7. **Validation sandbox** : Wasmtime doit compiler et instancier le module avec
   seulement `euromni.call`, une mémoire exportée et l’export d’entrée convenu.
   Refuser WASI, imports système, imports inconnus, versions ABI incompatibles,
   dépassements de taille, mémoire ou fuel.
8. **Publication atomique** : écrire d’abord le Wasm dans le stockage
   content-addressed, puis enregistrer hash, ABI, provenance et capabilities en
   base. Une transaction lie l’artefact au module installé ; l’échec laisse
   l’ancienne version active.
9. **Chargement et exécution** : `LoadInstalled` vérifie le hash, l’ABI et les
   capabilities ; `Run` les relit depuis PostgreSQL à chaque appel. Une
   modification du hash impose un rechargement explicite.

## Frontières de confiance

- Le ZIP PHP et ses fichiers sont des données ; aucune installation de plugin,
  autoload PHP, hook ou script d’installation n’est lancé par le CMS.
- L’analyseur et les compilateurs tournent comme workers isolés, sans accès à
  PostgreSQL, secrets, réseau ou filesystem hôte en écriture hors répertoire
  temporaire dédié.
- Le modèle intermédiaire est la frontière entre compréhension du plugin et
  génération exécutable ; une sortie incertaine reste non publiée.
- Le guest Wasm n’obtient ni WASI, ni SQL, ni accès réseau/disque. Chaque appel
  repasse par l’allowlist hôte, le contexte boutique/module et les capabilities.
- Les artefacts sont immuables et référencés par SHA-256. Le stockage courant
  est local au processus ; réplication, sauvegarde et rétention sont à définir
  pour la production.

## Pilote natif Selfnamed hors pipeline

[`pipeline/selfnamed-pilot/run.sh`](selfnamed-pilot/run.sh) lance le plugin
Selfnamed `2.0.19` original dans WordPress/WooCommerce avec un mock API local,
aligné sur la version de la copie WooCommerce de référence.
Ce test de développement exécute le PHP tiers dans les conteneurs éphémères,
mais hors du CMS et du pipeline EUROMNI : il ne valide ni import, ni lowering,
ni exécution Wasm.

Les images OCI sont épinglées par digest et le réseau Podman est `internal` ;
aucun port n’est publié. Avant le lancement, l’hôte télécharge les archives
officielles (SHA-256 vérifié) et récupère au besoin les images exactes par
digest ; ces accès ne sont pas faits par les conteneurs. Les sources sont
montées en lecture seule. Les données
utilisent trois fixtures produit et un acheteur synthétiques, l’API reçoit
uniquement un bearer token fictif et la stack n’a aucun credential Selfnamed.
Le pilote contrôle le mapping catalogue vers un produit brouillon (prix, poids,
volume et stock), les tags, le refus HTTP 401, le réimport sans doublon, puis
les appels de commande et le groupe de livraison ; les courriels sont
désactivés. Il vérifie aussi les notifications `PRODUCT`, `ORDER` (`SHIPPED`
et `CANCELLED`) et `STORE`, le rejet d'un token de callback invalide, la
séparation d'un panier mixte et les tarifs de livraison sur 1 000 g, 1 002 g
et une destination non desservie. Le cas de 1 001 g n'obtient pas de tarif
avec le calcul flottant kg→g du plugin 2.0.19 ; le scénario supérieur utilise
1 002 g et cette limite reste à traiter. Le test oracle Go est
`TestSelfnamedCatalogProductMappingCharacterization`. Les fixtures n’ont pas
d’image afin d’exclure les téléchargements.
La base et les sources temporaires sont supprimées au nettoyage. Aucun trafic
vers le service réel, aucune commande réelle et aucune expédition ne sont
testés. L’accès avec credentials réels reste une étape séparée, à autoriser
explicitement.

## Contrat d’artefact

Chaque build devra produire un manifeste vérifiable contenant au minimum :

- identifiant logique et version du module ;
- plateforme source, version, nom d’archive et hash source ;
- version d’adaptateur, du modèle intermédiaire et de la toolchain ;
- version ABI cible et capabilities demandées ;
- hash SHA-256 et taille du Wasm validé ;
- diagnostics, provenance de traduction et résultat des tests.

Le chemin physique est dérivé du digest ; il ne vient jamais directement du
nom de fichier fourni par l’utilisateur. Les octets source et Wasm doivent
rester distincts et avoir des politiques de rétention séparées.

## Portabilité et compatibilité toolchain

Le runtime courant n’expose qu’un import custom :

```wat
(import "euromni" "call"
  (func (param i32 i32 i32 i32) (result i32)))
(memory (export "memory") 1)
```

Le spike reproductible est dans `pipeline/testdata/go-wasip1` et se lance avec :

```sh
go test -tags=wasmtoolchain -v ./internal/wasm -run TestGoWASIP1ToolchainImportSet
```

Avec Go 1.26.8 (`GOOS=wasip1 GOARCH=wasm`), l’invité importe bien
`euromni.call`, mais aussi dix fonctions `wasi_snapshot_preview1` :
`sched_yield`, `proc_exit`, `args_get`, `args_sizes_get`, `clock_time_get`,
`environ_get`, `environ_sizes_get`, `fd_write`, `random_get` et `poll_oneoff`.
Il est donc **incompatible avec le linker actuel**, qui n’expose volontairement
pas WASI.

Ce résultat écarte cette configuration Go/`wasip1` précise ; il ne fixe pas
l’OS de production et ne justifie pas l’installation d’une toolchain sur le
poste de développement. QP-01 doit évaluer l’artefact obtenu : imports
exactement autorisés, appel ABI, reproductibilité et exécution sur les hôtes
Wasmtime approuvés. Un worker de build peut être versionné/isolé, mais le Wasm
livré doit rester indépendant de cet hôte et Docker ne devient pas une exigence
de production.

QP-10 traite séparément la portabilité du cœur : le binding Wasmtime actuel
utilise CGo et ses plateformes supportées doivent être déclarées puis testées.
Ne pas confondre compatibilité du guest avec disponibilité des bibliothèques
LLVM locales. Ne pas élargir le linker à WASI pour contourner une incompatibilité.

## Jalons de validation

1. Conformance ABI : fixtures WAT, imports autorisés/interdits, buffers limites,
   capabilities refusées et version inconnue.
2. Spike toolchain : même appel hôte compilé depuis le SDK invité retenu ; hash
   reproductible et aucune dépendance WASI non prévue.
3. Stockage et publication : interruption à chaque étape, déduplication,
   corruption, remplacement atomique et reprise après panne.
4. Adaptateur pilote WooCommerce : petit plugin explicitement autorisé,
   catalogue ou hook sans paiement/réseau, sorties comparées à un oracle.
5. Adaptateur pilote PrestaShop : cas métier équivalent, même IR et mêmes tests
   de conformance ABI.
6. Seulement après ces pilotes : élargir les hooks et couvrir les écarts
   spécifiques aux plateformes.