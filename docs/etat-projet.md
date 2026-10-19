# État du projet EUROMNI

## Cycle de paiement de développement — livré (19 octobre 2026)

La tranche est implémentée et poussée sur `main` dans le commit `e5682d4`
(`Implement development payment lifecycle`). Elle relie un résultat de paiement
à la commande, puis à son admissibilité à l’expédition, au moyen d’un
prestataire fictif réservé au développement et aux tests.

Le scénario cible est le suivant : le serveur crée une tentative de paiement
pour une commande en attente ; l’adaptateur fictif produit un résultat
déterministe accepté, refusé ou abandonné ; le serveur vérifie et applique ce
résultat ; seule une commande effectivement payée peut ensuite être déclarée
expédiée. Le flux existant de paiement manuel reste disponible.

Le faux prestataire doit exercer les mêmes frontières que celles attendues
d’un prestataire réel : résultat communiqué au serveur, et non une commande
« payé » affirmée par le navigateur ; vérification de la commande, du montant
et de la devise ; transitions d’état explicites ; traitement idempotent des
notifications répétées ; journalisation de la tentative. Il doit être
impossible d’activer cet adaptateur en production par simple choix du client :
le serveur doit être explicitement configuré avec
`EUROMNI_ENVIRONMENT=development` et `EUROMNI_PAYMENT_PROVIDER=development-fake`.
Les secrets et les appels externes restent du côté hôte, jamais dans Wasm.

Critères de validation de cette tranche :

- paiement accepté : commande marquée payée une seule fois, puis éligible au
  flux d’expédition existant ;
- paiement refusé ou abandonné : commande non payée et impossible à expédier ;
- notification répétée, montant/devise incohérents ou commande inconnue :
  résultat rejeté ou sans effet supplémentaire ;
- réservation de stock cohérente après un refus ou un abandon, sans perte ni
  double restitution du stock ;
- tests couvrant les transitions et protections en mémoire, PostgreSQL et
  MariaDB 11.8.8 sur des bases d’intégration dédiées.

Cette étape n’inclut pas encore de prestataire réel, d’encaissement, de
remboursement, de paiement différé ni d’intégration transporteur. Elle vise à
valider le cycle métier avec un adaptateur remplaçable, avant la connexion d’un
service financier réel.

## Suite convenue — CMS et imports avant paiement réel

Le prestataire de paiement réel est reporté jusqu’à ce que le CMS fonctionne
dans le parcours visé et que plusieurs imports hors paiement aient été éprouvés.
Le périmètre exact de ces imports (données, sources/formats et résultats
attendus) reste à cadrer avant leur implémentation ou leur campagne de tests.
L’ordre de reprise est donc :

1. vérifier et stabiliser le fonctionnement du CMS sur la distribution locale
   ou autonome retenue — vérification de reprise effectuée le 19 octobre 2026 ;
2. éprouver les fixtures et pilotes de modules non financiers — premières
   analyses statiques multi-plateformes et le pilote catalogue WooCommerce
   passent ; le critère des deux pilotes réels reste ouvert ;
3. reprendre les décisions CMS-05d encore ouvertes, dont la cible de sauvegarde
   hors machine et la mesure du RTO, avant un usage client ;
4. seulement ensuite, étudier un prestataire de paiement réel.

Vérifications de reprise CMS : `go test ./...`, le parcours d’intégration
`TestPostgresAPIEndToEnd`, les contrôles de syntaxe JavaScript admin/vitrine et
les tests d’analyse des fixtures non financières passent. Ces validations
confirment le socle testé ; elles ne remplacent pas une qualification de
production ni la vérification du CMS sur un environnement client.

Pour les imports, l’analyse statique a été éprouvée sur les modules non
financiers WooCommerce `woocommerce-extra-product-sorting-options` et PrestaShop
`ps_newproducts`, ainsi que sur PrestaShop `ps_emailalerts`, Shopify Discount
et checkout UI, et Magento Japan Tax et gRPC. Le test de bout en bout du pilote
catalogue WooCommerce passe sur PostgreSQL avec une source synthétique. Ces
preuves n’exécutent pas les sources PHP tierces : les nouveaux paquets amont
n’ont pas de manifeste EUROMNI et ne sont pas encore pris en charge par
l’importateur actuel. Des lowerings/oracles fidèles et deux publications
exécutées via Wasm restent à démontrer. Les fixtures de paiement WooCommerce
Stripe et PrestaShop `ps_cashondelivery` sont exclues de cette tranche.

## Sauvegardes hors machine — options examinées, phase en attente

La rétention retenue est de 14 sauvegardes quotidiennes, 8 hebdomadaires et
12 mensuelles, avec un RPO provisoire de 24 h. `cmd/db-backup` produit une
archive de base et de FileStore dans le répertoire configuré. Les options
examinées sont :

- un stockage objet compatible S3 en région européenne, par exemple IONOS
  Object Storage ou Scaleway Object Storage ;
- un dépôt distant SFTP sur une machine réellement distincte ;
- un volume ou système de fichiers distant monté, qui ne doit pas constituer
  l’unique copie hors machine.

Orientation provisoire : conserver `cmd/db-backup` pour créer les sauvegardes,
puis les transférer vers un bucket objet européen avec chiffrement côté client,
des identifiants dédiés et une protection contre l’effacement. Si le CMS est
hébergé chez IONOS, Scaleway apporterait une séparation de fournisseur ; IONOS
reste une option de simplicité si un compte adapté existe. Ce sont des options
à comparer, pas une décision de fournisseur : aucun bucket ni stockage extérieur
n’est actuellement configuré.

La phase de choix/configuration du stockage externe, de chiffrement et de mesure
du RTO est mise en attente. Elle reprendra après stabilisation du CMS et essais
de plusieurs imports hors paiement, et devra se conclure par une restauration
chronométrée depuis la copie extérieure. Le RPO provisoire de 24 h et la
rétention ci-dessus restent les hypothèses de reprise.

## Candidat d’import futur — module de paiement PrestaShop

Un module de paiement PrestaShop pourrait ensuite servir de cas d’étude réaliste
pour éprouver l’analyse des hooks et déclarations, la détection des dépendances,
et surtout les diagnostics de capacités non prises en charge (réseau, secrets,
callbacks, états asynchrones, remboursements). Le bénéfice serait de mieux
délimiter ce que le pipeline peut analyser ou refuser, pas de fournir un
prestataire de paiement EUROMNI ni de rendre le module exécutable.

Ce n’est pas retenu comme premier import : le pipeline n’ingère pas encore de
package PrestaShop Addons de bout en bout, ne traduit pas généralement les
modules métier et n’exécute jamais leur PHP. Le hook `paymentOptions` observé
dans les fixtures est actuellement classé `other`, sans sémantique de paiement.
Il est donc préférable de réussir d’abord plusieurs imports non liés au
paiement, puis d’ajouter éventuellement ce module comme fixture d’analyse
statique, sans accès réseau ni secrets et sans encaissement.

## Service de courriel intégré — à concevoir

Le CMS devra intégrer un service applicatif pour les courriels transactionnels
et notifications. « Intégré » ne signifie pas qu’un serveur SMTP devra être
embarqué : l’architecture envisagée garde événements, modèles, destinataires,
reprises et journalisation dans le CMS, avec un adaptateur de remise configuré
côté hôte et un mode local de développement/test. Aucun fournisseur ni choix
de transport n’est arrêté. La file d’attente, les retries, l’idempotence, la
localisation, la conformité et les limites anti-abus seront définis dans une
phase de conception dédiée ; aucun secret ni accès réseau ne sera donné aux
modules Wasm.

## État actuel du socle

Le dernier incrément poussé sur `main` est `e5682d4` (`Implement development
payment lifecycle`). Il ajoute le cycle de paiement fictif décrit ci-dessus.
Le pilote précédent `2a9665f` (`Add bundled Wasm shipping pricing pilot`)
ajoute une livraison gratuite opt-in dès 100 unités, revalidée par l’hôte et
conservée dans la commande ; il ne constitue pas un système générique
d’installation d’extensions Wasm.

Le CMS dispose aussi d’un catalogue administrable (marques, fournisseurs et
personnalisations prédéfinies), d’une vitrine et d’un panier invité, du checkout
REST, de la configuration et de l’historique des tarifs transport, ainsi que
du traitement manuel des commandes et paiements. Le faux paiement ne réalise
aucun encaissement ; aucun prestataire réel n’est intégré. Le détail des
capacités vendeur et acheteur est décrit dans [le contrat API](api.md) et
[la documentation de la vitrine](../frontend/README.md).

## Dernier point de reprise — sauvegarde MariaDB (19 octobre 2026)

Point de sauvegarde fonctionnel précédent : commit `6518a0e` (`Add shipping rate
history restoration`) sur `main`, poussé vers `origin/main`. Le transport distingue
maintenant transporteur, service et tarif; le checkout choisit le service et
enregistre sa version UUID. Les tarifs préexistants sont migrés vers le service
`legacy` (« Offre historique ») sans perte de prix.

Les tables `shipping_rate_versions` et `shipping_rate_version_rules` sont un
historique append-only. Chaque sauvegarde de configuration crée une nouvelle
version et copie les règles; les commandes pointent vers la version sélectionnée.
L’admin permet maintenant de parcourir l’historique paginé, y compris les
combinaisons qui ne sont plus dans la configuration courante, de comparer un
snapshot au tarif actif et de restaurer une révision comme nouvelle version.
Les lectures et restaurations sont réservées aux owners/admins. Une restauration
est transactionnelle et refusée si le service ou la zone requis a été supprimé.
Les snapshots historiques et les commandes déjà passées restent inchangés.

Routes d’administration ajoutées :

- `GET /v1/admin/shipping/versions/targets` : combinaisons ayant un historique.
- `GET /v1/admin/shipping/versions?carrier_code=…&service_code=…&zone_code=…&limit=…&offset=…`
  : snapshots et règles d’un service/zone.
- `POST /v1/admin/shipping/versions/{rateVersion}/restore` : publication d’une
  nouvelle version depuis le snapshot désigné.

Validations du checkpoint précédent :

- `go test -count=1 ./...` : 233 tests passent.
- `go test -tags integration -count=1 ./internal/store -run TestPostgresStoreIntegration` : passe sur PostgreSQL embarqué.
- `go test -tags integration -count=1 ./internal/api -run TestPostgresAPIEndToEnd` : passe sur PostgreSQL embarqué.
- `go test -v -tags integration -count=1 ./internal/store -run TestMariaDBMigrationAndCommerceFlow` : passe sur MariaDB 11.8.2 et une base locale dédiée.
- `node --check frontend/admin/admin.js` et `node --check frontend/euromni-catalog.js` : passent.

Validations de cette reprise :

- `go test -tags integration -count=1 ./internal/store -run TestPostgresStoreIntegration` : passe; vérifie l’historique, la restauration, les règles, le snapshot de commande et l’indisponibilité après suppression du service.
- `go test -tags integration -count=1 ./internal/api -run TestPostgresAPIEndToEnd` : passe; vérifie les routes protégées, la pagination/validation et la conservation du snapshot de commande.
- `go test ./...` : passe après l’ajout de l’historique/restauration.
- `node --check frontend/admin/admin.js` et `git diff --check` : passent.

Sauvegarde/restauration des bases :

- `cmd/db-backup` détecte le dialecte, utilise `pg_dump` pour PostgreSQL ou
  `mariadb-dump`/`mysqldump` pour MariaDB, et archive le FileStore avec les
  empreintes SHA-256.
- `-restore-database` restaure dans une base précréée et vide du même dialecte;
  le FileStore se restaure séparément avec `-restore-artifacts`.
- Intégration MariaDB 11.8.8 exécutée sur une instance et une base de test
  dédiées : la restauration des données, triggers et procédures fonctionne, et
  une destination non vide est refusée. Le flux de commerce et restauration
  tarifaire MariaDB a aussi passé sur cette instance.
- PostgreSQL backup/restauration intégration réussie avec clients PostgreSQL
  16.15, compatibles avec le serveur PostgreSQL 16 embarqué.
- `go test ./...` et `go vet ./...` : passent après l’ajout du support MariaDB.
- L’instance MariaDB locale sert uniquement aux tests d’intégration ; elle ne
  constitue ni une base CMS de production ni une cible de sauvegarde.

**Prochaine reprise convenue** : stabiliser le fonctionnement du CMS, puis
cadrer et éprouver plusieurs imports hors paiement. Les décisions CMS-05d
(chiffrement, cible de sauvegarde hors machine et mesure du RTO) restent à
résoudre avant un usage client. La sauvegarde/restauration MariaDB est exercée
localement ; l’accès IONOS reste non qualifié et la validation du connecteur ne
doit pas être généralisée. Le prestataire réel attendra ces étapes.

## Vision cible

Le but du projet est de construire un CMS souverain headless, avec :

- un cœur en Go
- une exécution de modules côté boutique via WebAssembly
- une couche API REST / GraphQL
- une persistance PostgreSQL
- un front-end d’expérience boutique autonome, sans dépendance PHP
- une chaîne de validation de modules (PHP → Wasm → runtime contrôlé)

L’architecture complète est le but final, et le front-end de démonstration sert aujourd’hui de support pour valider le comportement par l’exemple.

## Ce qui est déjà réalisé

### 1. Base du moteur Go

- point d’entrée principal dans [cmd/cms/main.go](../cmd/cms/main.go)
- runtime Wasm et validation dans [internal/wasm](../internal/wasm)
- API principale dans [internal/api](../internal/api)
- couche de persistance dans [internal/store](../internal/store)
- schéma SQL et seeds dans [migrations](../migrations)

### 2. Périmètre fonctionnel déjà présent

- API REST / GraphQL opérationnelle sur une base de preuve
- stockage de produits, paniers et commandes en mémoire / PostgreSQL selon les tests
- module minimal de validation Wasm
- tests unitaires et d’intégration présents dans les packages Go
- intégration PostgreSQL documentée dans [docs/testing-postgresql.md](./testing-postgresql.md) et [docs/local-postgresql.md](./local-postgresql.md)

### 3. Front-end de démonstration

- composants web de boutique démarrés dans [frontend](../frontend)
- documentation de l’interface dans [frontend/README.md](../frontend/README.md)
- le composant de vitrine est pensé pour illustrer le fonctionnement du catalogue, du panier et du checkout
- il sert à valider les flux métier par l’exemple, sans être la version finale du front produit

### 4. Documentation d’architecture

Les éléments existants couvrent déjà :

- architecture générale dans [docs/architecture.md](./architecture.md)
- contrat ABI dans [docs/abi.md](./abi.md)
- API dans [docs/api.md](./api.md)
- modèle de données dans [docs/database.md](./database.md)
- plan qualité et maîtrise documentaire dans [docs/plan-directeur-qualite.md](./plan-directeur-qualite.md)
- progression de validation du module minimal dans [docs/module-validation-progress.md](./module-validation-progress.md)

## Ce qui reste à construire

### 1. Architecture complète du projet

Le périmètre cible n’est pas encore entièrement consolidé en une architecture de production complète.

À finaliser :

- versionnement des artefacts Wasm et des modules installés
- séparation claire entre build host et runtime cible
- matrice de portabilité validée
- mécanismes de publication / rollback / restauration

### 2. Pipeline PHP → Wasm

Le point bloquant principal reste la preuve de toolchain portable :

- la conversion PHP → Wasm doit être validée sans imports WASI implicites
- le runtime autorise uniquement des imports contrés comme `euromni.call`
- l’ABI doit être stable, vérifiée et documentée

Les étapes de sécurité et d’analyse statique ont été validées sur le plan de la chaîne de réception PHP :

- validation d’archive ZIP PHP et rejet des chemins dangereux
- validation du manifest minimal requis
- analyse statique préliminaire des hooks WooCommerce (`add_action` / `add_filter`)
- analyse statique préliminaire de WooCommerce et de deux modules PrestaShop réels (`ps_emailalerts`, `ps_cashondelivery`)
- classification des hooks PrestaShop selon la documentation `9.x` (`action`, `display`, autre) et association à leurs handlers PHP observés
- lecture des manifests TOML de deux références Shopify (Discount Function et extension checkout)
- lecture du `module.xml`, des événements et de la DI de deux modules Magento (Japan Tax et gRPC)
- IR v2 compilable en Wasm, avec hash des octets PHP analysés, version plateforme/adaptateur, appels statiques et référence dynamique UUID limitée à `catalog.product.get`
- manifeste de build v1 vérifiable qui relie IR, provenance, ABI, capabilities et artefact Wasm
- publication artifact-first avec hash Wasm et manifeste JSONB écrits ensemble dans `modules`
- ordonnanceur autonome de réconciliation ; valeurs provisoires consignées : grâce 7 jours, cadence 24 heures, retry 5 minutes (à réviser pour la production)
- lowering déterministe d’un hook WooCommerce vers cet appel, puis exécution du Wasm généré comme module installé via PostgreSQL et `Runtime.Run`
- validation/résolution d’un champ trigger dynamique dans une enveloppe ABI et passage au guest via `Runtime.RunWithInput`

Les modules PrestaShop sont des snapshots de dépôts GitHub officiels épinglés à des commits ; leur source PHP est analysée mais n’est jamais exécutée. Ces snapshots ne sont pas les packages PrestaShop Addons et ne valident pas encore leur ingestion complète ni leur manifest. La traduction réelle de modules métier reste à construire.

La documentation développeur PrestaShop `9.x` est la baseline actuelle. Il n’y a pas de documentation développeur publique `10.x` disponible au 13 octobre 2026 ; l’adaptateur ne revendique donc pas la compatibilité PS10 et devra être révisé à partir des contrats officiels lors de leur publication.

Les quatre références Shopify et Magento sont épinglées à des commits et conservées dans `pipeline/fixtures/real-modules/` avec leurs fichiers de licence. Les tests confirment uniquement l’analyse déclarative ; aucune app ou extension n’est installée ou exécutée. Un test compile des IR statique et dynamique puis charge/exécute le Wasm par le chemin autorisé du runtime. Le manifeste de build v1 lie les hashes source, archive, IR et Wasm, avec versions et capabilities. `PublishModuleBuild` stocke le Wasm avant d’enregistrer ensemble hash et manifeste en base. L’ordonnanceur autonome utilise provisoirement une grâce de 7 jours, une cadence de 24 heures et un retry de 5 minutes ; son lancement reste indépendant du CMS et ces valeurs sont à revoir avant production. L’IR v2 ne traduit pas automatiquement les modules source.

### 3. Sécurité et validation des modules

À compléter :

- ingestion administrateur authentifiée
- quarantaine des archives
- rejet des archives dangereuses et des chemins non autorisés
- validation strictement déterministe des artefacts
- contrôle de hash, ABI, taille et capabilities
- planification et configuration de la réconciliation des artefacts Wasm non référencés

### 4. Intégration de modules e-commerce

Les adaptateurs ne sont encore que des analyses statiques ciblées. Il faut :

- définir les hooks et opérations métier supportés pour chacune des quatre plateformes
- capturer les différences fonctionnelles de chaque plate-forme
- valider les adaptations dans un cadre déterministe

### 5. Front-end final

Le front existant est un bon socle de démonstration, mais il n’est pas encore la boutique finale :

- pas encore un front de production complet
- pas encore un modèle de composants finalisé pour l’usage métier
- pas encore un système de checkout complet, taxes et livraison à formaliser

## Priorités immédiates

1. valider la toolchain portable de build et le contrat ABI
2. finaliser la matrice de portabilité runtime
3. verrouiller le manifeste/versionnement des modules
4. sécuriser l’ingestion et la publication des artefacts
5. ajouter la consultation, comparaison et restauration non destructive de
   l’historique des tarifs; maintenir la parité PostgreSQL / MariaDB
6. étendre les schémas dynamiques aux données métier requises et ajouter des
   règles déterministes de lowering pour d’autres triggers
7. compléter le front-end en s’appuyant sur les flux validés

## Conclusion

Le projet a déjà une base solide : architecture documentée, runtime Wasm validé, API et stockage structurés, front de démonstration en place, et plan qualité de référence. Il manque encore la consolidation de l’architecture cible et la preuve de production d’un pipeline de modules portables et sécurisés.

C’est donc bien un projet en évolution, avec un socle technique crédible, mais encore orienté vers la validation d’un noyau sécurisé avant la finalisation complète du système.
