# Contrat API

L’API REST et GraphQL partagent le même store Go. Les erreurs internes ne sont jamais renvoyées avec leur détail au client.

## REST

- L’API d’administration utilise `POST /v1/admin/login` avec `{ "email": "…", "password": "…", "shop_id": "…" }`. Le `shop_id` peut être omis pour la boutique de démonstration. Le jeton renvoyé s’utilise avec `Authorization: Bearer <token>`. L’authentification staff exige PostgreSQL et un utilisateur déjà associé à la boutique dans `shop_members` ; aucun compte ni mot de passe de démonstration n’est créé.
- `GET /v1/admin/me`, `GET /v1/admin/products` et `GET /v1/admin/categories` exigent une session staff. L’admin voit également les produits brouillons et archivés. `owner`, `admin` et `editor` peuvent modifier le catalogue ; `support` est en lecture seule.
- `POST /v1/admin/products` crée un produit à partir de `{ "slug": "…", "status": "draft", "translations": [{ "locale": "fr-FR", "name": "…", "description": "…" }] }`. `PUT /v1/admin/products/{id}` remplace le slug, le statut et l’ensemble des traductions. Les statuts acceptés sont `draft`, `active` et `archived`. `DELETE /v1/admin/products/{id}` archive le produit sans supprimer ses commandes ni ses variantes.
- `POST /v1/admin/categories` crée une catégorie avec un slug et des traductions. `PUT /v1/admin/categories/{id}` remplace le slug et les traductions. `DELETE /v1/admin/categories/{id}` supprime la catégorie et ses liens de produits.
- Les slugs sont en minuscules, avec lettres/chiffres séparés par un tiret simple. Les traductions doivent avoir des locales uniques et un nom non vide. Les requêtes admin acceptent un corps JSON de 1 Mio maximum. Un slug déjà utilisé renvoie `409 Conflict`, une ressource absente `404 Not Found`, et un rôle `support` qui tente une écriture `403 Forbidden`.
- `GET /v1/products` liste tous les produits **actifs** de la boutique, triés par slug puis id. `GET /v1/products?category={id}` filtre par appartenance à cette catégorie. `GET /v1/products/{id}` lit un produit actif.
- `GET /v1/categories` et `GET /v1/categories/{id}` exposent les catégories et leurs traductions.
- `POST /v1/cart` crée un panier invité et renvoie une seule fois un jeton Bearer aléatoire de 256 bits.
- `GET /v1/cart` lit le panier authentifié.
- `POST /v1/cart/add` accepte `{ "variant_id": "…", "quantity": 1 }`.
- `POST /v1/checkout` accepte l’e-mail, le nom et l’adresse d’expédition du client ; il crée une commande en attente et consomme le stock du panier de façon transactionnelle.

Les deux routes de lecture/modification et la création de panier exigent ou renvoient un jeton Bearer selon l’opération. Le jeton est un secret : le serveur ne stocke que son SHA-256, il expire après 30 jours, et les réponses panier utilisent `Cache-Control: no-store`. Le client doit transmettre le jeton dans `Authorization: Bearer <token>`, jamais dans l’URL. En production, ces routes doivent être servies uniquement sur HTTPS.

La quantité d’une ligne doit être comprise entre 1 et 99. Une quantité supérieure au stock suivi renvoie `409 Conflict`; un jeton absent ou invalide renvoie `401 Unauthorized`. Les montants sont calculés sans flottants binaires.

Le checkout exige un Bearer valide, un panier non vide et des coordonnées valides. Il renvoie `201 Created` avec l’instantané de commande ; panier vide ou stock devenu insuffisant renvoie `409 Conflict`. Le stock et les lignes du panier sont mis à jour dans la transaction de commande. Pour cette première tranche, la commande reste `pending`, sans paiement, taxes ni frais de livraison calculés ; le total correspond donc au sous-total.

La migration `0003_cart_access_tokens.sql` ajoute le hash et l’expiration. Les anciens paniers sans hash ne sont pas accessibles par jeton.

## GraphQL

`POST /graphql` accepte un JSON avec `query`, `operationName` et `variables`. Les requêtes peuvent aussi utiliser `GET`; les mutations via `GET` sont rejetées. Le corps POST est limité à 1 Mio. Les réponses GraphQL sont marquées `Cache-Control: no-store`.

Requêtes disponibles :

- `products` : liste des produits actifs (même payload que REST).
- `product(id)` avec statut, traductions, médias, variantes, options, prix et inventaire.
- `categories` et `category(id)` avec leurs traductions.
- `cart`, qui requiert le même Bearer que REST.

Mutations disponibles uniquement en POST :

- `createCart`, qui renvoie un jeton Bearer à conserver côté client.
- `addCartLine(variantId, quantity)`, qui requiert le Bearer dans l’en-tête `Authorization`.

Les options de variante sont renvoyées sous forme de liste `{ name, value }`, triée par nom pour une sortie déterministe. Le checkout REST est disponible ; le checkout GraphQL, les taxes, les livraisons et les paiements ne le sont pas encore.

La boutique de démo (`GET /storefront/`) sert le Web Component `euromni-catalog`, isolé en Shadow DOM. Il consomme `GET /v1/products` et ouvre les fiches via `?product=<id>` en appelant `GET /v1/products/{id}`. Les produits à variantes proposent un sélecteur de taille et ajoutent la variante choisie ; les produits mono-SKU n’affichent pas d’option. Le panier invité est créé à la première action d’ajout et appelle `GET /v1/cart` / `POST /v1/cart/add` avec le jeton conservé en `sessionStorage` et transmis uniquement dans `Authorization`. Le formulaire de checkout envoie les coordonnées à `POST /v1/checkout` ; le paiement, les taxes et la livraison restent hors périmètre.

La migration `0006_seed_representative_catalog.sql` élargit les données de démonstration à cinq produits avec un mélange de variantes multiples et de produits mono-SKU.
