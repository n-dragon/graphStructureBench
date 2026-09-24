# Résultats de campagne

Machine : 4 cœurs, Go 1.24.7. Campagne du 2026-09-24, comparée à celle du 2026-09-22 ; rapport engendré au commit `6c47401`.

Ce rapport est **engendré** par `scripts/make_results.py` à partir des CSV produits par `scripts/run-bench.sh` : les chiffres cités dans le texte sont extraits des mesures, pas recopiés. Les mesures brutes sont versionnées dans `campaigns/2026-09-24/` et `campaigns/2026-09-22/`.

## En bref

| graphe           |                                          parcours le plus rapide |                                        test d'arête le plus rapide |     index le plus compact |
| ---------------- | ---------------------------------------------------------------: | -----------------------------------------------------------------: | ------------------------: |
| er (n=1000000)   |           hybrid ≈ csr ≈ adjlist-exact ≈ csr-slices (30.6 req/s) |           csr ≈ csr-slices ≈ hybrid ≈ adjlist-exact (52.9 M req/s) | varint-csr (3.57 o/arête) |
| rmat (n=1048576) | csr ≈ hybrid ≈ adjlist ≈ adjlist-exact ≈ csr-slices (64.4 req/s) | csr ≈ csr-slices ≈ adjlist-exact ≈ adjlist ≈ hybrid (35.4 M req/s) | varint-csr (2.39 o/arête) |
| grid (n=1000000) |                                       hybrid ≈ csr (171.9 req/s) | csr-slices ≈ csr ≈ adjlist-exact ≈ hybrid ≈ adjlist (82.4 M req/s) | varint-csr (3.50 o/arête) |

« ≈ » relie les structures à moins de 17 % de la meilleure : c'est l'écart que deux campagnes identiques produisent sur cette machine (§ 7), elles ne sont donc pas départageables.


**Ce qu'on retient.** **Le choix par défaut est `csr`.** Il fait partie du groupe de tête sur toutes les charges et topologies, à égalité avec `hybrid` — l'écart de vitesse entre eux reste sous le seuil d'ex æquo. Ce qui le distingue est la mémoire : c'est le plus compact du groupe (`csr` 4.25, `hybrid` 4.51 o/arête). `adjmap` est l'index le plus lourd (10.92 o/arête, 2.6 fois le CSR) et se classe en médiane 6e sur 9 en débit : aucun critère ne le justifie ici. `varint-csr` est la réponse quand la mémoire prime : 16 % de moins que le CSR sur `er`, pour un débit de lecture d'adjacence de 30 % du sien.

## 1. Mémoire

Les chiffres ci-dessous sont des deltas de tas mesurés après GC forcé, pas des estimations — la colonne « analytique » donne le compte déclaré par la structure, pour contrôle.

Sur le graphe `er` à 1 000 000 sommets, le CSR occupe 64.9 Mio, soit 4.25 octets par arête — la borne théorique est de 4 octets pour la cible plus 4 octets par sommet pour les bornes. La structure la plus compacte est `varint-csr` avec 3.57 o/arête (0.84x le CSR).

**Le prix d'`append`.** Les deux variantes de liste d'adjacence ne diffèrent que par leur construction. Sur `er`, la croissance géométrique coûte 7.19 o/arête contre 5.87 pour un dimensionnement exact, soit 22 % de mémoire en trop pour un contenu identique.

**La compression suit la localité.** Le CSR varint encode les écarts entre voisins consécutifs : plus les identifiants voisins sont proches, moins il consomme.

- `er` : 3.57 o/arête contre 4.25 en CSR (16 % de moins)
- `rmat` : 2.39 o/arête contre 4.26 en CSR (44 % de moins)
- `grid` : 3.50 o/arête contre 5.00 en CSR (30 % de moins)

## 2. Parcours

**L'accès par bloc, et sa limite.** Même BFS, même structure, seule change la façon de lire l'adjacence : par callback, ou en remplissant un tampon que la boucle parcourt ensuite sans indirection. Sur le CSR :

- `er` : +39 % de débit (degré moyen 16.0)
- `rmat` : +34 % de débit (degré moyen 15.3)
- `grid` : -13 % de débit (degré moyen 4.0)

Le gain n'est donc pas acquis : il vient de l'appel indirect économisé par voisin, et il faut le payer en recopiant le bloc. Sur `grid`, dont le degré moyen est de 4.0, la copie n'est amortie sur personne et le bloc perd 13 %. C'est le compromis à retenir : lire par bloc gagne sur les sommets de fort degré, perd sur les graphes de faible degré.

En latence, un parcours complet mono-thread sur le CSR prend 182 ms sur `er` (1 000 000 sommets), 71 ms sur `rmat` (1 048 576 sommets), 20 ms sur `grid` (1 000 000 sommets).

## 3. Requêtes ponctuelles

La lecture d'adjacence et le test d'existence d'arête se mesurent par millions de requêtes, réparties sur les threads par un compteur atomique.

Le test d'arête sépare nettement les structures : sur `er`, le CSR (recherche binaire dans une liste courte) tient 52.9 M req/s quand la liste d'arêtes triée (recherche binaire sur les 15 999 866 arêtes) plafonne à 7.8 M req/s — un facteur 6.7.

## 4. Passage à l'échelle

Sur 4 cœurs, le gain médian entre 1 et 4 threads est de x3.79 sur 804 points de mesure. Mais la médiane cache l'essentiel : le gain **dépend de la taille de l'index**.

| sommets   | index csr | gain médian | points au-dessus de x4 |
| --------- | --------: | ----------: | ---------------------: |
| 19 881    |   0.4 Mio |       x3.67 |                 26/108 |
| 20 000    |   1.3 Mio |       x3.61 |                 16/108 |
| 32 768    |   1.9 Mio |       x3.76 |                 27/108 |
| 199 809   |   3.8 Mio |       x3.51 |                  11/80 |
| 200 000   |  13.0 Mio |       x3.77 |                  26/80 |
| 262 144   |  16.0 Mio |       x3.74 |                  16/80 |
| 1 000 000 |  64.9 Mio |       x4.03 |                 87/160 |
| 1 048 576 |  65.4 Mio |       x4.13 |                  50/80 |

**Le gain progresse quand l'index sort du cache.** À 19 881 sommets, l'index de 0.4 Mio tient dans le cache du processeur ; à 1 048 576 sommets, ses 65.4 Mio se lisent en mémoire vive. Une lecture d'adjacence aléatoire y est limitée par la latence mémoire plutôt que par le calcul, et chaque cœur ajouté apporte ses propres défauts de cache en vol (parallélisme mémoire).

| gain médian 1 → 4 threads | en cache | ~200 000 sommets | ~1 000 000 sommets |
| --- | ---: | ---: | ---: |
| cette campagne | x3.69 | x3.69 | x4.06 |
| campagne précédente | x3.72 | x3.84 | x4.46 |

**Un gain supérieur au nombre de cœurs n'est en revanche pas établi.** Hors cache, cette campagne mesure x4.06 et la précédente x4.46 : l'écart est de l'ordre de la variance entre campagnes. La tendance est solide, son amplitude au-delà de x4 ne l'est pas.

S'y ajoute une dispersion réelle : les répétitions d'un même point à 1 thread varient de 14 % en médiane, jusqu'à 139 % au pire. La colonne « dispersion » des tableaux détaillés donne cet écart point par point : **deux structures qui diffèrent de moins que leur dispersion ne sont pas départageables**.

Les index sont immuables après construction : aucune synchronisation n'est nécessaire en lecture, et le seul état par thread est le tampon de parcours.

## 5. Le prix de l'abstraction d'itération

Trois façons de lire l'adjacence d'un sommet tiré au hasard, sur un graphe de 500 000 sommets. `slice directe` n'est disponible que pour les structures qui stockent leurs voisins contigus en mémoire : c'est la borne basse, pas une option universelle.

| structure     | callback (ns) | bloc (ns) | gain du bloc | slice directe (ns) |
| ------------- | ------------: | --------: | -----------: | -----------------: |
| adjmap        |           286 |       170 |         68 % |                 93 |
| adjlist       |           241 |        94 |        155 % |                 54 |
| adjlist-exact |           236 |       102 |        130 % |                 51 |
| csr           |           169 |        90 |         88 % |                 40 |
| csr-slices    |           213 |       102 |        109 % |                 53 |
| edgelist      |           520 |       574 |        -10 % |                n/a |
| varint-csr    |           307 |       299 |          3 % |                n/a |
| hybrid        |           168 |        82 |        105 % |                n/a |

L'écart est considérable : sur le CSR, itérer par callback coûte 4.2 fois le parcours direct de la slice. C'est pourquoi le banc mesure les deux modes plutôt que d'en imposer un — sans quoi il classerait des interfaces d'itération plutôt que des structures de données.

## 6. Mesures détaillées

### er — 20 000 sommets, 319 858 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |        11 ms |  3.0 |    9.80 |    156.7 |  2.29x |        2.7 |
| adjlist       |         4 ms |  2.2 |    7.22 |    115.5 |  1.69x |        2.2 |
| adjlist-exact |         3 ms |  1.8 |    5.89 |     94.2 |  1.38x |        1.7 |
| csr           |         2 ms |  1.3 |    4.28 |     68.4 |  1.00x |        1.3 |
| csr-slices    |         2 ms |  1.7 |    5.53 |     88.5 |  1.29x |        1.7 |
| edgelist      |         2 ms |  2.4 |    8.02 |    128.2 |  1.87x |        2.4 |
| varint-csr    |         5 ms |  0.7 |    2.38 |     38.1 |  0.56x |        0.7 |
| hybrid        |         4 ms |  1.4 |    4.54 |     72.6 |  1.06x |        1.4 |
| bitmatrix     |        29 ms | 47.8 |  156.59 |   2504.3 | 36.61x |       47.8 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch |   dfs | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | ----: | --------: | --------------: | ------: |
| adjmap        | 1.4 k |     1.8 k | 1.2 k |    45.0 M |          58.5 M |  49.9 M |
| adjlist       | 2.2 k |     3.1 k | 1.6 k |    49.4 M |          80.3 M |  77.0 M |
| adjlist-exact | 2.7 k |     3.3 k | 1.8 k |    72.9 M |         101.3 M |  88.7 M |
| csr           | 3.6 k |     4.5 k | 2.2 k |   100.1 M |         124.9 M | 103.7 M |
| csr-slices    | 2.6 k |     3.4 k | 1.9 k |    74.2 M |          96.5 M |  96.4 M |
| edgelist      | 1.0 k |     981.2 | 987.0 |    22.4 M |          22.2 M |  27.9 M |
| varint-csr    | 1.8 k |     1.8 k | 1.4 k |    42.0 M |          45.0 M |  61.1 M |
| hybrid        | 3.2 k |     4.2 k | 2.3 k |    78.1 M |         116.4 M | 103.1 M |
| bitmatrix     | 358.8 |     367.1 | 345.3 |     7.2 M |           7.3 M | 281.2 M |

**Passage à l'échelle — « bfs-batch », lot de 512 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        | 497.8 | 928.5 | 1.8 k | x3.61 |       21 % |
| adjlist       | 900.1 | 1.5 k | 3.1 k | x3.42 |       26 % |
| adjlist-exact | 855.2 | 1.7 k | 3.3 k | x3.86 |       25 % |
| csr           | 1.0 k | 2.1 k | 4.5 k | x4.45 |       31 % |
| csr-slices    | 944.1 | 1.6 k | 3.4 k | x3.56 |       21 % |
| edgelist      | 248.7 | 524.7 | 946.4 | x3.81 |       14 % |
| varint-csr    | 421.0 | 869.0 | 1.8 k | x4.18 |       12 % |
| hybrid        | 1.1 k | 2.2 k | 4.2 k | x3.89 |       18 % |
| bitmatrix     |  92.5 | 185.4 | 365.7 | x3.95 |        9 % |

**Passage à l'échelle — « hasedge », lot de 500 000 requêtes**

| structure     |  1 th. |   2 th. |   4 th. |  gain | dispersion |
| ------------- | -----: | ------: | ------: | ----: | ---------: |
| adjmap        | 14.0 M |  25.8 M |  49.9 M | x3.55 |        7 % |
| adjlist       | 19.4 M |  40.6 M |  77.0 M | x3.97 |       22 % |
| adjlist-exact | 24.0 M |  44.4 M |  88.7 M | x3.70 |       15 % |
| csr           | 26.3 M |  51.5 M | 103.7 M | x3.94 |        7 % |
| csr-slices    | 22.7 M |  45.2 M |  96.4 M | x4.26 |       14 % |
| edgelist      |  6.5 M |  13.4 M |  27.9 M | x4.26 |       21 % |
| varint-csr    | 15.5 M |  30.9 M |  61.1 M | x3.94 |        4 % |
| hybrid        | 24.5 M |  56.8 M | 103.1 M | x4.21 |       25 % |
| bitmatrix     | 76.5 M | 155.6 M | 281.2 M | x3.68 |       13 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |      p50 |   p99 |
| ------------- | -------: | ----: |
| adjmap        |     2 ms |  2 ms |
| adjlist       |     1 ms |  1 ms |
| adjlist-exact |     1 ms |  1 ms |
| csr           | 939.9 µs |  1 ms |
| csr-slices    |     1 ms |  2 ms |
| edgelist      |     3 ms |  4 ms |
| varint-csr    |     2 ms |  3 ms |
| hybrid        | 900.1 µs |  1 ms |
| bitmatrix     |    10 ms | 12 ms |

### er — 200 000 sommets, 3 199 860 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |       118 ms | 27.4 |    8.97 |    143.5 |  2.11x |       26.5 |
| adjlist       |        33 ms | 21.9 |    7.19 |    115.1 |  1.69x |       21.9 |
| adjlist-exact |        29 ms | 17.9 |    5.87 |     94.0 |  1.38x |       16.8 |
| csr           |        29 ms | 13.0 |    4.25 |     68.0 |  1.00x |       13.0 |
| csr-slices    |        32 ms | 16.8 |    5.50 |     88.0 |  1.29x |       16.8 |
| edgelist      |        17 ms | 24.4 |    8.00 |    128.0 |  1.88x |       24.4 |
| varint-csr    |        58 ms |  8.9 |    2.91 |     46.5 |  0.68x |        8.9 |
| hybrid        |        46 ms | 13.8 |    4.51 |     72.2 |  1.06x |       13.8 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | --------: | --------------: | ------: |
| adjmap        |  80.9 |     101.7 |    24.9 M |          29.9 M |  36.4 M |
| adjlist       | 116.7 |     170.3 |    26.2 M |          58.2 M |  52.6 M |
| adjlist-exact | 108.4 |     183.5 |    24.9 M |          62.6 M |  58.9 M |
| csr           | 142.1 |     216.4 |    32.3 M |          76.6 M |  59.9 M |
| csr-slices    | 111.5 |     172.0 |    25.6 M |          60.6 M |  51.9 M |
| edgelist      |  46.0 |      45.8 |     9.2 M |           9.8 M |  13.0 M |
| varint-csr    |  86.4 |      82.7 |    20.3 M |          20.7 M |  27.1 M |
| hybrid        | 140.6 |     195.6 |    35.6 M |          73.2 M |  57.5 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |  23.4 |  47.7 | 101.7 | x4.34 |       17 % |
| adjlist       |  42.6 |  86.3 | 166.1 | x3.90 |       11 % |
| adjlist-exact |  43.1 |  89.2 | 183.5 | x4.25 |       25 % |
| csr           |  59.4 | 103.5 | 216.4 | x3.64 |       21 % |
| csr-slices    |  47.0 |  95.2 | 172.0 | x3.66 |       16 % |
| edgelist      |  12.1 |  23.3 |  45.8 | x3.80 |       10 % |
| varint-csr    |  23.3 |  43.6 |  82.7 | x3.55 |       18 % |
| hybrid        |  60.6 | 119.0 | 195.6 | x3.23 |       17 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | -----: | -----: | -----: | ----: | ---------: |
| adjmap        |  8.4 M | 16.8 M | 36.4 M | x4.31 |       32 % |
| adjlist       | 13.1 M | 25.5 M | 52.6 M | x4.03 |       27 % |
| adjlist-exact | 16.7 M | 30.5 M | 58.9 M | x3.52 |       35 % |
| csr           | 16.8 M | 33.6 M | 54.8 M | x3.26 |       17 % |
| csr-slices    | 15.3 M | 30.1 M | 51.9 M | x3.40 |       43 % |
| edgelist      |  3.1 M |  6.2 M | 13.0 M | x4.17 |       13 % |
| varint-csr    |  6.4 M | 13.5 M | 27.1 M | x4.25 |       15 % |
| hybrid        | 15.6 M | 29.9 M | 57.5 M | x3.69 |       11 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |   p50 |   p99 |
| ------------- | ----: | ----: |
| adjmap        | 42 ms | 44 ms |
| adjlist       | 24 ms | 26 ms |
| adjlist-exact | 20 ms | 20 ms |
| csr           | 16 ms | 17 ms |
| csr-slices    | 20 ms | 24 ms |
| edgelist      | 82 ms | 96 ms |
| varint-csr    | 42 ms | 47 ms |
| hybrid        | 16 ms | 18 ms |

### er — 1 000 000 sommets, 15 999 866 arêtes

**Mémoire et construction**

| structure     | construction |   Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ----: | ------: | -------: | -----: | ---------: |
| adjmap        |       934 ms | 166.7 |   10.92 |    174.8 |  2.57x |      132.6 |
| adjlist       |       192 ms | 109.8 |    7.19 |    115.1 |  1.69x |      109.7 |
| adjlist-exact |       140 ms |  89.6 |    5.87 |     94.0 |  1.38x |       83.9 |
| csr           |       146 ms |  64.9 |    4.25 |     68.0 |  1.00x |       64.8 |
| csr-slices    |       131 ms |  83.9 |    5.50 |     88.0 |  1.29x |       83.9 |
| edgelist      |        71 ms | 122.1 |    8.00 |    128.0 |  1.88x |      122.1 |
| varint-csr    |       427 ms |  54.5 |    3.57 |     57.1 |  0.84x |       54.5 |
| hybrid        |       212 ms |  68.8 |    4.51 |     72.1 |  1.06x |       68.8 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |  bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ---: | --------: | --------: | --------------: | ------: |
| adjmap        |  9.8 |      11.6 |    14.1 M |          22.1 M |  23.5 M |
| adjlist       | 15.7 |      17.7 |    15.9 M |          42.8 M |  40.8 M |
| adjlist-exact | 17.2 |      27.4 |    19.7 M |          50.0 M |  46.2 M |
| csr           | 21.0 |      29.0 |    22.8 M |          51.9 M |  52.9 M |
| csr-slices    | 18.9 |      27.2 |    19.5 M |          53.9 M |  49.2 M |
| edgelist      |  6.1 |       5.8 |     6.6 M |           6.3 M |   7.8 M |
| varint-csr    | 12.2 |      13.8 |    15.8 M |          15.5 M |  20.1 M |
| hybrid        | 22.1 |      30.6 |    22.7 M |          51.8 M |  47.5 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |   2.5 |   5.5 |  11.6 | x4.67 |       20 % |
| adjlist       |   3.9 |   8.2 |  17.7 | x4.50 |       13 % |
| adjlist-exact |   4.3 |  11.1 |  27.4 | x6.31 |       24 % |
| csr           |   5.2 |  13.2 |  29.0 | x5.55 |       18 % |
| csr-slices    |   4.8 |  11.5 |  27.2 | x5.61 |       27 % |
| edgelist      |   1.4 |   2.8 |   5.5 | x4.10 |        2 % |
| varint-csr    |   3.1 |   6.5 |  13.8 | x4.45 |       14 % |
| hybrid        |   7.0 |  15.3 |  30.6 | x4.40 |       30 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | -----: | -----: | -----: | ----: | ---------: |
| adjmap        |  4.4 M |  9.3 M | 23.5 M | x5.33 |       29 % |
| adjlist       |  9.5 M | 17.7 M | 40.5 M | x4.26 |       28 % |
| adjlist-exact | 11.5 M | 25.4 M | 46.2 M | x4.02 |       26 % |
| csr           | 13.0 M | 27.3 M | 52.9 M | x4.08 |       34 % |
| csr-slices    |  9.8 M | 22.0 M | 49.2 M | x5.03 |       20 % |
| edgelist      |  2.0 M |  4.3 M |  7.8 M | x4.01 |       16 % |
| varint-csr    |  4.5 M |  9.8 M | 20.1 M | x4.49 |       33 % |
| hybrid        | 10.9 M | 23.9 M | 47.5 M | x4.35 |       51 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |    p50 |    p99 |
| ------------- | -----: | -----: |
| adjmap        | 350 ms | 355 ms |
| adjlist       | 243 ms | 259 ms |
| adjlist-exact | 233 ms | 249 ms |
| csr           | 182 ms | 182 ms |
| csr-slices    | 210 ms | 222 ms |
| edgelist      | 624 ms | 704 ms |
| varint-csr    | 335 ms | 385 ms |
| hybrid        | 139 ms | 143 ms |

### rmat — 32 768 sommets, 467 546 arêtes

**Mémoire et construction**

| structure     | construction |   Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ----: | ------: | -------: | -----: | ---------: |
| adjmap        |        10 ms |   3.7 |    8.25 |    117.7 |  1.91x |        3.4 |
| adjlist       |         5 ms |   3.2 |    7.13 |    101.7 |  1.65x |        3.2 |
| adjlist-exact |         5 ms |   2.7 |    5.96 |     85.1 |  1.38x |        2.5 |
| csr           |         4 ms |   1.9 |    4.31 |     61.5 |  1.00x |        1.9 |
| csr-slices    |         4 ms |   2.5 |    5.69 |     81.3 |  1.32x |        2.5 |
| edgelist      |         3 ms |   3.6 |    8.01 |    114.3 |  1.86x |        3.6 |
| varint-csr    |         6 ms |   1.1 |    2.45 |     35.0 |  0.57x |        1.1 |
| hybrid        |         7 ms |   2.0 |    4.48 |     63.9 |  1.04x |        2.0 |
| bitmatrix     |        72 ms | 128.0 |  287.07 |   4096.0 | 66.60x |      128.0 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch |   dfs | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | ----: | --------: | --------------: | ------: |
| adjmap        | 1.5 k |     1.6 k | 834.6 |    34.0 M |          45.2 M |  42.2 M |
| adjlist       | 2.3 k |     3.0 k | 975.5 |    44.6 M |          78.0 M |  59.6 M |
| adjlist-exact | 2.3 k |     3.2 k | 1.1 k |    54.2 M |          85.0 M |  64.1 M |
| csr           | 2.7 k |     3.9 k | 1.3 k |    65.1 M |          97.2 M |  72.6 M |
| csr-slices    | 2.4 k |     3.3 k | 1.1 k |    56.6 M |          77.4 M |  67.2 M |
| edgelist      | 1.0 k |     1.1 k | 708.0 |    23.4 M |          22.9 M |  27.2 M |
| varint-csr    | 1.2 k |     1.2 k | 778.3 |    28.9 M |          27.7 M |  13.9 M |
| hybrid        | 2.5 k |     3.6 k | 1.2 k |    62.1 M |          87.8 M |  83.1 M |
| bitmatrix     | 292.2 |     317.8 | 246.3 |     6.2 M |           6.8 M | 223.8 M |

**Passage à l'échelle — « bfs-batch », lot de 512 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        | 420.6 | 805.7 | 1.6 k | x3.80 |        7 % |
| adjlist       | 719.1 | 1.4 k | 3.0 k | x4.12 |       13 % |
| adjlist-exact | 799.9 | 1.7 k | 3.2 k | x3.97 |       25 % |
| csr           | 1.1 k | 1.8 k | 3.9 k | x3.63 |       27 % |
| csr-slices    | 839.1 | 1.6 k | 3.3 k | x3.94 |       10 % |
| edgelist      | 271.2 | 560.1 | 1.0 k | x3.74 |       11 % |
| varint-csr    | 306.1 | 595.5 | 1.2 k | x4.05 |        7 % |
| hybrid        | 867.1 | 1.7 k | 3.6 k | x4.21 |       12 % |
| bitmatrix     |  63.2 | 125.0 | 317.8 | x5.03 |       17 % |

**Passage à l'échelle — « hasedge », lot de 500 000 requêtes**

| structure     |  1 th. |   2 th. |   4 th. |  gain | dispersion |
| ------------- | -----: | ------: | ------: | ----: | ---------: |
| adjmap        | 11.7 M |  22.4 M |  42.2 M | x3.60 |       15 % |
| adjlist       | 15.7 M |  31.3 M |  59.6 M | x3.80 |       26 % |
| adjlist-exact | 16.1 M |  33.9 M |  64.1 M | x3.97 |       21 % |
| csr           | 20.5 M |  39.2 M |  72.6 M | x3.53 |       23 % |
| csr-slices    | 15.9 M |  33.8 M |  67.2 M | x4.22 |        7 % |
| edgelist      |  7.2 M |  13.6 M |  27.2 M | x3.76 |       33 % |
| varint-csr    |  3.5 M |   6.9 M |  13.9 M | x3.93 |       21 % |
| hybrid        | 20.2 M |  39.7 M |  83.1 M | x4.12 |       18 % |
| bitmatrix     | 52.8 M | 120.0 M | 223.8 M | x4.24 |       36 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |      p50 |   p99 |
| ------------- | -------: | ----: |
| adjmap        |     2 ms |  3 ms |
| adjlist       |     1 ms |  2 ms |
| adjlist-exact |     1 ms |  1 ms |
| csr           | 914.0 µs |  1 ms |
| csr-slices    |     1 ms |  2 ms |
| edgelist      |     3 ms |  5 ms |
| varint-csr    |     3 ms |  3 ms |
| hybrid        |     1 ms |  1 ms |
| bitmatrix     |    16 ms | 22 ms |

### rmat — 262 144 sommets, 3 939 048 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |       111 ms | 31.6 |    8.41 |    126.3 |  1.97x |       28.2 |
| adjlist       |        39 ms | 27.6 |    7.34 |    110.3 |  1.72x |       27.4 |
| adjlist-exact |        43 ms | 22.0 |    5.85 |     87.9 |  1.37x |       21.0 |
| csr           |        43 ms | 16.0 |    4.27 |     64.2 |  1.00x |       16.0 |
| csr-slices    |        39 ms | 21.0 |    5.60 |     84.1 |  1.31x |       21.0 |
| edgelist      |        19 ms | 30.1 |    8.00 |    120.2 |  1.87x |       30.1 |
| varint-csr    |        67 ms |  9.0 |    2.40 |     36.1 |  0.56x |        9.0 |
| hybrid        |        74 ms | 17.0 |    4.54 |     68.2 |  1.06x |       17.0 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | --------: | --------------: | ------: |
| adjmap        | 131.0 |     154.7 |    19.7 M |          26.6 M |  25.5 M |
| adjlist       | 193.9 |     269.9 |    27.3 M |          44.8 M |  41.4 M |
| adjlist-exact | 200.2 |     296.2 |    28.2 M |          48.1 M |  45.2 M |
| csr           | 226.9 |     314.6 |    32.5 M |          51.7 M |  42.8 M |
| csr-slices    | 202.4 |     271.4 |    28.2 M |          47.2 M |  43.5 M |
| edgelist      |  76.1 |      82.4 |    10.8 M |          11.7 M |  13.2 M |
| varint-csr    | 106.0 |     118.0 |    16.1 M |          18.5 M |   5.8 M |
| hybrid        | 236.4 |     324.5 |    35.2 M |          53.8 M |  45.7 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |  41.9 |  74.7 | 154.7 | x3.70 |       26 % |
| adjlist       |  82.7 | 153.6 | 269.9 | x3.26 |       32 % |
| adjlist-exact |  76.3 | 162.0 | 296.2 | x3.88 |       21 % |
| csr           |  95.6 | 199.2 | 314.6 | x3.29 |       21 % |
| csr-slices    |  70.4 | 145.9 | 271.4 | x3.85 |       27 % |
| edgelist      |  20.9 |  44.1 |  82.4 | x3.94 |       19 % |
| varint-csr    |  30.5 |  65.7 | 118.0 | x3.87 |       26 % |
| hybrid        | 104.6 | 191.4 | 323.9 | x3.10 |       14 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | -----: | -----: | -----: | ----: | ---------: |
| adjmap        |  6.9 M | 14.0 M | 25.5 M | x3.69 |       20 % |
| adjlist       |  9.7 M | 20.0 M | 38.9 M | x4.01 |       20 % |
| adjlist-exact | 11.8 M | 22.4 M | 45.2 M | x3.82 |       44 % |
| csr           | 12.1 M | 24.7 M | 42.8 M | x3.53 |       24 % |
| csr-slices    | 12.0 M | 20.5 M | 41.4 M | x3.43 |       10 % |
| edgelist      |  3.3 M |  6.6 M | 13.2 M | x4.05 |       19 % |
| varint-csr    |  1.4 M |  2.8 M |  5.3 M | x3.78 |       21 % |
| hybrid        | 12.5 M | 26.0 M | 45.7 M | x3.65 |       28 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |   p50 |   p99 |
| ------------- | ----: | ----: |
| adjmap        | 23 ms | 24 ms |
| adjlist       | 12 ms | 13 ms |
| adjlist-exact | 12 ms | 13 ms |
| csr           | 11 ms | 11 ms |
| csr-slices    | 14 ms | 16 ms |
| edgelist      | 41 ms | 41 ms |
| varint-csr    | 30 ms | 31 ms |
| hybrid        | 10 ms | 10 ms |

### rmat — 1 048 576 sommets, 16 084 638 arêtes

**Mémoire et construction**

| structure     | construction |   Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ----: | ------: | -------: | -----: | ---------: |
| adjmap        |       480 ms | 120.8 |    7.88 |    120.8 |  1.85x |      105.2 |
| adjlist       |       187 ms | 104.8 |    6.83 |    104.8 |  1.60x |      104.1 |
| adjlist-exact |       148 ms |  89.3 |    5.82 |     89.3 |  1.37x |       85.4 |
| csr           |       134 ms |  65.4 |    4.26 |     65.4 |  1.00x |       65.4 |
| csr-slices    |       164 ms |  85.4 |    5.56 |     85.4 |  1.31x |       85.4 |
| edgelist      |        66 ms | 122.7 |    8.00 |    122.7 |  1.88x |      122.7 |
| varint-csr    |       242 ms |  36.7 |    2.39 |     36.7 |  0.56x |       36.7 |
| hybrid        |       278 ms |  69.5 |    4.53 |     69.5 |  1.06x |       69.5 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |  bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ---: | --------: | --------: | --------------: | ------: |
| adjmap        | 26.7 |      29.7 |    18.4 M |          22.9 M |  20.9 M |
| adjlist       | 39.7 |      61.8 |    23.3 M |          38.7 M |  33.2 M |
| adjlist-exact | 41.0 |      60.6 |    24.1 M |          39.6 M |  33.3 M |
| csr           | 48.0 |      64.4 |    24.9 M |          43.9 M |  35.4 M |
| csr-slices    | 40.0 |      56.8 |    23.4 M |          41.7 M |  34.1 M |
| edgelist      | 14.5 |      14.8 |     8.0 M |           8.0 M |   9.2 M |
| varint-csr    | 23.0 |      24.1 |    13.5 M |          15.4 M |   2.9 M |
| hybrid        | 48.5 |      64.0 |    27.5 M |          42.1 M |  32.5 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |   7.0 |  14.1 |  29.7 | x4.21 |       15 % |
| adjlist       |  12.8 |  28.6 |  61.8 | x4.81 |       34 % |
| adjlist-exact |  13.0 |  26.1 |  60.6 | x4.65 |       14 % |
| csr           |  15.2 |  30.2 |  64.4 | x4.24 |       32 % |
| csr-slices    |  11.8 |  29.9 |  56.8 | x4.81 |       29 % |
| edgelist      |   3.9 |   7.8 |  14.8 | x3.80 |       10 % |
| varint-csr    |   6.5 |  12.7 |  23.9 | x3.69 |       16 % |
| hybrid        |  18.8 |  36.1 |  64.0 | x3.40 |       30 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |   1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | ------: | -----: | -----: | ----: | ---------: |
| adjmap        |   4.5 M | 10.4 M | 20.9 M | x4.63 |       26 % |
| adjlist       |   8.4 M | 14.0 M | 33.2 M | x3.94 |       47 % |
| adjlist-exact |   7.0 M | 15.9 M | 32.9 M | x4.70 |       11 % |
| csr           |   8.1 M | 17.1 M | 35.4 M | x4.38 |       20 % |
| csr-slices    |   8.3 M | 15.6 M | 32.9 M | x3.95 |       25 % |
| edgelist      |   2.2 M |  4.4 M |  8.8 M | x4.02 |       11 % |
| varint-csr    | 640.0 k |  1.5 M |  2.9 M | x4.56 |       20 % |
| hybrid        |   7.7 M | 16.7 M | 32.5 M | x4.21 |       19 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |    p50 |    p99 |
| ------------- | -----: | -----: |
| adjmap        | 140 ms | 180 ms |
| adjlist       |  79 ms |  85 ms |
| adjlist-exact |  77 ms |  93 ms |
| csr           |  69 ms |  73 ms |
| csr-slices    |  81 ms |  84 ms |
| edgelist      | 252 ms | 293 ms |
| varint-csr    | 150 ms | 174 ms |
| hybrid        |  53 ms |  58 ms |

### grid — 19 881 sommets, 78 960 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet |  vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | ------: | ---------: |
| adjmap        |         3 ms |  1.6 |   20.65 |     82.0 |   4.06x |        1.2 |
| adjlist       |     935.5 µs |  0.8 |   10.15 |     40.3 |   2.00x |        0.8 |
| adjlist-exact |     815.1 µs |  0.8 |   10.15 |     40.3 |   2.00x |        0.8 |
| csr           |     420.1 µs |  0.4 |    5.08 |     20.2 |   1.00x |        0.4 |
| csr-slices    |     674.9 µs |  0.8 |   10.17 |     40.4 |   2.00x |        0.8 |
| edgelist      |     226.3 µs |  0.6 |    8.09 |     32.1 |   1.59x |        0.6 |
| varint-csr    |     853.4 µs |  0.3 |    3.63 |     14.4 |   0.71x |        0.3 |
| hybrid        |     744.6 µs |  0.5 |    6.16 |     24.5 |   1.21x |        0.5 |
| bitmatrix     |        26 ms | 47.2 |  626.54 |   2488.4 | 123.23x |       47.2 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |    bfs | bfs-batch |    dfs | neighbors | neighbors-batch | hasedge |
| ------------- | -----: | --------: | -----: | --------: | --------------: | ------: |
| adjmap        |  7.3 k |     7.7 k |  5.1 k |   165.3 M |         169.7 M |  94.7 M |
| adjlist       | 13.9 k |    17.9 k | 13.4 k |   293.8 M |         332.0 M | 195.2 M |
| adjlist-exact | 14.6 k |    18.8 k | 14.2 k |   297.3 M |         324.4 M | 190.8 M |
| csr           | 14.0 k |    18.1 k | 12.8 k |   282.9 M |         367.3 M | 211.0 M |
| csr-slices    | 14.5 k |    16.5 k | 14.6 k |   269.0 M |         352.7 M | 185.8 M |
| edgelist      |  2.2 k |     2.2 k |  3.1 k |    35.8 M |          34.9 M |  39.1 M |
| varint-csr    |  8.5 k |     8.9 k |  7.5 k |   150.2 M |         147.4 M | 151.0 M |
| hybrid        | 14.9 k |    17.6 k | 14.5 k |   271.9 M |         361.7 M | 187.2 M |
| bitmatrix     |  751.2 |     746.8 |  854.1 |    10.6 M |          11.8 M | 245.6 M |

**Passage à l'échelle — « bfs-batch », lot de 512 requêtes**

| structure     | 1 th. | 2 th. |  4 th. |  gain | dispersion |
| ------------- | ----: | ----: | -----: | ----: | ---------: |
| adjmap        | 1.8 k | 3.3 k |  7.7 k | x4.30 |       37 % |
| adjlist       | 5.9 k | 9.6 k | 17.4 k | x2.95 |       44 % |
| adjlist-exact | 4.6 k | 8.6 k | 18.8 k | x4.14 |       54 % |
| csr           | 5.4 k | 9.6 k | 18.1 k | x3.39 |       38 % |
| csr-slices    | 4.5 k | 8.6 k | 16.5 k | x3.71 |       34 % |
| edgelist      | 548.5 | 1.1 k |  2.2 k | x4.01 |       10 % |
| varint-csr    | 2.2 k | 4.1 k |  8.9 k | x4.06 |       18 % |
| hybrid        | 4.2 k | 8.1 k | 17.6 k | x4.19 |       23 % |
| bitmatrix     | 179.1 | 396.8 |  746.8 | x4.17 |       10 % |

**Passage à l'échelle — « hasedge », lot de 500 000 requêtes**

| structure     |  1 th. |   2 th. |   4 th. |  gain | dispersion |
| ------------- | -----: | ------: | ------: | ----: | ---------: |
| adjmap        | 24.1 M |  48.9 M |  94.7 M | x3.93 |       53 % |
| adjlist       | 60.7 M | 105.7 M | 195.2 M | x3.22 |        8 % |
| adjlist-exact | 48.1 M |  94.5 M | 190.8 M | x3.96 |        4 % |
| csr           | 49.2 M |  96.9 M | 211.0 M | x4.29 |        9 % |
| csr-slices    | 47.5 M |  94.7 M | 185.8 M | x3.91 |        6 % |
| edgelist      | 10.0 M |  19.5 M |  39.1 M | x3.91 |       10 % |
| varint-csr    | 42.8 M |  84.5 M | 151.0 M | x3.53 |        6 % |
| hybrid        | 47.0 M |  96.9 M | 187.2 M | x3.98 |        9 % |
| bitmatrix     | 64.8 M | 103.5 M | 245.6 M | x3.79 |       12 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |      p50 |      p99 |
| ------------- | -------: | -------: |
| adjmap        | 518.0 µs | 997.7 µs |
| adjlist       | 167.6 µs | 206.3 µs |
| adjlist-exact | 212.1 µs | 364.8 µs |
| csr           | 172.9 µs | 259.4 µs |
| csr-slices    | 219.8 µs | 301.4 µs |
| edgelist      |     2 ms |     2 ms |
| varint-csr    | 447.3 µs | 619.2 µs |
| hybrid        | 232.9 µs | 305.2 µs |
| bitmatrix     |     5 ms |     7 ms |

### grid — 199 809 sommets, 797 448 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |        65 ms | 13.1 |   17.17 |     68.5 |  3.43x |       12.2 |
| adjlist       |        13 ms |  7.6 |   10.03 |     40.0 |  2.00x |        7.6 |
| adjlist-exact |        10 ms |  7.6 |   10.03 |     40.0 |  2.00x |        7.6 |
| csr           |         8 ms |  3.8 |    5.01 |     20.0 |  1.00x |        3.8 |
| csr-slices    |         9 ms |  7.6 |   10.03 |     40.0 |  2.00x |        7.6 |
| edgelist      |         3 ms |  6.1 |    8.00 |     31.9 |  1.60x |        6.1 |
| varint-csr    |        12 ms |  2.7 |    3.51 |     14.0 |  0.70x |        2.7 |
| hybrid        |        10 ms |  4.6 |    6.05 |     24.2 |  1.21x |        4.6 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | --------: | --------------: | ------: |
| adjmap        | 228.7 |     214.1 |    52.5 M |          52.6 M |  43.3 M |
| adjlist       | 726.4 |     754.8 |    81.0 M |         104.4 M |  96.7 M |
| adjlist-exact | 723.6 |     938.9 |    79.8 M |         116.6 M | 100.3 M |
| csr           | 1.1 k |     1.0 k |   103.8 M |         157.6 M | 105.3 M |
| csr-slices    | 779.8 |     869.0 |    86.7 M |         115.7 M | 100.4 M |
| edgelist      | 151.1 |     177.0 |    21.2 M |          20.8 M |  21.6 M |
| varint-csr    | 752.6 |     652.8 |    70.4 M |          79.4 M |  82.8 M |
| hybrid        | 1.0 k |     899.9 |   103.8 M |         150.2 M | 103.5 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |  60.3 | 116.5 | 214.1 | x3.55 |       18 % |
| adjlist       | 211.7 | 431.1 | 754.8 | x3.57 |       66 % |
| adjlist-exact | 283.5 | 495.2 | 938.9 | x3.31 |       39 % |
| csr           | 319.4 | 610.1 | 1.0 k | x3.15 |       46 % |
| csr-slices    | 232.2 | 453.7 | 869.0 | x3.74 |       46 % |
| edgelist      |  43.8 |  98.4 | 177.0 | x4.04 |       23 % |
| varint-csr    | 182.2 | 365.2 | 652.8 | x3.58 |       75 % |
| hybrid        | 256.2 | 560.1 | 899.9 | x3.51 |       20 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |   4 th. |  gain | dispersion |
| ------------- | -----: | -----: | ------: | ----: | ---------: |
| adjmap        | 11.4 M | 21.2 M |  43.3 M | x3.80 |       23 % |
| adjlist       | 28.4 M | 52.1 M |  96.7 M | x3.41 |       26 % |
| adjlist-exact | 29.4 M | 58.8 M | 100.3 M | x3.41 |       48 % |
| csr           | 25.2 M | 49.7 M | 105.3 M | x4.18 |       13 % |
| csr-slices    | 27.9 M | 52.7 M | 100.4 M | x3.60 |        8 % |
| edgelist      |  4.9 M | 10.9 M |  21.0 M | x4.31 |       19 % |
| varint-csr    | 21.4 M | 42.3 M |  82.8 M | x3.86 |       30 % |
| hybrid        | 28.5 M | 55.9 M | 103.5 M | x3.63 |       23 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |   p50 |   p99 |
| ------------- | ----: | ----: |
| adjmap        | 14 ms | 15 ms |
| adjlist       |  5 ms |  5 ms |
| adjlist-exact |  3 ms |  4 ms |
| csr           |  3 ms |  5 ms |
| csr-slices    |  4 ms |  4 ms |
| edgelist      | 23 ms | 29 ms |
| varint-csr    |  5 ms |  5 ms |
| hybrid        |  3 ms |  4 ms |

### grid — 1 000 000 sommets, 3 996 000 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |       432 ms | 95.1 |   24.97 |     99.8 |  4.99x |       61.0 |
| adjlist       |        69 ms | 38.1 |   10.01 |     40.0 |  2.00x |       38.1 |
| adjlist-exact |        52 ms | 38.1 |   10.01 |     40.0 |  2.00x |       38.1 |
| csr           |        35 ms | 19.1 |    5.00 |     20.0 |  1.00x |       19.1 |
| csr-slices    |        57 ms | 38.1 |   10.01 |     40.0 |  2.00x |       38.1 |
| edgelist      |        15 ms | 30.5 |    8.00 |     32.0 |  1.60x |       30.5 |
| varint-csr    |        60 ms | 13.4 |    3.50 |     14.0 |  0.70x |       13.3 |
| hybrid        |        67 ms | 23.0 |    6.04 |     24.1 |  1.21x |       23.0 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | --------: | --------------: | ------: |
| adjmap        |  36.7 |      36.4 |    40.3 M |          41.0 M |  32.1 M |
| adjlist       | 111.8 |     111.7 |    57.0 M |          83.4 M |  75.4 M |
| adjlist-exact | 116.9 |     124.5 |    63.6 M |          85.5 M |  79.2 M |
| csr           | 196.2 |     170.8 |    76.1 M |         104.8 M |  80.8 M |
| csr-slices    | 116.7 |     136.1 |    61.1 M |          86.3 M |  82.4 M |
| edgelist      |  15.6 |      16.2 |    11.2 M |          11.3 M |  13.1 M |
| varint-csr    | 133.8 |     128.7 |    45.5 M |          45.2 M |  49.3 M |
| hybrid        | 177.5 |     171.9 |    63.0 M |          93.6 M |  76.9 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |   8.0 |  15.7 |  36.4 | x4.55 |       24 % |
| adjlist       |  32.8 |  65.8 | 111.7 | x3.41 |       62 % |
| adjlist-exact |  30.8 |  67.6 | 124.5 | x4.05 |       39 % |
| csr           |  49.9 |  93.8 | 170.8 | x3.42 |       39 % |
| csr-slices    |  45.0 |  75.2 | 136.1 | x3.02 |       35 % |
| edgelist      |   4.5 |   7.9 |  15.8 | x3.53 |       14 % |
| varint-csr    |  37.5 |  71.6 | 128.7 | x3.43 |       37 % |
| hybrid        |  35.1 |  83.0 | 171.9 | x4.90 |       24 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | -----: | -----: | -----: | ----: | ---------: |
| adjmap        |  7.4 M | 14.9 M | 32.1 M | x4.34 |       54 % |
| adjlist       | 19.5 M | 37.1 M | 75.4 M | x3.86 |       33 % |
| adjlist-exact | 24.4 M | 39.1 M | 79.2 M | x3.25 |       24 % |
| csr           | 20.5 M | 44.5 M | 80.3 M | x3.92 |       11 % |
| csr-slices    | 23.3 M | 41.5 M | 82.4 M | x3.54 |       51 % |
| edgelist      |  3.0 M |  6.5 M | 13.1 M | x4.39 |       23 % |
| varint-csr    | 13.7 M | 27.0 M | 49.3 M | x3.61 |       69 % |
| hybrid        | 18.2 M | 39.7 M | 76.9 M | x4.22 |       10 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |    p50 |    p99 |
| ------------- | -----: | -----: |
| adjmap        | 125 ms | 134 ms |
| adjlist       |  29 ms |  50 ms |
| adjlist-exact |  26 ms |  29 ms |
| csr           |  20 ms |  27 ms |
| csr-slices    |  22 ms |  25 ms |
| edgelist      | 212 ms | 225 ms |
| varint-csr    |  25 ms |  30 ms |
| hybrid        |  28 ms |  35 ms |

## 7. Reproductibilité entre campagnes

La même campagne, même code de mesure et même machine, a été rejouée le 2026-09-24 après celle du 2026-09-22. Sur 2 412 points de mesure communs :

- **la mémoire est reproductible** : écart maximal de 0.29 % ;
- **le débit l'est beaucoup moins** : rapport médian de 1.034 entre les deux campagnes, mais 80 % des points entre 0.91 et 1.24, et 23 % des points s'écartent de plus que leur propre dispersion interne ;
- **la structure la plus rapide d'un point donné** est la même dans 126 cas sur 288 seulement.

La dispersion mesurée *au sein* d'une campagne sous-estime donc la variance réelle : des répétitions rapprochées partagent l'état de la machine, deux campagnes à des jours d'écart non. D'où les deux règles appliquées dans ce rapport : les structures à moins de 17 % l'une de l'autre sont déclarées ex æquo, et seul ce qui se retrouve dans les deux campagnes est présenté comme une conclusion.

`python3 scripts/make_results.py campaigns/2026-09-24 campaigns/2026-09-22` refait cette comparaison depuis les mesures versionnées.

## 8. Ce que ce rapport ne dit pas

- **Une seule machine.** Les écarts entre structures inférieurs au seuil d'ex æquo ne sont pas significatifs ; seuls les ordres de grandeur le sont.
- **Graphes synthétiques.** Les topologies sont choisies pour isoler des effets (localité, hubs, uniformité), pas pour imiter un graphe réel.
- **Structures figées.** Rien n'est mesuré après construction : ni insertion, ni suppression, ni mise à jour.
- **Le seuil de promotion des hubs** de la structure hybride est dérivé d'un calcul mémoire (`deg > n/32`). À un million de sommets, presque aucun sommet ne l'atteint, même en loi de puissance : la structure se réduit alors à un CSR et ses écarts avec lui sont du bruit.
- **Le périmètre complet** est décrit dans `SPEC.md`, les choix de protocole dans `DECISIONS.md`.

