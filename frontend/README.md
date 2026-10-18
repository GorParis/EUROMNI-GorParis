# Front-end boutique

Composants web universels (Custom Elements + Shadow DOM isolé).

Le composant `<euromni-catalog>` charge le catalogue actif depuis l’API REST
`/v1/products`. Il accepte les attributs `src` (URL de l’API) et `locale` (locale
utilisée pour choisir les traductions et formater les prix). Les changements de
ces attributs relancent le chargement ; une requête précédente est annulée.

Chaque carte ouvre une fiche partageable via l’URL `?product=<id>`, chargée
depuis `GET /v1/products/{id}`. Les fiches regroupent les options de variante,
mettent à jour le prix et le stock selon la sélection, et ajoutent au panier la
variante choisie. Les produits avec un SKU unique ne présentent pas de sélecteur.

La page de démonstration est servie sur `/storefront/`. Le composant affiche les
images, descriptions et prix disponibles et fournit un état accessible pendant
le chargement ou en cas d’erreur. Les variantes ont un bouton d’ajout au panier ;
le panier invité est créé à la première utilisation, puis son jeton est conservé
dans `sessionStorage` et envoyé exclusivement dans l’en-tête `Authorization`.
Le panier affiche les articles, les quantités et le total fourni par l’API.

Les attributs `cart-src` (`/v1/cart` par défaut) et `checkout-src`
(`/v1/checkout` par défaut) configurent les points d’entrée du panier et du
checkout. Le checkout crée une commande en attente, enregistre les coordonnées
et l’adresse de livraison, puis décrémente le stock dans la même transaction.
Le sélecteur de pays charge les modes disponibles via `/v1/shipping-options`
(configurable avec `shipping-src`) et affiche les tarifs correspondants. Le
client choisit son transporteur ; le serveur revalide le tarif et la zone puis
enregistre leur snapshot sur la commande avec les frais dans le total. Les
paiements et taxes ne sont pas encore calculés. La configuration initiale ne
contient aucun tarif actif : un membre `owner` ou `admin` doit configurer les
transporteurs, les zones et au moins un prix avant de pouvoir commander.

Le cadrage des marchés distingue le pays de livraison, la langue, la devise,
les taxes, le transport et le paiement. La boutique de démonstration conserve
le français comme langue par défaut ; le code pays ne force pas la langue.
L’extension aux 27 pays de l’UE ne les active pas automatiquement pour la vente
ou la livraison. Le checkout charge les destinations depuis
`/v1/reference/eu-countries`, présélectionne la France et n’affiche que les
transporteurs configurés pour le pays sélectionné. Les devises indicatives et
locales candidates du référentiel ne déclenchent ni conversion ni changement
automatique de langue. Un sélecteur de langue du contenu n’offre que les
locales réellement présentes dans les traductions du catalogue et les mémorise
dans le navigateur ; le français reste le fallback déterministe. Cette
préférence ne déduit ni ne modifie le pays de livraison et ne prétend pas
traduire toute l’interface d’administration ou le texte fixe de la boutique.
Voir [le modèle des marchés et localisations européens](../docs/marches-localisation-europe.md).

Le seed de démonstration présente cinq produits : cabas, ceinture et chemise
avec plusieurs tailles au choix pour les deux derniers, plus une tasse et une
bougie en format unique.

L’administration est disponible sur `/admin/`. Dans le détail d’une commande,
un membre `owner` ou `admin` peut enregistrer un paiement reçu et vérifié hors
du CMS, puis confirmer l’expédition avec un numéro de suivi facultatif. Cette
confirmation manuelle ne débite aucune carte et ne remplace pas l’intégration
d’un prestataire de paiement.

Consomme uniquement les APIs du cœur EUROMNI (REST / GraphQL). Pas de thème PHP.
