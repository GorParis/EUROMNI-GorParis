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

## Pilote portable Windows sans privilèges administrateur

Pour un essai local uniquement, le dépôt fournit des scripts qui démarrent et
arrêtent l'instance préparée dans le profil Windows courant :

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\local-pilot\Start-EuromniPilot.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\local-pilot\Stop-EuromniPilot.ps1
```

Le CMS écoute sur `http://127.0.0.1:18081/admin/` ; PostgreSQL 16.9 écoute
uniquement sur `127.0.0.1:55432`. Les exécutables, le cluster de démonstration
et ses données restent sous `%LOCALAPPDATA%\EUROMNI\pilot`. Les commandes
recréent le démarrage après une fermeture ou un redémarrage de la machine ; aucun
service Windows ni démarrage automatique n'est installé. Le mot de passe owner
et le secret PostgreSQL sont dans des fichiers `PSCredential` protégés par
DPAPI pour le compte Windows courant. Pour révéler localement le mot de passe
owner nécessaire à la connexion web :

```powershell
$owner = Import-Clixml "$env:LOCALAPPDATA\EUROMNI\pilot\owner-credential.xml"
$owner.UserName
$owner.GetNetworkCredential().Password
```

Cette instance PostgreSQL embarquée provient de la fixture de développement
`embedded-postgres`, et non d'une distribution serveur de production. Elle ne
contient que des données de démonstration. Les outils `pg_dump` et `pg_restore`
ne sont pas inclus dans cette fixture ; les sauvegardes du pilote ne sont donc
pas validées. Ne pas y stocker de données client ni exposer ses ports au réseau.
Pour un usage durable, installer PostgreSQL standard, configurer ses sauvegardes
protégées, puis suivre la procédure ci-dessous.

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

### Sauvegarde locale de développement

`cmd/db-backup` crée un dump PostgreSQL au format custom et une archive du
FileStore, puis écrit un manifeste contenant leurs tailles et empreintes
SHA-256. La sauvegarde n'est publiée qu'après la réussite des deux exports.
`-verify` contrôle les empreintes avant une restauration ; il ne remplace pas
un essai de restauration PostgreSQL.

Lancer la commande une fois par jour avec l'ordonnanceur local, en surveillant
son code de sortie et l'espace disque. Le dossier de destination est protégé
pour le compte Windows courant (avec SYSTEM et Administrateurs) ou en
permissions POSIX privées ; il doit être distinct du FileStore. Le dump ne
contient pas le secret de connexion `DATABASE_URL`, mais les données métier
sauvegardées restent sensibles.

Exemple PowerShell, après avoir configuré `DATABASE_URL` comme pour les autres
commandes du CMS :

```powershell
$env:EUROMNI_ARTIFACT_DIR = "C:\EUROMNI\data\artifacts"
$env:EUROMNI_BACKUP_DIR = "D:\EUROMNI\backups"
go run ./cmd/db-backup
$latest = Get-ChildItem $env:EUROMNI_BACKUP_DIR -Directory |
  Sort-Object Name -Descending | Select-Object -First 1
go run ./cmd/db-backup -verify $latest.FullName
```

Dans une archive autonome, remplacer `go run ./cmd/db-backup` par
`.\euromni-db-backup.exe`.

`pg_dump` doit être disponible dans le `PATH` et être de la même version
majeure ou plus récente que le serveur PostgreSQL. Le programme refuse les
paramètres de connexion qu'il ne sait pas transmettre explicitement au client.
La restauration PostgreSQL requiert aussi `pg_restore` dans le `PATH`.
L'ordonnanceur doit exécuter le programme au moins une fois toutes les 24 h et
signaler tout échec pour respecter le RPO provisoire de 24 h.

Après chaque sauvegarde réussie, la rotation garde jusqu'à 14 sauvegardes
quotidiennes, 8 hebdomadaires et 12 mensuelles (au plus 34, avec chevauchements
dédupliqués). Une entrée de sauvegarde invalide bloque la purge et produit une
erreur visible au lieu d'être supprimée silencieusement. `-verify` contrôle les
empreintes des payloads ; la purge valide les noms, répertoires et manifestes.

Cette première étape ne chiffre pas les sauvegardes et un autre dossier ou
disque de la même machine ne protège pas contre la perte de celle-ci. Pour le
développement, utiliser un volume chiffré si les données sont sensibles. Le
stockage distant/hors machine reste à comparer et à décider avant tout usage
client. Le RTO n'est pas fixé : il sera mesuré lors d'un exercice complet de
restauration.

Pour restaurer, travailler sur une nouvelle base vide et un FileStore vide,
jamais sur les données actives :

```powershell
$backup = "D:\EUROMNI\backups\<nom-de-la-sauvegarde>"
go run ./cmd/db-backup -verify $backup
pg_restore --no-owner --no-acl --dbname=euromni_restore "$backup\database.dump"
go run ./cmd/db-backup -restore-artifacts $backup `
  -artifact-dir "C:\EUROMNI\restore\artifacts"
```

Dans une archive autonome, remplacer `go run ./cmd/db-backup` par
`.\euromni-db-backup.exe`.

Configurer les variables PostgreSQL attendues par `pg_restore` dans
l'environnement du processus sans mettre le mot de passe dans les arguments.
La restauration FileStore revérifie l'archive, valide chaque chemin et chaque
SHA-256, et refuse un répertoire de destination déjà existant.
Après restauration, vérifier migrations, artefacts référencés, commandes et
snapshots avant toute bascule.

Le test d'intégration utilise `pg_dump`/`pg_restore` réels et une instance
PostgreSQL 16 isolée ; il vérifie le dump et le FileStore restaurés :

```powershell
go test -tags integration ./internal/backup -run TestBackupAndRestorePostgreSQL -count=1
```

Il ne fixe pas le RTO de production : le temps de restauration d'un volume de
données représentatif et les vérifications fonctionnelles restent à mesurer
avant une bascule.

Un volume local ou un disque de machine protège contre certaines pannes, mais
ne constitue pas une sauvegarde hors machine.

## Exercice de restauration CMS-05d

Le 17 octobre 2026, les binaires Windows du CMS ont été lancés contre un
PostgreSQL 16.15 temporaire et isolé. Après les migrations et le bootstrap du
owner de démonstration, un produit et une commande ont été créés via l’API.
L’authentification, le produit, le client et le snapshot de ligne de commande
ont été vérifiés après redémarrage du CMS.

Séparément, l’archive Windows publiée par le workflow GitHub Actions
[#39015220199](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39015220199)
a été téléchargée, extraite et installée sur un cluster PostgreSQL 16.15
temporaire. Le binaire empaqueté a appliqué les migrations, permis le bootstrap
du owner et servi le catalogue ; l’authentification staff PostgreSQL a réussi.

La base a ensuite été sauvegardée avec `pg_dump` au format custom (`-Fc`,
`--no-owner`, `--no-acl`) et restaurée avec `pg_restore` dans une base distincte
du même cluster temporaire. Des requêtes SQL ont confirmé la présence du
produit créé, du compte owner et de la commande avec son client et son SKU
snapshot. Le cluster et le dump de test ont été supprimés après vérification.
Cet exercice valide la procédure technique, pas une sauvegarde de production :
la rétention locale et le RPO de 24 h sont maintenant définis et la commande est
disponible. Le stockage protégé hors machine, le chiffrement et les contrôles
périodiques de restauration restent à définir avant un usage client.

Sur une base neuve issue des migrations actuelles, la boutique `atelier-demo`
et son owner sans mot de passe `owner@atelier-demo.example` sont précréés. Le
bootstrap initial doit cibler cette boutique et cette adresse ; un owner déjà
initialisé ou une adresse différente est refusé.

Les tests d’intégration utilisent une instance PostgreSQL embarquée dans le processus de test et confinée sous `.tmp/embedded-postgres`. Ils n’utilisent jamais le volume `postgres-data` du CMS.
