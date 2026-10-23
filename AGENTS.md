# Consignes pour les agents

## Avant toute modification

- Vérifier `git status --short --branch` et les derniers commits. Préserver les
  changements locaux et ne pas supposer que le dépôt est propre.
- Lire [`docs/reprise-prochain-developpeur.md`](docs/reprise-prochain-developpeur.md),
  [`docs/plan-directeur-qualite.md`](docs/plan-directeur-qualite.md) et
  [`pipeline/README.md`](pipeline/README.md), puis les documents du domaine
  concerné.
- `.vscode/` contient des fichiers locaux qui ne doivent pas être ajoutés à un
  commit sans accord explicite. Ne jamais versionner les secrets, `.env`, les
  identifiants de test ou les fichiers locaux d’accès.

## Architecture et sécurité

- Le cœur est en Go, avec PostgreSQL comme référence et MariaDB selon la matrice
  documentée. Respecter les frontières API/store et les migrations existantes.
- Le pipeline PHP→Wasm est limité à l’IR et aux adaptateurs effectivement
  supportés. Ne pas prétendre à une traduction PHP générale ; ne jamais exécuter
  le PHP source. Les modules Wasm passent par l’ABI, les capacités, la
  validation de manifeste et les contrôles de ressources du runtime.
- Les actions admin sont authentifiées, tenant-scopées et soumises aux rôles
  existants. Ne pas élargir les permissions ou les effets inter-boutiques.
- Ne pas utiliser ou modifier une base locale persistante pour des essais sans
  demande explicite. Préférer une base temporaire dédiée, puis arrêter et
  nettoyer uniquement les ressources créées pour le test.
- Ne pas activer en production la quarantaine d’imports sans revue de sécurité
  indépendante et décisions explicites sur rétention, limitation de débit et
  déploiement multi-instance.

## Changements et vérifications

- Faire des changements ciblés, respecter les types et conventions existants,
  et mettre à jour la documentation directement concernée.
- Pour Go, lancer les tests les plus ciblés pertinents ; avant une livraison
  plus large, utiliser `go test ./...` et `go vet ./...`. Formater le Go avec
  `gofmt`.
- Pour l’administration et le front-end, vérifier au minimum la syntaxe des
  scripts JavaScript touchés et les tests `frontend`. N’utiliser Playwright
  contre une base persistante qu’avec autorisation et données de test maîtrisées.
- Exécuter `git diff --check`. Rapporter exactement les tests faits, leur
  environnement et les limites restantes.
- Ne créer ni commit ni push sans demande explicite. Ne pas inclure des
  modifications sans rapport dans un commit autorisé.
