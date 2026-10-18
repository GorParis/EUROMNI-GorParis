# Modèle de transport extensible

## Objectif

Le transport doit être un domaine natif du CMS, assez structuré pour exprimer
des offres simples ou des grilles détaillées, et extensible par des modules
métier compilés en Wasm. EUROMNI ne doit ni enfermer ses règles dans le modèle
d’un seul transporteur, ni transformer les exceptions de chaque prestataire en
colonnes ad hoc.

Le modèle couvre quatre préoccupations distinctes :

1. le **service de livraison** présenté au client ;
2. le **calcul du prix** et ses conditions ;
3. la **promesse de délai** et les restrictions applicables ;
4. l’**exécution**, le suivi et le snapshot de l’offre acceptée.

Un transporteur externe réalise la prestation, mais le commerçant configure
l’offre de sa boutique, la présente au client et demeure son interlocuteur
commercial. Le logiciel ne doit pas suggérer qu’un contrat transporteur retire
automatiquement au commerçant ses obligations envers le client. Les mentions
et responsabilités précises dépendent des contrats et du droit applicable ;
elles ne sont pas déterminées par le schéma technique.

## État actuel

Le modèle offre désormais :

- des transporteurs identifiés par code ;
- des zones de pays sans chevauchement ;
- un tarif fixe ou une matrice de tarifs par couple transporteur-zone, dans la
  devise de la boutique ;
- un poids unitaire facultatif en grammes par variante ;
- le devis d’un panier par intersection de tranche de poids et de sous-total
  produits ;
- la sélection d’une option au checkout ;
- un snapshot sur la commande du code/nom du transporteur, de la zone et du
  montant, et pour une matrice le poids total des articles, le sous-total
  tarifaire et la règle appliquée ;
- la confirmation administrative d’expédition et un numéro de suivi manuel.

Le premier calculateur conserve le tarif fixe historique et ajoute une matrice
2D. Les bornes basses sont inclusives, les hautes exclusives, et une borne haute
`null` signifie « sans limite ». Deux règles d’un même tarif ne peuvent pas se
chevaucher. Le sous-total est celui des articles, hors taxes et livraison. Le
poids est la somme des poids unitaires multipliés par les quantités ; emballage
et colis ne sont pas estimés. Si un poids manque, ou si aucune règle ne couvre
le panier, aucun devis pondéral n’est proposé : aucun forfait de secours
implicite.

`GET /v1/shipping-options?country=FR` continue d’exposer les tarifs fixes
compatibles sans panier. `POST /v1/shipping-quotes` exige l’autorisation du
panier et un pays `{ "country": "FR" }` pour calculer les tarifs matriciels.
Le checkout recalcule le montant. Pour les matrices, il exige le champ
`expected_shipping_total`; tout décalage avec le devis présenté provoque un
conflit et demande une nouvelle sélection. Le champ reste facultatif pour les
clients historiques utilisant uniquement les forfaits. Les options exposent
également un `quote_id`, empreinte déterministe de l’option complète; le
storefront le renvoie dans `expected_shipping_quote_id`. Si la règle, le libellé,
la zone ou tout autre élément de l’option change, le checkout refuse le devis
même si le montant reste identique. Ce champ est additif et facultatif pour les
clients API historiques; l’empreinte détecte une évolution, elle ne remplace ni
l’autorisation du panier ni une durée d’expiration serveur.

L’admin configure les règles matricielles en JSON sous chaque tarif. Exemple :

```json
{
  "mode": "weight_value",
  "rules": [
    {
      "min_weight_grams": 0,
      "max_weight_grams": 1000,
      "min_subtotal": "0",
      "max_subtotal": "50",
      "amount": "5.90"
    },
    {
      "min_weight_grams": 0,
      "max_weight_grams": 1000,
      "min_subtotal": "50",
      "max_subtotal": null,
      "amount": "4.90"
    }
  ]
}
```

La distinction entre transporteur et services sélectionnables, les profils
physiques, les origines, délais, restrictions produit, colis, tarifs retournés
par un service externe et références versionnées de règles restent à
implémenter. Les transporteurs configurés sont encore les options sélectionnées
au checkout ; ne pas déduire un service précis de leur nom.

## Concepts natifs

### Transporteur et service

- Un transporteur représente l’opérateur ou le prestataire configuré par la
  boutique : par exemple La Poste, Colissimo, Chronopost ou DPD. Les noms sont
  des données/configurations, jamais des enums fermés.
- Un transporteur propose un ou plusieurs services sélectionnables, chacun avec
  son code stable dans la boutique, son nom affiché, son état actif et ses
  caractéristiques opérationnelles.
- Exemples de distinction : livraison à domicile standard, express, point
  relais. « Transporteur » et « service acheté » ne doivent pas être confondus.
- Les capacités d’un service (domicile, relais, remise contre signature,
  assurance, suivi) doivent être explicites ; une intégration ne les déduit pas
  du nom commercial.

### Origine et profil physique

- Une origine d’expédition décrit l’adresse et le pays de départ d’un stock ou
  d’un entrepôt. Le modèle doit pouvoir en référencer plusieurs par boutique,
  même si le premier incrément n’en utilise qu’une.
- Un produit/une variante expédiable peut porter un poids entier en grammes,
  des dimensions entières en millimètres et un profil de colisage ou de
  restriction.
- Les caractéristiques de colisage doivent pouvoir signaler les marchandises
  hors gabarit, non empilables, fragiles, dangereuses ou incompatibles avec
  certains services, sans faire croire que tous les transporteurs utilisent
  les mêmes limites.
- Une quantité commandée, plusieurs origines ou des articles incompatibles
  peuvent imposer plusieurs colis. La politique de regroupement et
  d’allocation aux colis devra être définie explicitement ; ne pas supposer
  qu’un poids total suffit à calculer un emballage réel.

### Règles et calcul de prix

Une règle de tarif appartient à une boutique, un service, une destination ou
zone et une devise. Elle possède un identifiant/version, une période
d’application, un état actif, une priorité non ambiguë et une explication
lisible par l’administration. Le montant doit toujours utiliser une précision
décimale, jamais un flottant.

Le cœur doit prévoir au minimum les familles natives suivantes :

- forfait par service et destination ;
- matrice poids × sous-total, avec bornes et comportement aux limites
  déterministes (première tranche native implémentée) ;
- tranches de valeur de panier, combinées à une tranche de poids ;
- montant par unité ou colis, si la boutique le configure explicitement ;
- combinaison déclarée de conditions prises en charge par le moteur.

Une condition non couverte n’est pas évaluée approximativement. Le calcul
renvoie une erreur explicite, ou aucune offre si cette absence est une issue
métier attendue. La priorité, le cumul, le choix de la meilleure règle, les
minimums/maximums et les éventuels frais par colis doivent être configurables
et documentés, jamais implicites.

Le modèle doit permettre d’ajouter ultérieurement des devis en direct via une
API transporteur. Leur indisponibilité ou expiration ne doit pas se transformer
silencieusement en tarif fixe de secours, sauf si le commerçant a configuré ce
fallback et que le checkout l’indique clairement.

### Délai et restrictions

Une offre sépare :

- le temps de préparation avant remise au transporteur ;
- l’estimation de transit fournie par une règle ou une intégration ;
- la fenêtre annoncée au client, avec unité, fuseau/calendrier si nécessaire,
  et source de cette estimation ;
- les restrictions de destination, de produit, de poids, de dimensions et de
  service.

Une estimation ne doit pas être présentée comme une garantie contractuelle si
la source ne la garantit pas. Conserver l’estimation et sa source au moment de
la commande pour expliquer ultérieurement l’offre présentée.

## Contrat de devis et extensions Wasm

Le cœur orchestre un calcul de devis avec des entrées métier explicites :

- boutique et origine d’expédition sélectionnées ;
- destination normalisée ;
- devise et politique de prix applicables ;
- lignes de panier (identifiant de variante, quantité, poids/dimensions et
  attributs d’expédition disponibles) ;
- colis ou hypothèses de colisage, avec version d’algorithme ;
- services, règles natives et contexte de calcul borné.

Le résultat est une liste de devis validés par le cœur. Chaque offre doit
identifier au minimum le service, le montant et sa devise, le détail des
composants tarifaires, une estimation de délai si disponible, ses restrictions,
sa durée de validité, la source de calcul et une référence/version stable.
Toutes les sorties sont vérifiées : champs requis, unités, montants, devise,
identifiants connus, limites de taille et expiration.

Un module métier Wasm peut contribuer au devis par une capability versionnée,
par exemple `shipping.quote`. Le cœur lui transmet un contrat typé et borné,
puis valide sa réponse avant de la présenter ou de l’enregistrer. Le module :

- ne reçoit ni connexion SQL ni accès direct au filesystem, réseau ou secrets ;
- ne peut sélectionner que les capacités accordées lors de son installation ;
- ne peut pas fournir un montant négatif, une devise inattendue ou un service
  non configuré ;
- doit s’identifier avec son module, sa version/build et la version du contrat ;
- ne devient pas transporteur ou source de vérité à lui seul : le cœur décide
  quelles offres sont admissibles et écrit le snapshot.

Si un adaptateur hôte communique avec l’API d’un transporteur, les identifiants
et appels réseau restent dans le processus hôte, sous configuration explicite,
allowlist, délais, quotas, journalisation et traitement d’erreurs. Une réponse
du module ou du prestataire externe est une donnée non fiable. Une mise à jour
du module ne doit pas modifier le calcul historique d’une commande.

## Snapshot et cycle de vie de commande

Au checkout, l’offre sélectionnée est validée une dernière fois : service actif,
destination desservie, prix/devise conformes, devis non expiré et restrictions
satisfaites. Si l’offre n’est plus valide, le checkout échoue avec un conflit
explicite et demande au client de choisir à nouveau ; il ne substitue pas un
autre service ou tarif sans consentement.

La commande conserve un snapshot immuable suffisant pour reproduire et
expliquer l’offre acceptée :

- transporteur et service (codes et noms affichés) ;
- origine pertinente, zone/destination et adresse expédiée ;
- montant total, devise et détail de calcul ;
- poids, dimensions, quantités, colis ou hypothèses utilisés ;
- délai présenté, source et nature estimative/garantie ;
- identifiant et version de la règle, ou module/build Wasm et contrat de devis ;
- horodatage, expiration connue et référence du devis transporteur, s’il y a
  lieu.

Le snapshot de devis ne vaut pas preuve d’achat d’une étiquette ni confirmation
de prise en charge. Ces étapes opérationnelles sont distinctes : préparation,
étiquette créée, remis au transporteur, en transit, livré, incident, retour.
Le suivi manuel déjà présent reste valable jusqu’à l’ajout éventuel d’une
intégration de suivi.

## Évolution du schéma

Les concepts communs consultés et filtrés souvent (transporteurs, services,
origines, profils, versions de règle, zones, tarifs et snapshots de commande)
doivent être normalisés et indexables. Des paramètres spécifiques à une famille
de règle ou à un module peuvent être conservés dans un JSON versionné si leur
schéma est validé par le cœur. Ne pas faire d’un document JSON arbitraire
l’unique stockage de prix, statut, devise ou version.

Évolution compatible à prévoir :

1. conserver les tables actuelles, les forfaits et snapshots historiques ;
2. ajouter les poids facultatifs, grilles poids × sous-total et snapshots
  additifs (`0010_shipping_weight_value_rates.sql`, migration MariaDB
  `0003_shipping_weight_value_rates.sql` et contrainte de mode
  `0004_shipping_rate_mode_check.sql`) ;
3. séparer transporteurs et services sélectionnables, sans rendre obligatoires
   des données que la boutique démo ne possède pas encore ;
4. ajouter les profils physiques et origines, puis définir une politique
   explicite de colisage ;
5. ajouter délais, restrictions, références versionnées, extensions Wasm et
   devis externes ;
6. maintenir les migrations PostgreSQL et MariaDB cohérentes et distinctes ;
   ne déclarer MariaDB validé qu’après exécution réelle de sa suite dédiée.

Les devises et pays viennent des référentiels associés, mais leur présence
n’active ni conversion, ni marché, ni tarif, ni obligation fiscale. Le calcul du
transport ne doit pas inclure ni estimer la TVA ; la fiscalité est un domaine
distinct dont les montants peuvent modifier le total de commande.

## Critères de validation avant intégration transporteur

- calcul déterministe des forfaits, tranches et limites de poids/valeur ;
- égalité des résultats aux frontières de tranches, précision et devise ;
- destination desservie/non desservie, service inactif et restriction produit ;
- devis absent, invalide, expiré, échec API, timeout et fallback explicitement
  configuré ;
- checkout concurrent et snapshot inchangé après modification de tarifs ou
  version du module ;
- refus des sorties Wasm malformées et validation des capabilities/imports ;
- migrations et flux de devis testés sur PostgreSQL et MariaDB ; ne déclarer le
  second validé qu’après exécution réelle de sa suite d’intégration dédiée.

## Référence de grille externe

La grille Selfnamed mentionnée durant le cadrage est une inspiration de
présentation par destination, coût et délai, pas un barème EUROMNI à recopier :
[Selfnamed — temps et coûts d’expédition](https://help.selfnamed.com/en/articles/10009962-what-is-selfnamed-s-shipping-time-and-costs).
Chaque boutique reste responsable de saisir ou d’importer ses propres tarifs et
conditions contractuels.
