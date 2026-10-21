# Plan directeur qualité et actions

| Champ | Valeur |
|---|---|
| Identifiant | EUROMNI-QP-001 |
| Version | 3.0 |
| État | Brouillon contrôlé, approbation humaine à renseigner |
| Date de révision | 2026-10-17 |
| Responsable du document | Responsable produit/technique, à désigner |
| Périmètre | Architecture, pipeline PHP→Wasm, ABI, artefacts et validation du cœur EUROMNI |
| Exigence de portabilité | Artefacts Wasm indépendants de l’hôte de build ; plateformes runtime déclarées et testées |

Ce document organise les actions et preuves nécessaires à la réalisation du
projet. Il s’inspire des pratiques de maîtrise documentaire, des risques, de la
conception, de la vérification et de l’amélioration d’ISO 9001. **Il ne constitue
ni une déclaration de conformité ni une certification ISO 9001.** L’approbation
et les responsabilités qualité doivent être confirmées par le responsable
produit.

## Références et exigences

- Architecture cible : [`architecture.md`](architecture.md).
- Matrice runtime/toolchain : [`portability.md`](portability.md).
- Contrat ABI : [`abi.md`](abi.md).
- Chaîne PHP→Wasm et frontières de confiance : [`../pipeline/README.md`](../pipeline/README.md).
- Exécution Wasmtime : imports limités à `euromni.call`, capabilities médiées,
  validation de hash/ABI, plafonds de mémoire et fuel.
- Contrainte d’exploitation : Docker est facultatif pour les tests et n’est pas
  une exigence de production.

## Rôles

- **Responsable produit** : confirme le périmètre, les plateformes pilotes et
  les critères d’acceptation métier.
- **Responsable qualité** : à désigner ; approuve ce document, les changements
  de statut et les dérogations.
- **Responsable technique** : maintient les contrats, les revues, les preuves
  de tests et la traçabilité des versions.
- **Contributeur/agent** : travaille sur une action identifiée, exécute les
  contrôles indiqués et rapporte les résultats sans déclarer une conformité
  non démontrée.

## État de référence

Au 2026-10-13, le runtime Wasmtime, l’ABI hôte, le registre de modules installés
et le stockage local d’artefacts par SHA-256 existent. Le stockage est streamé,
borné à 64 Mio et vérifie l’intégrité à la lecture. Le dépôt contient aussi une
validation d’archives PHP, des analyses statiques ciblées, une IR v2, une
génération WAT/Wasm limitée et un helper de publication de builds. Il ne contient
pas de traduction générique de PHP ni d’endpoint d’ingestion HTTP authentifié.
Le test Go 1.26.8 `wasip1` consigné dans le pipeline observe des imports WASI
supplémentaires ; cette configuration ne respecte pas le linker strict. Les
cibles OS/architecture du runtime hôte n’ont pas encore de matrice de tests
approuvée. Voir [`portability.md`](portability.md) ; ni QP-01 ni QP-10 ne sont
clos par la seule documentation.

Avant toute reprise de code, vérifier l’état réel de l’arbre de travail et
l’historique Git. Les notes de version précédentes ne permettent pas de déduire
si ces changements ont été commités ni si d’autres modifications locales sont
présentes.

## Registre d’actions

Statuts : **À faire**, **En cours**, **Bloqué**, **Vérification**, **Terminé**.
Une action ne passe à Terminé qu’avec ses preuves référencées dans le commit ou
la documentation du projet.

| ID | Priorité | Action / dépendance | Critère d’acceptation | Preuve attendue | Statut |
|---|---|---|---|---|---|
| QP-01 | P0 | Prouver une toolchain qui produit un artefact Wasm portable compatible avec `euromni.call`, sans imports système implicites. Dépend du contrat ABI actuel. | Build reproductible depuis un worker épinglé ; module n’importe que l’ABI autorisée et s’exécute sur la matrice Wasmtime approuvée. Aucun compilateur/LLVM local requis en production. | Spike Go 1.26.8 : imports WASI incompatibles. Candidate Rust 1.99.1/Cargo `--frozen` : imports, exports, exécution Wasmtime et deux builds en répertoires propres identiques localement sur Windows/amd64 ; CI Linux réussie (run 39030961826). Le workflow natif complet Windows/Linux a réussi (run 39032334862), y compris le test Rust sur les deux runners ; restent à définir et évaluer le processus complet de build/promotion et sa provenance. | En cours |
| QP-02 | P0 | Définir le manifeste/versionnement d’artefact et son lien atomique avec `modules` et `shop_modules`. | Hash source, hash Wasm, plateforme/version, versions d’adaptateur/IR/compilateur, ABI et capabilities sont traçables ; un échec ne remplace pas la version active. | Builds immuables, pointeur courant et pin par installation migrés en PostgreSQL/MariaDB ; publication et promotion transactionnelles couvertes par intégration PostgreSQL, migration et parcours MariaDB dédiés réussis. Manifeste strict, allowlist des versions approuvées, promotion dans le CMS de test et exécution du build promu vérifiés. | Terminé |
| QP-03 | P0 | Concevoir une ingestion admin authentifiée avec quarantaine d’archives. | Taille/nombre d’entrées bornés ; traversal, chemins absolus, liens symboliques, archives bombes et extensions non autorisées rejetés ; aucun PHP exécuté. | Tests négatifs d’archives et revue sécurité ; aucun endpoint anonyme. | À faire |
| QP-04 | P1 | Créer les adaptateurs statiques WooCommerce et PrestaShop et leur matrice de versions/hooks supportés. | Un jeu de fixtures autorisé par plateforme produit des diagnostics déterministes et identifie les appels de données/effets de bord. | Fixtures, version source, résultats d’analyse golden et rapports de limites. | À faire |
| QP-05 | P1 | Définir l’IR métier et sa validation avant toute génération. | Chaque opération IR déclare types, capability, erreurs et comportement ; capacités inconnues ou accès système impossibles à générer. | Schémas versionnés, tests de validation et cas rejetés. | À faire |
| QP-06 | P1 | Implémenter la traduction contrôlée de l’IR et la compilation reproductible. | Aucun résultat LLM n’est exécuté directement ; source/config identiques produisent le même artefact, ou une différence est expliquée. | Toolchain verrouillée, provenance, tests de reproductibilité et SBOM selon dépendances retenues. | À faire |
| QP-07 | P1 | Valider Wasm avant publication et enregistrer l’artefact. | Imports, export mémoire/entrée, ABI, hash, taille, mémoire et fuel conformes ; publication atomique ; artefact corrompu ou non conforme rejeté. | Tests d’imports interdits, hash, limites, rollback et rechargement. | À faire |
| QP-08 | P2 | Exécuter deux pilotes autorisés, un WooCommerce et un PrestaShop, sur une fonction catalogue ou hook simple, sans paiement. | Les deux passent par la même ABI sans SQL/réseau/filesystem côté guest et leurs sorties correspondent à un oracle documenté. | Sources réelles non financières, Lowerings et exécutions publiées : WooCommerce `woocommerce-extra-product-sorting-options` (GPL-3.0), épinglé au commit `bc5829d4d2383ae956869386eb40e772cf4a76a0` ; `alphabetical` / `reverse_alpha` couverts par oracle et Wasmtime. PrestaShop `ps_newproducts` (AFL-3.0), import et exécution publiés avec réglages. Tests dans PostgreSQL jetable ; aucun PHP exécuté et invités limités à `euromni.call`/capability `catalog`. Restent la compatibilité sur instances plateformes réelles, l’élargissement de versions et les options WooCommerce non traduites (`by_stock`, `review_count`, `on_sale_first`). Stripe et `ps_cashondelivery` exclus. Essai Selfnamed WooCommerce 2.1.0 natif isolé, distinct et non comptabilisé dans QP-08. | En cours |
| QP-09 | P2 | Définir rétention, sauvegarde, restauration et suppression des sources/artefacts. | Règles par type d’artefact, stockage hors machine, restauration vérifiée, purge traçable. | Procédure et exercice de restauration/purge. | À faire |
| QP-10 | P0 | Définir la matrice de portabilité du cœur et du runtime Wasmtime, distincte de la toolchain invitée. | OS/architectures de production décidés et testés ; dépendances natives/CGo, PostgreSQL, stockage et procédure de déploiement documentés. Une cible non testée n’est pas déclarée supportée. | [`portability.md`](portability.md) : Windows/amd64 et Linux/amd64 retenus ; tests natifs, Wasmtime, intégrations PostgreSQL 16, FileStore, builds CGo et smoke tests réussis sur les deux runners dans le run 39032334862. Limites explicites par système de fichiers et architectures non testées consignées. | Terminé |

Ordre de reprise recommandé : traiter **QP-01 et QP-10 ensemble**. Le poste de
développement n’est pas la cible d’architecture ; un worker de build épinglé
est acceptable s’il produit un artefact portable et reproductible, sans rendre
Docker nécessaire en production. Continuer ensuite QP-02/03, QP-04/05,
QP-06/07, puis QP-08/09. Ne pas commencer la traduction générique avant d’avoir
validé l’IR avec les deux plateformes pilotes.

La matrice de portabilité sépare les guests Wasm, le binaire Go/CGo du runtime,
le worker de build et le FileStore. Elle ne declare aucune plateforme supportee :
les cibles de production et leurs preuves d'execution restent a approuver.

## Contrôles de passage

Pour chaque action :

1. Lier le changement à un ID QP et aux exigences concernées.
2. Énoncer les risques et les critères avant l’édition ; limiter le périmètre.
3. Ajouter tests positifs et négatifs proportionnels au risque, puis exécuter
   tests unitaires, intégration concernée, `go vet` et `git diff --check` selon
   le changement.
4. Consigner commande, résultat, commit et limites non vérifiées. Un test non
   exécuté n’est pas marqué conforme.
5. Faire approuver les dérogations et changements de critères par le
   responsable qualité/produit ; ne jamais élargir silencieusement les imports,
   capabilities ou permissions.

## Maîtrise documentaire et reprise

- Toute modification du présent plan incrémente la version et ajoute une entrée
  d’historique en bas de page ; le responsable qualité valide l’approbation.
- Les décisions techniques font référence à ce document par ID QP dans le
  commit, la PR ou le rapport de validation.
- Les preuves sont conservées dans Git (tests, contrats, migrations) ou
  référencées par identifiant immuable ; aucun secret ni archive client n’est
  commité.
- À la reprise : lire ce plan, `git status`, le dernier commit, puis les docs
  référencées ; reprendre la première action non terminée sans supposer que les
  conclusions non testées sont acquises.

## Feuille de route CMS catalogue

Cette feuille de route suit la stabilisation du CMS catalogue et de son parcours
de commande ; elle ne remplace pas les actions QP du pipeline PHP→Wasm. La
priorité est un CMS installable sur une machine Windows ou Linux avec PostgreSQL
standard. Docker et tout hébergeur cloud restent optionnels.

| Étape | Priorité | Action / dépendance | Critère d’acceptation et preuve | Statut |
|---|---|---|---|---|
| CMS-01 | P0 | Valider l’interface admin dans un navigateur intégré : rendu, connexion, consultation/édition du catalogue, erreurs et déconnexion. S’appuyer sur les tests Go PostgreSQL du parcours staff. | Écran et gestion d’erreur observés au navigateur ; défaut du formulaire asynchrone corrigé. Connexion/CRUD/déconnexion validés par l’intégration Go/PostgreSQL. | Terminé |
| CMS-02 | P1 | Ajouter la gestion des médias aux produits en réutilisant `media` et `product_media`. Dépend de CMS-01. | L’admin peut associer, ordonner, éditer le texte alternatif et retirer des URI d’images ; URI validées, accès boutique isolé, aucune ingestion de fichier local dans cette tranche. Tests API/store réussis et vérification visuelle authentifiée du parcours média réussie. | Terminé |
| CMS-03 | P1 | Compléter le cycle de vie des comptes staff et protéger l’accès CMS. Dépend de CMS-02. | Gestion du personnel limitée au rôle owner, révocation des sessions affectées et tests des permissions/échecs ; pas d’identifiants de démonstration par défaut. Tests API/PostgreSQL couvrent création, mise à jour, révocation, retrait et refus d’accès. | Terminé |
| CMS-04 | P1 | Ajouter la consultation des commandes et les transitions de statut sûres au CMS. Dépend de CMS-03. | Liste et détail tenant-scopés ; transitions explicites, snapshots commande conservés et tests API/store couvrant pagination, permissions, annulation et refus avec paiement ou statut incompatible. Tests Go/PostgreSQL réussis. | Terminé |
| CMS-05 | P1 | Terminer l’installation et l’exploitation du CMS sur machine locale/serveur autonome, sans dépendance obligatoire à Docker ni à un hébergeur. Dépend de CMS-01 à CMS-04. | Sous-étapes CMS-05a à CMS-05d ci-dessous terminées : démarrage sûr, migrations reproductibles, binaire portable et exercice vérifié de persistance/restauration. | En cours |
| CMS-06 | P3 | Évaluer un hébergeur distant (IONOS ou autre) uniquement si le besoin est confirmé, après CMS-05. | Environnement de test isolé, secrets hors Git, TLS, migrations et restauration vérifiés. Cette étape est facultative et ne bloque pas la livraison du CMS autonome. | Reporté — décision différée |
| CMS-07 | P1 | Ajouter un circuit de livraison tarifée : configuration admin de transporteurs, zones pays et tarifs fixes ; choix du client au checkout ; total serveur et snapshot transport sur commande. | Les tarifs sont tenant-scopés et revalidés dans la transaction de commande ; destinations sans mode actif refusées clairement ; le snapshot reste stable après modification des tarifs. Tests API, store PostgreSQL et frontend couvrent configuration, permissions, calcul du total et affichage client/admin. Les intégrations transporteur, étiquettes, points relais, taxes et paiement restent hors tranche. | Terminé |

### Décomposition de CMS-05

| Sous-étape | Action / dépendance | Critère d’acceptation et preuve | Statut |
|---|---|---|---|
| CMS-05a | Fiabiliser le démarrage et la configuration portable. | Le défaut d’écoute est limité à `127.0.0.1`; `LISTEN_ADDR` permet un bind explicite ; un `DATABASE_URL` invalide fait échouer le démarrage plutôt qu’un repli silencieux en mémoire. Tests unitaires, documentation et configuration Compose alignés. | Terminé |
| CMS-05b | Fournir un outil de migration de schéma utilisable sans Docker. Dépend de CMS-05a. | `cmd/db-migrate` applique les migrations embarquées dans l’ordre sous verrou PostgreSQL ; un bootstrap vierge est atomique, chaque mise à jour et son entrée de registre sont atomiques, et le ledger non contigu ou un schéma non géré sont refusés. Intégrations PostgreSQL vérifient le bootstrap utilisé par les suites, l’idempotence, l’échec/rollback et la réutilisation de la connexion. `go test ./...` et `go vet ./...` réussis. La validation Compose reste à faire sur une machine équipée de Docker. | Terminé |
| CMS-05c | Construire et lancer une distribution autonome sur les cibles retenues. Dépend de CMS-05b. | Terminé le 2026-10-17 : workflow natif sur Windows Server 2022 et Ubuntu 22.04 avec vet, tests, intégrations PostgreSQL, builds CGo, extraction, lancement et smoke tests HTTP des deux archives ([exécution #39032334862](https://github.com/GorParis/EUROMNI-GorParis/actions/runs/39032334862)). Les deux artifacts sont publiés ; PostgreSQL reste un service séparé. | Terminé |
| CMS-05d | Vérifier persistance, sauvegarde et restauration locales. Dépend de CMS-05c. | Exercice réussi le 2026-10-17 sur PostgreSQL 16.15 isolé : création catalogue/commande via l'API, vérification après redémarrage, dump custom puis restauration dans une base isolée ; contrôles SQL positifs pour le produit, le owner et les snapshots client/ligne. `cmd/db-backup` sauvegarde PostgreSQL ou MariaDB, archive le FileStore, inscrit le dialecte et les empreintes SHA-256, vérifie les dumps et restaure la base dans une destination vide ainsi que le FileStore séparément. Intégration MariaDB 11.8.8 réussie le 2026-10-19 pour données, triggers, procédures et refus d'une destination non vide. Le pilote portable Windows sur PostgreSQL 16.9 confirme migrations, connexion owner et persistance des cinq produits après redémarrage du CMS et de PostgreSQL. Rétention choisie : 14 quotidiennes/8 hebdomadaires/12 mensuelles ; RPO provisoire : 24 h ; RTO à mesurer. Options examinées : stockage objet S3 européen (IONOS/Scaleway) ou SFTP distant ; orientation provisoire vers un bucket objet chiffré côté client, fournisseur non choisi. La configuration du stockage extérieur, le chiffrement et l'exercice RTO sont mis en attente jusqu'à stabilisation du CMS et essais de plusieurs imports hors paiement. | En attente — phase externe différée |

Le périmètre média initial accepte les URI gérées par le propriétaire (par
exemple un CDN) ; l’upload binaire et l’intégration à un fournisseur d’objets
feront l’objet d’une décision et d’une validation de sécurité séparées. La cible
de déploiement intermédiaire est une machine locale ou un serveur autonome ;
Docker peut rester une commodité de développement. L’ordre de travail est
CMS-05b (migrations), CMS-05c (distribution), puis CMS-05d (restauration).
IONOS ne sera évalué que si le besoin est confirmé.

## Historique des révisions

| Version | Date | Modification | Approbation |
|---|---|---|---|
| 1.0 | 2026-10-12 | Création du plan directeur initial pour le pipeline PHP→Wasm et la maîtrise des artefacts. | En attente du responsable produit/qualité |
| 1.1 | 2026-10-12 | Résultat du spike Go WASI consigné ; QP-01 bloqué en attente d’un invité sans imports WASI. | En attente du responsable produit/qualité |
| 1.2 | 2026-10-12 | Clarification de l’exigence de portabilité et ajout d’une matrice d’hôtes runtime distincte du poste de build. | En attente du responsable produit/qualité |
| 1.3 | 2026-10-13 | Ajout d’une matrice initiale séparant runtime CGo, guests Wasm et worker de build ; aucune cible n’est déclarée supportée sans validation. | En attente du responsable produit/qualité |
| 1.4 | 2026-10-14 | Suite Windows/amd64 validee ; correction du FileStore avec DACL NTFS protege et documentation du test. QP-10 reste ouvert en attente de l'integration et des cibles de production. | En attente du responsable produit/qualité |
| 1.5 | 2026-10-15 | Ajout de la feuille de route vérifiable du CMS : navigateur, médias, cycle staff, commandes et répétition de déploiement IONOS isolée. | En attente du responsable produit/qualité |
| 1.6 | 2026-10-15 | Ajout de la documentation API staff/commandes et mise à jour des preuves des étapes CMS-02 à CMS-04 après validation Go/PostgreSQL. | En attente du responsable produit/qualité |
| 1.7 | 2026-10-15 | Réorientation de CMS-05 vers le déploiement portable sur machine locale/serveur autonome ; IONOS devient optionnel et hors chemin critique. | En attente du responsable produit/qualité |
| 1.8 | 2026-10-15 | CMS-02 vérifié dans le navigateur ; objectif de portabilité locale précisé et sécurisation du démarrage direct en cours. | En attente du responsable produit/qualité |
| 1.9 | 2026-10-15 | Décomposition de CMS-05 en démarrage sûr, outil de migrations, distribution Windows/Linux et exercice de sauvegarde/restauration ; IONOS isolé en étape facultative CMS-06. | En attente du responsable produit/qualité |
| 2.0 | 2026-10-15 | CMS-05b terminé : commande Go de migration embarquée, bootstrap et mises à jour atomiques, registre contrôlé, documentation autonome mise à jour et intégrations PostgreSQL réussies. | En attente du responsable produit/qualité |
| 2.1 | 2026-10-15 | CMS-05c démarré : paquetage autonome Go et workflow de build natif Windows/Linux amd64 ; build et démarrage avec assets embarqués vérifiés localement sous Windows, validation des runners encore attendue. | En attente du responsable produit/qualité |
| 2.2 | 2026-10-17 | QP-10 : tests Go, vet, intégrations PostgreSQL 16 et build natif Windows/amd64 exécutés ; les tests IR inspectent désormais les imports du Wasm généré. Décision de cibles de production encore ouverte. | En attente du responsable produit/qualité |
| 2.3 | 2026-10-17 | CMS-05c : archive Windows autonome construite et extraite ; health, interface admin et catalogue du binaire empaqueté vérifiés en HTTP hors dépôt. Runners GitHub et installation PostgreSQL restent à valider. | En attente du responsable produit/qualité |
| 2.4 | 2026-10-17 | CMS-05c : workflow natif GitHub Actions réussi sur Windows/Linux amd64 ; tests, builds et deux artifacts confirmés. Exécution du paquet Linux et installation avec PostgreSQL standard restent ouvertes. | En attente du responsable produit/qualité |
| 2.5 | 2026-10-17 | CMS-05c : installation des exécutables Windows construits localement contre PostgreSQL 16.15 isolé. CMS-05d : persistance après redémarrage et sauvegarde/restauration vérifiées ; politique de rétention/hors machine et lancement de l'archive Linux restent ouverts. | En attente du responsable produit/qualité |
| 2.6 | 2026-10-17 | CMS-05c terminé : distributions Windows/Linux extraites et lancées sur runners natifs ; archive Windows publiée installée contre PostgreSQL 16.15. CMS-05d reste en cours, car rétention et stockage protégé hors machine sont à décider. | En attente du responsable produit/qualité |
| 2.7 | 2026-10-17 | CMS-05d : rétention locale, RPO de 24 h et stockage protégé de développement cadrés ; commande de sauvegarde/restauration PostgreSQL/FileStore et intégration réelle validées. Stockage hors machine, chiffrement et mesure du RTO restent ouverts. | En attente du responsable produit/qualité |
| 2.8 | 2026-10-17 | QP-10 : Windows/amd64 et Linux/amd64 retenus par le responsable produit ; le workflow natif de distribution exécute désormais la suite d’intégration PostgreSQL sur les deux runners. Support non déclaré avant réussite du workflow sur les deux cibles. | En attente du responsable qualité |
| 2.9 | 2026-10-17 | QP-01 : spike Rust `no_std` `wasm32-unknown-unknown` épinglé à Rust 1.99.1, Cargo sans dépendances et lockfile vérifié en `--frozen` ; import unique, exports, appel ABI Wasmtime et égalité octet à octet de builds depuis deux répertoires propres validés localement sur Windows/amd64. CI générale Linux réussie ; premier run natif a validé le spike sur les deux OS, puis révélé des prérequis de runner d'intégration, corrigés dans le workflow. Confirmation matricielle et chaîne de promotion restent attendues. | En attente du responsable qualité |
| 2.10 | 2026-10-17 | QP-01/QP-10 : le run 39032009205 confirme l'exécution du workflow sur les runners mais échoue avant les intégrations Linux, car Ubuntu 22.04 ne trouve pas `postgresql-client-16` dans ses dépôts activés. Le workflow ajoute le dépôt PGDG ; un nouveau run complet reste requis avant validation des cibles. | En attente du responsable qualité |
| 2.11 | 2026-10-17 | QP-10 : le run 39032334862 passe sur Windows/Linux amd64, intégrations PostgreSQL 16, tests Rust/Wasm, builds CGo, smoke HTTP et publication des deux artifacts ; QP-10 est terminé pour cette matrice. CI générale verte (run 39032300222). QP-01 reste en cours pour la chaîne de build/promotion. | En attente du responsable qualité |
| 3.0 | 2026-10-17 | Pilote CMS Windows sans privilèges admin : binaires natifs Go/CGo installés sous `%LOCALAPPDATA%`, PostgreSQL 16.9 de développement limité à loopback, scripts utilisateur de démarrage/arrêt, migrations idempotentes, bootstrap et authentification owner, cinq produits conservés après redémarrage CMS/PostgreSQL. Les identifiants sont protégés par DPAPI. Les outils `pg_dump`/`pg_restore`, une sauvegarde chiffrée hors machine et la mesure du RTO restent à traiter ; pilote non destiné à la production. | En attente du responsable qualité |
| 3.1 | 2026-10-17 | CMS-07 : configuration transporteurs/zones/tarifs fixes dans l’admin, endpoint public d’options, choix livraison au checkout, calcul transactionnel du total et snapshot transport consultable dans l’admin ; intégrations transporteur et tarifs commerciaux restent à décider. | En attente du responsable qualité |
| 3.2 | 2026-10-19 | CMS-05d : `cmd/db-backup` prend en charge les dumps PostgreSQL et MariaDB, les empreintes/dialectes, la restauration dans une base vide et les artefacts séparés. Test réel sur MariaDB 11.8.8 : données, trigger, routine et destination non vide vérifiés ; stockage hors machine, chiffrement et mesure du RTO restent ouverts. | En attente du responsable qualité |
| 3.3 | 2026-10-19 | Cycle de paiement fictif livré, réservé au développement/tests ; décision produit : prioriser le fonctionnement du CMS puis éprouver plusieurs imports hors paiement avant d’étudier un prestataire réel. Le périmètre précis des imports et la cible de sauvegarde hors machine restent à cadrer ; le RTO reste à mesurer. | En attente du responsable qualité |
| 3.4 | 2026-10-19 | CMS-05d externe mise en attente après examen des options S3 européennes (IONOS/Scaleway) et SFTP distant ; orientation vers un bucket objet chiffré côté client sans sélection de fournisseur. Reprise après stabilisation du CMS et essais d’imports ; prévoir une restauration chronométrée. Un module de paiement PrestaShop est proposé comme candidat ultérieur d’analyse statique, non comme intégration d’encaissement. | En attente du responsable qualité |
| 3.5 | 2026-10-19 | Reprise CMS vérifiée : suite Go, API/PostgreSQL, syntaxe admin/vitrine ; modules non financiers WooCommerce (`woocommerce-extra-product-sorting-options`) et PrestaShop (`ps_newproducts`) ajoutés depuis des commits licenciés et analysés statiquement ; reconnaissance de `WidgetInterface.renderWidget` ajoutée. Le pilote catalogue Wasm exécuté reste basé sur une source synthétique. QP-08 reste en cours : import amont/manifeste, lowerings fidèles et publication/runtime des deux modules restent à réaliser ; aucun PHP tiers n’est exécuté. | En attente du responsable qualité |
| 3.6 | 2026-10-19 | Pilote natif Selfnamed WooCommerce 2.1.0 réussi dans WordPress 6.9.2/WooCommerce 10.9.0/MariaDB 11.8.3, sous Podman rootless, réseau interne sans port publié, mock API, données synthétiques et bearer token fictif. Le test vérifie une commande, son groupe de livraison et l’absence de doublon ; volumes temporaires nettoyés. Cela valide uniquement le plugin original, pas son import ni son exécution EUROMNI/Wasm. Aucun credential réel utilisé ; leur éventuel usage reste à autoriser séparément. | En attente du responsable qualité |