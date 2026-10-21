# Reprise du projet EUROMNI

Ce document est le point de départ pour le prochain développeur. Lire d'abord
les consignes du dépôt et le statut Git actuel ; ce guide décrit le contexte,
pas une preuve que l'arbre de travail est propre.

## État connu

- Branche de travail : `main`. Vérifier `git status --short --branch` et
  `git log -5 --oneline` avant de modifier quoi que ce soit.
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
- `.vscode/` est un dossier non suivi préexistant : ne pas l'ajouter au commit
  sans accord explicite.

## Vérifications déjà exécutées

- `go test ./...` : réussi après les changements QP-03.
- `go vet ./...` : réussi.
- `git diff --check` : réussi.
- Tests package quarantaine compilés pour Windows/amd64. La chaîne CGO/Windows
  reste à vérifier avec un développeur Windows ; ne pas la déclarer qualifiée
  sur la base de l'environnement local.
- La CI du commit QP-01 précédent (`ed73e39`) a réussi sur Linux et Windows.
  Vérifier le statut du run du commit courant après le push ; ne pas présumer
  qu'il est terminé.
- `go test ./internal/quarantine ./cmd/quarantine-reconciler` : réussi après
  l'ajout de la purge ponctuelle (21 octobre 2026).

## Prochaines actions recommandées

1. Vérifier le hash du dernier commit poussé et le statut de la CI Linux/Windows.
2. Demander une revue de sécurité ciblée sur le validateur ZIP, l'endpoint
   d'upload et le stockage en quarantaine ; corriger toute finding avant
   activation hors environnement de test.
3. Décider la durée et la fréquence de rétention, puis planifier
  `cmd/quarantine-reconciler` avec `EUROMNI_QUARANTINE_DIR` et une option
  `-older-than` explicite (par exemple `720h` pour 30 jours). La commande ne
  crée pas le répertoire cible et ne définit pas de politique par défaut.
  Garder la quarantaine isolée des artefacts publiés.
4. Évaluer l'absence de limitation de débit, l'usage du stockage local et le
   scénario multi-instance ; documenter les limites ou choisir une solution
   coordonnée avant le déploiement.
5. Rejouer `go test ./...`, `go vet ./...`, les tests d'intégration pertinents,
   et vérifier la matrice CI Windows/Linux après chaque changement.
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
