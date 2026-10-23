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
- QP-03 est le travail actif : premier endpoint d'ingestion admin d'un ZIP brut,
  validation en mémoire, puis dépôt dans une quarantaine privée distincte des
  builds. Aucun contenu PHP n'est extrait ou exécuté.
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
- Le back-office est organisé en six sections (Boutique, Catalogue, Commandes,
  Livraison, Modules et Équipe). Le parcours Chromium du 23 octobre a exercé les
  principales actions sur un CMS et une base PostgreSQL 16 jetables ; les
  résultats et la portée du runner existant sont consignés dans
  [`etat-projet.md`](etat-projet.md). L’instance jetable est arrêtée. Le runner
  Playwright versionné ne couvre pas encore toutes les actions administratives.
- `.vscode/` est un dossier non suivi préexistant : ne pas l'ajouter au commit
  sans accord explicite.

## Vérifications déjà exécutées

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

1. Obtenir une revue de sécurité indépendante du validateur ZIP, de l'endpoint
  d'upload et du stockage en quarantaine ; traiter les findings avant toute
  activation hors environnement de test. Le contrôle local du 21 octobre était
  ciblé et ne remplace pas cette revue.
2. Décider la durée et la fréquence de rétention, puis planifier
  `cmd/quarantine-reconciler` avec `EUROMNI_QUARANTINE_DIR` et une option
  `-older-than` explicite (par exemple `720h` pour 30 jours). La commande ne
  crée pas le répertoire cible et ne définit pas de politique par défaut.
  Garder la quarantaine isolée des artefacts publiés.
3. Évaluer l'absence de limitation de débit, l'usage du stockage local et le
   scénario multi-instance ; documenter les limites ou choisir une solution
   coordonnée avant le déploiement.
4. Rejouer `go test ./...`, `go vet ./...`, les tests d'intégration pertinents,
   et vérifier la matrice CI Windows/Linux après chaque changement.
5. Si des régressions UI deviennent fréquentes, étendre le runner Playwright
   avec des scénarios isolés pour les actions administratives actuellement
   vérifiées manuellement ; garder toute base de test séparée de la base locale
   persistante.
6. Ne pas commencer la traduction PHP générale. QP-04/05 (contrats des
    adaptateurs et de l'IR) sont des prérequis avant l'élargissement métier.

## Message de reprise à copier dans une nouvelle conversation

> Je reprends le dépôt EUROMNI-GorParis. Lis d'abord
> `docs/reprise-prochain-developpeur.md`, `docs/plan-directeur-qualite.md` et
> `pipeline/README.md`. Vérifie ensuite `git status --short --branch`, le dernier
> commit et l'état du run GitHub Actions correspondant. Ne touche pas à
> `.vscode/`. Le chantier actif est QP-03 : endpoint admin d'ingestion ZIP et
> quarantaine privée, sans extraction ni exécution PHP. Avant toute activation
> en production, fais une revue de sécurité ciblée et traite la rétention/purge,
> le rate-limit et le cas multi-instance. Exécute les validations pertinentes,
> mets à jour le suivi qualité, puis présente les changements et limites avant
> tout commit/push si je n'ai pas donné mon accord.

## Choix de conversation

Il est préférable que le prochain développeur ouvre une **nouvelle
conversation** une fois connecté avec son propre compte, dans le workspace du
dépôt. Cela établit clairement son identité et ses accès ; ce document sert de
transmission durable et vérifiable. Il peut garder cette conversation comme
historique complémentaire, mais ne devrait pas dépendre de son contexte implicite.
