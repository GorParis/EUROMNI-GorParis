# Connecteurs de bases de données

Le CMS choisit son connecteur depuis `DATABASE_URL`. Le format de la base
détermine aussi les migrations à appliquer : les migrations PostgreSQL et
MariaDB sont distinctes, et SQLite possède son propre prototype. Changer
seulement le pilote ne convertit ni le schéma ni les données.

## Comparer les options

| Base | Périmètre du CMS | État de validation | Quand la choisir |
|------|------------------|--------------------|------------------|
| PostgreSQL 16+ | Connecteur de référence ; migrations, CMS complet et sauvegarde avec `db-backup`. | Suite `go test ./...` et intégrations PostgreSQL ; les workflows de distribution exécutent aussi les intégrations sur Linux et Windows. | Choix recommandé pour une instance persistante aujourd’hui. |
| MariaDB 11.8 | Connecteur applicatif et migrateur dédiés ; `db-backup` utilise `mariadb-dump` et restaure dans une base vide. | `TestMariaDBMigrationAndCommerceFlow` et `TestBackupAndRestoreMariaDB` réussis sur MariaDB 11.8.8 locale et des bases temporaires dédiées; la sauvegarde/restauration vérifie données, triggers, routines et refus d’une destination non vide. L’hôte IONOS reste inaccessible depuis le poste (`NXDOMAIN`); cette validation locale ne qualifie pas l’hébergement. | Envisageable si l’hébergeur impose MariaDB, avec validation complémentaire sur l’environnement cible. |
| SQLite | Prototype isolé dans `internal/sqlitestore`, limité au catalogue, panier et checkout. | Suite de tests propre au prototype ; sauvegarde cohérente testée. Ne couvre pas les fonctions complètes du CMS et n’est pas câblé à `cmd/cms`. | Expérimentation mono-boutique seulement ; pas le choix d’une installation CMS complète. |
| Mémoire | Catalogue de démonstration lorsque `DATABASE_URL` est absente. | Utilisé par les tests et le mode de démonstration. Les données ne sont pas persistantes et l’authentification staff est indisponible. | Démo locale sans persistance uniquement. |

La validation MariaDB reste donc **partielle** : migrations, flux commerce et
sauvegarde/restauration ont été exercés sur un serveur local dédié, mais toutes
les opérations ne sont pas couvertes et l’environnement IONOS n’est pas
qualifié. Ne pas confondre ces résultats avec une qualification générale du
connecteur. La sauvegarde nécessite
`mariadb-dump` (ou `mysqldump`) et la restauration `mariadb` (ou `mysql`) dans le
`PATH`. Voir la [procédure commune de sauvegarde/restauration](database-backup.md)
et la [procédure MariaDB](mariadb-setup.md) pour le DSN, les migrations et les
tests d’intégration opt-in.

## Configurer le connecteur

Pour PostgreSQL, configurez une URL `postgres://` ou `postgresql://`. Pour
MariaDB, configurez une URL `mariadb://` ou `mysql://`, avec le nom de la base
et un mot de passe encodé pour URL si nécessaire. Ne placez pas de secret dans
le dépôt, les arguments d’une commande ou un historique de shell.

Le choix du connecteur ne fournit pas une conversion automatique entre les
moteurs. Pour déplacer une boutique, il faut exporter les données, les mapper
vers le schéma cible puis vérifier les contraintes, les JSON, les montants,
les dates, les identifiants et les relations. Les fixtures de démonstration ne
sont pas un outil de migration de données métier.
