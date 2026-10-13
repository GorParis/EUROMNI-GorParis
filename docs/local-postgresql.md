# Base locale du CMS

Cette configuration Docker est un environnement local de développement, pas la solution de déploiement finale. Le CMS se connecte au PostgreSQL standard via `DATABASE_URL` ; la production pourra utiliser un PostgreSQL externe sans dépendre de Docker.

## Démarrer

Créer un fichier local non versionné `.env` à partir du modèle `.env.example`, remplacer sa valeur par un secret long composé de caractères sûrs pour une URL, puis lancer depuis la racine du dépôt :

```sh
docker compose --profile cms up -d --wait
```

Compose démarre PostgreSQL et le CMS sur le réseau privé du projet. Seul le port HTTP du CMS est publié, sur loopback ; `docker compose --profile cms port cms 8081` affiche l’adresse locale attribuée.

Au premier démarrage sur un volume vide, les migrations numérotées et le seed d’exemple sont exécutés ; `schema_migrations` conserve les versions enregistrées. Les démarrages suivants réutilisent les données existantes et ne rejouent pas les scripts d’initialisation. Toute nouvelle migration doit être appliquée explicitement, dans l’ordre, et enregistrer sa version dans `schema_migrations` au sein de la même transaction.

## Persistance et sauvegardes

Les données PostgreSQL résident dans le volume Docker nommé du projet. `stop` et `down` retirent ou arrêtent les conteneurs mais conservent ce volume. **Ne pas utiliser `down -v`** pour une base qui contient des commandes ou des preuves de transaction : cette option supprime les données.

Un volume local protège contre la recréation du conteneur, mais ne constitue pas une sauvegarde. Une exploitation client exige une politique de sauvegardes vérifiées et hors machine, une procédure de restauration testée, ainsi que la conservation des snapshots de commande et de l’historique des migrations.

Les tests d’intégration utilisent une instance PostgreSQL embarquée dans le processus de test et confinée sous `.tmp/embedded-postgres`. Ils n’utilisent jamais le volume `postgres-data` du CMS.
