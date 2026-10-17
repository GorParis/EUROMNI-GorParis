# Tests PostgreSQL réels

Les tests d’intégration lancent PostgreSQL 16 depuis Go ; aucun service natif ni port fixe n’est requis. Chaque test démarre sa propre instance, son port et ses données isolés, applique les migrations puis supprime l’instance à la fin. Seule l’archive des binaires PostgreSQL, vérifiée par SHA-256, est partagée sous `.tmp/embedded-postgres/cache` et réutilisée par les exécutions suivantes.

Depuis la racine du dépôt :

```sh
go test -tags=integration -count=1 ./...
```

L’archive partagée et les binaires extraits, runtime et données propres à chaque instance résident dans `.tmp/embedded-postgres`, sous le dépôt ; `.tmp` est ignoré par Git.

La CI GitHub exécute `go vet ./...` et les tests sans tag sur un runner Linux à chaque push sur `main` et pull request vers `main`. Le workflow manuel/de publication des distributions (`.github/workflows/cms-distribution.yml`) exécute également cette commande d'intégration sur ses runners natifs Linux amd64 et Windows amd64, avant de construire les archives.

Pour exécuter les tests dans un runner Linux conteneurisé, Docker reste une option :

```sh
docker compose --profile integration run --rm integration-test
```

Ce runner utilise le même fixture embarqué et monte `.tmp` en mémoire. Il ne démarre pas le profil CMS et ne peut pas supprimer son volume persistant.
