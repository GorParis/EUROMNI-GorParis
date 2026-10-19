# Sauvegarde et restauration des bases

`cmd/db-backup` détecte le moteur depuis `DATABASE_URL`. Une sauvegarde contient
le dump SQL de la base, une archive du FileStore et un manifeste avec les
empreintes SHA-256 et le dialecte source. Les sauvegardes PostgreSQL existantes
sans champ `database_dialect` restent reconnues comme PostgreSQL.

Configurer les variables d’environnement hors des arguments de commande et de
l’historique shell :

```sh
export DATABASE_URL='mariadb://<user>:<password-url-encoded>@<host>:3306/<database>?tls=true'
export EUROMNI_ARTIFACT_DIR='/var/lib/euromni/artifacts'
export EUROMNI_BACKUP_DIR='/var/backups/euromni'
go run ./cmd/db-backup
```

Le même chemin fonctionne avec une URL `postgres://` ou `postgresql://`.
PostgreSQL requiert `pg_dump`; MariaDB requiert `mariadb-dump` (le programme
essaie aussi `mysqldump`). Le mot de passe MariaDB est transmis au client via un
fichier d’options temporaire dans un répertoire privé, jamais comme argument ou
variable d’environnement du sous-processus. `tls=true` est pris en charge;
les profils TLS personnalisés ne le sont pas.

Vérifier l’intégrité avant toute restauration :

```sh
go run ./cmd/db-backup -verify "$EUROMNI_BACKUP_DIR/<nom-de-sauvegarde>"
```

## Restaurer dans une base vide

Créer au préalable une nouvelle base vide, puis définir `DATABASE_URL` pour
cette destination. La restauration refuse les objets déjà présents et exige le
même dialecte que celui inscrit dans le manifeste.

```sh
export DATABASE_URL='mariadb://<user>:<password-url-encoded>@<host>:3306/<new-empty-database>?tls=true'
backup="$EUROMNI_BACKUP_DIR/<nom-de-sauvegarde>"
go run ./cmd/db-backup -verify "$backup"
go run ./cmd/db-backup -restore-database "$backup"
go run ./cmd/db-backup -restore-artifacts "$backup" \
  -artifact-dir '/var/lib/euromni/restore/artifacts'
```

Pour PostgreSQL, définir `DATABASE_URL` avec l’URL de la nouvelle base vide et
exécuter les mêmes commandes. Les outils `pg_restore` ou `mariadb` (avec repli
sur `mysql`) doivent être dans le `PATH`. Le compte SQL doit pouvoir créer les
objets restaurés. Le dump ne sauvegarde pas les comptes, rôles ou privilèges
globaux du serveur.

La restauration doit viser une base nouvellement créée et isolée, jamais la base
active. PostgreSQL restaure le dump dans une transaction. MariaDB effectue le
chargement SQL avec des DDL qui ne sont pas transactionnels : en cas d’échec,
la base cible peut être partiellement restaurée; la supprimer et en recréer une
vide avant de réessayer. La restauration du FileStore est une commande séparée,
qui refuse un répertoire de destination existant.

Le manifeste et ses empreintes détectent les altérations accidentelles; ils ne
constituent pas une signature. Ne restaurer que des sauvegardes provenant d’un
emplacement de confiance. `EUROMNI_BACKUP_DIR` désigne le répertoire de sortie ;
le projet ne configure ni ne désigne actuellement de cible ou fournisseur de
stockage hors machine (« FS »). Un chemin local ou un volume monté n’est pas, à
lui seul, une sauvegarde hors machine. Le chiffrement et la mesure du RTO
restent à traiter avant un usage client.

Options examinées pour la copie extérieure : stockage objet S3 en région
européenne (IONOS Object Storage ou Scaleway Object Storage), ou dépôt SFTP sur
une machine distincte. L’orientation provisoire est de transférer les archives
produites par `cmd/db-backup` vers un bucket objet avec chiffrement côté client,
identifiants dédiés et protection contre l’effacement. Aucun fournisseur n’est
encore choisi ni configuré. La phase est mise en attente jusqu’à la stabilisation
du CMS et aux essais de plusieurs imports hors paiement ; la reprise devra
inclure une restauration chronométrée depuis la copie extérieure.

## Tester l’intégration MariaDB

Utiliser une base source temporaire dédiée. Le test crée une table, un trigger
et une procédure, crée une deuxième base vide pour la restauration puis supprime
ses objets de test. Le compte doit pouvoir créer et supprimer cette base. Ne
jamais fournir une URL de production ou d’une base contenant des données à
conserver.

```sh
MARIADB_BACKUP_TEST_DATABASE_URL='mariadb://<user>:<password-url-encoded>@127.0.0.1:3306/<dedicated-test-database>' \
  go test -tags integration -count=1 ./internal/backup \
  -run '^TestBackupAndRestoreMariaDB$'
```

Le test PostgreSQL correspondant utilise PostgreSQL 16 isolé :

```sh
go test -tags integration -count=1 ./internal/backup \
  -run '^TestBackupAndRestorePostgreSQL$'
```
