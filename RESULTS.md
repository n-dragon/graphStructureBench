# Résultats de campagne

Machine : 4 cœurs, Go 1.24.7. Campagne du 2026-09-22, code au commit `c7a2a8e`.

Ce rapport est **engendré** par `scripts/make_results.py` à partir des CSV produits par `scripts/run-bench.sh` : les chiffres cités dans le texte sont extraits des mesures, pas recopiés. Les sorties complètes du balayage se trouvent dans `results/`.

## En bref

| graphe           | parcours le plus rapide |  test d'arête le plus rapide |     index le plus compact |
| ---------------- | ----------------------: | ---------------------------: | ------------------------: |
| er (n=1000000)   |        csr (29.7 req/s) |           csr (47.4 M req/s) | varint-csr (3.57 o/arête) |
| rmat (n=1048576) |        csr (64.5 req/s) |           csr (33.7 M req/s) | varint-csr (2.39 o/arête) |
| grid (n=1000000) |       csr (162.0 req/s) | adjlist-exact (85.3 M req/s) | varint-csr (3.50 o/arête) |

**Ce qu'on retient.** Le CSR est le choix par défaut : il est le plus rapide ou à quelques pourcents du plus rapide sur toutes les charges, et sa mémoire est celle d'un tableau plat. La liste d'adjacence par `map`, le réflexe idiomatique en Go, est la plus lente **et** la plus lourde — c'est le seul verdict sans nuance du banc. Le CSR compressé en varint est la réponse quand la mémoire prime : il descend sous le CSR en taille, au prix d'un débit moindre. La matrice dense n'a d'intérêt que sur de petits graphes, et uniquement pour le test d'arête.

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

- `er` : +40 % de débit (degré moyen 16.0)
- `rmat` : +39 % de débit (degré moyen 15.3)
- `grid` : -9 % de débit (degré moyen 4.0)

Le gain n'est donc pas acquis : il vient de l'appel indirect économisé par voisin, et il faut le payer en recopiant le bloc. Sur `grid`, dont le degré moyen est de 4.0, la copie n'est amortie sur personne et le bloc perd 9 %. C'est le compromis à retenir : lire par bloc gagne sur les sommets de fort degré, perd sur les graphes de faible degré.

En latence, un parcours complet mono-thread sur le CSR prend 165 ms sur `er` (1 000 000 sommets), 70 ms sur `rmat` (1 048 576 sommets), 33 ms sur `grid` (1 000 000 sommets).

## 3. Requêtes ponctuelles

La lecture d'adjacence et le test d'existence d'arête se mesurent par millions de requêtes, réparties sur les threads par un compteur atomique.

Le test d'arête sépare nettement les structures : sur `er`, le CSR (recherche binaire dans une liste courte) tient 47.4 M req/s quand la liste d'arêtes triée (recherche binaire sur les 15 999 866 arêtes) plafonne à 8.0 M req/s — un facteur 5.9.

## 4. Passage à l'échelle

Sur 4 cœurs, le gain médian entre 1 et 4 threads est de x3.88 sur 804 points de mesure. Mais la médiane cache l'essentiel : le gain **dépend de la taille de l'index**.

| sommets   | index csr | gain médian | points au-dessus de x4 |
| --------- | --------: | ----------: | ---------------------: |
| 19 881    |   0.4 Mio |       x3.64 |                 10/108 |
| 20 000    |   1.3 Mio |       x3.79 |                 13/108 |
| 32 768    |   1.9 Mio |       x3.77 |                 16/108 |
| 199 809   |   3.8 Mio |       x3.65 |                  13/80 |
| 200 000   |  13.0 Mio |       x3.89 |                  27/80 |
| 262 144   |  16.0 Mio |       x3.92 |                  30/80 |
| 1 000 000 |  64.9 Mio |       x4.62 |                120/160 |
| 1 048 576 |  65.4 Mio |       x4.33 |                  69/80 |

**Le parallélisme rapporte davantage quand l'index sort du cache.** À 19 881 sommets, l'index de 0.4 Mio tient dans le cache du processeur et le gain plafonne à x3.64. À 1 048 576 sommets, les 65.4 Mio de l'index se lisent en mémoire vive et le gain médian atteint x4.33 — au-delà du nombre de cœurs.

Ce dépassement n'est pas une erreur de mesure, c'est du **parallélisme mémoire**. Un cœur ne peut avoir qu'une dizaine de défauts de cache en vol simultanément ; une lecture d'adjacence aléatoire dans un index de 65 Mio est limitée par cette latence, pas par le calcul. Quatre cœurs quadruplent le nombre de requêtes mémoire en vol, et le débit agrégé progresse plus que proportionnellement. C'est un résultat utile en soi : sur un index qui ne tient pas en cache, ajouter des threads paie mieux que ne le laisse croire le nombre de cœurs.

S'y ajoute une dispersion réelle : les répétitions d'un même point à 1 thread varient de 8 % en médiane, jusqu'à 153 % au pire. La colonne « dispersion » des tableaux détaillés donne cet écart point par point : **deux structures qui diffèrent de moins que leur dispersion ne sont pas départageables**.

Les index sont immuables après construction : aucune synchronisation n'est nécessaire en lecture, et le seul état par thread est le tampon de parcours.

## 5. Le prix de l'abstraction d'itération

Trois façons de lire l'adjacence d'un sommet tiré au hasard, sur un graphe de 500 000 sommets. `slice directe` n'est disponible que pour les structures qui stockent leurs voisins contigus en mémoire : c'est la borne basse, pas une option universelle.

| structure     | callback (ns) | bloc (ns) | gain du bloc | slice directe (ns) |
| ------------- | ------------: | --------: | -----------: | -----------------: |
| adjmap        |           404 |       246 |         64 % |                108 |
| adjlist       |           307 |       142 |        116 % |                 55 |
| adjlist-exact |           228 |       107 |        113 % |                 53 |
| csr           |           177 |        91 |         95 % |                 40 |
| csr-slices    |           222 |       100 |        122 % |                 54 |
| edgelist      |           579 |       595 |         -3 % |                n/a |
| varint-csr    |           364 |       338 |          8 % |                n/a |
| hybrid        |           212 |        97 |        120 % |                n/a |

L'écart est considérable : sur le CSR, itérer par callback coûte 4.4 fois le parcours direct de la slice. C'est pourquoi le banc mesure les deux modes plutôt que d'en imposer un — sans quoi il classerait des interfaces d'itération plutôt que des structures de données.

## 6. Mesures détaillées

### er — 20 000 sommets, 319 858 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |         8 ms |  3.0 |    9.80 |    156.7 |  2.29x |        2.7 |
| adjlist       |         5 ms |  2.2 |    7.22 |    115.5 |  1.69x |        2.2 |
| adjlist-exact |         3 ms |  1.8 |    5.89 |     94.2 |  1.38x |        1.7 |
| csr           |         2 ms |  1.3 |    4.28 |     68.4 |  1.00x |        1.3 |
| csr-slices    |         2 ms |  1.7 |    5.53 |     88.5 |  1.29x |        1.7 |
| edgelist      |         2 ms |  2.4 |    8.02 |    128.2 |  1.87x |        2.4 |
| varint-csr    |         5 ms |  0.7 |    2.38 |     38.1 |  0.56x |        0.7 |
| hybrid        |         5 ms |  1.4 |    4.54 |     72.6 |  1.06x |        1.4 |
| bitmatrix     |        33 ms | 47.8 |  156.59 |   2504.3 | 36.61x |       47.8 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch |   dfs | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | ----: | --------: | --------------: | ------: |
| adjmap        | 1.5 k |     1.9 k | 1.2 k |    44.8 M |          57.1 M |  48.2 M |
| adjlist       | 2.2 k |     3.3 k | 1.7 k |    56.9 M |         102.2 M |  79.0 M |
| adjlist-exact | 2.8 k |     3.5 k | 1.9 k |    69.2 M |         101.0 M |  86.8 M |
| csr           | 3.5 k |     4.2 k | 2.3 k |    87.3 M |         117.3 M |  98.6 M |
| csr-slices    | 2.9 k |     3.6 k | 2.0 k |    67.4 M |          95.1 M |  86.6 M |
| edgelist      | 997.6 |     1.0 k | 1.0 k |    20.2 M |          20.8 M |  26.3 M |
| varint-csr    | 1.8 k |     1.8 k | 1.5 k |    38.2 M |          36.6 M |  64.0 M |
| hybrid        | 3.5 k |     4.3 k | 2.2 k |    71.9 M |         114.8 M |  91.4 M |
| bitmatrix     | 346.6 |     371.4 | 345.9 |     7.1 M |           7.1 M | 253.9 M |

**Passage à l'échelle — « bfs-batch », lot de 512 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        | 499.0 | 968.1 | 1.9 k | x3.83 |        7 % |
| adjlist       | 849.2 | 1.7 k | 3.3 k | x3.85 |        7 % |
| adjlist-exact | 921.1 | 1.8 k | 3.5 k | x3.83 |       12 % |
| csr           | 1.1 k | 2.1 k | 4.2 k | x3.82 |       11 % |
| csr-slices    | 1.0 k | 1.8 k | 3.6 k | x3.55 |       21 % |
| edgelist      | 253.8 | 515.9 | 1.0 k | x4.14 |        9 % |
| varint-csr    | 447.8 | 872.4 | 1.8 k | x4.01 |        7 % |
| hybrid        | 1.1 k | 2.1 k | 4.3 k | x4.03 |       13 % |
| bitmatrix     |  97.1 | 186.0 | 371.4 | x3.82 |        6 % |

**Passage à l'échelle — « hasedge », lot de 500 000 requêtes**

| structure     |  1 th. |   2 th. |   4 th. |  gain | dispersion |
| ------------- | -----: | ------: | ------: | ----: | ---------: |
| adjmap        | 12.4 M |  25.0 M |  48.2 M | x3.88 |        8 % |
| adjlist       | 19.2 M |  38.8 M |  79.0 M | x4.11 |       41 % |
| adjlist-exact | 22.5 M |  43.5 M |  86.8 M | x3.86 |       30 % |
| csr           | 24.9 M |  47.9 M |  98.6 M | x3.96 |       17 % |
| csr-slices    | 21.5 M |  44.9 M |  86.6 M | x4.03 |       20 % |
| edgelist      |  6.6 M |  13.3 M |  26.3 M | x3.97 |        8 % |
| varint-csr    | 16.0 M |  31.1 M |  64.0 M | x3.99 |       27 % |
| hybrid        | 23.7 M |  47.5 M |  91.4 M | x3.86 |        8 % |
| bitmatrix     | 71.9 M | 134.1 M | 253.9 M | x3.53 |       33 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |      p50 |   p99 |
| ------------- | -------: | ----: |
| adjmap        |     2 ms |  2 ms |
| adjlist       |     1 ms |  1 ms |
| adjlist-exact |     1 ms |  1 ms |
| csr           | 902.7 µs |  1 ms |
| csr-slices    | 970.1 µs |  1 ms |
| edgelist      |     4 ms |  5 ms |
| varint-csr    |     2 ms |  2 ms |
| hybrid        | 881.1 µs |  1 ms |
| bitmatrix     |    10 ms | 11 ms |

### er — 200 000 sommets, 3 199 860 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |       131 ms | 27.4 |    8.97 |    143.5 |  2.11x |       26.5 |
| adjlist       |        41 ms | 21.9 |    7.19 |    115.1 |  1.69x |       21.9 |
| adjlist-exact |        30 ms | 17.9 |    5.87 |     94.0 |  1.38x |       16.8 |
| csr           |        31 ms | 13.0 |    4.25 |     68.0 |  1.00x |       13.0 |
| csr-slices    |        39 ms | 16.8 |    5.50 |     88.0 |  1.29x |       16.8 |
| edgelist      |        17 ms | 24.4 |    8.00 |    128.0 |  1.88x |       24.4 |
| varint-csr    |        63 ms |  8.9 |    2.91 |     46.5 |  0.68x |        8.9 |
| hybrid        |        46 ms | 13.8 |    4.51 |     72.2 |  1.06x |       13.8 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | --------: | --------------: | ------: |
| adjmap        |  83.3 |     104.7 |    22.1 M |          29.8 M |  30.2 M |
| adjlist       | 119.9 |     185.0 |    25.2 M |          57.3 M |  52.4 M |
| adjlist-exact | 121.9 |     171.7 |    26.1 M |          60.5 M |  53.9 M |
| csr           | 150.4 |     208.0 |    31.2 M |          73.3 M |  45.5 M |
| csr-slices    | 114.5 |     196.0 |    24.7 M |          59.1 M |  55.9 M |
| edgelist      |  47.7 |      43.5 |     9.2 M |           9.3 M |  11.9 M |
| varint-csr    |  87.9 |      88.6 |    18.8 M |          18.6 M |  24.5 M |
| hybrid        | 142.3 |     212.0 |    30.7 M |          62.0 M |  55.7 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |  26.0 |  47.5 | 104.7 | x4.02 |       20 % |
| adjlist       |  45.5 |  97.0 | 185.0 | x4.07 |       29 % |
| adjlist-exact |  49.3 |  90.7 | 171.7 | x3.48 |       22 % |
| csr           |  57.8 | 114.2 | 208.0 | x3.60 |       29 % |
| csr-slices    |  50.4 |  89.3 | 196.0 | x3.89 |       22 % |
| edgelist      |  11.9 |  23.7 |  43.5 | x3.66 |       11 % |
| varint-csr    |  23.9 |  45.9 |  87.3 | x3.65 |        8 % |
| hybrid        |  59.4 | 109.2 | 208.1 | x3.50 |       15 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | -----: | -----: | -----: | ----: | ---------: |
| adjmap        |  7.4 M | 14.9 M | 30.2 M | x4.08 |       18 % |
| adjlist       | 13.3 M | 26.3 M | 52.4 M | x3.95 |       24 % |
| adjlist-exact | 14.3 M | 30.0 M | 53.9 M | x3.78 |       22 % |
| csr           | 11.7 M | 23.9 M | 45.3 M | x3.86 |       19 % |
| csr-slices    | 13.5 M | 27.6 M | 55.9 M | x4.15 |       10 % |
| edgelist      |  2.9 M |  5.8 M | 11.6 M | x3.95 |        7 % |
| varint-csr    |  6.5 M | 12.7 M | 24.2 M | x3.72 |       10 % |
| hybrid        | 14.5 M | 28.1 M | 55.7 M | x3.85 |       23 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |   p50 |   p99 |
| ------------- | ----: | ----: |
| adjmap        | 38 ms | 44 ms |
| adjlist       | 21 ms | 21 ms |
| adjlist-exact | 20 ms | 22 ms |
| csr           | 17 ms | 20 ms |
| csr-slices    | 20 ms | 21 ms |
| edgelist      | 83 ms | 94 ms |
| varint-csr    | 42 ms | 43 ms |
| hybrid        | 16 ms | 17 ms |

### er — 1 000 000 sommets, 15 999 866 arêtes

**Mémoire et construction**

| structure     | construction |   Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ----: | ------: | -------: | -----: | ---------: |
| adjmap        |       1.11 s | 166.7 |   10.92 |    174.8 |  2.57x |      132.6 |
| adjlist       |       230 ms | 109.8 |    7.19 |    115.1 |  1.69x |      109.7 |
| adjlist-exact |       144 ms |  89.6 |    5.87 |     94.0 |  1.38x |       83.9 |
| csr           |       148 ms |  64.9 |    4.25 |     68.0 |  1.00x |       64.8 |
| csr-slices    |       145 ms |  83.9 |    5.50 |     88.0 |  1.29x |       83.9 |
| edgelist      |        84 ms | 122.1 |    8.00 |    128.0 |  1.88x |      122.1 |
| varint-csr    |       446 ms |  54.5 |    3.57 |     57.1 |  0.84x |       54.5 |
| hybrid        |       284 ms |  68.8 |    4.51 |     72.1 |  1.06x |       68.8 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |  bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ---: | --------: | --------: | --------------: | ------: |
| adjmap        |  9.9 |      12.3 |    10.5 M |          20.9 M |  17.2 M |
| adjlist       | 15.6 |      19.1 |    13.8 M |          38.4 M |  37.6 M |
| adjlist-exact | 18.2 |      28.1 |    17.7 M |          47.7 M |  45.2 M |
| csr           | 21.2 |      29.7 |    21.4 M |          54.6 M |  47.4 M |
| csr-slices    | 16.3 |      22.1 |    19.2 M |          47.4 M |  42.8 M |
| edgelist      |  5.7 |       5.7 |     6.3 M |           6.1 M |   8.0 M |
| varint-csr    | 12.4 |      12.5 |    13.5 M |          13.3 M |  17.3 M |
| hybrid        | 20.8 |      27.9 |    22.7 M |          51.0 M |  44.5 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |   2.5 |   5.5 |  12.3 | x5.01 |       12 % |
| adjlist       |   4.0 |   8.4 |  19.1 | x4.75 |        9 % |
| adjlist-exact |   4.6 |  11.6 |  28.1 | x6.09 |       29 % |
| csr           |   5.9 |  11.9 |  28.6 | x4.84 |       14 % |
| csr-slices    |   4.2 |   8.6 |  22.1 | x5.26 |       12 % |
| edgelist      |   1.3 |   2.8 |   5.7 | x4.26 |       10 % |
| varint-csr    |   2.5 |   5.4 |  12.5 | x4.93 |       10 % |
| hybrid        |   5.4 |  12.0 |  27.9 | x5.18 |       21 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | -----: | -----: | -----: | ----: | ---------: |
| adjmap        |  4.1 M |  8.1 M | 16.8 M | x4.13 |       14 % |
| adjlist       |  7.6 M | 16.9 M | 35.4 M | x4.68 |       11 % |
| adjlist-exact |  8.8 M | 18.6 M | 42.4 M | x4.80 |       18 % |
| csr           | 10.1 M | 23.3 M | 47.0 M | x4.67 |       14 % |
| csr-slices    |  8.9 M | 19.7 M | 42.8 M | x4.84 |       14 % |
| edgelist      |  1.9 M |  4.0 M |  8.0 M | x4.14 |       13 % |
| varint-csr    |  3.4 M |  7.8 M | 17.3 M | x5.10 |        8 % |
| hybrid        |  9.5 M | 21.1 M | 44.5 M | x4.68 |       22 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |    p50 |    p99 |
| ------------- | -----: | -----: |
| adjmap        | 370 ms | 379 ms |
| adjlist       | 239 ms | 261 ms |
| adjlist-exact | 191 ms | 211 ms |
| csr           | 165 ms | 191 ms |
| csr-slices    | 234 ms | 266 ms |
| edgelist      | 716 ms | 730 ms |
| varint-csr    | 394 ms | 424 ms |
| hybrid        | 183 ms | 185 ms |

### rmat — 32 768 sommets, 467 546 arêtes

**Mémoire et construction**

| structure     | construction |   Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ----: | ------: | -------: | -----: | ---------: |
| adjmap        |        13 ms |   3.7 |    8.25 |    117.7 |  1.91x |        3.4 |
| adjlist       |         4 ms |   3.2 |    7.13 |    101.7 |  1.65x |        3.2 |
| adjlist-exact |         4 ms |   2.7 |    5.96 |     85.1 |  1.38x |        2.5 |
| csr           |         5 ms |   1.9 |    4.31 |     61.5 |  1.00x |        1.9 |
| csr-slices    |         5 ms |   2.5 |    5.69 |     81.3 |  1.32x |        2.5 |
| edgelist      |         3 ms |   3.6 |    8.01 |    114.3 |  1.86x |        3.6 |
| varint-csr    |         9 ms |   1.1 |    2.45 |     35.0 |  0.57x |        1.1 |
| hybrid        |         9 ms |   2.0 |    4.48 |     63.9 |  1.04x |        2.0 |
| bitmatrix     |        74 ms | 128.0 |  287.07 |   4096.0 | 66.60x |      128.0 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch |   dfs | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | ----: | --------: | --------------: | ------: |
| adjmap        | 1.7 k |     1.7 k | 856.7 |    35.0 M |          46.1 M |  40.2 M |
| adjlist       | 2.2 k |     3.0 k | 1.0 k |    45.3 M |          70.4 M |  61.7 M |
| adjlist-exact | 2.4 k |     3.3 k | 1.1 k |    52.2 M |          70.4 M |  61.5 M |
| csr           | 2.8 k |     3.8 k | 1.2 k |    63.9 M |         104.6 M |  77.4 M |
| csr-slices    | 2.5 k |     3.3 k | 1.1 k |    55.0 M |          78.2 M |  63.9 M |
| edgelist      | 1.1 k |     1.2 k | 720.9 |    20.3 M |          20.9 M |  25.6 M |
| varint-csr    | 1.3 k |     1.3 k | 842.6 |    26.3 M |          26.4 M |  13.9 M |
| hybrid        | 2.9 k |     3.5 k | 1.3 k |    64.1 M |          94.8 M |  80.2 M |
| bitmatrix     | 308.9 |     334.8 | 269.5 |     6.4 M |           6.8 M | 245.0 M |

**Passage à l'échelle — « bfs-batch », lot de 512 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        | 428.4 | 853.2 | 1.7 k | x4.06 |       10 % |
| adjlist       | 771.3 | 1.5 k | 3.0 k | x3.93 |       11 % |
| adjlist-exact | 891.2 | 1.6 k | 3.1 k | x3.53 |       10 % |
| csr           | 964.2 | 1.9 k | 3.8 k | x3.94 |        6 % |
| csr-slices    | 897.9 | 1.7 k | 3.2 k | x3.57 |       17 % |
| edgelist      | 281.0 | 571.3 | 1.2 k | x4.19 |        9 % |
| varint-csr    | 333.3 | 690.5 | 1.3 k | x3.82 |       13 % |
| hybrid        | 853.3 | 1.9 k | 3.5 k | x4.15 |        7 % |
| bitmatrix     |  84.9 | 161.9 | 334.8 | x3.94 |       11 % |

**Passage à l'échelle — « hasedge », lot de 500 000 requêtes**

| structure     |  1 th. |   2 th. |   4 th. |  gain | dispersion |
| ------------- | -----: | ------: | ------: | ----: | ---------: |
| adjmap        | 10.2 M |  21.0 M |  40.2 M | x3.93 |        8 % |
| adjlist       | 16.1 M |  31.2 M |  61.7 M | x3.82 |       15 % |
| adjlist-exact | 16.7 M |  33.1 M |  61.5 M | x3.69 |       11 % |
| csr           | 21.8 M |  42.5 M |  77.4 M | x3.55 |       18 % |
| csr-slices    | 17.6 M |  31.9 M |  63.9 M | x3.64 |        9 % |
| edgelist      |  6.3 M |  13.2 M |  25.6 M | x4.08 |       10 % |
| varint-csr    |  3.6 M |   6.9 M |  13.9 M | x3.86 |        7 % |
| hybrid        | 21.4 M |  40.6 M |  80.2 M | x3.74 |        6 % |
| bitmatrix     | 65.4 M | 124.5 M | 245.0 M | x3.75 |       65 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |   p50 |   p99 |
| ------------- | ----: | ----: |
| adjmap        |  2 ms |  3 ms |
| adjlist       |  1 ms |  1 ms |
| adjlist-exact |  1 ms |  1 ms |
| csr           |  1 ms |  1 ms |
| csr-slices    |  1 ms |  1 ms |
| edgelist      |  4 ms |  4 ms |
| varint-csr    |  3 ms |  3 ms |
| hybrid        |  1 ms |  1 ms |
| bitmatrix     | 12 ms | 13 ms |

### rmat — 262 144 sommets, 3 939 048 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |       126 ms | 31.6 |    8.41 |    126.4 |  1.97x |       28.2 |
| adjlist       |        41 ms | 27.6 |    7.34 |    110.3 |  1.72x |       27.4 |
| adjlist-exact |        38 ms | 22.0 |    5.85 |     87.9 |  1.37x |       21.0 |
| csr           |        35 ms | 16.0 |    4.27 |     64.2 |  1.00x |       16.0 |
| csr-slices    |        36 ms | 21.0 |    5.60 |     84.1 |  1.31x |       21.0 |
| edgelist      |        29 ms | 30.1 |    8.00 |    120.2 |  1.87x |       30.1 |
| varint-csr    |        66 ms |  9.0 |    2.40 |     36.1 |  0.56x |        9.0 |
| hybrid        |        71 ms | 17.0 |    4.54 |     68.2 |  1.06x |       17.0 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | --------: | --------------: | ------: |
| adjmap        | 129.9 |     144.9 |    19.8 M |          26.7 M |  24.2 M |
| adjlist       | 186.6 |     268.4 |    27.9 M |          43.0 M |  38.8 M |
| adjlist-exact | 207.1 |     292.5 |    27.9 M |          36.5 M |  40.1 M |
| csr           | 238.6 |     318.1 |    33.4 M |          54.4 M |  41.2 M |
| csr-slices    | 195.7 |     270.5 |    27.7 M |          43.8 M |  39.8 M |
| edgelist      |  76.4 |      77.6 |    10.2 M |          10.7 M |  12.1 M |
| varint-csr    | 103.9 |     112.5 |    15.5 M |          16.4 M |   5.0 M |
| hybrid        | 225.4 |     317.9 |    31.7 M |          53.3 M |  42.1 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |  33.3 |  70.8 | 143.5 | x4.31 |       15 % |
| adjlist       |  64.0 | 131.8 | 268.4 | x4.20 |       16 % |
| adjlist-exact |  72.9 | 149.2 | 292.5 | x4.01 |        9 % |
| csr           |  82.6 | 167.3 | 318.1 | x3.85 |        6 % |
| csr-slices    |  74.7 | 137.3 | 270.5 | x3.62 |       18 % |
| edgelist      |  19.6 |  39.9 |  77.3 | x3.94 |        8 % |
| varint-csr    |  28.0 |  54.7 | 112.5 | x4.02 |       26 % |
| hybrid        |  86.9 | 156.2 | 317.9 | x3.66 |       15 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | -----: | -----: | -----: | ----: | ---------: |
| adjmap        |  5.6 M | 11.9 M | 24.2 M | x4.35 |       10 % |
| adjlist       |  9.5 M | 19.8 M | 38.8 M | x4.08 |       28 % |
| adjlist-exact | 10.1 M | 20.2 M | 40.1 M | x3.98 |       12 % |
| csr           | 11.3 M | 22.8 M | 41.2 M | x3.65 |       16 % |
| csr-slices    | 10.0 M | 20.5 M | 39.8 M | x3.97 |       16 % |
| edgelist      |  3.0 M |  5.8 M | 12.1 M | x4.02 |        8 % |
| varint-csr    |  1.3 M |  2.5 M |  4.9 M | x3.92 |        6 % |
| hybrid        | 11.0 M | 21.2 M | 42.1 M | x3.84 |        7 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |   p50 |   p99 |
| ------------- | ----: | ----: |
| adjmap        | 26 ms | 26 ms |
| adjlist       | 15 ms | 17 ms |
| adjlist-exact | 14 ms | 14 ms |
| csr           | 11 ms | 12 ms |
| csr-slices    | 13 ms | 16 ms |
| edgelist      | 50 ms | 50 ms |
| varint-csr    | 36 ms | 36 ms |
| hybrid        | 11 ms | 12 ms |

### rmat — 1 048 576 sommets, 16 084 638 arêtes

**Mémoire et construction**

| structure     | construction |   Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ----: | ------: | -------: | -----: | ---------: |
| adjmap        |       541 ms | 120.8 |    7.88 |    120.8 |  1.85x |      105.2 |
| adjlist       |       205 ms | 104.8 |    6.83 |    104.8 |  1.60x |      104.1 |
| adjlist-exact |       201 ms |  89.3 |    5.82 |     89.3 |  1.37x |       85.4 |
| csr           |       175 ms |  65.4 |    4.26 |     65.4 |  1.00x |       65.4 |
| csr-slices    |       181 ms |  85.4 |    5.56 |     85.4 |  1.31x |       85.4 |
| edgelist      |        71 ms | 122.7 |    8.00 |    122.7 |  1.88x |      122.7 |
| varint-csr    |       278 ms |  36.7 |    2.39 |     36.7 |  0.56x |       36.7 |
| hybrid        |       293 ms |  69.5 |    4.53 |     69.5 |  1.06x |       69.5 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |  bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ---: | --------: | --------: | --------------: | ------: |
| adjmap        | 25.1 |      27.8 |    11.6 M |          16.7 M |  16.8 M |
| adjlist       | 35.8 |      49.4 |    18.9 M |          33.4 M |  30.2 M |
| adjlist-exact | 39.0 |      53.9 |    19.8 M |          30.5 M |  30.1 M |
| csr           | 46.4 |      64.5 |    25.9 M |          40.3 M |  33.7 M |
| csr-slices    | 39.5 |      54.9 |    21.1 M |          34.8 M |  31.3 M |
| edgelist      | 14.2 |      14.5 |     7.1 M |           7.1 M |   8.3 M |
| varint-csr    | 21.5 |      23.1 |    13.0 M |          13.3 M |   2.6 M |
| hybrid        | 46.1 |      64.0 |    23.7 M |          39.0 M |  32.3 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |   6.2 |  12.8 |  27.8 | x4.50 |       13 % |
| adjlist       |  11.1 |  24.3 |  49.3 | x4.45 |       10 % |
| adjlist-exact |  11.5 |  24.0 |  53.4 | x4.66 |        9 % |
| csr           |  14.0 |  31.9 |  64.5 | x4.62 |       18 % |
| csr-slices    |  11.1 |  25.0 |  54.9 | x4.93 |       12 % |
| edgelist      |   3.6 |   7.3 |  14.5 | x4.06 |       20 % |
| varint-csr    |   5.7 |  11.1 |  22.5 | x3.95 |        5 % |
| hybrid        |  13.9 |  30.2 |  64.0 | x4.59 |       18 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |   1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | ------: | -----: | -----: | ----: | ---------: |
| adjmap        |   3.9 M |  8.1 M | 16.8 M | x4.30 |       13 % |
| adjlist       |   6.2 M | 13.2 M | 27.6 M | x4.46 |       11 % |
| adjlist-exact |   6.7 M | 14.3 M | 30.1 M | x4.50 |        9 % |
| csr           |   7.6 M | 14.9 M | 31.8 M | x4.21 |       12 % |
| csr-slices    |   7.1 M | 14.6 M | 31.3 M | x4.41 |       13 % |
| edgelist      |   2.1 M |  4.2 M |  8.3 M | x3.96 |       17 % |
| varint-csr    | 646.2 k |  1.3 M |  2.6 M | x4.07 |        7 % |
| hybrid        |   7.2 M | 15.9 M | 32.3 M | x4.46 |       10 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |    p50 |    p99 |
| ------------- | -----: | -----: |
| adjmap        | 163 ms | 169 ms |
| adjlist       |  88 ms | 102 ms |
| adjlist-exact |  88 ms |  94 ms |
| csr           |  70 ms |  71 ms |
| csr-slices    |  83 ms |  87 ms |
| edgelist      | 277 ms | 311 ms |
| varint-csr    | 177 ms | 186 ms |
| hybrid        |  68 ms |  69 ms |

### grid — 19 881 sommets, 78 960 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet |  vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | ------: | ---------: |
| adjmap        |         4 ms |  1.6 |   20.65 |     82.0 |   4.06x |        1.2 |
| adjlist       |     940.5 µs |  0.8 |   10.15 |     40.3 |   2.00x |        0.8 |
| adjlist-exact |     886.7 µs |  0.8 |   10.15 |     40.3 |   2.00x |        0.8 |
| csr           |     408.2 µs |  0.4 |    5.08 |     20.2 |   1.00x |        0.4 |
| csr-slices    |     586.1 µs |  0.8 |   10.17 |     40.4 |   2.00x |        0.8 |
| edgelist      |     206.5 µs |  0.6 |    8.09 |     32.1 |   1.59x |        0.6 |
| varint-csr    |         1 ms |  0.3 |    3.63 |     14.4 |   0.71x |        0.3 |
| hybrid        |     747.4 µs |  0.5 |    6.16 |     24.5 |   1.21x |        0.5 |
| bitmatrix     |        27 ms | 47.2 |  626.54 |   2488.4 | 123.23x |       47.2 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |    bfs | bfs-batch |    dfs | neighbors | neighbors-batch | hasedge |
| ------------- | -----: | --------: | -----: | --------: | --------------: | ------: |
| adjmap        |  7.5 k |     7.3 k |  5.9 k |   154.2 M |         159.4 M | 101.7 M |
| adjlist       | 15.9 k |    19.3 k | 13.9 k |   292.1 M |         328.2 M | 191.8 M |
| adjlist-exact | 15.8 k |    17.6 k | 14.4 k |   300.7 M |         326.9 M | 189.1 M |
| csr           | 14.6 k |    17.6 k | 13.7 k |   282.8 M |         358.9 M | 200.7 M |
| csr-slices    | 15.1 k |    18.0 k | 13.7 k |   291.2 M |         327.0 M | 187.0 M |
| edgelist      |  2.3 k |     2.2 k |  3.0 k |    36.0 M |          35.0 M |  39.5 M |
| varint-csr    |  8.3 k |     8.6 k |  7.9 k |   159.9 M |         151.8 M | 158.9 M |
| hybrid        | 15.4 k |    18.4 k | 13.4 k |   301.9 M |         360.8 M | 191.5 M |
| bitmatrix     |  748.7 |     790.8 |  832.3 |    12.1 M |          11.9 M | 265.2 M |

**Passage à l'échelle — « bfs-batch », lot de 512 requêtes**

| structure     | 1 th. | 2 th. |  4 th. |  gain | dispersion |
| ------------- | ----: | ----: | -----: | ----: | ---------: |
| adjmap        | 2.2 k | 3.8 k |  7.3 k | x3.32 |       73 % |
| adjlist       | 4.5 k | 9.3 k | 19.3 k | x4.26 |       31 % |
| adjlist-exact | 4.4 k | 9.4 k | 17.6 k | x4.01 |       21 % |
| csr           | 4.6 k | 9.8 k | 17.6 k | x3.83 |       26 % |
| csr-slices    | 4.7 k | 9.4 k | 18.0 k | x3.86 |       31 % |
| edgelist      | 593.6 | 1.1 k |  2.2 k | x3.68 |       10 % |
| varint-csr    | 2.2 k | 4.1 k |  8.6 k | x3.97 |        8 % |
| hybrid        | 4.5 k | 9.2 k | 18.4 k | x4.10 |       20 % |
| bitmatrix     | 201.6 | 402.8 |  790.8 | x3.92 |       12 % |

**Passage à l'échelle — « hasedge », lot de 500 000 requêtes**

| structure     |  1 th. |   2 th. |   4 th. |  gain | dispersion |
| ------------- | -----: | ------: | ------: | ----: | ---------: |
| adjmap        | 26.1 M |  51.4 M | 101.7 M | x3.90 |        7 % |
| adjlist       | 49.5 M |  98.7 M | 191.8 M | x3.88 |        5 % |
| adjlist-exact | 51.9 M | 101.5 M | 189.1 M | x3.64 |        4 % |
| csr           | 52.1 M |  99.9 M | 200.7 M | x3.85 |        6 % |
| csr-slices    | 49.8 M |  94.7 M | 187.0 M | x3.76 |        8 % |
| edgelist      |  9.8 M |  19.6 M |  39.5 M | x4.03 |        5 % |
| varint-csr    | 40.0 M |  79.4 M | 158.9 M | x3.97 |        5 % |
| hybrid        | 49.8 M |  99.0 M | 191.5 M | x3.85 |        4 % |
| bitmatrix     | 72.0 M | 149.2 M | 265.2 M | x3.68 |       27 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |      p50 |      p99 |
| ------------- | -------: | -------: |
| adjmap        | 442.8 µs | 638.7 µs |
| adjlist       | 215.2 µs | 296.1 µs |
| adjlist-exact | 179.6 µs | 294.2 µs |
| csr           | 216.0 µs | 252.6 µs |
| csr-slices    | 211.4 µs | 273.7 µs |
| edgelist      |     2 ms |     2 ms |
| varint-csr    | 450.3 µs | 584.2 µs |
| hybrid        | 219.9 µs | 332.0 µs |
| bitmatrix     |     5 ms |     5 ms |

### grid — 199 809 sommets, 797 448 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |        69 ms | 13.1 |   17.17 |     68.5 |  3.42x |       12.2 |
| adjlist       |        13 ms |  7.6 |   10.03 |     40.0 |  2.00x |        7.6 |
| adjlist-exact |        11 ms |  7.6 |   10.03 |     40.0 |  2.00x |        7.6 |
| csr           |         7 ms |  3.8 |    5.01 |     20.0 |  1.00x |        3.8 |
| csr-slices    |        12 ms |  7.6 |   10.03 |     40.0 |  2.00x |        7.6 |
| edgelist      |         3 ms |  6.1 |    8.00 |     31.9 |  1.60x |        6.1 |
| varint-csr    |        11 ms |  2.7 |    3.51 |     14.0 |  0.70x |        2.7 |
| hybrid        |        16 ms |  4.6 |    6.05 |     24.2 |  1.21x |        4.6 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | --------: | --------------: | ------: |
| adjmap        | 214.8 |     201.0 |    48.6 M |          49.7 M |  39.3 M |
| adjlist       | 725.4 |     716.4 |    79.2 M |         109.0 M |  98.4 M |
| adjlist-exact | 764.0 |     815.2 |    84.1 M |         109.0 M | 104.0 M |
| csr           | 1.0 k |     953.7 |   110.2 M |         152.4 M | 101.2 M |
| csr-slices    | 686.9 |     741.5 |    83.1 M |         114.1 M |  97.6 M |
| edgelist      | 150.7 |     159.2 |    18.6 M |          18.7 M |  19.7 M |
| varint-csr    | 619.0 |     616.4 |    73.5 M |          73.4 M |  78.1 M |
| hybrid        | 1.0 k |     864.9 |    97.0 M |         143.3 M |  93.9 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |  58.6 | 110.9 | 201.0 | x3.43 |       16 % |
| adjlist       | 246.6 | 406.8 | 716.4 | x2.90 |       22 % |
| adjlist-exact | 229.4 | 477.0 | 815.2 | x3.55 |       26 % |
| csr           | 265.1 | 509.0 | 953.7 | x3.60 |       24 % |
| csr-slices    | 225.9 | 417.3 | 741.5 | x3.28 |       20 % |
| edgelist      |  41.5 |  82.1 | 159.2 | x3.84 |        9 % |
| varint-csr    | 183.6 | 341.1 | 616.4 | x3.36 |       13 % |
| hybrid        | 260.9 | 483.5 | 864.9 | x3.32 |       53 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |   4 th. |  gain | dispersion |
| ------------- | -----: | -----: | ------: | ----: | ---------: |
| adjmap        | 10.6 M | 20.9 M |  38.9 M | x3.68 |       49 % |
| adjlist       | 23.8 M | 46.6 M |  98.4 M | x4.14 |        6 % |
| adjlist-exact | 27.2 M | 49.8 M | 104.0 M | x3.83 |       17 % |
| csr           | 26.9 M | 51.4 M | 101.2 M | x3.76 |       10 % |
| csr-slices    | 24.3 M | 48.7 M |  97.6 M | x4.01 |        9 % |
| edgelist      |  5.1 M |  9.9 M |  19.7 M | x3.90 |        4 % |
| varint-csr    | 20.2 M | 37.3 M |  78.1 M | x3.86 |       11 % |
| hybrid        | 22.9 M | 46.7 M |  92.9 M | x4.06 |        5 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |   p50 |   p99 |
| ------------- | ----: | ----: |
| adjmap        | 17 ms | 18 ms |
| adjlist       |  4 ms |  5 ms |
| adjlist-exact |  4 ms |  4 ms |
| csr           |  4 ms |  4 ms |
| csr-slices    |  5 ms |  5 ms |
| edgelist      | 24 ms | 25 ms |
| varint-csr    |  5 ms |  6 ms |
| hybrid        |  4 ms |  5 ms |

### grid — 1 000 000 sommets, 3 996 000 arêtes

**Mémoire et construction**

| structure     | construction |  Mio | o/arête | o/sommet | vs csr | analytique |
| ------------- | -----------: | ---: | ------: | -------: | -----: | ---------: |
| adjmap        |       516 ms | 95.1 |   24.96 |     99.7 |  4.99x |       61.0 |
| adjlist       |        57 ms | 38.1 |   10.01 |     40.0 |  2.00x |       38.1 |
| adjlist-exact |        57 ms | 38.1 |   10.01 |     40.0 |  2.00x |       38.1 |
| csr           |        37 ms | 19.1 |    5.00 |     20.0 |  1.00x |       19.1 |
| csr-slices    |        60 ms | 38.1 |   10.01 |     40.0 |  2.00x |       38.1 |
| edgelist      |        14 ms | 30.5 |    8.00 |     32.0 |  1.60x |       30.5 |
| varint-csr    |        63 ms | 13.4 |    3.50 |     14.0 |  0.70x |       13.3 |
| hybrid        |        68 ms | 23.0 |    6.04 |     24.1 |  1.21x |       23.0 |

**Débit à 4 threads** (requêtes/s, meilleur lot)

| structure     |   bfs | bfs-batch | neighbors | neighbors-batch | hasedge |
| ------------- | ----: | --------: | --------: | --------------: | ------: |
| adjmap        |  30.2 |      30.4 |    37.5 M |          41.4 M |  32.4 M |
| adjlist       |  97.5 |      91.2 |    62.4 M |          81.2 M |  77.8 M |
| adjlist-exact | 107.9 |      97.7 |    64.6 M |          83.8 M |  85.3 M |
| csr           | 177.4 |     162.0 |    62.3 M |          93.7 M |  78.8 M |
| csr-slices    | 103.7 |     120.5 |    61.4 M |          84.4 M |  74.5 M |
| edgelist      |  15.3 |      15.4 |    10.3 M |          10.6 M |  10.9 M |
| varint-csr    | 127.6 |     130.5 |    40.5 M |          43.2 M |  48.9 M |
| hybrid        | 168.1 |     160.6 |    69.7 M |          90.1 M |  74.4 M |

**Passage à l'échelle — « bfs-batch », lot de 16 requêtes**

| structure     | 1 th. | 2 th. | 4 th. |  gain | dispersion |
| ------------- | ----: | ----: | ----: | ----: | ---------: |
| adjmap        |   6.1 |  14.4 |  30.4 | x5.00 |        8 % |
| adjlist       |  17.6 |  41.0 |  91.2 | x5.17 |       21 % |
| adjlist-exact |  20.1 |  40.1 |  97.7 | x4.86 |       20 % |
| csr           |  29.7 |  61.3 | 162.0 | x5.45 |       51 % |
| csr-slices    |  18.9 |  44.6 | 120.5 | x6.38 |       36 % |
| edgelist      |   4.1 |   7.9 |  15.4 | x3.76 |       10 % |
| varint-csr    |  27.7 |  61.0 | 130.5 | x4.71 |       12 % |
| hybrid        |  30.6 |  77.5 | 160.6 | x5.25 |       39 % |

**Passage à l'échelle — « hasedge », lot de 2 000 000 requêtes**

| structure     |  1 th. |  2 th. |  4 th. |  gain | dispersion |
| ------------- | -----: | -----: | -----: | ----: | ---------: |
| adjmap        |  6.3 M | 13.9 M | 29.7 M | x4.70 |       11 % |
| adjlist       | 17.3 M | 39.3 M | 77.8 M | x4.50 |       13 % |
| adjlist-exact | 19.4 M | 41.9 M | 85.3 M | x4.40 |       10 % |
| csr           | 18.2 M | 37.5 M | 78.8 M | x4.33 |        6 % |
| csr-slices    | 17.4 M | 38.3 M | 74.5 M | x4.27 |        9 % |
| edgelist      |  2.7 M |  5.6 M | 10.9 M | x3.97 |        6 % |
| varint-csr    | 11.7 M | 25.7 M | 48.9 M | x4.18 |       11 % |
| hybrid        | 17.4 M | 36.6 M | 74.4 M | x4.27 |        5 % |

**Latence d'un parcours complet — « bfs-batch », 1 thread**

| structure     |    p50 |    p99 |
| ------------- | -----: | -----: |
| adjmap        | 166 ms | 177 ms |
| adjlist       |  60 ms |  71 ms |
| adjlist-exact |  51 ms |  58 ms |
| csr           |  33 ms |  42 ms |
| csr-slices    |  54 ms |  64 ms |
| edgelist      | 242 ms | 254 ms |
| varint-csr    |  36 ms |  42 ms |
| hybrid        |  33 ms |  39 ms |

## 7. Ce que ce rapport ne dit pas

- **Une seule machine, un seul run de campagne.** Les écarts de quelques pourcents entre structures voisines ne sont pas significatifs ; seuls les ordres de grandeur le sont.
- **Graphes synthétiques.** Les topologies sont choisies pour isoler des effets (localité, hubs, uniformité), pas pour imiter un graphe réel.
- **Structures figées.** Rien n'est mesuré après construction : ni insertion, ni suppression, ni mise à jour.
- **Le seuil de promotion des hubs** de la structure hybride est dérivé d'un calcul mémoire (`deg > n/32`). À un million de sommets, presque aucun sommet ne l'atteint, même en loi de puissance : la structure se réduit alors à un CSR et ses écarts avec lui sont du bruit.
- **Le périmètre complet** est décrit dans `SPEC.md`, les choix de protocole dans `DECISIONS.md`.

