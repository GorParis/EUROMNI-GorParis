# Base locale du CMS

Le CMS se connecte à PostgreSQL standard via `DATABASE_URL`. Docker Compose est
une commodité locale, pas une dépendance d’exécution : le binaire Go peut tourner
directement sur une machine Windows ou Linux avec un service PostgreSQL local ou
accessible sur le réseau.

## Démarrer

Créer un fichier local non versionné `.env` à partir du modèle `.env.example`, remplacer sa valeur par un secret long composé de caractères sûrs pour une URL, puis lancer depuis la racine du dépôt :

```sh
docker compose --profile cms up -d --wait
```

Compose démarre PostgreSQL et le CMS sur le réseau privé du projet. Seul le port HTTP du CMS est publié, sur loopback ; `docker compose --profile cms port cms 8081` affiche l’adresse locale attribuée.

Au démarrage, le CMS applique `cmd/db-migrate` avant de lancer le serveur. Sur
une base vide, l’outil installe le schéma et le catalogue exemple ; sur les
démarrages suivants, il n’applique que les nouvelles migrations. Le registre
`schema_migrations` est vérifié à chaque passage.

## Lancer sans Docker

Installer et démarrer PostgreSQL comme service sur la machine, puis créer une
base et un rôle dédiés. Définir `DATABASE_URL` et lancer l’outil de migrations ;
il découvre les scripts embarqués, vérifie leur ordre et leur registre, applique
les migrations manquantes de façon idempotente et échoue explicitement en cas de
ledger incohérent :

```powershell
$env:DATABASE_URL = "postgres://euromni:<mot-de-passe>@127.0.0.1:5432/euromni?sslmode=disable"
go run ./cmd/db-migrate
```

La même commande sous Linux/macOS est `go run ./cmd/db-migrate` après
configuration de `DATABASE_URL` avec `export`. Les scripts SQL sont aussi
embarqués dans le binaire, aucune copie du dossier des migrations n’est requise
à l’exécution.

Initialisez ensuite le premier propriétaire avec la commande documentée dans le
README, en fournissant le mot de passe sur stdin, jamais dans les arguments.
Lancez enfin `go run ./cmd/cms`. Le CMS écoute par défaut sur
`127.0.0.1:8081`; `LISTEN_ADDR` permet de choisir explicitement l’adresse et le
port. Si `DATABASE_URL` est fourni mais que PostgreSQL est injoignable, le
processus échoue au démarrage au lieu de basculer silencieusement vers le store
mémoire.

Pour un accès depuis un réseau local de confiance, configurez une adresse
d’interface dédiée ou `0.0.0.0:8081`, limitez l’accès avec le pare-feu et placez
un reverse proxy TLS devant le CMS. Ne publiez pas directement le serveur HTTP
sur Internet. Les secrets de base et de bootstrap doivent rester dans le
gestionnaire de secrets ou l’environnement du processus, jamais dans Git.

## Persistance et sauvegardes

Avec Compose, les données PostgreSQL résident dans le volume Docker nommé du
projet. `stop` et `down` retirent ou arrêtent les conteneurs mais conservent ce
volume. **Ne pas utiliser `down -v`** pour une base qui contient des commandes
ou des preuves de transaction : cette option supprime les données.

Un volume local ou un disque de machine protège contre certaines pannes, mais
ne constitue pas une sauvegarde. Une exploitation client exige une politique de
sauvegardes vérifiées et hors machine, une procédure de restauration testée,
ainsi que la conservation des snapshots de commande et de l’historique des
migrations.

## Exercice de restauration CMS-05d

Le 17 octobre 2026, les binaires Windows du CMS ont été lancés contre un
PostgreSQL 16.15 temporaire et isolé. Après les migrations et le bootstrap du
owner de démonstration, un produit et une commande ont été créés via l’API.
L’authentification, le produit, le client et le snapshot de ligne de commande
ont été vérifiés après redémarrage du CMS.

La base a ensuite été sauvegardée avec `pg_dump` au format custom (`-Fc`,
`--no-owner`, `--no-acl`) et restaurée avec `pg_restore` dans une base distincte
du même cluster temporaire. Des requêtes SQL ont confirmé la présence du
produit créé, du compte owner et de la commande avec son client et son SKU
snapshot. Le cluster et le dump de test ont été supprimés après vérification.
Cet exercice valide la procédure technique, pas une sauvegarde de production :
la fréquence de rétention, le stockage protégé hors machine et les contrôles
périodiques de restauration restent à définir.

Sur une base neuve issue des migrations actuelles, la boutique `atelier-demo`
et son owner sans mot de passe `owner@atelier-demo.example` sont précréés. Le
bootstrap initial doit cibler cette boutique et cette adresse ; un owner déjà
initialisé ou une adresse différente est refusé.

Les tests d’intégration utilisent une instance PostgreSQL embarquée dans le processus de test et confinée sous `.tmp/embedded-postgres`. Ils n’utilisent jamais le volume `postgres-data` du CMS.
