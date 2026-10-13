# Tests PostgreSQL réels

Les tests d’intégration lancent PostgreSQL 16 depuis Go ; aucun service natif ni port fixe n’est requis. Les tests du store et de l’API démarrent chacun une instance, appliquent les six migrations, puis la suppriment à la fin. Le binaire PostgreSQL est téléchargé et vérifié par SHA-256 à la première exécution, puis supprimé avec les données de test.

Depuis la racine du dépôt :

```sh
go test -tags=integration -count=1 ./...
```

Les binaires, runtime et données résident dans `.tmp/embedded-postgres`, sous le dépôt ; `.tmp` est ignoré par Git.

Pour exécuter les tests dans un runner Linux conteneurisé, Docker reste une option :

```sh
docker compose --profile integration run --rm integration-test
```

Ce runner utilise le même fixture embarqué et monte `.tmp` en mémoire. Il ne démarre pas le profil CMS et ne peut pas supprimer son volume persistant.
