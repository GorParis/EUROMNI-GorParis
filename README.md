# EUROMNI 

CMS souverain **headless** et **sandboxed** : le cœur d’exécution est en Go, les extensions du marché (WooCommerce, PrestaShop, etc.) sont transpilées vers WebAssembly et isolées dans Wasmtime, le front boutique est livré en composants web (Shadow DOM).
Orienté ecommerce, sa technologie pourra aussi permettre la conteneurisation et la reproductabilité d'essais sur des particules intriquées localement.

## Architecture (3 couches)

1. **Pipeline de modules** — analyse statique ciblée et génération Wasm depuis une IR limitée ; la traduction générique assistée par IA reste à construire.
2. **Cœur CMS (Go)** — runtime Wasm (Wasmtime), API GraphQL **et** REST, PostgreSQL de référence et connecteur MariaDB dédié.
3. **Front boutique** — Web Components universels, isolation Shadow DOM.

Docs : [`docs/architecture.md`](docs/architecture.md), [`docs/api.md`](docs/api.md), [`docs/database.md`](docs/database.md), [`docs/database-support.md`](docs/database-support.md) (comparatif des connecteurs et état de validation), [`docs/database-backup.md`](docs/database-backup.md) (sauvegarde/restauration PostgreSQL et MariaDB), [`docs/mariadb-setup.md`](docs/mariadb-setup.md) (installation et test MariaDB), [`docs/sqlite-prototype.md`](docs/sqlite-prototype.md) (prototype mono-boutique, non destiné à la production), [`docs/abi.md`](docs/abi.md) (mapping Wasm → SQL), [`docs/portability.md`](docs/portability.md) (runtime CGo et guests Wasm), [`docs/local-postgresql.md`](docs/local-postgresql.md) (persistance locale), [`docs/cms-distribution.md`](docs/cms-distribution.md) (paquet autonome sans Docker), [`docs/testing-postgresql.md`](docs/testing-postgresql.md) (intégration réelle).

## Structure

```
cmd/cms/          # point d’entrée du moteur
internal/api/     # couche GraphQL / REST
internal/wasm/    # runtime Wasmtime et ABI hôte
internal/store/   # accès PostgreSQL et MariaDB
migrations/       # schémas et migrations PostgreSQL et MariaDB
pipeline/         # analyse de modules, IR et compilation Wasm limitée
frontend/         # composants de démonstration de la boutique
docs/             # architecture, portabilité et modèle de données
```

## Lancer le CMS localement

```bash
go test ./...
go run ./cmd/cms
```

Prérequis : Go 1.25+ et PostgreSQL 16+ (recommandé), ou MariaDB 11.8 après avoir pris connaissance de son état de validation dans [`docs/database-support.md`](docs/database-support.md). Docker et un hébergeur cloud sont facultatifs. Le CMS écoute par défaut sur `127.0.0.1:8081`; utilisez `LISTEN_ADDR` pour définir explicitement une autre adresse. L’ABI Wasm n’est pas exposée en HTTP.

Sans `DATABASE_URL`, le serveur démarre volontairement avec un catalogue mémoire
de démonstration et l’authentification staff est indisponible. Pour une instance
CMS persistante, configurez une base prise en charge et appliquez les migrations décrites dans
[`docs/local-postgresql.md`](docs/local-postgresql.md) avant le démarrage.

## Administration du catalogue

L’interface CMS minimale est servie sous `/admin/` et utilise l’API REST staff. La connexion requiert un compte associé à la boutique dans le connecteur configuré. Aucun mot de passe de démonstration n’est fourni.

Le schéma est installé ou mis à jour avec le migrateur intégré ; Compose lance
la même commande avant le serveur CMS. Pour un lancement autonome, configurez
`DATABASE_URL`, puis exécutez `go run ./cmd/db-migrate`.

Initialisez le premier propriétaire en transmettant son mot de passe sur l’entrée standard (12 à 72 octets), jamais dans les arguments de la commande :

```sh
go run ./cmd/staff-bootstrap --shop '<shop-id>' --email '<owner-email>' --name '<owner-name>' --password-stdin
```

Pour la boutique seedée, le compte owner `owner@atelier-demo.example` doit être configuré avec cette même adresse e-mail. Le bootstrap ne remplace pas un compte existant. La déconnexion CMS révoque la session staff courante.

Le flux initial de confirmation de commande et le worker SMTP sont décrits dans
[`docs/email-service.md`](docs/email-service.md). Le profil Compose `mail`
utilise Mailpit pour les essais locaux isolés ; aucun serveur d’envoi de
production n’est configuré.

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
