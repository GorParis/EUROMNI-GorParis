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
Le paiement, les taxes et la livraison ne sont pas encore calculés ; la vitrine
de démonstration le signale avant confirmation.

Le seed de démonstration présente cinq produits : cabas, ceinture et chemise
avec plusieurs tailles au choix pour les deux derniers, plus une tasse et une
bougie en format unique.

Consomme uniquement les APIs du cœur EUROMNI (REST / GraphQL). Pas de thème PHP.
