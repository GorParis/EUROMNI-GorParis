# Marchés européens, langues, devises, taxes et livraison

## Décision de conception

EUROMNI doit pouvoir préparer la vente dans les **27 États membres de l’Union
européenne**, sans confondre le pays du marché avec la langue de l’interface, la
devise, la fiscalité ou les transporteurs.

Le français (`fr-FR`) reste la langue par défaut de l’interface. Il ne doit pas
être remplacé automatiquement par une langue déduite du pays de livraison. Une
boutique pourra proposer plusieurs langues et un client pourra choisir une
langue indépendamment de son pays.

Les codes pays utilisés par le checkout sont ISO 3166-1 alpha-2. Le périmètre
initial à prendre en charge est :

| Pays | Code | Pays | Code | Pays | Code |
|---|---|---|---|---|---|
| Allemagne | DE | Autriche | AT | Belgique | BE |
| Bulgarie | BG | Chypre | CY | Croatie | HR |
| Danemark | DK | Espagne | ES | Estonie | EE |
| Finlande | FI | France | FR | Grèce | GR |
| Hongrie | HU | Irlande | IE | Italie | IT |
| Lettonie | LV | Lituanie | LT | Luxembourg | LU |
| Malte | MT | Pays-Bas | NL | Pologne | PL |
| Portugal | PT | Roumanie | RO | Slovaquie | SK |
| Slovénie | SI | Suède | SE | Tchéquie | CZ |

Cette liste définit une **capacité de configuration**, pas une activation
commerciale : aucun pays ne devient automatiquement livrable, taxable ou
disponible au paiement du seul fait qu’il figure dans la liste. L’appartenance
à l’UE et les codes pays devront être révisables à partir de sources
institutionnelles et normalisées.

## Séparer les dimensions

- **Marché et destination** : le pays de livraison identifie le marché pertinent
  pour les règles de vente et la destination. Il ne détermine pas à lui seul la
  langue affichée.
- **Langue et locale** : les locales servent au choix des traductions et au
  formatage des nombres, dates et montants. Plusieurs langues peuvent être
  proposées dans un pays ; une langue peut servir plusieurs pays. La valeur
  initiale demeure `fr-FR`, avec une préférence explicite du client si elle
  existe.
- **Devise** : une devise est associée à un contexte de vente ou de prix, avec
  un code normalisé. Le fait de formater ou d’afficher un montant dans une
  devise ne réalise pas une conversion. La conversion, ses taux, son arrondi,
  son moment de fixation et le règlement par le prestataire doivent faire
  l’objet d’une décision distincte.
- **Taxes** : le moteur fiscal doit utiliser des règles par juridiction et
  catégorie fiscale de produit, avec période de validité et provenance des
  données. Le pays de destination seul ne suffit pas à calculer une taxe
  exacte. Les situations transfrontalières, le type de client et de vente, les
  exemptions et les régimes applicables doivent être modélisés avant de
  produire un total présenté comme dû.
- **Livraison** : les pays desservis, transporteurs, services, restrictions,
  zones et tarifs restent une configuration indépendante. Ajouter un pays à
  la liste des marchés ne l’active pas chez un transporteur et ne crée aucun
  tarif.
- **Paiement** : les méthodes et devises acceptées dépendent de l’intégration
  et de sa configuration marchande. Elles ne sont pas déduites de la langue ou
  de la seule appartenance à un marché. Viva.com en environnement de test est
  une piste retenue ; son contrat d’API et la vérification serveur de ses
  notifications devront être validés avant implémentation.

## Modèle fonctionnel visé

Un marché de boutique pourra référencer un pays et définir sa disponibilité,
ses langues proposées, sa devise de prix et ses configurations de taxe et de
livraison. Ces références doivent pointer vers des données séparées et
versionnées, et non entasser leurs règles dans une table de localisations.

Les prix du catalogue restent explicites par devise. Un marché configuré en
EUR ne transforme pas automatiquement un prix qui n’existe que dans une autre
devise. Les totaux et snapshots de commande doivent conserver la devise, les
montants et les éléments de calcul effectivement retenus lors du checkout ; une
modification ultérieure des règles ne doit pas réécrire les anciennes
commandes.

## État actuel et étapes proposées

Le checkout interroge désormais `GET /v1/reference/eu-countries` pour présenter
les 27 destinations UE dans un sélecteur français et présélectionne la France.
Le référentiel comprend code, nom, devise indicative et locales candidates ;
ces locales sont des pistes de formatage/contenu, pas la preuve que la boutique
est traduite dans ces langues. Le français reste la locale par défaut. Le
checkout demande ensuite les options d’expédition configurées pour le pays et
fige transporteur, zone et tarif dans la commande. La configuration de transport
fonctionne par zones de pays et tarifs fixes.

Les pays du référentiel ne sont pas pour autant des marchés activés. Une
destination ne devient livrable que si un transporteur et un tarif actifs la
couvrent. Le sélecteur de langue du contenu propose les locales trouvées dans
les traductions du catalogue et des catégories, mémorise la préférence dans le
navigateur et reste indépendant du pays de livraison. `fr-FR` demeure le défaut
et le fallback déterministe. Le texte fixe de l’interface n’est pas encore
traduit ; la configuration générale des locales de boutique et leur gestion
dans l’administration restent à faire. La conversion de devises et les taxes
au checkout ne sont pas encore calculées.

Ordre de réalisation proposé :

1. **Réalisé** — Ajouter le référentiel de référence des 27 pays, avec devises
   indicatives et locales candidates, sans activer par défaut ventes, transport
   ou taxes. La source API est `GET /v1/reference/eu-countries`.
2. **Partiellement réalisé** — Le sélecteur de langue du contenu propose les
   locales trouvées dans les traductions du catalogue et des catégories,
   mémorise la préférence dans le navigateur et reste indépendant du pays de
   livraison. `fr-FR` demeure le défaut et le fallback déterministe. Le texte
   fixe de l’interface n’est pas encore traduit ; la configuration générale
   des locales de boutique et leur gestion dans l’administration restent à
   faire.
3. Définir la politique de prix et de devise de la boutique ; ne pas présenter
   une conversion comme un prix réellement disponible sans règle explicite.
4. Ajouter un moteur de taxes fondé sur des données officielles révisables,
   datées et reliées aux catégories fiscales des produits ; couvrir ses cas
   de calcul et conserver les snapshots de commande.
5. Configurer et valider séparément les destinations, tarifs et services de
   livraison pour chaque transporteur.
6. Tester le prestataire de paiement choisi avec les devises et pays réellement
   activés, en confirmant le paiement à partir d’une preuve serveur vérifiée,
   jamais sur une simple redirection navigateur.

L’activation d’un nouveau marché devra être explicite et vérifiable : prix
disponibles, traitement fiscal défini, livraison configurée, moyen de paiement
compatible, contenu et parcours client testés.

## Sources de référence

- [Pays de l’Union européenne](https://european-union.europa.eu/principles-countries-history/eu-countries_en) — périmètre des États membres.
- [Langues de l’Union européenne](https://european-union.europa.eu/principles-countries-history/languages_en) — l’UE compte 24 langues officielles ; cela ne correspond pas à une langue unique par pays.
- [Taux de TVA dans l’UE](https://taxation-customs.ec.europa.eu/taxation/vat/vat-directive/vat-rates_en) — cadre commun, taux et catégories déterminés par les États membres ; les taux standards, réduits et dérogations existent.
- [ISO Online Browsing Platform](https://www.iso.org/obp/ui/) — référence pour vérifier les codes de pays.

Les pages et tables fiscales doivent être revérifiées au moment de l’intégration.
Ce document fixe le modèle fonctionnel ; il ne constitue pas un avis juridique
et ne publie pas de taux fiscaux.
