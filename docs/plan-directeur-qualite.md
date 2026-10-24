# Plan directeur qualité et actions

| Champ | Valeur |
|---|---|
| Identifiant | EUROMNI-QP-001 |
| Version | 3.37 |
| État | Brouillon contrôlé, approbation humaine à renseigner |
| Date de révision | 2026-10-24 |
| Responsable du document | Responsable produit/technique, à désigner |
| Périmètre | Développement et vérification du CMS EUROMNI, des interfaces, migrations, modules et pipeline PHP→Wasm |
| Exigence de portabilité | Artefacts Wasm indépendants de l’hôte de build ; plateformes runtime déclarées et testées |

Ce document organise les actions et preuves techniques du projet. Le cadre du
système de management de la qualité, son périmètre, ses responsabilités et ses
lacunes sont décrits dans le [Système de management de la qualité](systeme-management-qualite.md).
Les deux documents sont des brouillons contrôlés. Ils s’inspirent de la
structure d’ISO 9001 et **ne constituent ni une déclaration de conformité ni
une certification ISO 9001**. L’approbation, les responsabilités et les
objectifs qualité restent à confirmer par le responsable produit/technique.

## Références et exigences

- Architecture cible : [`architecture.md`](architecture.md).
- Système de management de la qualité : [`systeme-management-qualite.md`](systeme-management-qualite.md).
- Matrice runtime/toolchain : [`portability.md`](portability.md).
- Contrat ABI : [`abi.md`](abi.md).
- Chaîne PHP→Wasm et frontières de confiance : [`../pipeline/README.md`](../pipeline/README.md).
- Exécution Wasmtime : imports limités à `euromni.call`, capabilities médiées,
  validation de hash/ABI, plafonds de mémoire et fuel.
- Contrainte d’exploitation : Docker est facultatif pour les tests et n’est pas
  une exigence de production.

## Rôles

- **Responsable produit** : confirme le périmètre, les plateformes pilotes et
  les critères d’acceptation métier.
- **Responsable qualité** : à désigner ; approuve ce document, les changements
  de statut et les dérogations.
- **Responsable technique** : maintient les contrats, les revues, les preuves
  de tests et la traçabilité des versions.
- **Contributeur/agent** : travaille sur une action identifiée, exécute les
  contrôles indiqués et rapporte les résultats sans déclarer une conformité
  non démontrée.

## État de référence

Au 2026-10-24, le runtime Wasmtime, l’ABI hôte, le registre immuable de builds
et le stockage local d’artefacts par SHA-256 existent. La génération reste
limitée à l’IR supportée ; le build Rust `no_std` du prototype Selfnamed est
lié à son IR approuvé et reproductible, sans constituer une traduction PHP
générale. Le package transport-indépendant (manifeste, IR canonique, Wasm) a
un validateur strict et un prototype de signature détachée, sans branchement
aux flux de réception ou de promotion ni politique de production. Les cibles
runtime Windows/amd64 et Linux/amd64 ont été testées
dans le workflow natif de distribution ; les architectures et systèmes de
fichiers non testés restent exclus. L'ingestion HTTP QP-03 existe en opt-in et
dépose uniquement des ZIP validés en quarantaine privée ; elle ne compile,
n'extrait ni n'exécute le PHP.
Voir [`portability.md`](portability.md) pour les limites et preuves ; QP-01
reste ouvert sur le processus de build/promotion métier.

Un prototype Ed25519 détaché des trois digests est implémenté : la clé publique
de confiance est injectée hors package par l’appelant. Ce périmètre a été
approuvé pour le prototype le 24 octobre 2026 ; ce n’est pas une politique de
production. La base de politique de confiance est décidée ci-dessous ; son
administration opérationnelle et les règles détaillées restent à spécifier.

Le 24 octobre 2026, l'orientation de distribution hybride a été retenue :
les builds locaux restent possibles et un catalogue central de packages signés
est facultatif. Ce choix ne rend pas le CMS dépendant d'un service central et
est indépendant de sa disponibilité.

La base de politique de confiance a été décidée : l'éditeur signe ses propres
packages ; chaque installation configure explicitement ses clés de confiance ;
un package reçu, après vérification cryptographique et structurelle, reste un
candidat ; sa promotion exige une action humaine explicite par un rôle autorisé.
La signature d'un build local n'est pas obligatoire. Cette décision produit
n'autorise pas encore un déploiement opérationnel : le canal et la protection
des clés, leur rotation/retrait, l'identité affichée et les rôles exacts de
promotion restent à spécifier. Le prototype de réception utilise un ZIP exact
plafonné à 64 MiB et un staging séparé de 256 MiB par boutique ; ces plafonds
provisoires ne qualifient pas le déploiement de production.

Le lecteur `pipeline.LoadTrustedBuildPackageKeys(path)` établit le premier
contrat de configuration de clés par installation : JSON versionné strict,
taille maximale 64 KiB, maximum 256 clés, encodage Base64 canonique, ID égal à
l'empreinte SHA-256 de la clé Ed25519 et rejet des doublons. Le CMS le charge
au démarrage via `EUROMNI_BUILD_TRUST_FILE` et échoue explicitement si le
fichier configuré est invalide. La route authentifiée de réception vérifie
désormais le ZIP, la signature et le package, puis stocke l'archive comme
candidat privé par boutique via `EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR`. Les routes
de lecture par digest revalident le package ; l'approbation backend `owner` ou
`admin` est attribuée et horodatée, immuable et distincte de la publication ou
promotion. Le rejet exige un motif ; il est révisable par une réouverture
explicite. Chaque transition est conservée dans un historique local borné et
attribué ; cette trace n'est pas inviolable contre un administrateur système.
Aucune transition ne publie ou ne promeut. L'inventaire admin paginé ne revalide
pas les archives, ce que fait la lecture détaillée avant décision. L'interface
Modules expose l'inventaire, le détail revalidé, l'approbation, le rejet et la
réouverture. La rétention des rejets et la conservation de l'historique sont
implémentées ; la passerelle de publication reste à implémenter.

La capacité des candidats signés est coordonnée entre instances au moyen d'un
verrou advisory transactionnel PostgreSQL par boutique ; imports, décisions et
purges prennent ce verrou avant de lire ou modifier le FileStore. Le CMS et le
réconciliateur doivent partager la même base PostgreSQL ainsi qu'un volume
commun supportant les hard links atomiques et la visibilité cohérente. Un test
d'intégration PostgreSQL lance deux FileStores concurrents et vérifie qu'un
seul upload peut occuper le quota restant. Ce test ne qualifie pas les garanties
d'un NAS ou d'un système de fichiers partagé en production.

### Revue de sécurité indépendante — packages signés

Une revue indépendante en lecture seule du parcours des packages signés a été
effectuée le 24 octobre 2026 ; son résultat a été consigné à
**10:49:38 (UTC+02:00, Europe/Paris)**. Périmètre examiné sur le commit
`df7a6a7` : réception et validation ZIP/signature, autorisation et isolation
par boutique, stockage des candidats, transitions et historique des décisions,
purge des rejets après 90 jours et API/interface de consultation de l'historique
conservé. **Aucune vulnérabilité exploitable n'a été identifiée dans ce
périmètre.** La planification opérationnelle de la purge et le comportement
multi-instance n'ont pas été vérifiés ; ce résultat ne vaut ni approbation de
production ni revue des modifications locales exclues du commit.

Avant toute reprise de code, vérifier l’état réel de l’arbre de travail et
l’historique Git. Les notes de version précédentes ne permettent pas de déduire
si ces changements ont été commités ni si d’autres modifications locales sont
présentes.

## Registre d’actions

Statuts : **À faire**, **En cours**, **Bloqué**, **Vérification**, **Terminé**.
Une action ne passe à Terminé qu’avec ses preuves référencées dans le commit ou
la documentation du projet.

| ID | Priorité | Action / dépendance | Critère d’acceptation | Preuve attendue | Statut |
|---|---|---|---|---|---|
| QP-01 | P0 | Prouver une toolchain qui produit un artefact Wasm portable compatible avec `euromni.call`, sans imports système implicites. Dépend du contrat ABI actuel. | Build reproductible depuis un worker épinglé ; module n’importe que l’ABI autorisée et s’exécute sur la matrice Wasmtime approuvée. Aucun compilateur/LLVM local requis en production. | Spike Rust `no_std` 1.99.1/Cargo `--frozen` : imports, exports, exécution Wasmtime et égalité SHA-256 de deux builds propres ; CI Go/Rust Linux/Windows verte (run 39580872372), distribution Linux/Windows avec intégrations, smoke tests et artefacts publiés (run 39581659670). Les incréments Selfnamed `9f99694` et `912545b` ajoutent le mapping guest et la génération liée à l’IR ; cette preuve ciblée ne qualifie pas la traduction métier générale ni un worker de production. | En cours |
| QP-02 | P0 | Définir le manifeste/versionnement d’artefact et son lien atomique avec `modules` et `shop_modules`. | Hash source, hash Wasm, plateforme/version, versions d’adaptateur/IR/compilateur, ABI et capabilities sont traçables ; un échec ne remplace pas la version active. | Builds immuables, pointeur courant et pin par installation migrés en PostgreSQL/MariaDB ; publication et promotion transactionnelles couvertes par intégration PostgreSQL, migration et parcours MariaDB dédiés réussis. Manifeste strict, allowlist des versions approuvées, promotion dans le CMS de test et exécution du build promu vérifiés. `ValidateBuildPackage` vérifie le triplet manifeste/IR/Wasm avant stockage. Le prototype Ed25519 lie les octets par leurs digests et vérifie une clé explicitement fournie hors package ; ni réception distribuable ni politique de production ne sont branchées. | Terminé |
| QP-03 | P0 | Concevoir et implémenter une ingestion admin authentifiée avec quarantaine isolée. Le premier incrément accepte un ZIP brut (pas multipart), le valide sans extraction ni exécution, le stocke sous empreinte dans un répertoire privé configurable et ne le publie pas comme build. | Authentification admin, charge plafonnée, nombre d’entrées et taille décompressée totale bornés ; doublons, traversal, chemins absolus/Windows, liens/types spéciaux, archive corrompue/bombe et extensions non autorisées rejetés. Échec de validation sans écriture en quarantaine ; aucune exécution de PHP ; aucun endpoint anonyme. | Tests API, stockage et `PurgeBefore` réussis localement, y compris Go 1.25. `gosec` G201/G202 : 0 finding SQL ; `govulncheck` après pgx v5.9.2/x-text v0.41.0 : 0 vulnérabilité atteignable. Go CI #60 a réussi sur Linux/Windows avant le relèvement Go 1.25. Standalone #8 tentative 2 a réussi sur Linux/Windows mais cible `6610399`, avant la mise à jour des actions. Un nouveau Go CI doit qualifier Go 1.25 sur Windows ; un nouveau dispatch standalone sur le commit courant doit valider le paquetage. Gosec complet garde 22 alertes à trier (conversions/path/exec/fixtures) ; timeouts HTTP du CMS ajoutés après G114. Revue de sécurité indépendante, rate-limit et multi-instance restent ouverts. La rétention distincte des candidats signés est fixée : rejet purgé après 90 jours, pending/approved conservés ; exécuteur ponctuel à planifier quotidiennement et historique conservé. | En cours |
| QP-04 | P1 | Créer les adaptateurs statiques WooCommerce et PrestaShop et leur matrice de versions/hooks supportés. | Un jeu de fixtures autorisé par plateforme produit des diagnostics déterministes et identifie les appels de données/effets de bord. | Fixtures, version source, résultats d’analyse golden et rapports de limites. | À faire |
| QP-05 | P1 | Définir l’IR métier et sa validation avant toute génération. | Chaque opération IR déclare types, capability, erreurs et comportement ; capacités inconnues ou accès système impossibles à générer. | Contrat `euromni.ir.v3` documenté pour les déclencheurs `hook`/`manual` et les six appels supportés ; les types/champs sont validés contre les schémas explicites, les capabilities restent dérivées de `wasm.Mapping`, et tout type de déclencheur, appel ou champ non pris en charge est rejeté. Tests `go test ./internal/pipeline` réussis sous Linux. Le contrat ne couvre ni expressions ni traduction générale. | Terminé |
| QP-06 | P1 | Implémenter la traduction contrôlée de l’IR et la compilation reproductible. | Aucun résultat LLM n’est exécuté directement ; source/config identiques produisent le même artefact, ou une différence est expliquée. | Prototype Selfnamed : `CompileSelfnamedPreviewIRToWasm` n’accepte que l’IR approuvé, utilise Rust/Cargo épinglés et lockfile, lie hash IR/compiler au Wasm ; deux builds propres identiques et intégration Wasmtime. Le DTO peut être transmis explicitement à un endpoint admin qui crée un brouillon tenant-scopé ; ce staging ne déclenche pas le guest Wasm. Les workflows CI Go et distribution sélectionnent explicitement Rust 1.99.1 ; le job `wasm-toolchain-spike` exécute le test `TestCompileSelfnamedPreviewIRToWasmIsReproducibleAndBoundToIR` sur Linux/Windows. `go test -tags=wasmtoolchain ./internal/pipeline -run '^TestCompileSelfnamedPreviewIRToWasmIsReproducibleAndBoundToIR$'` passe localement sous Linux ; les workflows modifiés restent à confirmer par leur prochain run distant. Pas de traduction PHP générale, worker isolé ni SBOM. | En cours |
| QP-07 | P1 | Valider Wasm avant publication et enregistrer l’artefact. | Imports, export mémoire/entrée, ABI, hash, taille, mémoire et fuel conformes ; publication atomique ; artefact corrompu ou non conforme rejeté. | `ValidateBuildPackage` vérifie JSON strict/canonique, hashes, IR/compiler/ABI approuvés, import `euromni.call`, exports `memory`/`run`, types de fonctions et plafond mémoire ; `storeBuildArtifact` l'exécute avant `artifact.Store.Put`. L’exécution garde les limites fuel/mémoire Wasmtime. Signature producteur, limite de transfert, rollback adversarial et rechargement d’un package distribué restent à tester. | Vérification |
| QP-08 | P2 | Exécuter deux pilotes autorisés, un WooCommerce et un PrestaShop, sur une fonction catalogue ou hook simple, sans paiement. | Les deux passent par la même ABI sans SQL/réseau/filesystem côté guest et leurs sorties correspondent à un oracle documenté. | Sources réelles non financières, Lowerings et exécutions publiées : WooCommerce `woocommerce-extra-product-sorting-options` (GPL-3.0), épinglé au commit `bc5829d4d2383ae956869386eb40e772cf4a76a0` ; `alphabetical` / `reverse_alpha` couverts par oracle et Wasmtime. PrestaShop `ps_newproducts` (AFL-3.0), import et exécution publiés avec réglages. Tests dans PostgreSQL jetable ; aucun PHP exécuté et invités limités à `euromni.call`/capability `catalog`. Restent la compatibilité sur instances plateformes réelles, l’élargissement de versions et les options WooCommerce non traduites (`by_stock`, `review_count`, `on_sale_first`). Stripe et `ps_cashondelivery` exclus. Essai Selfnamed WooCommerce 2.1.0 natif isolé, distinct et non comptabilisé dans QP-08. | En cours |
| QP-09 | P2 | Définir rétention, sauvegarde, restauration et suppression des sources/artefacts. | Règles par type d’artefact, stockage hors machine, restauration vérifiée, purge traçable. | Procédure et exercice de restauration/purge. | À faire |
| QP-10 | P0 | Définir la matrice de portabilité du cœur et du runtime Wasmtime, distincte de la toolchain invitée. | OS/architectures de production décidés et testés ; dépendances natives/CGo, PostgreSQL, stockage et procédure de déploiement documentés. Une cible non testée n’est pas déclarée supportée. | [`portability.md`](portability.md) : Windows/amd64 et Linux/amd64 restent les cibles. Go 1.25.0 est testé localement sous Linux et dans Compose/PostgreSQL, avec CMS et Playwright. La validation Windows sous Go 1.25 reste à obtenir par CI après push. Les runs #60 et standalone #8 validaient l’ancien socle ; les limites par filesystem/architecture restent documentées. | Vérification |

Les cibles Windows/amd64 et Linux/amd64 de QP-10 ont été qualifiées avec le
socle précédent ; la matrice est en vérification pendant le relèvement Go 1.25,
en attente du run CI Windows sur le nouveau commit. QP-01 dispose maintenant d'un parcours pilote de publication/promotion
testable, sans preuve de traduction PHP générale ni de compatibilité avec un
CMS réel. QP-06/07 ont une preuve expérimentale limitée au prototype Selfnamed
et au validateur du package transport-indépendant ; l'authenticité du
producteur, la distribution et l'élargissement de la traduction métier restent
ouverts. QP-03 reste un incrément d'ingestion séparé, sans extraction ni
exécution du PHP. La réorganisation générale du backend reste différée.

### Parcours de référence — QP-01 : contrat du flux de build métier

**Pilote retenu pour le test d'acceptation :** snapshot PrestaShop `ps_newproducts`
2.0.1, commit amont
`72e9476406319cbb91ac383121ca9967dc61f277`, fourni par
`pipeline/fixtures/prestashop-ps_newproducts.zip`, SHA-256
`0f0750d8c2a1e6e12fc67169224f597f166b5c80b99cb306f53e9b339b17af77`.
Ce choix réutilise l'importeur et le parcours PostgreSQL/Wasmtime existants.
La référence `euromni-test-adapter` des tests est exclusivement une étiquette
de provenance isolée : elle ne représente pas une version réelle de PrestaShop
et ne permet aucune revendication de compatibilité CMS.

**Contrat d'entrée :** archive ZIP unique, bornée par les limites déjà
documentées, identité module/version correspondant au snapshot, SHA-256
attendu fourni ou épinglé par la fixture, plateforme de test explicite et
versions approuvées de l'adaptateur, de l'IR, du compilateur et de l'ABI.
Aucun accès réseau pendant le build ; aucune exécution du PHP importé.

**Contrat de sortie en cas de succès :** IR validé et déterministe, Wasm
validé par imports/exports/ABI, artefact adressé par son SHA-256, manifeste
cohérent contenant les hashes source/ZIP/IR/Wasm et les versions de
toolchain, enregistrement immuable du candidat et rapport de build auditable.
La publication candidate fait avancer le pointeur courant mais ne change pas
le pin des boutiques déjà installées. Seule une promotion explicite par hash,
après relecture de l'artefact et vérification de son manifeste, modifie le pin.

**Test d'acceptation**, isolé en PostgreSQL et Wasmtime :

1. Vérifier le SHA-256 exact du ZIP avant analyse ; rejeter un octet modifié
   sans changer le build courant ni le pin installé. Les rejets d'identité,
   d'archive dangereuse ou hors limites sont couverts par les tests d'archive.
2. Construire deux fois sans état mutable partagé avec la même configuration
   épinglée ; comparer l'IR sérialisé, les octets Wasm et les manifestes.
   Exiger uniquement l'import `euromni.call`, les exports `memory` et `run`,
   l'ABI supportée et les capabilities calculées.
3. Vérifier que manifeste, IR, package et artefact concordent ; rejeter hash
   source/Wasm incohérent, version de compilateur/adaptateur/IR non approuvée,
   import/ABI interdits, Wasm absent ou corrompu. Tout rejet laisse le pointeur
   courant et les installations inchangés.
4. Publier le candidat sans installer : contrôler la présence de l'artefact
   immuable et du manifeste, le pointeur courant au nouveau digest et le pin
   installé inchangé ; ensuite promouvoir explicitement ce digest, charger le
   même artefact dans Wasmtime et comparer la réponse à l'oracle catalogue
   seedé.

**Couvert par les tests ciblés :** vérification du hash avant analyse ZIP ;
rejet d'une fixture altérée ; égalité de deux IR, Wasm et manifestes issus de
builds répétés ; imports/exports requis ; validations de manifeste et d'archives ;
publication du candidat avec `PublishModuleBuildOnly`, relecture et validation
de l'artefact/manifeste, pointeur courant vers le candidat sans changement du
pin installé, promotion explicite, puis exécution Wasmtime de ce même artefact
via `internal/pipeline/prestashop_import_integration_test.go`. Le manifeste
déterministe constitue le rapport reliant provenance, IR, ABI, toolchain et
artefact. Reste à faire évoluer ces tests si le pilote acquiert une toolchain
ou des entrées de build externes ; le test actuel couvre le compilateur IR
embarqué sans dépendance à un répertoire de travail.

Le contrat d’un package distribuable est maintenant transport-indépendant :
`manifest.json`, `module.ir.json` canonique et `module.wasm`. Son validateur
strict contrôle hashes, versions approuvées, capabilities, imports/exports,
signatures de fonctions et mémoire via Wasmtime. Il démontre la cohérence et
l’intégrité, mais pas l’identité du producteur ; signature, authentification
de la source, limites de transfert et choix catalogue central/import local
restent des décisions ouvertes avant distribution tierce ou activation en
production.

Hors périmètre : installation du PHP dans un vrai PrestaShop, parité complète
avec l'amont, traduction PHP générale, adaptateurs multi-versions, endpoint
d'ingestion admin et réorganisation backend. QP-03/04/05 restent des
prérequis avant d'élargir ce pilote.

La matrice sépare guests Wasm, runtime Go/CGo, worker de build et FileStore.
Windows/amd64 et Linux/amd64 sont qualifiés dans les configurations CI testées ;
cela ne constitue pas une déclaration générique de support pour toutes les
versions de systèmes, toolchains ou configurations de production.

## Contrôles de passage

Pour chaque action :

1. Lier le changement à un ID QP et aux exigences concernées.
2. Énoncer les risques et les critères avant l’édition ; limiter le périmètre.
3. Ajouter tests positifs et négatifs proportionnels au risque, puis exécuter
   tests unitaires, intégration concernée, `go vet` et `git diff --check` selon
   le changement.
4. Consigner commande, résultat, commit et limites non vérifiées. Un test non
   exécuté n’est pas marqué conforme.
5. Faire approuver les dérogations et changements de critères par le
   responsable qualité/produit ; ne jamais élargir silencieusement les imports,
   capabilities ou permissions.

## Maîtrise documentaire et reprise

- Toute modification du présent plan incrémente la version et ajoute une entrée
  d’historique en bas de page ; le responsable qualité valide l’approbation.
- Les décisions techniques font référence à ce document par ID QP dans le
  commit, la PR ou le rapport de validation.
- Les preuves sont conservées dans Git (tests, contrats, migrations) ou
  référencées par identifiant immuable ; aucun secret ni archive client n’est
  commité.
- À la reprise : lire ce plan, `git status`, le dernier commit, puis les docs
  référencées ; reprendre la première action non terminée sans supposer que les
  conclusions non testées sont acquises.

## Feuille de route CMS catalogue

Cette feuille de route suit la stabilisation du CMS catalogue et de son parcours
de commande ; elle ne remplace pas les actions QP du pipeline PHP→Wasm. La
priorité est un CMS installable sur une machine Windows ou Linux avec PostgreSQL
standard. Docker et tout hébergeur cloud restent optionnels.

| Étape | Priorité | Action / dépendance | Critère d’acceptation et preuve | Statut |
|---|---|---|---|---|
| CMS-01 | P0 | Valider l’interface admin dans un navigateur intégré : rendu, connexion, consultation/édition du catalogue, erreurs et déconnexion. S’appuyer sur les tests Go PostgreSQL du parcours staff. | Écran et gestion d’erreur observés au navigateur ; défaut du formulaire asynchrone corrigé. Connexion/CRUD/déconnexion validés par l’intégration Go/PostgreSQL. | Terminé |
| CMS-02 | P1 | Ajouter la gestion des médias aux produits en réutilisant `media` et `product_media`. Dépend de CMS-01. | L’admin peut associer, ordonner, éditer le texte alternatif et retirer des URI d’images ; URI validées, accès boutique isolé, aucune ingestion de fichier local dans cette tranche. Tests API/store réussis et vérification visuelle authentifiée du parcours média réussie. | Terminé |
| CMS-03 | P1 | Compléter le cycle de vie des comptes staff et protéger l’accès CMS. Dépend de CMS-02. | Gestion du personnel limitée au rôle owner, révocation des sessions affectées et tests des permissions/échecs ; pas d’identifiants de démonstration par défaut. Tests API/PostgreSQL couvrent création, mise à jour, révocation, retrait et refus d’accès. | Terminé |
| CMS-04 | P1 | Ajouter la consultation des commandes et les transitions de statut sûres au CMS. Dépend de CMS-03. | Liste et détail tenant-scopés ; transitions explicites, snapshots commande conservés et tests API/store couvrant pagination, permissions, annulation et refus avec paiement ou statut incompatible. Tests Go/PostgreSQL réussis. | Terminé |
| CMS-05 | P1 | Terminer l’installation et l’exploitation du CMS sur machine locale/serveur autonome, sans dépendance obligatoire à Docker ni à un hébergeur. Dépend de CMS-01 à CMS-04. | Sous-étapes CMS-05a à CMS-05d ci-dessous terminées : démarrage sûr, migrations reproductibles, binaire portable et exercice vérifié de persistance/restauration. | En cours |
| CMS-06 | P3 | Évaluer un hébergeur distant (IONOS ou autre) uniquement si le besoin est confirmé, après CMS-05. | Environnement de test isolé, secrets hors Git, TLS, migrations et restauration vérifiés. Cette étape est facultative et ne bloque pas la livraison du CMS autonome. | Reporté — décision différée |
| CMS-07 | P1 | Ajouter un circuit de livraison tarifée : configuration admin de transporteurs, zones pays et tarifs fixes ; choix du client au checkout ; total serveur et snapshot transport sur commande. | Les tarifs sont tenant-scopés et revalidés dans la transaction de commande ; destinations sans mode actif refusées clairement ; le snapshot reste stable après modification des tarifs. Tests API, store PostgreSQL et frontend couvrent configuration, permissions, calcul du total et affichage client/admin. Les intégrations transporteur, étiquettes, points relais, taxes et paiement restent hors tranche. | Terminé |
| CMS-08 | P1 | Ajouter les confirmations et notifications du cycle de commande par outbox transactionnelle et remise SMTP côté hôte. | Événements de paiement reçu, traitement, préparation, expédition et livraison créés atomiquement avec historique horodaté ; worker avec TLS, bail et retries ; tests PostgreSQL/MariaDB et SMTP local ; aucun secret ou réseau SMTP dans Wasm. Mailpit est réservé au développement. Connecteurs réels de paiement, fournisseur et transporteur, domaine, MTA/SMTP, délivrabilité, rétention, rebonds et supervision restent à qualifier avant production. | En cours |

### Décomposition de CMS-05

| Sous-étape | Action / dépendance | Critère d’acceptation et preuve | Statut |
|---|---|---|---|
| CMS-05a | Fiabiliser le démarrage et la configuration portable. | Le défaut d’écoute est limité à `127.0.0.1`; `LISTEN_ADDR` permet un bind explicite ; un `DATABASE_URL` invalide fait échouer le démarrage plutôt qu’un repli silencieux en mémoire. Tests unitaires, documentation et configuration Compose alignés. | Terminé |
| CMS-05b | Fournir un outil de migration de schéma utilisable sans Docker. Dépend de CMS-05a. | `cmd/db-migrate` applique les migrations embarquées dans l’ordre sous verrou PostgreSQL ; un bootstrap vierge est atomique, chaque mise à jour et son entrée de registre sont atomiques, et le ledger non contigu ou un schéma non géré sont refusés. Intégrations PostgreSQL vérifient le bootstrap utilisé par les suites, l’idempotence, l’échec/rollback et la réutilisation de la connexion. `go test ./...` et `go vet ./...` réussis. La validation Compose reste à faire sur une machine équipée de Docker. | Terminé |
| CMS-05c | Construire et lancer une distribution autonome sur les cibles retenues. Dépend de CMS-05b. | Terminé le 2026-10-17 : workflow natif sur Windows Server 2022 et Ubuntu 22.04 avec vet, tests, intégrations PostgreSQL, builds CGo, extraction, lancement et smoke tests HTTP des deux archives ([exécution #39032334862](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39032334862)). Les deux artifacts sont publiés ; PostgreSQL reste un service séparé. | Terminé |
| CMS-05d | Vérifier persistance, sauvegarde et restauration locales. Dépend de CMS-05c. | Exercice réussi le 2026-10-17 sur PostgreSQL 16.15 isolé : création catalogue/commande via l'API, vérification après redémarrage, dump custom puis restauration dans une base isolée ; contrôles SQL positifs pour le produit, le owner et les snapshots client/ligne. `cmd/db-backup` sauvegarde PostgreSQL ou MariaDB, archive le FileStore, inscrit le dialecte et les empreintes SHA-256, vérifie les dumps et restaure la base dans une destination vide ainsi que le FileStore séparément. Intégration MariaDB 11.8.8 réussie le 2026-10-19 pour données, triggers, procédures et refus d'une destination non vide. Le pilote portable Windows sur PostgreSQL 16.9 confirme migrations, connexion owner et persistance des cinq produits après redémarrage du CMS et de PostgreSQL. Rétention choisie : 14 quotidiennes/8 hebdomadaires/12 mensuelles ; RPO provisoire : 24 h ; RTO à mesurer. Options examinées : stockage objet S3 européen (IONOS/Scaleway) ou SFTP distant ; orientation provisoire vers un bucket objet chiffré côté client, fournisseur non choisi. La configuration du stockage extérieur, le chiffrement et l'exercice RTO sont mis en attente jusqu'à stabilisation du CMS et essais de plusieurs imports hors paiement. | En attente — phase externe différée |

Le périmètre média initial accepte les URI gérées par le propriétaire (par
exemple un CDN) ; l’upload binaire et l’intégration à un fournisseur d’objets
feront l’objet d’une décision et d’une validation de sécurité séparées. La cible
de déploiement intermédiaire est une machine locale ou un serveur autonome ;
Docker peut rester une commodité de développement. L’ordre de travail est
CMS-05b (migrations), CMS-05c (distribution), puis CMS-05d (restauration).
IONOS ne sera évalué que si le besoin est confirmé.

## Historique des révisions

| Version | Date | Modification | Approbation |
|---|---|---|---|
| 1.0 | 2026-10-12 | Création du plan directeur initial pour le pipeline PHP→Wasm et la maîtrise des artefacts. | En attente du responsable produit/qualité |
| 1.1 | 2026-10-12 | Résultat du spike Go WASI consigné ; QP-01 bloqué en attente d’un invité sans imports WASI. | En attente du responsable produit/qualité |
| 1.2 | 2026-10-12 | Clarification de l’exigence de portabilité et ajout d’une matrice d’hôtes runtime distincte du poste de build. | En attente du responsable produit/qualité |
| 1.3 | 2026-10-13 | Ajout d’une matrice initiale séparant runtime CGo, guests Wasm et worker de build ; aucune cible n’est déclarée supportée sans validation. | En attente du responsable produit/qualité |
| 1.4 | 2026-10-14 | Suite Windows/amd64 validee ; correction du FileStore avec DACL NTFS protege et documentation du test. QP-10 reste ouvert en attente de l'integration et des cibles de production. | En attente du responsable produit/qualité |
| 1.5 | 2026-10-15 | Ajout de la feuille de route vérifiable du CMS : navigateur, médias, cycle staff, commandes et répétition de déploiement IONOS isolée. | En attente du responsable produit/qualité |
| 1.6 | 2026-10-15 | Ajout de la documentation API staff/commandes et mise à jour des preuves des étapes CMS-02 à CMS-04 après validation Go/PostgreSQL. | En attente du responsable produit/qualité |
| 1.7 | 2026-10-15 | Réorientation de CMS-05 vers le déploiement portable sur machine locale/serveur autonome ; IONOS devient optionnel et hors chemin critique. | En attente du responsable produit/qualité |
| 1.8 | 2026-10-15 | CMS-02 vérifié dans le navigateur ; objectif de portabilité locale précisé et sécurisation du démarrage direct en cours. | En attente du responsable produit/qualité |
| 1.9 | 2026-10-15 | Décomposition de CMS-05 en démarrage sûr, outil de migrations, distribution Windows/Linux et exercice de sauvegarde/restauration ; IONOS isolé en étape facultative CMS-06. | En attente du responsable produit/qualité |
| 2.0 | 2026-10-15 | CMS-05b terminé : commande Go de migration embarquée, bootstrap et mises à jour atomiques, registre contrôlé, documentation autonome mise à jour et intégrations PostgreSQL réussies. | En attente du responsable produit/qualité |
| 2.1 | 2026-10-15 | CMS-05c démarré : paquetage autonome Go et workflow de build natif Windows/Linux amd64 ; build et démarrage avec assets embarqués vérifiés localement sous Windows, validation des runners encore attendue. | En attente du responsable produit/qualité |
| 2.2 | 2026-10-17 | QP-10 : tests Go, vet, intégrations PostgreSQL 16 et build natif Windows/amd64 exécutés ; les tests IR inspectent désormais les imports du Wasm généré. Décision de cibles de production encore ouverte. | En attente du responsable produit/qualité |
| 2.3 | 2026-10-17 | CMS-05c : archive Windows autonome construite et extraite ; health, interface admin et catalogue du binaire empaqueté vérifiés en HTTP hors dépôt. Runners GitHub et installation PostgreSQL restent à valider. | En attente du responsable produit/qualité |
| 2.4 | 2026-10-17 | CMS-05c : workflow natif GitHub Actions réussi sur Windows/Linux amd64 ; tests, builds et deux artifacts confirmés. Exécution du paquet Linux et installation avec PostgreSQL standard restent ouvertes. | En attente du responsable produit/qualité |
| 2.5 | 2026-10-17 | CMS-05c : installation des exécutables Windows construits localement contre PostgreSQL 16.15 isolé. CMS-05d : persistance après redémarrage et sauvegarde/restauration vérifiées ; politique de rétention/hors machine et lancement de l'archive Linux restent ouverts. | En attente du responsable produit/qualité |
| 2.6 | 2026-10-17 | CMS-05c terminé : distributions Windows/Linux extraites et lancées sur runners natifs ; archive Windows publiée installée contre PostgreSQL 16.15. CMS-05d reste en cours, car rétention et stockage protégé hors machine sont à décider. | En attente du responsable produit/qualité |
| 2.7 | 2026-10-17 | CMS-05d : rétention locale, RPO de 24 h et stockage protégé de développement cadrés ; commande de sauvegarde/restauration PostgreSQL/FileStore et intégration réelle validées. Stockage hors machine, chiffrement et mesure du RTO restent ouverts. | En attente du responsable produit/qualité |
| 2.8 | 2026-10-17 | QP-10 : Windows/amd64 et Linux/amd64 retenus par le responsable produit ; le workflow natif de distribution exécute désormais la suite d’intégration PostgreSQL sur les deux runners. Support non déclaré avant réussite du workflow sur les deux cibles. | En attente du responsable qualité |
| 2.9 | 2026-10-17 | QP-01 : spike Rust `no_std` `wasm32-unknown-unknown` épinglé à Rust 1.99.1, Cargo sans dépendances et lockfile vérifié en `--frozen` ; import unique, exports, appel ABI Wasmtime et égalité octet à octet de builds depuis deux répertoires propres validés localement sur Windows/amd64. CI générale Linux réussie ; premier run natif a validé le spike sur les deux OS, puis révélé des prérequis de runner d'intégration, corrigés dans le workflow. Confirmation matricielle et chaîne de promotion restent attendues. | En attente du responsable qualité |
| 2.10 | 2026-10-17 | QP-01/QP-10 : le run 39032009205 confirme l'exécution du workflow sur les runners mais échoue avant les intégrations Linux, car Ubuntu 22.04 ne trouve pas `postgresql-client-16` dans ses dépôts activés. Le workflow ajoute le dépôt PGDG ; un nouveau run complet reste requis avant validation des cibles. | En attente du responsable qualité |
| 2.11 | 2026-10-17 | QP-10 : le run 39032334862 passe sur Windows/Linux amd64, intégrations PostgreSQL 16, tests Rust/Wasm, builds CGo, smoke HTTP et publication des deux artifacts ; QP-10 est terminé pour cette matrice. CI générale verte (run 39032300222). QP-01 reste en cours pour la chaîne de build/promotion. | En attente du responsable qualité |
| 3.0 | 2026-10-17 | Pilote CMS Windows sans privilèges admin : binaires natifs Go/CGo installés sous `%LOCALAPPDATA%`, PostgreSQL 16.9 de développement limité à loopback, scripts utilisateur de démarrage/arrêt, migrations idempotentes, bootstrap et authentification owner, cinq produits conservés après redémarrage CMS/PostgreSQL. Les identifiants sont protégés par DPAPI. Les outils `pg_dump`/`pg_restore`, une sauvegarde chiffrée hors machine et la mesure du RTO restent à traiter ; pilote non destiné à la production. | En attente du responsable qualité |
| 3.1 | 2026-10-17 | CMS-07 : configuration transporteurs/zones/tarifs fixes dans l’admin, endpoint public d’options, choix livraison au checkout, calcul transactionnel du total et snapshot transport consultable dans l’admin ; intégrations transporteur et tarifs commerciaux restent à décider. | En attente du responsable qualité |
| 3.2 | 2026-10-19 | CMS-05d : `cmd/db-backup` prend en charge les dumps PostgreSQL et MariaDB, les empreintes/dialectes, la restauration dans une base vide et les artefacts séparés. Test réel sur MariaDB 11.8.8 : données, trigger, routine et destination non vide vérifiés ; stockage hors machine, chiffrement et mesure du RTO restent ouverts. | En attente du responsable qualité |
| 3.3 | 2026-10-19 | Cycle de paiement fictif livré, réservé au développement/tests ; décision produit : prioriser le fonctionnement du CMS puis éprouver plusieurs imports hors paiement avant d’étudier un prestataire réel. Le périmètre précis des imports et la cible de sauvegarde hors machine restent à cadrer ; le RTO reste à mesurer. | En attente du responsable qualité |
| 3.4 | 2026-10-19 | CMS-05d externe mise en attente après examen des options S3 européennes (IONOS/Scaleway) et SFTP distant ; orientation vers un bucket objet chiffré côté client sans sélection de fournisseur. Reprise après stabilisation du CMS et essais d’imports ; prévoir une restauration chronométrée. Un module de paiement PrestaShop est proposé comme candidat ultérieur d’analyse statique, non comme intégration d’encaissement. | En attente du responsable qualité |
| 3.5 | 2026-10-19 | Reprise CMS vérifiée : suite Go, API/PostgreSQL, syntaxe admin/vitrine ; modules non financiers WooCommerce (`woocommerce-extra-product-sorting-options`) et PrestaShop (`ps_newproducts`) ajoutés depuis des commits licenciés et analysés statiquement ; reconnaissance de `WidgetInterface.renderWidget` ajoutée. Le pilote catalogue Wasm exécuté reste basé sur une source synthétique. QP-08 reste en cours : import amont/manifeste, lowerings fidèles et publication/runtime des deux modules restent à réaliser ; aucun PHP tiers n’est exécuté. | En attente du responsable qualité |
| 3.6 | 2026-10-19 | Pilote natif Selfnamed WooCommerce 2.1.0 réussi dans WordPress 6.9.2/WooCommerce 10.9.0/MariaDB 11.8.3, sous Podman rootless, réseau interne sans port publié, mock API, données synthétiques et bearer token fictif. Le test vérifie une commande, son groupe de livraison et l’absence de doublon ; volumes temporaires nettoyés. Cela valide uniquement le plugin original, pas son import ni son exécution EUROMNI/Wasm. Aucun credential réel utilisé ; leur éventuel usage reste à autoriser séparément. | En attente du responsable qualité |
| 3.7 | 2026-10-21 | QP-01/QP-10 : la preuve Rust journalise versions et SHA-256 de deux builds propres ; le CI ordinaire est configuré pour exécuter le guest sur Linux/amd64 et Windows/amd64 ; les archives de distribution reçoivent un checksum adjacent. Test local Linux réussi ; la matrice CI de ce changement reste à exécuter. QP-01 reste en cours : aucun processus métier de traduction/promotion n’est encore qualifié. La réorganisation du backend est différée. | En attente du responsable qualité |
| 3.8 | 2026-10-21 | Le run de distribution 39577013455 échoue avant packaging : sous Windows, le test de credentials MariaDB détecte que le dossier temporaire a été créé avant l’application de l’ACL privée ; sous Linux, les tests d’intégration contiennent des attentes antérieures au registre de migrations et aux pins `module_builds`. Le code et les tests sont corrigés localement, et la CI ordinaire étendue à Windows ; nouvelle exécution Linux/Windows requise. La suite locale échoue encore dans le test dump/restore car les outils locaux PostgreSQL 18.6 sont incompatibles avec le serveur embarqué PostgreSQL 16. | En attente du responsable qualité |
| 3.9 | 2026-10-21 | Le run CI 39580273018 valide Linux et le guest Wasm sur les deux runners, mais échoue sur Windows parce que le test des credentials comparait `Mode().Perm()` (sémantique POSIX) au lieu de vérifier l’ACL Windows. Le test vérifie désormais le dossier privé hérité. Le run 39580872372 passe ensuite les tests Go complets sur Linux/Windows et le guest Rust/Wasm sur les deux cibles ; une nouvelle qualification du workflow de distribution reste requise. | En attente du responsable qualité |
| 3.10 | 2026-10-23 | Le pilote natif Selfnamed est aligné sur la version `2.0.19` de la boutique restaurée (archive WordPress.org SHA-256 `6e464f7130f905d35686c1ded1faeadc010da784c8a18222b354b7f9be87809f`). Trois fixtures synthétiques caractérisent le brouillon, les prix/normalisation décimale, le stock, les tags, le volume, les poids et le réimport idempotent ; le mock couvre aussi le refus HTTP 401. L’intégration WordPress/WooCommerce, le test oracle Go et le flux commande mock passent. Stack interne éphémère, aucun appel API externe ni téléchargement d’image. Cela ne valide ni ingestion ni exécution Wasm et reste distinct de QP-08. Garde-fou pour la prochaine étape : l’API d’écriture catalogue EUROMNI ne sait pas encore conserver le statut brouillon source ; pas de publication Wasm avant définition d’un staging sûr. Détails dans [`module-validation-progress.md`](module-validation-progress.md#caractérisation-du-mapping-catalogue-selfnamed). | En attente du responsable qualité |
| 3.10 | 2026-10-21 | QP-10 consolidé : CI Go/Rust verte sur Linux/amd64 et Windows/amd64 (run 39580872372) ; distribution CMS verte sur les deux cibles (run 39581659670, commit 6610399), avec tests d'intégration PostgreSQL 16, smoke HTTP, checksum SHA-256 et deux artefacts publiés. QP-01 reste en cours : prochaine tranche proposée, cadrer le contrat et le test d'acceptation du flux de build métier sur un seul module/version épinglé, sans refonte backend ni traduction PHP. | En attente du responsable qualité |
| 3.11 | 2026-10-21 | QP-01 cadrage précisé sur le snapshot PrestaShop `ps_newproducts` 2.0.1, commit amont `72e9476406319cbb91ac383121ca9967dc61f277`, ZIP SHA-256 `0f0750d8c2a1e6e12fc67169224f597f166b5c80b99cb306f53e9b339b17af77`. Contrat d'entrée/sortie et test d'acceptation décrits ; l'étiquette `euromni-test-adapter` est explicitement non représentative d'une version CMS réelle. Restent à implémenter l'épinglage obligatoire de la source et la preuve de reproductibilité/imports/exports dans le parcours de publication candidat. | En attente du responsable qualité |
| 3.12 | 2026-10-21 | QP-01 implémenté pour le pilote épinglé : l'importeur refuse toute archive autre que le ZIP PrestaShop dont le SHA-256 est documenté ; tests vérifient le rejet sans déplacement du build/pin, la reproductibilité de l'IR/Wasm/manifeste, et l'interface Wasm (seul import `euromni.call`, exports `memory`/`run`). Tests archive ciblés et intégration PostgreSQL/Wasmtime réussis. Le périmètre reste le snapshot et le compilateur IR embarqué, sans revendication de compatibilité CMS réel. | En attente du responsable qualité |
| 3.13 | 2026-10-21 | QP-01 : le test d'acceptation utilise `PublishModuleBuildOnly` pour publier un candidat immuable sans installation ; il relit et valide artefact/manifeste, constate le pointeur courant candidat avec pin installé inchangé, puis promeut explicitement le digest et exécute le build promu via Wasmtime. Tests pipeline et intégration PostgreSQL/Wasmtime ciblée réussis ; CI Go Linux/Windows du commit `ed73e39` verte (run 39600548552). | En attente du responsable qualité |
| 3.14 | 2026-10-21 | QP-03 lancé : contrat du premier incrément limité à un POST admin authentifié de ZIP brut, validation en mémoire sans extraction/exécution, puis stockage privé en quarantaine séparée, sans publication de build. Limites globales, chemins/types/doublons et tests d'abus à couvrir ; habilitation production conditionnée aux validations multiplateformes et à une revue sécurité. | En attente du responsable qualité |
| 3.15 | 2026-10-21 | QP-03 premier incrément codé localement : route d'import authentifiée pour les rôles owner/admin, activée uniquement par `EUROMNI_QUARANTINE_DIR`, validation stricte du manifeste et du ZIP, écriture privée adressée par SHA-256 dans un espace boutique séparé, quota local de 256 MiB, aucune extraction/exécution/publication. Tests unitaires complets `go test ./...` réussis. Les tests du package quarantaine compilent pour Windows ; les packages Wasmtime nécessitent une toolchain CGo Windows et restent à valider en CI. Revue de sécurité, rétention/rate-limit et déploiement multi-instance encore en attente. | En attente du responsable qualité |
| 3.16 | 2026-10-21 | Relèvement Go 1.25 et mises à niveau sécurité pgx v5.9.2/x-text v0.41.0 ; govulncheck ne trouve plus de vulnérabilité atteignable. Ajout de timeouts HTTP. Tests Go 1.25, vet, intégrations PostgreSQL/Compose et Playwright passent localement. Gosec SQL G201/G202 à zéro ; 22 alertes gosec générales restent à trier. Validation Windows Go 1.25 en CI et revue sécurité indépendante à venir ; QP-10 repasse en vérification. | En attente du responsable qualité |
| 3.16 | 2026-10-21 | Relèvement Go 1.25 et mises à niveau sécurité pgx v5.9.2/x-text v0.41.0 ; govulncheck ne trouve plus de vulnérabilité atteignable. Ajout de timeouts HTTP. Tests Go 1.25, vet, intégrations PostgreSQL/Compose et Playwright passent localement. Gosec SQL G201/G202 à zéro ; 22 alertes gosec générales restent à trier. Validation Windows Go 1.25 en CI et revue sécurité indépendante à venir ; QP-10 repasse en vérification. | En attente du responsable qualité |
| 3.17 | 2026-10-21 | Go 1.25.0 aligné dans `go.mod`, Compose et documentation ; pgx v5.9.2/x-text v0.41.0 corrigent les avis atteignables. Timeouts HTTP CMS ajoutés. Tests/vet, intégrations PostgreSQL Compose et Playwright passent sous Go 1.25 ; govulncheck atteint zéro finding, G114 corrigée. La CI Windows Go 1.25 reste à exécuter après push ; 22 alertes gosec non SQL restent à trier. | En attente du responsable qualité |
| 3.18 | 2026-10-21 | Garde-fou ajouté : le store applicatif refuse `default_query_exec_mode=simple_protocol` dans `DATABASE_URL`, avec test sans connexion ; le migrateur conserve son usage simple uniquement pour le SQL embarqué. `go test`/`go vet` Go 1.25, intégrations PostgreSQL Compose, CMS et Playwright passent ; govulncheck ne trouve aucune vulnérabilité atteignable. Reste la CI Windows sous Go 1.25, l’audit indépendant et le tri des 22 alertes gosec restantes. | En attente du responsable qualité |
| 3.19 | 2026-10-23 | CMS-08 démarré : outbox de confirmation créée dans la transaction checkout, worker SMTP séparé, STARTTLS/TLS implicite et retries, profil Mailpit pour développement. L’envoi réel et l’exploitation du MTA attendent un choix de domaine et d’hébergement. | En attente du responsable qualité |
| 3.20 | 2026-10-24 | QP-01/02/06/07 : build Selfnamed `no_std` lié à l’IR et reproductible, puis contrat transport-indépendant manifeste/IR canonique/Wasm avec validation structurelle ABI/imports/exports/mémoire. `go test ./...`, `go vet ./...`, test de reproductibilité `-tags=wasmtoolchain` et intégration PostgreSQL 16/Wasmtime isolée passent localement sous Linux/amd64. Les hashes démontrent l’intégrité, pas l’authenticité ; aucune signature, politique de confiance ni distribution n’est encore définie. La traduction PHP générale et le worker de production ne sont pas qualifiés. | En attente du responsable qualité |
| 3.21 | 2026-10-24 | QP-02/07 : `ValidateBuildPackage` est maintenant exécuté par les parcours de publication avant `artifact.Store.Put`; un module avec import Wasm additionnel est refusé sans stockage. `go test ./...`, `go vet ./...`, test de reproductibilité `-tags=wasmtoolchain`, intégrations PostgreSQL 16/Wasmtime ciblées et `git diff --check` passent localement sous Linux/amd64. Le run CI Go #76 du commit `60178f5` passe sur Linux/Windows, tests/vet et guest Rust ; le run #77 du commit `fe4e90b` passe également. Réception de package, authenticité et distribution restent hors périmètre. | En attente du responsable qualité |
| 3.22 | 2026-10-24 | Cadrage de confiance sans choix de transport : proposition non approuvée de signature Ed25519 détachée sur les digests exacts du manifeste, de l’IR et du Wasm ; clés publiques identifiées par empreinte et approuvées hors package ; vérification distincte de la validation structurelle et de la promotion. Rotation, révocation, provisionnement, rôle des signataires, approbation métier, limites de transfert et décision catalogue central/import local restent ouverts. Aucun code de signature ajouté. | En attente du responsable qualité |
| 3.23 | 2026-10-24 | Prototype Ed25519 détaché implémenté après approbation explicite du périmètre : message domain-separated contenant les trois SHA-256, enveloppe JSON canonique plafonnée à 4 KiB, ID de clé par empreinte, clé de confiance injectée hors package, pas d’horodatage/expiration ; tests d’altération, signature non fiable/malformée et package structurellement invalide. Aucun service de signature ni branchement import/publication/promotion ; gestion des clés et politique de production restent ouvertes. | En attente du responsable qualité |
| 3.24 | 2026-10-24 | Orientation de distribution hybride retenue : builds locaux possibles et catalogue central signé facultatif. La prochaine tranche doit définir la politique minimale de confiance et les limites de transfert avant de relier le vérificateur à une réception de packages ; package reçu candidat seulement, promotion explicite conservée. | En attente du responsable qualité |
| 3.25 | 2026-10-24 | Base de politique de confiance retenue : l'éditeur signe, chaque installation configure ses clés de confiance explicitement, un package distribué validé devient candidat et sa promotion reste humaine/explicite ; les builds locaux restent possibles sans catalogue et sans signature obligatoire. Canal/protection/rotation des clés, limites de transfert et rôles de promotion à spécifier avant tout flux opérationnel. | En attente du responsable qualité |
| 3.26 | 2026-10-24 | Premier socle de configuration des clés implémenté : lecteur de fichier JSON strict et versionné, borné à 64 KiB/256 clés, validant Base64, empreintes et doublons ; chargé au démarrage par `EUROMNI_BUILD_TRUST_FILE` avec échec explicite si invalide. Tests pipeline et configuration CMS ciblés réussis. Pas encore relié à un récepteur de packages ; protection opérationnelle et limites de transfert restent à faire. | En attente du responsable qualité |
| 3.27 | 2026-10-24 | Réception prototype `POST /v1/admin/modules/build-packages` : ZIP signé exact plafonné à 64 MiB, lecture bornée sans extraction, validation clé Ed25519 + package ABI/manifeste, staging privé content-addressed par boutique plafonné provisoirement à 256 MiB. Tests ciblés pipeline/API/store passent. Aucun build publié, pin, installation ni promotion n'est modifié ; liste/revue/rétention, quota multi-instance et revue sécurité indépendante restent ouverts. | En attente du responsable qualité |
| 3.28 | 2026-10-24 | Approbation humaine backend : lecture par digest et approbation après revalidation de la signature avec les clés courantes ; autorisation `owner`/`admin`, scope boutique et attribution utilisateur/rôle/heure UTC. Le sidecar local est immuable via l'API mais n'est pas un journal d'audit centralisé ; aucun build/pin/installation/promotion ne change. Tests ajoutés pour les deux rôles, rôle refusé, non-authentifié, candidat absent/altéré, clé retirée, répétition et isolation boutique. `go test ./...`, `go vet ./...` et `git diff --check` réussis sous Linux ; CI Windows et revue sécurité indépendante non exécutées. Inventaire, UI, rejet, rétention et quota multi-instance restent ouverts. | En attente du responsable qualité |
| 3.29 | 2026-10-24 | Inventaire backend `GET /v1/admin/modules/build-packages/candidates` ajouté avec pagination bornée (50 par défaut, 100 maximum), digest/taille/état et approbation, limité à la boutique ; les rôles restent `owner`/`admin`. La liste ne lit pas les ZIP et n'affirme pas leur validation ; le détail revalide avant décision. Tests store/API ajoutés. `go test ./...`, `go vet ./...` et `git diff --check` réussis sous Linux. UI, rejet, rétention, coordination multi-instance et revue sécurité indépendante restent ouverts. | En attente du responsable qualité |
| 3.30 | 2026-10-24 | Interface Modules pour consulter les candidats signés, ouvrir le détail revalidé et approuver explicitement avec confirmation. Pagination, états accessibles, erreurs explicites et invalidation des réponses en changement de boutique ; aucun chemin de publication/activation ajouté. `go test ./...`, `go vet ./...`, `go test ./frontend`, `node --check frontend/admin/admin.js` et `git diff --check` réussis sous Linux. Rejet, rétention, coordination multi-instance et revue sécurité indépendante restent ouverts. | En attente du responsable qualité |
| 3.31 | 2026-10-24 | Décisions de package étendues au rejet avec motif obligatoire et à la réouverture d'un candidat rejeté. Historique local borné (256 décisions / 256 KiB), état vérifié, attribution actor/rôle/UTC ; transitions invalides refusées et détail/API exposent l'historique. L'API ne modifie pas les événements existants, mais la trace locale n'est pas inviolable contre un administrateur système et ne coordonne pas les instances. Admin permet rejeter/rouvrir avec confirmation. Tests de transitions API/store et de conservation d'historique ajoutés. Validation complète, vet, frontend et syntaxe JS réussis sous Linux ; CI Windows et revue indépendante à venir. | En attente du responsable qualité |
| 3.32 | 2026-10-24 | Politique de rétention validée par le responsable produit : purger après 90 jours le payload des candidats dont le dernier état est rejeté ; préserver pending/approved et le sidecar des décisions. `PurgeRejectedBefore` vérifie le digest avant suppression ; la liste et le détail exposent l'historique conservé avec `payload_available:false`, sans action de réouverture. `cmd/build-package-candidate-reconciler` fournit le passage ponctuel à planifier quotidiennement et est inclus dans les paquets Linux/Windows. Tests couvrent expiration, rejeu après réouverture, états conservés, historique consultable et annulation. `go test ./...`, `go vet ./...`, `go test ./frontend`, `node --check frontend/admin/admin.js`, compilation du reconciler et `git diff --check` réussis sous Linux ; ordonnanceur externe, CI Windows, multi-instance et revue de sécurité indépendante restent à faire. | En attente du responsable qualité |
| 3.33 | 2026-10-24, 10:49:38 UTC+02:00 | Revue de sécurité indépendante en lecture seule du parcours des packages signés, sur `df7a6a7` : ZIP/signature, authentification et scope boutique, stockage, décisions, purge et historique retenu. Aucun problème de sécurité exploitable identifié dans le périmètre. L'ordonnancement opérationnel de la purge et le multi-instance n'ont pas été vérifiés ; aucune approbation de production n'en découle. | Résultat à examiner par le responsable qualité |
| 3.34 | 2026-10-24 | Quota de candidats signés coordonné par verrou advisory transactionnel PostgreSQL par boutique. Imports, décisions et purges sont sérialisés entre processus ; le reconciler exige `DATABASE_URL` et la même base que le CMS. Le déploiement multi-instance exige aussi un volume partagé avec hard links et visibilité cohérente ; ce support stockage n'est pas qualifié en production. `go test -p 1 -tags=integration -count=1 ./...` passe sous Linux avec les clients PostgreSQL 16.15 ; `go vet ./...`, frontend, syntaxe JS et `git diff --check` passent également. Le run CI Linux a réussi ; Windows a révélé que les deux tests d’intégration créaient des répertoires avec des ACL héritées, refusées correctement par `privatefs`. Les fixtures de ces tests utilisent désormais `privatefs.EnsureDirectory` ; le nouveau run CI doit confirmer la correction. Aucun système de fichiers partagé de production n'est qualifié et la revue de sécurité antérieure ne couvre pas cette tranche. | En attente du responsable qualité |
| 3.35 | 2026-10-24 | Selfnamed : ajout d’un staging admin explicite en brouillon, idempotent par boutique/fournisseur/référence, avec locale/devise prises dans la boutique, tags, volume à unité inconnue et disponibilité respectée par ajout panier/checkout. Migrations PostgreSQL/MariaDB, API REST, GraphQL et modèle mémoire raccordés ; aucun appel automatique du guest Wasm, téléchargement d’image ou publication. Tests unitaires store/API/migrations, intégration PostgreSQL de staging et API, flux checkout PostgreSQL/MariaDB réussis sous Linux sur bases temporaires. Le staging ne qualifie pas la traduction PHP ni l’intégration automatique runtime→import. | En attente du responsable qualité |
| 3.36 | 2026-10-24 | Création du cadre documentaire SMQ EUROMNI : périmètre du développement, contexte/parties intéressées, responsabilités, processus, objectifs proposés, registre initial des risques, maîtrise opérationnelle, écarts/actions correctives, audit et revue de direction, avec correspondance indicative aux clauses 4–10 d’ISO 9001. Brouillon non approuvé ; responsables, objectifs, édition applicable du référentiel, audit interne et revue de direction restent à confirmer ou réaliser. | En attente du responsable produit/technique |
| 3.37 | 2026-10-24 | QP-05 : contrat IR v3 documenté pour déclencheurs `hook`/`manual` et six appels catalogue ; validation désormais stricte des types de déclencheur et des noms, capabilities exclusivement dérivées du mapping hôte, schémas explicites et tests de rejets. `go test ./internal/pipeline` passe sous Linux. Pas d’expressions, de chaînes d’opérations ni de traduction PHP générale. | En attente du responsable produit/technique |
| 3.38 | 2026-10-24 | QP-06 : les workflows CI Go et distribution sélectionnent explicitement Rust 1.99.1 ; le job `wasm-toolchain-spike` couvre aussi la compilation Selfnamed reproductible sur Linux/Windows avec fetch Cargo verrouillé. `go test -tags=wasmtoolchain ./internal/pipeline -run '^TestCompileSelfnamedPreviewIRToWasmIsReproducibleAndBoundToIR$'` réussi localement sous Linux ; les workflows modifiés restent à confirmer par leur prochain run distant. | En attente du run CI |
