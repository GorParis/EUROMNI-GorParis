# Plan directeur qualité et actions

| Champ | Valeur |
|---|---|
| Identifiant | EUROMNI-QP-001 |
| Version | 1.2 |
| État | Brouillon contrôlé, approbation humaine à renseigner |
| Date de révision | 2026-10-12 |
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

Au 2026-10-12, le runtime Wasmtime, l’ABI hôte, le registre de modules installés
et le stockage local d’artefacts par SHA-256 existent. Le stockage est streamé,
borné à 64 Mio et vérifie l’intégrité à la lecture. La chaîne d’analyse PHP, les
adaptateurs, l’IR, le traducteur, l’endpoint d’upload et la publication
orchestrée ne sont pas implémentés. Le choix de toolchain Go/Wasm compatible
avec l’import custom `euromni.call` reste un point bloquant à lever avant de
générer des modules réels. Cette vérification doit porter sur les propriétés
de l’artefact et la matrice d’exécution cible, pas sur les compilateurs présents
sur le poste de développement.

Les changements de registre/runtime et d’artefacts sont en cours sur `main` et
doivent être validés puis commités séparément ; se fier à `git status` avant de
reprendre. Le commit de base de cette tranche est `8e9f18e`.

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
| QP-10 | P0 | Définir la matrice de portabilité du cœur et du runtime Wasmtime, distincte de la toolchain invitée. | OS/architectures de production décidés et testés ; dépendances natives/CGo, PostgreSQL, stockage et procédure de déploiement documentés. Une cible non testée n’est pas déclarée supportée. | Matrice versionnée et CI/tests de déploiement reproductibles. | À faire |

Ordre de reprise recommandé : traiter **QP-01 et QP-10 ensemble**. Le poste de
développement n’est pas la cible d’architecture ; un worker de build épinglé
est acceptable s’il produit un artefact portable et reproductible, sans rendre
Docker nécessaire en production. Continuer ensuite QP-02/03, QP-04/05,
QP-06/07, puis QP-08/09. Ne pas commencer la traduction générique avant d’avoir
validé l’IR avec les deux plateformes pilotes.

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

## Historique des révisions

| Version | Date | Modification | Approbation |
|---|---|---|---|
| 1.0 | 2026-10-12 | Création du plan directeur initial pour le pipeline PHP→Wasm et la maîtrise des artefacts. | En attente du responsable produit/qualité |
| 1.1 | 2026-10-12 | Résultat du spike Go WASI consigné ; QP-01 bloqué en attente d’un invité sans imports WASI. | En attente du responsable produit/qualité |
| 1.2 | 2026-10-12 | Clarification de l’exigence de portabilité et ajout d’une matrice d’hôtes runtime distincte du poste de build. | En attente du responsable produit/qualité |