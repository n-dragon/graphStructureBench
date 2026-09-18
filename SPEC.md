# Spécification du banc

Document de référence : ce que le banc mesure, comment, et ce qu'il ne mesure
pas. Les décisions qui ont mené à ces choix sont historisées dans
[DECISIONS.md](DECISIONS.md).

## 1. Objectif

Comparer plusieurs **structures d'index de graphe** sur leur capacité à
**parcourir efficacement un graphe**, selon deux critères :

1. **Rapidité** — débit en requêtes/seconde, latences p50 et p99, temps de
   construction de l'index.
2. **Consommation mémoire** — empreinte de l'index en mémoire, ramenée à
   l'arête et au sommet, plus le coût par thread des tampons de parcours.

Le banc doit permettre de répondre à : *pour tel graphe, telle charge et tel
niveau de parallélisme, quelle structure choisir, et ce que coûte ce choix en
mémoire ?*

## 2. Axes des scénarios

Les scénarios sont le **produit cartésien** de trois axes, balayé en entier.

| # | axe | valeurs typiques | drapeau |
|---|---|---|---|
| 1 | **nombre de sommets** | 20 000 / 200 000 / 1 000 000 | `-nodes` |
| 2 | **requêtes concurrentes** (taille du lot à absorber) | 4 / 64 / 200 000 / 2 000 000 | `-queries` |
| 3 | **nombre de threads** | 1 / 2 / 4 | `-threads` |

Ce produit est lui-même croisé avec la **topologie** du graphe (`-kinds`) et la
**charge de travail** (`-workloads`), ce qui donne l'espace de mesure complet :

```
topologie × sommets × requêtes × threads × charge × structure
```

Le volume de requêtes n'a pas le même sens selon la charge : un BFS complet se
mesure par dizaines, un test d'arête par millions. Les valeurs par défaut sont
donc propres à chaque charge, et `-queries` les remplace quand on veut imposer
l'axe.

## 3. Structures à comparer

Neuf représentations, toutes derrière l'interface `graph.Graph`.

| structure | représentation | coût théorique | `HasEdge` |
|---|---|---|---|
| `adjmap` | `map[uint32][]uint32` | ~48 o/sommet + 4 o/arête | hachage + O(log d) |
| `adjlist` | `[][]uint32` par `append` | 24 o/sommet + jusqu'à 8 o/arête | O(log d) |
| `adjlist-exact` | `[][]uint32` dimensionné | 24 o/sommet + 4 o/arête | O(log d) |
| `csr` | `offsets` + `targets` | 4 o/sommet + 4 o/arête | O(log d) |
| `csr-slices` | arène + en-têtes de slice | 24 o/sommet + 4 o/arête | O(log d) |
| `edgelist` | `[]uint64` trié | 8 o/arête, 0 o/sommet | O(log m) |
| `varint-csr` | écarts en varint LEB128 | 4 o/sommet + 1 à 5 o/arête | O(d) |
| `hybrid` | bitmaps des hubs + CSR | CSR + n/8 o par hub | O(1) sur un hub |
| `bitmatrix` | matrice dense | n²/8 octets | O(1) |

Règle du mode hybride : un sommet coûte `4·deg` octets en CSR contre `n/8` en
bitmap, donc le bitmap est promu **dès que `deg > n/32`**, sous plafond global
(les bitmaps ne dépassent jamais la taille du tableau de cibles remplacé).

## 4. Charges de travail

| charge | requête unitaire | ce qu'elle isole |
|---|---|---|
| `bfs` | parcours en largeur complet depuis une source | parcours réel, accès en cascade |
| `bfs-batch` | le même, en lisant l'adjacence par blocs | ce que coûte l'abstraction sur un parcours réel |
| `dfs` | parcours en profondeur itératif | même chose, ordre d'accès différent |
| `neighbors` | lecture de l'adjacence d'un sommet | coût d'accès pur, sans le bruit du parcours |
| `neighbors-batch` | la même, par bloc sans appel indirect | coût d'accès sans le prix de l'itération |
| `hasedge` | test d'existence, 50 % présentes / 50 % absentes | qualité de l'index en recherche |

Les sources de parcours sont tirées parmi les sommets de degré non nul : partir
d'un sommet isolé mesurerait la boucle du runner, pas la structure.

## 5. Topologies

| topologie | forme | ce qu'elle met à l'épreuve |
|---|---|---|
| `er` | Erdős–Rényi, degrés homogènes | pire cas cache : aucun voisin n'est prévisible |
| `rmat` | loi de puissance (paramètres Graph500) | structures qui traitent tous les sommets pareil |
| `grid` | grille 2D à 4 voisins | récompense la compression par écarts |

Les arêtes sont triées et dédupliquées : toutes les structures voient le même
ensemble. Le graphe est symétrisé par défaut (`-directed` pour ne pas le faire).
`rmat` arrondit le nombre de sommets à la puissance de deux supérieure, `grid`
au carré parfait le plus proche.

## 6. Protocole de mesure

### Mémoire

- **Mesure de référence** : delta de `runtime.MemStats.HeapAlloc` entre avant et
  après construction, GC forcé des deux côtés. Elle attrape tout, y compris ce
  qu'une table de hachage ne déclare pas.
- **Contrôle** : compte analytique `MemoryBytes()` déclaré par la structure.
- La **liste d'arêtes source** est allouée avant la mesure : elle n'entre pas
  dans le delta.
- Les **tampons de parcours** (bitset de marquage, file, pile) sont comptés
  séparément : c'est un coût **par thread**, pas par index.

### Temps

- Les requêtes sont distribuées aux goroutines par un **compteur atomique** :
  le modèle d'un service qui encaisse du trafic sur un index partagé en lecture
  seule.
- `GOMAXPROCS` est **fixé à la valeur de l'axe threads** pendant la mesure.
- Chaque point : une passe de **chauffe**, puis `-runs` passes chronométrées
  (3 par défaut), **meilleur temps retenu**.
- Les requêtes de quelques dizaines de nanosecondes sont chronométrées **par
  lots** (64 pour `neighbors`, 256 pour `hasedge`) : la latence est alors
  annotée `p50 = 12 µs/256`.

### Équité et mode d'accès

L'adjacence se lit de deux façons, toutes deux exposées par l'interface et
mesurées séparément :

- **par callback** (`ForEachNeighbor`) — un appel indirect par voisin ;
- **par bloc** (`AppendNeighbors`) — la structure remplit un tampon fourni
  par l'appelant, qui le parcourt ensuite sans indirection.

Le second mode existe parce que le premier coûte environ 13 ns par voisin :
une charge de lecture d'adjacence mesurée uniquement par callback dit plus de
choses sur l'abstraction que sur la structure. `BenchmarkCallbackOverhead`
compare les deux modes et, pour les structures qui savent rendre une slice, la
boucle directe — ce qui donne la borne basse.

Aucune structure ne bénéficie d'un traitement de faveur : les neuf implémentent
les deux modes.

## 7. Invariants de correction

Comparer des débits n'a de sens que si les structures répondent la même chose.

1. `go test ./...` compare les neuf structures à une implémentation de
   référence naïve : listes d'adjacence, degrés, `HasEdge` sur des couples
   présents et absents, BFS et DFS.
2. Pendant le banc, chaque résultat porte une **somme de contrôle commutative**
   (somme des identifiants visités, insensible à l'ordonnancement des threads).
   Toute divergence entre structures est signalée sur la sortie d'erreur.
3. Les index sont **immuables après construction** : un test dédié vérifie que
   des lectures concurrentes donnent le même résultat que des lectures
   séquentielles.

## 8. Livrables

| livrable | rôle |
|---|---|
| `cmd/gsbench` | ligne de commande, balayage du produit cartésien |
| `go test ./bench -bench ... -cpu=1,2,4` | benchmarks Go standards, axe threads via `-cpu` |
| `scripts/run-bench.sh` | campagne reproductible en quatre volets |
| `results/*.txt`, `*.md`, `*.csv` | sorties brutes (régénérables, non versionnées) |
| `RESULTS.md` | rapport retenu, versionné |
| `README.md` | mode d'emploi et lecture des résultats |

## 9. Hors périmètre

- **Graphes dynamiques** : les structures sont construites puis figées. Aucune
  insertion ni suppression après construction n'est mesurée.
- **Persistance disque et graphes hors-mémoire** : tout tient en RAM.
- **Attributs** sur les sommets et les arêtes (poids, étiquettes).
- **Algorithmes de plus court chemin pondéré, PageRank, composantes** : le banc
  s'en tient aux primitives de parcours, qui sont ce que les structures
  d'index déterminent réellement.
- **Renumérotation des sommets** (reordering par localité) : elle avantagerait
  fortement `varint-csr` et mérite son propre banc.
