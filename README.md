# EUROMNI dasegole

CMS souverain **headless** et **sandboxed** : le cœur d’exécution est en Go, les extensions du marché (WooCommerce, PrestaShop, etc.) sont transpilées vers WebAssembly et isolées dans Wasmtime, le front boutique est livré en composants web (Shadow DOM).

## Architecture (3 couches)

1. **Pipeline de modules** — analyse statique ciblée et génération Wasm depuis une IR limitée ; la traduction générique assistée par IA reste à construire.
2. **Cœur CMS (Go)** — runtime Wasm (Wasmtime), API GraphQL **et** REST, PostgreSQL.
3. **Front boutique** — Web Components universels, isolation Shadow DOM.

Docs : [`docs/architecture.md`](docs/architecture.md), [`docs/api.md`](docs/api.md), [`docs/database.md`](docs/database.md), [`docs/abi.md`](docs/abi.md) (mapping Wasm → SQL), [`docs/portability.md`](docs/portability.md) (runtime CGo et guests Wasm), [`docs/local-postgresql.md`](docs/local-postgresql.md) (persistance locale), [`docs/testing-postgresql.md`](docs/testing-postgresql.md) (intégration réelle).

## Structure

```
cmd/cms/          # point d’entrée du moteur
internal/api/     # couche GraphQL / REST
internal/wasm/    # runtime Wasmtime et ABI hôte
internal/store/   # accès PostgreSQL
migrations/       # schéma SQL + seed exemple
pipeline/         # analyse de modules, IR et compilation Wasm limitée
frontend/         # composants de démonstration de la boutique
docs/             # architecture, portabilité et modèle de données
```

## Lancer le CMS localement

```bash
go test ./...
go run ./cmd/cms
```

Prérequis : Go 1.23+, PostgreSQL 16+ (hébergeur libre ; actuellement IONOS). L’ABI Wasm n’est pas exposée en HTTP.

## Ordonnanceur d’artefacts Wasm

Un processus Go indépendant réconcilie périodiquement les Wasm référencés par
PostgreSQL avec le FileStore. Il lance un passage immédiatement au démarrage,
puis à l’intervalle configuré ; il ne requiert pas Docker.

```bash
DATABASE_URL='<url-postgresql>' \
EUROMNI_ARTIFACT_DIR='/var/lib/euromni/artifacts' \
go run ./cmd/artifact-reconciler
```

`EUROMNI_ARTIFACT_DIR` doit désigner exactement le même répertoire privé que le
FileStore utilisé pour publier les artefacts. Il doit être accessible par le
compte du processus avec des permissions privées (`0700`).

Valeurs provisoires à réviser avant production : intervalle cible de `24h` et
délai de grâce de `168h` avant suppression d’un artefact sans référence. Elles
se configurent avec `EUROMNI_ARTIFACT_RECONCILIATION_INTERVAL` et
`EUROMNI_ARTIFACT_ORPHAN_GRACE_PERIOD`. Après une erreur transitoire, le retry
est provisoirement de `5m`, configurable avec
`EUROMNI_ARTIFACT_RECONCILIATION_RETRY_INTERVAL` ; après succès, la cadence
normale reprend. Le processus s’arrête proprement sur SIGINT/SIGTERM.
