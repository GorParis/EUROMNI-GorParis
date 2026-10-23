# Instructions Copilot pour EUROMNI

Appliquer également les consignes générales de [`AGENTS.md`](../AGENTS.md).
Cette page précise le contexte fonctionnel utile aux changements assistés.

## Repères du dépôt

- [`docs/architecture.md`](../docs/architecture.md) décrit les couches du CMS
  et leurs frontières ; [`docs/api.md`](../docs/api.md) décrit les contrats
  REST/GraphQL.
- [`docs/reprise-prochain-developpeur.md`](../docs/reprise-prochain-developpeur.md)
  et [`docs/plan-directeur-qualite.md`](../docs/plan-directeur-qualite.md)
  décrivent l’état et les preuves, qui doivent être revalidés dans l’arbre de
  travail actuel.
- L’admin et la vitrine sont des assets embarqués dans `frontend/`. Consulter
  `frontend/README.md` et les tests d’assets avant de modifier leur structure ou
  leur comportement.
- Le runner Playwright du pilote est dans
  `scripts/local-pilot/playwright/`. Il vérifie santé, connexion, sections de
  navigation et vitrine ; ne pas le présenter comme une couverture complète de
  toutes les actions d’administration.

## Interface d’administration

- Les sections sont Boutique, Catalogue, Commandes, Livraison, Modules et
  Équipe. Conserver les fragments URL, le retour navigateur et la navigation
  clavier lors de changements aux onglets.
- Conserver les permissions selon les rôles existants et garder les actions
  d’administration limitées à la boutique active.
- Les builds proposés à l’activation sont ceux conservés parce qu’ils sont
  courants ou épinglés par une boutique. Ne pas modifier ce critère pour rendre
  visibles des builds historiques non référencés.
- Mettre à jour ensemble le HTML, le JavaScript, les tests d’assets et la
  documentation lorsque le contrat de navigation change.
