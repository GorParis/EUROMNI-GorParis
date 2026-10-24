# Tests PostgreSQL réels

Les tests d’intégration lancent PostgreSQL 16 depuis Go ; aucun service natif ni port fixe n’est requis. Chaque test démarre sa propre instance, son port et ses données isolés, applique les migrations puis supprime l’instance à la fin. Seule l’archive des binaires PostgreSQL, vérifiée par SHA-256, est partagée sous `.tmp/embedded-postgres/cache` et réutilisée par les exécutions suivantes.

Depuis la racine du dépôt :

```sh
go test -tags=integration -count=1 ./...
```

L’archive partagée et les binaires extraits, runtime et données propres à chaque instance résident dans `.tmp/embedded-postgres`, sous le dépôt ; `.tmp` est ignoré par Git.

Le workflow Go CI (`.github/workflows/ci.yml`) exécute `go vet ./...`, les tests sans tag et les tests d’intégration (`go test -p 1 -tags=integration -count=1 ./...`) sur des runners natifs Linux amd64 et Windows amd64 à chaque push sur `main` et pull request vers `main`. Il installe Rust 1.99.1 et précharge avec `cargo fetch --locked` les dépendances du guest qui est compilé par un test d’intégration ; sur Linux, il installe aussi le client PostgreSQL 16 depuis PGDG pour faire correspondre `pg_dump`/`pg_restore` au serveur de test. Les intégrations sont sérialisées (`-p 1`) pour éviter les courses sur le cache partagé de l'archive PostgreSQL embarquée. Les tests de FileStore créent aussi leurs répertoires avec le helper `privatefs.EnsureDirectory`, afin de tester les ACL protégées requises sur Windows plutôt que des répertoires temporaires aux ACL héritées. Le workflow manuel/de publication des distributions (`.github/workflows/cms-distribution.yml`) exécute également les intégrations sur ses runners natifs avant de construire les archives.

Pour exécuter les tests dans un runner Linux conteneurisé, Docker reste une option :

```sh
docker compose --profile integration run --rm integration-test
```

Ce runner utilise le même fixture embarqué et monte `.tmp` en mémoire. Il ne démarre pas le profil CMS et ne peut pas supprimer son volume persistant.
