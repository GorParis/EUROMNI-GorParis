# Plan directeur qualité et actions

| Champ | Valeur |
|---|---|
| Identifiant | EUROMNI-QP-001 |
| Version | 1.8 |
| État | Brouillon contrôlé, approbation humaine à renseigner |
| Date de révision | 2026-10-15 |
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
| QP-01 | P0 | Prouver une toolchain qui produit un artefact Wasm portable compatible avec `euromni.call`, sans imports système implicites. Dépend du contrat ABI actuel. | Build reproductible depuis un worker épinglé ; module n’importe que l’ABI autorisée et s’exécute sur la matrice Wasmtime approuvée. Aucun compilateur/LLVM local requis en production. | Spike Go 1.26.8 conservé comme preuve que `wasip1` ajoute des imports WASI ; fixture ABI compilée et testée sur les hôtes runtime approuvés. | Bloqué |
| QP-02 | P0 | Définir le manifeste/versionnement d’artefact et son lien atomique avec `modules` et `shop_modules`. | Hash source, hash Wasm, plateforme/version, versions d’adaptateur/IR/compilateur, ABI et capabilities sont traçables ; un échec ne remplace pas la version active. | Schéma/migration, contrat store et test d’échec/reprise. | À faire |
| QP-03 | P0 | Concevoir une ingestion admin authentifiée avec quarantaine d’archives. | Taille/nombre d’entrées bornés ; traversal, chemins absolus, liens symboliques, archives bombes et extensions non autorisées rejetés ; aucun PHP exécuté. | Tests négatifs d’archives et revue sécurité ; aucun endpoint anonyme. | À faire |
| QP-04 | P1 | Créer les adaptateurs statiques WooCommerce et PrestaShop et leur matrice de versions/hooks supportés. | Un jeu de fixtures autorisé par plateforme produit des diagnostics déterministes et identifie les appels de données/effets de bord. | Fixtures, version source, résultats d’analyse golden et rapports de limites. | À faire |
| QP-05 | P1 | Définir l’IR métier et sa validation avant toute génération. | Chaque opération IR déclare types, capability, erreurs et comportement ; capacités inconnues ou accès système impossibles à générer. | Schémas versionnés, tests de validation et cas rejetés. | À faire |
| QP-06 | P1 | Implémenter la traduction contrôlée de l’IR et la compilation reproductible. | Aucun résultat LLM n’est exécuté directement ; source/config identiques produisent le même artefact, ou une différence est expliquée. | Toolchain verrouillée, provenance, tests de reproductibilité et SBOM selon dépendances retenues. | À faire |
| QP-07 | P1 | Valider Wasm avant publication et enregistrer l’artefact. | Imports, export mémoire/entrée, ABI, hash, taille, mémoire et fuel conformes ; publication atomique ; artefact corrompu ou non conforme rejeté. | Tests d’imports interdits, hash, limites, rollback et rechargement. | À faire |
| QP-08 | P2 | Exécuter deux pilotes autorisés, un WooCommerce et un PrestaShop, sur une fonction catalogue ou hook simple. | Les deux passent par la même ABI sans SQL/réseau/filesystem côté guest et leurs sorties correspondent à un oracle documenté. | Licence/source autorisée, fixtures, résultats de tests et écarts consignés. | À faire |
| QP-09 | P2 | Définir rétention, sauvegarde, restauration et suppression des sources/artefacts. | Règles par type d’artefact, stockage hors machine, restauration vérifiée, purge traçable. | Procédure et exercice de restauration/purge. | À faire |
| QP-10 | P0 | Définir la matrice de portabilité du cœur et du runtime Wasmtime, distincte de la toolchain invitée. | OS/architectures de production décidés et testés ; dépendances natives/CGo, PostgreSQL, stockage et procédure de déploiement documentés. Une cible non testée n’est pas déclarée supportée. | [`portability.md`](portability.md) consigne les validations unitaires Windows/amd64 (CGo, Wasmtime et DACL du FileStore) ; test d’intégration et choix de production restent à faire. | En cours |

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
de commande ; elle ne remplace pas les actions QP du pipeline PHP→Wasm. Les tests
automatisés et les validations locales restent indépendants de la base IONOS.
Toute validation IONOS doit utiliser une base isolée, sans données client, et
des secrets fournis directement par le responsable du compte.

| Étape | Priorité | Action / dépendance | Critère d’acceptation et preuve | Statut |
|---|---|---|---|---|
| CMS-01 | P0 | Valider l’interface admin dans un navigateur intégré : rendu, connexion, consultation/édition du catalogue, erreurs et déconnexion. S’appuyer sur les tests Go PostgreSQL du parcours staff. | Écran et gestion d’erreur observés au navigateur ; défaut du formulaire asynchrone corrigé. Connexion/CRUD/déconnexion validés par l’intégration Go/PostgreSQL. | Terminé |
| CMS-02 | P1 | Ajouter la gestion des médias aux produits en réutilisant `media` et `product_media`. Dépend de CMS-01. | L’admin peut associer, ordonner, éditer le texte alternatif et retirer des URI d’images ; URI validées, accès boutique isolé, aucune ingestion de fichier local dans cette tranche. Tests API/store réussis et vérification visuelle authentifiée du parcours média réussie. | Terminé |
| CMS-03 | P1 | Compléter le cycle de vie des comptes staff et protéger l’accès CMS. Dépend de CMS-02. | Gestion du personnel limitée au rôle owner, révocation des sessions affectées et tests des permissions/échecs ; pas d’identifiants de démonstration par défaut. Tests API/PostgreSQL couvrent création, mise à jour, révocation, retrait et refus d’accès. | Terminé |
| CMS-04 | P1 | Ajouter la consultation des commandes et les transitions de statut sûres au CMS. Dépend de CMS-03. | Liste et détail tenant-scopés ; transitions explicites, snapshots commande conservés et tests API/store couvrant pagination, permissions, annulation et refus avec paiement ou statut incompatible. Tests Go/PostgreSQL réussis. | Terminé |
| CMS-05 | P2 | Préparer et valider le lancement du CMS sur une machine locale/serveur autonome, sans dépendance obligatoire à Docker ni à un hébergeur particulier. Dépend de CMS-01 à CMS-04 ; IONOS est une option ultérieure, pas un prérequis. | Procédure portable Windows/Linux documentée ; PostgreSQL fourni comme service local ou distant via `DATABASE_URL` ; binaire Go et assets embarqués démarrent, restent limités au réseau configuré, conservent les données et disposent d’une procédure de sauvegarde/restauration vérifiée. | À faire |

Le périmètre média initial accepte les URI gérées par le propriétaire (par
exemple un CDN) ; l’upload binaire et l’intégration à un fournisseur d’objets
feront l’objet d’une décision et d’une validation de sécurité séparées. La cible
de déploiement intermédiaire est une machine locale ou un serveur autonome ;
Docker peut rester une commodité de développement. IONOS ne sera évalué que si
le besoin est confirmé et n’est pas requis pour terminer le CMS portable.

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