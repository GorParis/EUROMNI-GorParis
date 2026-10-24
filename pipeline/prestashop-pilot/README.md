# Pilote PrestaShop persistant

Instance locale PrestaShop `9.0.3` (interface initiale en anglais) avec
MariaDB `11.8.9`, dédiée aux comparaisons du module officiel `ps_newproducts`
épinglé dans les fixtures.
Les images sont épinglées par digest. Une copie locale du module est contrôlée
par SHA-256 avant montage ; elle est inscriptible car l’image officielle
réapplique les fichiers dans le volume de la boutique au démarrage. Seul le
trafic de l’hôte vers `127.0.0.1:18084` est publié. Le réseau des conteneurs
est interne et sans accès sortant.

L’archive anglaise officielle PrestaShop `9.0.3` est téléchargée côté hôte
une seule fois, vérifiée par son SHA-256 épinglé, puis copiée dans le volume
de la boutique avant l’installation. Cela fournit l’archive absente de l’image
sans ouvrir le réseau aux conteneurs. Le script pose aussi le marqueur vide
`translations/en.gzip` attendu par un contrôle de compatibilité hérité de
l’installateur 9.0.3 ; le chemin de code utilise ensuite l’archive XLF vérifiée.
La copie vérifiée reste dans `.tmp/prestashop-pilot/`.

La base et les fichiers de boutique persistent dans deux volumes Podman
nommés. Les identifiants sont générés localement dans
`.tmp/prestashop-pilot/instance.env` avec des permissions privées ; ils ne sont
pas versionnés. La base est recréée uniquement tant que l’installation initiale
n’a pas abouti. Les produits `euromni-*` sont synthétiques et leurs dates sont
réinitialisées par la commande de test.

```sh
pipeline/prestashop-pilot/manage.sh start
pipeline/prestashop-pilot/manage.sh test
pipeline/prestashop-pilot/manage.sh status
pipeline/prestashop-pilot/manage.sh credentials
pipeline/prestashop-pilot/manage.sh stop
```

Ouvrir `http://127.0.0.1:18084/` pour la vitrine ou
`http://127.0.0.1:18084/admin-dev/` pour l’administration. Le login d’essai
est affiché par `manage.sh credentials`. L’instance n’est pas exposée sur le
réseau local et ne contient aucune donnée réelle.

`start` et `test` vérifient le module natif : fenêtre globale PrestaShop de
30 jours, limite de module par défaut à 8, exclusion d’un produit vieux de
31 jours, résultat configuré limité à deux produits et désactivation lorsque
la fenêtre globale vaut zéro. Ils vérifient aussi le rendu HTTP de la vitrine
avec les réglages par défaut, la limite à deux et la désactivation. Le script
rend accessible à `www-data` le seul répertoire de cache CSS du thème qui doit
être écrit pour ce rendu. Les conteneurs exécutent le PHP PrestaShop et le
module épinglé uniquement dans cette instance de test isolée ; le runtime
EUROMNI ne charge jamais leur source PHP.

`stop` conserve les volumes pour une reprise ultérieure. La suppression
irréversible nécessite une action explicite :

```sh
pipeline/prestashop-pilot/manage.sh destroy --confirm
```

Cette commande ne cible que le projet Podman `euromni-prestashop-pilot`.
