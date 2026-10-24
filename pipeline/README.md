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

### Contrat portable de build distribuable

La représentation d’un build est indépendante de tout transport et de tout
fournisseur :

- `manifest.json` : manifeste `euromni.build.v1` strict ;
- `module.ir.json` : IR canonique `euromni.ir.v3` ;
- `module.wasm` : artefact Wasm binaire adressé par SHA-256.

Le package ne contient pas les sources PHP ni l’archive amont ; le manifeste
conserve leurs empreintes et références de provenance. L’IR et le manifeste
peuvent être distribués avec le Wasm depuis un catalogue central ou transmis à
la suite d’un build local. Aucun format ZIP, endpoint de téléchargement ou
catalogue n’est imposé par ce contrat.

`ValidateBuildPackage` refuse les champs JSON inconnus, les documents IR non
canoniques, tout écart entre manifeste/IR/Wasm et tout compilateur, ABI, IR ou
adaptateur non approuvé. Il compile structurellement le Wasm avec Wasmtime et
exige le seul import `euromni.call` avec sa signature ABI, les seuls exports
`memory` et `run`, la signature `run` dictée par l’IR et une mémoire linéaire
32 bits dont la mémoire initiale respecte 64 MiB et dont le maximum déclaré,
s’il existe, ne dépasse pas cette limite. Le store Wasmtime impose aussi un
plafond d’exécution de 64 MiB, y compris pour un module sans maximum déclaré.
Le Wasm porte les sections custom exactes
`euromni.ir.sha256` et `euromni.compiler` ; le manifeste inclut source, archive,
plateforme, versions d’adaptateur/IR/compilateur/ABI, capabilities dérivées,
hash du Wasm et taille. La compatibilité de runtime est actuellement une
égalité stricte de version ABI, pas une plage de versions implicite.

La validation structurelle seule prouve la cohérence et l’intégrité par hash,
pas l’identité du producteur. L’activation d’un hash reste distincte de sa
distribution et requiert la promotion explicite existante.

### Authenticité des packages — prototype Ed25519

Le prototype ajoute une signature Ed25519 détachée, séparée des trois blobs de
build. Le message signé est une concaténation binaire versionnée et
domain-separated :

```text
euromni.build-package-signature.v1\0
SHA-256(manifest.json)[32 octets]
SHA-256(module.ir.json)[32 octets]
SHA-256(module.wasm)[32 octets]
```

L’enveloppe détachée est un JSON canonique limité à 4 KiB, de la forme
`{"version":"euromni.build-package-signature.v1","key_id":"<sha256 hex>","signature":"<base64>"}`.
La signature utilise Base64 standard paddé. L’identifiant est l’empreinte
SHA-256 hexadécimale minuscule de la clé publique Ed25519 brute. Signer les
digests des octets exacts lie tous les composants sans imposer de format
d’archive ni de sérialisation canonique supplémentaire au manifeste ; le
validateur existant continue d’exiger l’IR canonique et de comparer les valeurs
du manifeste aux composants.

Le prototype est implémenté par `SignBuildPackage`,
`MarshalBuildPackageSignature` et `ValidateSignedBuildPackage`. Le signataire
fournit explicitement une clé privée Ed25519 ; la fonction signe uniquement
après validation structurelle du package. La vérification prend une map
`key_id -> ed25519.PublicKey` fournie par l’appelant : la clé est comparée à
son empreinte, puis la signature est vérifiée avant `ValidateBuildPackage`.
Une clé publique incluse dans le package ne crée jamais sa propre confiance.
Les clés privées restent hors du CMS, du runtime Wasm, du dépôt et des
packages ; leur usage relève d’un environnement de signature contrôlé. Le
prototype n’ajoute ni timestamp ni expiration ; le retrait d’une clé de la map
de confiance la rend non fiable pour les vérifications suivantes.

Une signature prouve qu’une clé autorisée a signé ces octets, pas que le code
est sûr, compatible avec un CMS réel ou approuvé pour installation. Cette
primitive n’est pas reliée à un service de signature, à l’import ZIP, à la
publication de build ou à la promotion.

Le prototype a été approuvé le 24 octobre 2026. Le même jour, l'orientation
de distribution hybride a été retenue : les builds locaux restent possibles
et un catalogue central de packages signés est facultatif. La politique de
confiance suivante a été décidée comme base de conception :

- l'éditeur du module signe ses propres packages ; la signature atteste
  l'origine cryptographique du package, pas son innocuité ni sa compatibilité ;
- chaque installation configure explicitement les clés publiques auxquelles
  elle fait confiance ; aucune clé fournie par le package ni aucune racine
  implicite du catalogue ne crée de confiance ;
- un package reçu doit passer la vérification de signature et la validation
  structurelle avant d'être enregistré comme candidat ;
- la réception ou la vérification ne promeut jamais un build et ne modifie pas
  les installations actives ; un rôle autorisé doit lancer explicitement la
  promotion selon le flux existant ;
- les builds locaux ne dépendent pas de la disponibilité d'un catalogue. Cette
  politique de signature vise les packages distribués/reçus et ne rend pas
  obligatoire la signature d'un build local.

Ce sont des règles produit retenues, pas encore une politique de production
opérationnelle. Le canal prototype est fixé à un fichier explicite par
installation. Restent à fixer le déploiement et la protection de ce fichier, la
rotation et le retrait, l'identité présentée à l'administrateur, la durée de
validité, les éventuelles multi-signatures et l'autorisation précise de
promotion. Le prototype de réception décrit ci-dessous est limité à la
vérification et au staging privé ; il ne constitue pas une autorisation
d'activation ou de production.

#### Fichier explicite de clés approuvées

Le premier incrément de cette politique est un lecteur strict,
`pipeline.LoadTrustedBuildPackageKeys(path)`. Le CMS le charge au démarrage si
`EUROMNI_BUILD_TRUST_FILE` pointe vers le fichier de l'installation ; en
l'absence de cette variable, aucune clé n'est approuvée. Un fichier manquant ou
mal formé fait échouer le démarrage au lieu de continuer avec une confiance
partielle. Les clés publiques sont les octets bruts Ed25519 encodés en Base64
standard paddé et `key_id` est leur SHA-256 hexadécimal minuscule :

```json
{
  "version": "euromni.trusted-build-keys.v1",
  "keys": [
    {
      "key_id": "<sha256-hex-de-la-cle-publique>",
      "public_key": "<base64-de-32-octets>"
    }
  ]
}
```

Le lecteur refuse les champs JSON inconnus, les versions non reconnues, les
clés mal encodées, les identifiants incohérents et les doublons ; le fichier est
plafonné à 64 KiB et à 256 clés. Une liste vide représente une installation qui
n'accorde confiance à aucun signataire. Le fichier et ses répertoires parents
doivent être protégés contre les modifications par des utilisateurs non
autorisés.

#### Réception de packages signés

`POST /v1/admin/modules/build-packages` accepte le transport ZIP plafonné à
64 MiB avec exactement quatre fichiers racine : `manifest.json`,
`module.ir.json`, `module.wasm` et `signature.json`. Le manifeste est limité à
64 KiB, l'IR à 1 MiB, le Wasm à 64 MiB et l'enveloppe de signature à 4 KiB.
Seuls les fichiers ordinaires utilisant ZIP Store ou Deflate sont acceptés ;
entrées manquantes, répétées, supplémentaires, chemins et répertoires sont
refusés. Le serveur lit les entrées en mémoire avec limites et CRC ZIP, sans
extraire de fichier. Il valide ensuite la signature avec la liste de confiance
et appelle `ValidateBuildPackage` avant toute écriture.

La route exige `owner` ou `admin` et stocke l'archive originale comme candidat
dans le répertoire privé configuré par `EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR`,
sous un chemin adressé par boutique et SHA-256. La limite provisoire est
256 MiB par boutique ; elle est comptée par processus et ne coordonne pas les
instances. Une seule validation d'import est exécutée à la fois par processus ;
les demandes concurrentes reçoivent `503`. Un package accepté répond `202` avec son digest, son signataire,
son module et son état `candidate`. Il ne crée pas de build dans le registre,
ne change aucun pin et ne déclenche aucune installation ni promotion.
L'absence de configuration du répertoire désactive cette route (`503`) ; une
liste de confiance absente la désactive également. Une liste explicitement
vide ne fait confiance à aucun signataire.

`GET /v1/admin/modules/build-packages/candidates` fournit à `owner` ou `admin`
un inventaire paginé des digest, tailles et états d'approbation des candidats
de la boutique (`limit` 1–100, défaut 50 ; `offset` 0–100000, défaut 0). Il
n'ouvre pas les archives et ne revalide pas les signatures ; le détail reste
nécessaire avant toute décision. `GET
/v1/admin/modules/build-packages/candidates/{packageSHA256}` permet ensuite à
`owner` ou `admin` de relire le candidat de sa boutique. La route relit le ZIP
stocké et revalide la signature avec les clés de confiance courantes ainsi que
le contenu du package ; elle répond avec les métadonnées et l'état `pending` ou
`approved`. `POST /v1/admin/modules/build-packages/candidates/{packageSHA256}/approval`
accepte uniquement `{"decision":"approve"}` pour les mêmes rôles. L'approbation
immuable conserve le digest du package, l'identifiant de l'acteur, son rôle et
l'heure UTC dans le sidecar `<digest>.approval.json` du stockage local ; elle ne
constitue pas un journal d'audit centralisé en base. Les répétitions répondent
`409`. Un candidat absent répond `404`,
et une clé retirée ou un package qui échoue désormais à la validation bloque
l'approbation. Cette décision ne publie, n'installe et ne promeut aucun build,
ne modifie aucun pin et ne constitue pas une preuve d'innocuité.

L'interface Modules permet à `owner` ou `admin` d'examiner ces métadonnées et
d'enregistrer l'approbation ; aucune décision de rejet, téléchargement,
suppression, passerelle de publication ou purge automatique n'est disponible.
La rétention est donc manuelle et la capacité limitée ;
ne pas activer ce stockage pour la production avant d'avoir défini rétention,
quota coordonné multi-instance et contrôles opérationnels. L'ingestion PHP ZIP
de `/v1/admin/modules/imports` reste entièrement distincte.

Les parcours `PublishModuleBuild`, `PublishModuleBuildOnly` et l'installation
PrestaShop passent tous par `storeBuildArtifact`, qui appelle
`ValidateBuildPackage` avant `artifact.Store.Put`. Un Wasm mal formé ou avec un
import/export incompatible n'est donc ni stocké ni enregistré par ces
parcours. Après validation, le Wasm est écrit dans le stockage
content-addressed, puis les métadonnées sont enregistrées ; si l'écriture DB
échoue, l'artefact reste immuable mais non référencé.
`ReconcileModuleArtifacts` récupère les hashes de
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
natif et au test oracle. `CompileSelfnamedPreviewIRToWasm` n’accepte que l’IR
Selfnamed approuvé, compile le crate Rust `no_std` avec son toolchain/lockfile
épinglés, puis ajoute au Wasm le hash exact de l’IR et l’identité du compilateur.
Le manifeste indique ce compilateur dédié au lieu du compilateur WAT générique.
Un test construit deux artefacts dans des répertoires indépendants et exige
l’égalité octet pour octet. Le test d’intégration publie et charge ce même
artefact dans Wasmtime, vérifie chaque projection et le rejet ABI `invalid`
d’un produit malformé ou supérieur à 8 KiB ; le catalogue persistant reste
inchangé après tous les appels. Le guest effectue le mapping et soumet le DTO à
`catalog.supplier.product.preview.result`, que l’hôte valide sans accès aux
données. Ce n’est pas une traduction du PHP Selfnamed : aucune image ni requête
réseau n’est impliquée.

Le build dédié est vérifié avec :

```sh
go test -tags=wasmtoolchain ./internal/pipeline -run TestCompileSelfnamedPreviewIRToWasmIsReproducibleAndBoundToIR
```
Les limites et champs encore indéterminés sont
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
séparation d'un panier mixte et six zones synthétiques pour FR/BE, DE, GB et US,
avec des options standard/express et des tranches allant de 0 à 15 kg. Les cas
exécutés couvrent huit tranches françaises, plusieurs destinations, deux
options simultanées aux États-Unis, une limite de service express, un poids
hors barème et un pays non desservi. La matrice versionnée est
[`shipping-zones.json`](selfnamed-pilot/shipping-zones.json) ; sa structure
s'inspire des scénarios de l'article d'aide Selfnamed, mais tous ses tarifs sont
fictifs et ne représentent ni les prix publics ni ceux d'une boutique réelle
([article de référence](https://help.selfnamed.com/en/articles/10009962-what-is-selfnamed-s-shipping-time-and-costs)).
Le cas de 1 001 g n'obtient pas de tarif avec le calcul flottant kg→g du plugin
2.0.19 ; le test inclut le poids 1 000 g et une valeur intérieure de tranche à
1 002 g, et cette limite reste à traiter. Le test oracle Go est
`TestSelfnamedCatalogProductMappingCharacterization`. Les fixtures n’ont pas
d’image afin d’exclure les téléchargements.

Le mock renvoie aussi le catalogue en deux pages de deux produits et les six
zones en deux pages de trois zones, avec des en-têtes de pagination cohérents.
Dans le CMS, l’import catalogue demande explicitement la page 1 puis la page 2,
vérifie que le produit de la page suivante n’existe pas avant son import et
rejoue les deux pages pour tester l’idempotence. La version 2.0.19 n’itère pas
automatiquement le catalogue dans `create_all_products` : l’appelant fournit la
page. À l’inverse, `get_all_shipping_zones` suit le nombre de pages annoncé par
l’API et agrège les zones ; le pilote vérifie les six IDs et les demandes des
deux pages sur les parcours direct et méthode de livraison.

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

Le guest Rust Selfnamed `no_std` est un prototype séparé, compilé avec son
`Cargo.lock` et `--frozen`. La vérification Wasmtime sans base se lance avec :

```sh
go test -tags=wasmtoolchain -v ./internal/wasm -run TestRustSelfnamedPreviewGuestMapsProductInGuest
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