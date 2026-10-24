# Reprise du projet EUROMNI

Ce document est le point de départ pour le prochain développeur. Lire d'abord
les consignes du dépôt et le statut Git actuel ; ce guide décrit le contexte,
pas une preuve que l'arbre de travail est propre.

## État connu

- Branche de travail : `main`. Vérifier `git status --short --branch` et
  `git log -5 --oneline` avant de modifier quoi que ce soit.
- Go minimal du projet : 1.25.0 (`go.mod`). Les dépendances `pgx/v5` et
  `x/text` ont été mises à niveau vers les versions correctives v5.9.2 et
  v0.41.0 ; les images Compose utilisent `golang:1.25-bookworm`.
- QP-01 dispose d'un parcours PrestaShop `ps_newproducts` 2.0.1 épinglé par
  son commit source et SHA-256 d'archive. Il couvre builds répétés,
  vérification du manifeste, publication candidate sans installation, pin
  inchangé, promotion explicite et exécution Wasmtime. Cela ne qualifie ni
  PrestaShop réel ni la traduction PHP générale.
- Le contrat prototype d'un build distribuable comprend trois composants
  séparés (`manifest.json`, `module.ir.json` canonique et `module.wasm`) ; il
  reste indépendant d'une archive, d'un endpoint ou d'un fournisseur. Le helper
  `pipeline.ValidateBuildPackage` vérifie leur cohérence, les versions
  approuvées et l'interface Wasm (import `euromni.call`, exports `memory`/`run`,
  signatures et limites mémoire). Les parcours existants de publication et
  d'installation passent par ce validateur avant `artifact.Store.Put`. La
  réception de packages signés et leur approbation humaine backend existent
  maintenant ; elles ne déclenchent toujours aucune publication ni promotion.
- Les hashes prouvent l'intégrité, pas l'identité de l'éditeur. Le prototype
  Ed25519 approuvé signe les digests exacts des trois blobs via une enveloppe
  détachée ; `ValidateSignedBuildPackage` exige une clé publique explicitement
  fournie hors package. La clé privée vient du signataire et n'est jamais
  stockée par ces helpers. Le 24 octobre 2026, le choix d'orientation est
  devenu hybride : les builds locaux restent possibles et un catalogue de
  packages signés est facultatif. La base de politique retenue est : signature
  par l'éditeur du module, clés de confiance configurées explicitement pour
  chaque installation, validation des packages reçus avant leur enregistrement
  comme candidats et promotion humaine explicite ; la signature d'un build
  local n'est pas obligatoire. Cette politique n'est pas encore opérationnelle :
  protection des clés, rotation/retrait, identité affichée, durée de validité
  et rôles autorisés à promouvoir restent à spécifier. Le socle comprend
  maintenant
  `pipeline.LoadTrustedBuildPackageKeys(path)`, qui lit un JSON versionné strict
  (64 KiB maximum, 256 clés maximum, Base64 canonique, ID SHA-256 vérifié,
  doublons refusés). Le CMS le charge au démarrage via
  `EUROMNI_BUILD_TRUST_FILE` ; le fichier configuré invalide fait échouer le
  démarrage.
- La route admin `POST /v1/admin/modules/build-packages` reçoit un ZIP signé
  exact sous 64 MiB, vérifie la confiance et le package sans extraction, puis
  stocke l'archive originale comme candidat dans un FileStore privé par boutique
  (`EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR`, quota provisoire de 256 MiB par
  boutique). `GET /v1/admin/modules/build-packages/candidates/{packageSHA256}`
  permet de relire le candidat en revalidant signature et contenu ;
  `GET /v1/admin/modules/build-packages/candidates` fournit l'inventaire paginé
  de la boutique (digest, taille et état) sans ouvrir les archives. Le détail
  expose les décisions. `POST
  /v1/admin/modules/build-packages/candidates/{packageSHA256}/approval`,
  `/rejection` et `/reopen` enregistrent des événements attribués à l'acteur,
  son rôle et l'heure UTC ; un rejet exige un motif et peut être rouvert pour
  revenir à `pending`. L'historique local est immuable ; ces décisions ne créent
  aucun build publié et n'installent/promeuvent rien. L'admin affiche la liste
  paginée, le détail revalidé et les actions avec confirmation ; la liste seule
  ne présente pas le candidat comme validé. Les archives rejetées expirent 90
  jours après le dernier rejet ; la commande
  `cmd/build-package-candidate-reconciler` doit être planifiée quotidiennement.
  Elle retire uniquement le payload, conserve la trace locale et laisse la
  ligne d'historique consultable dans Modules sans action de réouverture.
  Candidats `pending`/`approved` non purgés automatiquement. Quota multi-instance,
  sauvegarde et restauration restent à traiter.
- Revue de sécurité indépendante en lecture seule du parcours packages signés,
  sur `df7a6a7` : effectuée le 24 octobre 2026, résultat consigné à 10:49:38
  (UTC+02:00). Aucun problème exploitable identifié sur ZIP/signature, accès
  tenant, stockage, décisions, purge et historique conservé. La planification
  effective de la purge et le multi-instance n'ont pas été vérifiés ; ne pas
  interpréter ce résultat comme une autorisation de production.
- QP-06 est en cours et QP-07 en vérification, uniquement pour le prototype
  Selfnamed lié à son IR et le validateur de package ; le chemin de publication
  rejette désormais un Wasm non conforme avant stockage. Aucune traduction PHP
  générale ni toolchain/worker de production n'est qualifié. QP-04/05 restent
  des prérequis à tout élargissement métier.
- QP-03 demeure un chantier distinct : l'endpoint admin accepte un ZIP brut,
  le valide en mémoire, puis le dépose en quarantaine privée séparée des builds.
  Aucun contenu PHP n'est extrait ou exécuté.
- L'endpoint `POST /v1/admin/modules/imports` accepte `application/zip`,
  requiert un Bearer owner/admin, une archive valide avec manifeste, et répond
  `503` si `EUROMNI_QUARANTINE_DIR` n'est pas configuré.
- La quarantaine utilise un chemin par boutique et SHA-256, un répertoire privé
  local, une limite de 16 MiB par ZIP et un quota local de 256 MiB par boutique.
  Ce quota n'est pas coordonné entre processus ; ne pas partager le répertoire
  entre plusieurs instances.
- Le circuit ne fournit pas encore de revue/promotion depuis la quarantaine ni
  de limitation de débit. `cmd/quarantine-reconciler` permet une purge
  ponctuelle par âge de fichier, mais aucune durée ou fréquence automatique
  n'est définie. Ne pas l'activer en production avant décision sur ces contrôles
  et revue de sécurité.
- Une copie WooCommerce exportée a été restaurée le 23 octobre 2026 par
  Duplicator Lite 5.0.4 dans une sandbox Podman persistante distincte,
  WordPress 6.9.2 / PHP 8.3 / MariaDB 11.8.8, à
  `http://127.0.0.1:18083`. Le réseau `euromni-woo-sandbox` est interne,
  les connexions sortantes ont été vérifiées bloquées, WP HTTP externe, cron
  et mises à jour sont désactivés ; les volumes et le redémarrage persistent.
  L’import a atteint l’écran final avec les validations au vert ; les fichiers
  générés `installer.php`, `dup-installer/`, le journal de démarrage et l’archive
  recopiée dans le document-root ont été supprimés manuellement. L’accès à
  `/wp-admin/` est ouvert avec le compte administrateur local temporaire
  `sandbox-review`, créé car les identifiants restaurés ne sont pas disponibles.
  L’utilisateur doit changer ce mot de passe depuis son profil ; ne pas consigner
  le secret. L’export source sous `/home/fedobastian/woo` est intact.
  L’utilisateur a déclaré l’export assaini, mais il n’a pas été audité
  indépendamment : conserver sa confidentialité, ne pas demander ou noter de
  credentials. Voir les détails et les limites dans
  [`etat-projet.md`](etat-projet.md) et
  [`module-validation-progress.md`](module-validation-progress.md).
- Cette installation de référence sert à étudier le comportement réel du
  plugin Selfnamed et à orienter l’ingestion/traduction future de modules ;
  elle ne représente ni une importation EUROMNI ni une traduction Wasm du plugin.
  Maintenir les intégrations, paiements, e-mails et toute sortie réseau
  désactivés. La prochaine étape après finalisation est une inspection
  non-destructive de versions, hooks, flux catalogue/transport/commande et
  dépendances réseau, puis un test ciblé avec mock.
- Le back-office est organisé en six sections (Boutique, Catalogue, Commandes,
  Livraison, Modules et Équipe). Le parcours Chromium du 23 octobre a exercé les
  principales actions sur un CMS et une base PostgreSQL 16 jetables ; les
  résultats et la portée du runner existant sont consignés dans
  [`etat-projet.md`](etat-projet.md). L’instance jetable est arrêtée. Le runner
  Playwright versionné ne couvre pas encore toutes les actions administratives.
- `.vscode/` est un dossier non suivi préexistant : ne pas l'ajouter au commit
  sans accord explicite.
- Au début de la reprise du 24 octobre 2026, des changements locaux préexistants
  dans `cmd/quarantine-reconciler/main.go` (newline final) et
  `internal/store/store.go` (séparation d'import) ont été laissés hors du
  commit du package ; `.vscode/` est également resté hors commit. Vérifier leur
  état Git courant avant toute action, sans les réinitialiser ni les ajouter.

## Vérifications déjà exécutées

- Pour le contrat de package et le build lié à l'IR : `go test ./...`,
  `go vet ./...`, le test
  `go test -tags=wasmtoolchain ./internal/pipeline -run
  '^TestCompileSelfnamedPreviewIRToWasmIsReproducibleAndBoundToIR$' -count=1`,
  l'intégration PostgreSQL 16/Wasmtime isolée
  `go test -tags=integration -count=1 ./internal/pipeline -run
  '^TestUpstreamPrestaShopArchivePublishesAndRunsInIsolatedCMS$'` et
  `git diff --check` ont réussi sous Linux/amd64.
- Le run GitHub Actions Go CI #76 du commit `60178f5` est vert : tests/vet Go
  sous Linux/amd64 et Windows/amd64, plus exécution du guest Rust sur les deux
  runners. [Détails du run](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/40069691917).
- Le run GitHub Actions Go CI #77 du commit `fe4e90b` est vert après ajout du
  validateur avant publication. [Détails du run](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/40073494999).
- Après le run #76, la validation avant publication passe :
  `go test ./...`, `go vet ./...`, le test de reproductibilité
  `go test -tags=wasmtoolchain ./internal/pipeline -run
  '^TestCompileSelfnamedPreviewIRToWasmIsReproducibleAndBoundToIR$' -count=1`,
  l'intégration PostgreSQL 16/Wasmtime isolée
  `go test -tags=integration -count=1 ./internal/pipeline -run
  '^(TestUpstreamPrestaShopArchivePublishesAndRunsInIsolatedCMS|TestSelfnamedProductPreviewWasmRunsWithoutPersistence)$'`
  et `git diff --check` ont réussi sous Linux/amd64.
- `go test ./...` : réussi après les changements QP-03.
- `go vet ./...` : réussi.
- `git diff --check` : réussi.
- Après la mise à niveau Go/dépendances : `GOTOOLCHAIN=go1.25.0 go test ./...`,
  `GOTOOLCHAIN=go1.25.0 go vet ./...` et
  `podman compose --podman-run-args=--userns=keep-id -p euromni-cms-test --profile integration run --rm integration-test`
  passent. Sans `--userns=keep-id`, le conteneur ne peut pas nettoyer des
  outils PostgreSQL temporaires laissés dans `.tmp` par le mapping rootless.
- La stack CMS persistante a été recréée sans supprimer le volume PostgreSQL ;
  Go 1.25.14 est actif et health/admin/storefront répondent HTTP 200.
- Playwright sur le CMS PostgreSQL mis à niveau : connexion owner, dashboard et
  vitrine passent (10 produits).
- `gosec` G201/G202 : zéro finding SQL sur 78 fichiers. Le scan complet a
  signalé G114, corrigé par des timeouts HTTP ; 22 alertes restantes sont à
  trier. `govulncheck ./...` : zéro vulnérabilité atteignable après upgrade ;
  il reste un avis de package importé et 22 avis de modules sans chemin d'appel.
- Le store applicatif refuse `default_query_exec_mode=simple_protocol` dans
  `DATABASE_URL` ; un test le vérifie sans connexion réseau. Seul le migrateur
  utilise le protocole simple, pour les migrations embarquées non paramétrées.
- La CI Go du commit `7b166c5` (run #60) a réussi sur Ubuntu 24.04 et
  Windows 2022. Le job Windows a installé GCC UCRT64 puis passé `go vet ./...`
  et `go test ./...`, y compris les tests quarantaine. Cela qualifie le runner
  CI Windows pour l'ancien plancher Go ; le nouveau plancher 1.25 attend encore
  un run CI Windows après push.
- Le workflow standalone #8, tentative 2, a réussi sur Linux et Windows
  (intégrations, builds, smoke tests et publication d'artefacts), mais il cible
  le commit antérieur `6610399`. Il ne valide pas encore les références
  `checkout@v7`, `setup-go@v7` et `upload-artifact@v7` de `7b166c5`. Pour lever
  ce point, lancer un nouveau `workflow_dispatch` de `CMS standalone distribution`
  depuis `main`, debug activé, puis confirmer que le SHA du run est
  `7b166c503667726789b14b1120cae8f88a502fbc`. Ne pas rerun le run #8, car cela
  réutiliserait `6610399`. Vérifier les deux jobs de packaging, les smoke tests
  et les archives/checksums Linux et Windows publiés.
- `go test ./internal/quarantine ./cmd/quarantine-reconciler` : réussi après
  l'ajout de la purge ponctuelle (21 octobre 2026).
- Pour l’administration réorganisée : `go test ./frontend`,
  `node --check frontend/admin/admin.js`,
  `node --check scripts/local-pilot/playwright/run.mjs` et `git diff --check`
  réussissent. Chromium confirme la navigation par section, le motif et la
  création d’un fournisseur, la promotion d’un build, l’accès à la vitrine et
  la déconnexion. La campagne d’actions a utilisé une base de test jetable,
  supprimée après le test ; elle n’est pas encore un scénario complet du runner.

## Prochaines actions recommandées

1. Qualifier la primitive Ed25519 et le récepteur ZIP avec tests complets, CI
   Linux/Windows et revue de sécurité indépendante avant tout usage de production.
2. Pour les candidats reçus, définir la rétention avant toute passerelle vers
   le registre global de builds. L'inventaire et la revue admin (approbation,
   rejet avec motif, réouverture auditée) sont disponibles ; aucune action ne
   publie ni ne promeut. Le quota local reste provisoire et non coordonné
   multi-instance.
3. Garder les builds locaux disponibles et le catalogue facultatif. Une
   signature prouve l'origine cryptographique seulement, jamais l'innocuité ;
   tout passage futur au registre puis toute promotion doivent rester distincts
   et explicites.
4. Documenter et tester le déploiement de `EUROMNI_BUILD_TRUST_FILE` et
   `EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR`, notamment les droits d'accès.
   Compléter la politique : rotation/retrait, identité affichée, validité et
   rôles autorisés à promouvoir.
5. Obtenir une revue de sécurité indépendante du validateur ZIP, de l'endpoint
   d'upload et du stockage en quarantaine ; traiter les findings avant toute
   activation hors environnement de test. Le contrôle local du 21 octobre était
   ciblé et ne remplace pas cette revue.
6. Décider la durée et la fréquence de rétention, puis planifier
   `cmd/quarantine-reconciler` avec `EUROMNI_QUARANTINE_DIR` et une option
   `-older-than` explicite (par exemple `720h` pour 30 jours). La commande ne
   crée pas le répertoire cible et ne définit pas de politique par défaut.
   Garder la quarantaine isolée des artefacts publiés.
7. Évaluer l'absence de limitation de débit, l'usage du stockage local et le
   scénario multi-instance ; documenter les limites ou choisir une solution
   coordonnée avant le déploiement.
8. Rejouer `go test ./...`, `go vet ./...`, les tests d'intégration pertinents,
   et vérifier la matrice CI Windows/Linux après chaque changement.
9. Si des régressions UI deviennent fréquentes, étendre le runner Playwright
   avec des scénarios isolés pour les actions administratives actuellement
   vérifiées manuellement ; garder toute base de test séparée de la base locale
   persistante.
10. Ne pas commencer la traduction PHP générale. QP-04/05 (contrats des
     adaptateurs et de l'IR) sont des prérequis avant l'élargissement métier.

## Message de reprise à copier dans une nouvelle conversation

> Je reprends le dépôt EUROMNI-GorParis. Lis d'abord
> `docs/reprise-prochain-developpeur.md`, `docs/plan-directeur-qualite.md` et
> `pipeline/README.md`. Vérifie ensuite `git status --short --branch`, le dernier
> commit et l'état du run GitHub Actions correspondant. Ne touche pas à
> `.vscode/` ni aux modifications locales préexistantes non liées. Le contrat
> prototype manifeste/IR/Wasm et la réception ZIP signée sont implémentés :
> `EUROMNI_BUILD_TRUST_FILE` configure la confiance, et
> `EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR` active le staging privé par boutique.
> `GET .../candidates` fournit l'inventaire paginé, `GET
> .../candidates/{packageSHA256}` les métadonnées revalidées et `POST
> .../candidates/{packageSHA256}/approval`, `/rejection` et `/reopen` enregistrent
> les décisions auditables owner/admin (le rejet exige un motif). L'interface
> Modules expose ce parcours, sans publier, installer ni promouvoir. Consulte
> les prochaines actions pour la rétention, les quotas multi-instance et la revue
> sécurité indépendante. L'orientation
> reste hybride : build local possible, catalogue signé facultatif.
> QP-03 (quarantaine ZIP) est un chantier distinct. Ne lance aucune traduction
> PHP générale avant les prérequis QP-04/05. Exécute les validations pertinentes
> et mets à jour le suivi avant tout commit/push non expressément demandé.

## Choix de conversation

Il est préférable que le prochain développeur ouvre une **nouvelle
conversation** une fois connecté avec son propre compte, dans le workspace du
dépôt. Cela établit clairement son identité et ses accès ; ce document sert de
transmission durable et vérifiable. Il peut garder cette conversation comme
historique complémentaire, mais ne devrait pas dépendre de son contexte implicite.
