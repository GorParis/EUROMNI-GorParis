# État du projet EUROMNI

## Reprise en cours — intégration transport (18 octobre 2026)

Le travail local porte sur la première grille native de tarifs de transport :
tarifs fixes conservés, poids facultatif des variantes, calcul par intersection
poids × sous-total, devis au checkout et snapshot de commande. L’administration
permet de configurer ces règles. Les migrations, adaptateurs et tests PostgreSQL
et MariaDB correspondants sont présents dans les changements locaux.

État Git observé à la reprise : branche `main`, identique à `origin/main` au
commit `9f3d92d`. Les 26 fichiers de cette intégration sont des changements
indexés, pas des commits locaux en avance. Ne pas réinitialiser l’index, créer
un commit ou pousser sans demande explicite. Préserver ces changements avant
toute modification supplémentaire.

Vérification exécutée :

```sh
go test -count=1 ./internal/api ./internal/store ./internal/migrations
```

Résultat : les trois packages passent. Cette vérification ne prouve pas que les
tests d’intégration ont utilisé une base PostgreSQL ou MariaDB réelle. Exécuter
`go test ./...`, puis les intégrations avec des bases de test dédiées si elles
sont configurées. Ne pas déclarer MariaDB validé sans exécution réelle.

Prochaine étape : relire le diff indexé et la documentation de transport, puis
vérifier les frontières de tranches, les conflits de devis au checkout, les
snapshots historiques et la parité des migrations. Le découpage transporteur /
service, les origines, colis, délais et devis externes restent hors de cette
première tranche.

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
5. terminer la validation de la première grille de transport et maintenir la
   parité PostgreSQL / MariaDB
6. étendre les schémas dynamiques aux données métier requises et ajouter des
   règles déterministes de lowering pour d’autres triggers
7. compléter le front-end en s’appuyant sur les flux validés

## Conclusion

Le projet a déjà une base solide : architecture documentée, runtime Wasm validé, API et stockage structurés, front de démonstration en place, et plan qualité de référence. Il manque encore la consolidation de l’architecture cible et la preuve de production d’un pipeline de modules portables et sécurisés.

C’est donc bien un projet en évolution, avec un socle technique crédible, mais encore orienté vers la validation d’un noyau sécurisé avant la finalisation complète du système.
