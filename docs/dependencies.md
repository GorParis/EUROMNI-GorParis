# Dépendances de CI et d’installation

Ce document distingue les outils nécessaires pour vérifier/construire EUROMNI
des prérequis d’un CMS installé. Les versions et manifestes du dépôt sont la
source de vérité ; les commandes CI doivent rester minimales et ne pas installer
des outils destinés uniquement au développement sur le serveur CMS.

## CI et tests

| Dépendance | Périmètre et raison | Version/source |
|---|---|---|
| Runner Linux et Windows | Validation native des binaires CGo, Wasmtime et FileStore sur les deux cibles qualifiées. | GitHub Actions `ubuntu-24.04` et `windows-2022` dans `ci.yml` et `cms-distribution.yml`. |
| Go | Compilation, `go vet`, tests et création des binaires. | Go `1.25.0`, fixé par `go.mod` et installé depuis `go-version-file` avec `actions/setup-go`. `go.mod` et `go.sum` verrouillent les modules Go. |
| Toolchain C / CGo | `wasmtime-go/v36` s’appuie sur CGo ; les jobs construisent et exécutent le runtime natif. | `CGO_ENABLED=1`. Le runner Ubuntu fournit le compilateur C ; Windows installe GCC UCRT64 via `msys2/setup-msys2` et le paquet `mingw-w64-ucrt-x86_64-gcc`. |
| Rust et cible Wasm | Vérification du guest Rust et test d’intégration qui le compile depuis le pipeline. | Rust `1.99.1`, profil minimal, cible `wasm32-unknown-unknown`, selon `pipeline/testdata/rust-selfnamed-preview/rust-toolchain.toml`. |
| Crates Rust du guest | Le build du guest utilise Cargo en mode `--frozen` : les crates verrouillées doivent être présentes avant les tests. | `cargo fetch --locked` dans `pipeline/testdata/rust-selfnamed-preview`, résolu par son `Cargo.lock`. |
| PostgreSQL de test | Les tests d’intégration démarrent eux-mêmes une instance temporaire ; aucun service PostgreSQL n’est nécessaire sur le runner. | PostgreSQL 16 téléchargé par `embedded-postgres` (`go.mod`). Les instances et le cache de binaires sont sous `.tmp/embedded-postgres`. |
| Client PostgreSQL | Le test d’intégration de sauvegarde/restauration appelle les exécutables externes `pg_dump` et `pg_restore`. Ils doivent correspondre au serveur 16 embarqué. | Linux installe `postgresql-client-16` depuis le dépôt officiel PGDG. Sous Windows, le workflow ne les installe pas explicitement : le test les cherche dans le `PATH`, puis dans `.tmp/postgresql-16-tools/pgsql/bin`, et est ignoré si `pg_dump` ou `pg_restore` manque. Pour reproduire ce test sur Windows, fournir les outils PostgreSQL 16 à l’un de ces emplacements. |
| Outils du packaging | Extraction, archives, sommes et vérifications des endpoints de smoke test du paquet autonome. | Fournis par les runners : Bash, `tar`, `curl` et `sha256sum` sous Linux ; PowerShell et `Compress-Archive` sous Windows. Les artifacts sont publiés avec `actions/upload-artifact`. |

Les workflows à consulter sont [Go CI](../.github/workflows/ci.yml),
[CMS standalone distribution](../.github/workflows/cms-distribution.yml) et
[Tests PostgreSQL réels](./testing-postgresql.md). Docker n’est pas requis par
la CI : l’option Docker documentée sert uniquement à lancer localement le runner
d’intégration. Les actions GitHub et les images de runners sont également des
dépendances de plateforme ; leurs références actuelles sont indiquées dans les
fichiers de workflow.

Pour reproduire les tests d’intégration sur Linux, utiliser Go 1.25.0, Rust
1.99.1 avec sa cible Wasm, Cargo avec les crates du lockfile, les outils
PostgreSQL 16 dans le `PATH`, et lancer :

```sh
cargo fetch --locked --manifest-path pipeline/testdata/rust-selfnamed-preview/Cargo.toml
go vet ./...
go test -count=1 ./...
go test -p 1 -tags=integration -count=1 ./...
```

Sur une machine où un client PostgreSQL plus récent est installé, placer les
exécutables PostgreSQL 16 avant ceux du système dans le `PATH`. Par exemple,
`pg_restore` 18 peut émettre `SET transaction_timeout`, que PostgreSQL 16 ne
reconnaît pas.

## Prérequis d’une installation CMS

Le paquet autonome est déjà construit avec Go/CGo et Wasmtime pour un OS et une
architecture ciblés. Il embarque le CMS, les migrations et les assets :
l’utilisateur n’a pas à installer Go, Rust, Cargo, Node.js ou Docker pour
exécuter le CMS.

| Prérequis côté installation | Obligatoire ou conditionnel | À proposer/contrôler par un futur installateur |
|---|---|---|
| Windows/amd64 ou Linux/amd64 qualifié | Obligatoire pour les paquets actuellement distribués. | Vérifier OS et architecture avant de proposer le paquet correspondant. |
| Base externe | Obligatoire pour la persistance : PostgreSQL 16+ est la référence. MariaDB 11.8 est disponible avec une validation fonctionnelle encore partielle. | Demander le moteur, l’adresse et la base ; valider la connexion et les migrations. Ne pas installer ni modifier une base sans choix explicite de l’utilisateur. |
| Outils client de base | Conditionnels aux opérations de sauvegarde/restauration. PostgreSQL : `pg_dump` et `pg_restore`; MariaDB : `mariadb-dump` (ou `mysqldump`) et `mariadb` (ou `mysql`). | Expliquer que ces outils sont distincts du binaire CMS et vérifier leur présence/version lorsque la fonction correspondante est configurée. |
| Répertoires privés de données | Conditionnels aux FileStores activés (artefacts, candidats signés). | Demander des chemins persistants accessibles au compte de service et vérifier leurs permissions/ACL. |
| Base PostgreSQL et volume partagé | Conditionnels à l’activation des candidats de packages signés multi-instance. | Expliquer l’exigence d’une base commune et d’un système de fichiers partagé avec visibilité cohérente et hard links atomiques ; ne pas activer automatiquement cette option. |
| Ordonnanceur | Conditionnel aux purges/réconciliations planifiées. | Indiquer la commande à planifier et son intervalle après configuration explicite de la fonction. |

L’installation guidée n’est pas encore implémentée. Cette liste est un contrat
de préparation : un futur assistant peut détecter et expliquer les prérequis,
puis laisser l’utilisateur choisir les services et chemins. Elle ne justifie
pas l’installation silencieuse de PostgreSQL, de clients système ou d’un
ordonnanceur. Voir aussi [Distribution autonome du CMS](./cms-distribution.md),
[Connecteurs de bases de données](./database-support.md),
[Sauvegarde/restauration](./database-backup.md) et
[Portabilité](./portability.md).
