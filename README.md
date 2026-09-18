# graphStructureBench

Banc de comparaison, en Go, de **neuf structures d'index** pour représenter un
graphe et le parcourir. Deux critères : **rapidité** et **consommation
mémoire**.

Les scénarios balayent le produit cartésien de trois axes :

| axe | ce qu'il fait varier | drapeau |
|---|---|---|
| **sommets** | taille du graphe (et donc du working set vs les caches CPU) | `-nodes 200000,1000000` |
| **requêtes concurrentes** | taille du lot de requêtes à absorber | `-queries 200000,2000000` |
| **threads** | goroutines qui tapent en parallèle dans l'index partagé | `-threads 1,2,4` |

Le tout est croisé avec trois topologies de graphe (uniforme, loi de puissance,
grille) et quatre charges de travail (BFS, DFS, lecture d'adjacence, test
d'existence d'arête).

**Documents** : [SPEC.md](SPEC.md) — ce que le banc mesure et comment ·
[DECISIONS.md](DECISIONS.md) — journal des décisions de conception ·
`RESULTS.md` — rapport de la campagne retenue.

## Démarrage

```bash
go test ./...                 # vérifie que les 9 structures répondent la même chose
./scripts/run-bench.sh        # campagne complète, ~25 min, résultats dans results/
go run ./cmd/gsbench -list    # catalogue des structures et des charges

# un point de mesure précis
go run ./cmd/gsbench -kinds rmat -nodes 1000000 -threads 1,2,4 \
  -queries 200000,2000000 -workloads hasedge
```

Benchmarks Go standards également disponibles, l'axe threads passant par `-cpu` :

```bash
go test ./bench -bench 'Neighbors|HasEdge|BFS' -cpu=1,2,4 -benchtime=2s
go test ./bench -bench Build -benchmem
```

## Les structures comparées

Toutes implémentent la même interface `graph.Graph` (`ForEachNeighbor`,
`HasEdge`, `Degree`, `MemoryBytes`), donc les parcours sont écrits **une seule
fois** : d'une mesure à l'autre, seule la structure d'index change.

| structure | représentation | coût mémoire théorique | `HasEdge` |
|---|---|---|---|
| `adjmap` | `map[uint32][]uint32` | ~48 o/sommet + 4 o/arête | hachage + O(log d) |
| `adjlist` | `[][]uint32` rempli par `append` | 24 o/sommet + jusqu'à 8 o/arête | O(log d) |
| `adjlist-exact` | `[][]uint32` dimensionné exactement | 24 o/sommet + 4 o/arête | O(log d) |
| `csr` | `offsets []uint32` + `targets []uint32` | 4 o/sommet + 4 o/arête | O(log d) |
| `csr-slices` | arène plate + un en-tête de slice par sommet | 24 o/sommet + 4 o/arête | O(log d) |
| `edgelist` | `[]uint64` trié (`src<<32\|dst`) | 8 o/arête, **0 o/sommet** | O(log m) |
| `varint-csr` | CSR compressé, écarts en varint LEB128 | 4 o/sommet + 1 à 5 o/arête | O(d) décodage |
| `hybrid` | bitmaps pour les hubs, CSR pour le reste | CSR + n/8 o par hub | **O(1)** sur un hub |
| `bitmatrix` | matrice d'adjacence dense en bitset | **n²/8 octets** | **O(1)** |

Quelques points de conception qui décident du résultat :

- **CSR** n'a aucun pointeur : le GC n'a rien à tracer, et un parcours lit la
  mémoire de façon séquentielle. C'est la référence à battre.
- **`adjlist` vs `adjlist-exact`** isole un seul effet : la capacité laissée en
  trop par la croissance géométrique d'`append`. Même code, même vitesse, pas
  la même facture mémoire.
- **`csr-slices`** garde l'arène plate du CSR mais remplace le tableau
  d'offsets par des en-têtes de slice : +24 octets par sommet, contre une
  indirection en moins.
- **`edgelist`** ne paie strictement rien par sommet — la bonne structure quand
  l'espace des identifiants est très creux — mais commence chaque accès par une
  recherche binaire sur les m arêtes.
- **`varint-csr`** troque du CPU (décodage) contre de la bande passante
  mémoire. Le pari est gagnant quand les voisins ont des identifiants proches,
  c'est-à-dire sur les graphes à forte localité comme la grille.
- **`hybrid`** applique une règle simple : un sommet coûte `4·deg` octets en CSR
  et `n/8` en bitmap, donc le bitmap devient plus compact **dès que
  `deg > n/32`** — exactement le régime des hubs d'un graphe en loi de
  puissance, où il fait tomber `HasEdge` en O(1).
- **`bitmatrix`** est là comme borne : imbattable en latence, inutilisable
  au-delà de quelques dizaines de milliers de sommets (12,5 Gio pour un million
  de sommets). Le runner la saute automatiquement.

## Méthodologie

**Correction d'abord.** `go test ./...` vérifie que les neuf structures rendent
les mêmes listes d'adjacence, les mêmes réponses à `HasEdge` et les mêmes
parcours qu'une implémentation de référence naïve. Pendant le banc, chaque
résultat porte une somme de contrôle (somme des identifiants visités, donc
insensible à l'ordonnancement des threads) : toute divergence entre structures
est signalée.

**Mémoire.** Deux chiffres sont rapportés :

- le **delta de tas** (`runtime.MemStats.HeapAlloc` avant/après construction,
  GC forcé des deux côtés) — c'est la mesure de référence, elle attrape tout,
  y compris ce que les tables de hachage ne déclarent pas ;
- le compte **analytique** déclaré par chaque structure (`MemoryBytes()`),
  qui sert de contrôle de cohérence.

La liste d'arêtes source est allouée **avant** la mesure : elle n'entre pas
dans le delta. Le coût des tampons de parcours (bitset de marquage + file),
lui, est compté à part : c'est un coût **par thread**, pas par index.

**Temps.** Les requêtes sont distribuées aux goroutines par un compteur
atomique, comme un service qui encaisse du trafic sur un index partagé en
lecture seule. `GOMAXPROCS` est fixé à la valeur de l'axe « threads » pendant
la mesure, pour que le parallélisme soit réellement borné. Chaque point est
précédé d'une passe de chauffe, puis répété (`-runs`, 3 par défaut) : le
meilleur temps est retenu. Les requêtes très courtes (quelques dizaines de
nanosecondes) sont chronométrées par lots — la colonne de latence indique le
lot, par exemple `p50 = 12 µs/256`.

**Équité.** Toutes les structures sont lues via le même callback
`ForEachNeighbor`. C'est un léger handicap pour celles qui pourraient rendre
une slice directement ; `BenchmarkCallbackOverhead` chiffre précisément ce
handicap, pour pouvoir le déduire mentalement des tableaux.

## Topologies

| topologie | forme | ce qu'elle met à l'épreuve |
|---|---|---|
| `er` (Erdős–Rényi) | degrés homogènes, aucune localité | le pire cas pour les caches : chaque voisin est un défaut de cache |
| `rmat` (Graph500) | loi de puissance, quelques hubs énormes | les structures qui traitent tous les sommets pareil |
| `grid` | grille 2D à 4 voisins | récompense les représentations compressées par écarts |

Les arêtes sont triées et dédupliquées à la génération : toutes les structures
voient exactement le même ensemble. Par défaut le graphe est symétrisé
(`-directed` pour ne pas le faire).

## Organisation du code

```
graph/      les neuf structures d'index, derrière une interface commune
gen/        générateurs de graphes (er, rmat, grid)
traverse/   BFS, DFS, lecture d'adjacence — écrits contre l'interface
bench/      charges de travail, exécution concurrente, mesures, rapports
cmd/gsbench/ ligne de commande et balayage du produit cartésien
scripts/    campagne reproductible
results/    sorties de la campagne (texte, markdown, CSV brut)
```
