# Journal des décisions

Historique des choix de conception du banc : ce qu'on a décidé, pourquoi, et ce
que ça coûte. Les décisions ouvertes sont en fin de document.

**Format d'une entrée.** Numéro, titre, date, origine (*demandé* = venu de la
demande initiale ou d'un message en cours de route, *proposé* = proposition
retenue par défaut faute d'objection), puis contexte / décision / conséquence.
Une décision annulée n'est pas effacée : elle est marquée ~~barrée~~ et
renvoie à celle qui la remplace.

---

## 1. Go, et un module unique par couche

*2026-09-18 — demandé (langage), proposé (découpage)*

**Contexte.** Le banc doit comparer des structures sur vitesse et mémoire.

**Décision.** Go 1.24, un module, et une séparation stricte :
`graph/` (les structures) · `gen/` (génération de graphes) · `traverse/`
(algorithmes) · `bench/` (mesure et rapports) · `cmd/gsbench/` (CLI).

**Conséquence.** Les algorithmes de parcours sont écrits **une seule fois**,
contre l'interface : d'une mesure à l'autre, seule la structure d'index change.
C'est ce qui rend les chiffres comparables. Le prix est une interface imposée à
toutes les structures (voir décision 3).

---

## 2. Neuf structures, dont deux paires de variantes proches

*2026-09-18 — proposé*

**Décision.** `adjmap`, `adjlist`, `adjlist-exact`, `csr`, `csr-slices`,
`edgelist`, `varint-csr`, `hybrid`, `bitmatrix`.

Deux paires sont volontairement très proches pour **isoler un seul effet** :

- `adjlist` vs `adjlist-exact` : même code, même vitesse, mais la croissance
  géométrique d'`append` laisse de la capacité inutilisée. L'écart mesure
  exactement ce gaspillage.
- `csr` vs `csr-slices` : même arène plate, mais le tableau d'offsets est
  remplacé par un en-tête de slice par sommet. L'écart mesure le prix des 24
  octets par sommet contre une indirection en moins.

**Conséquence.** Deux lignes du tableau apportent peu en valeur absolue mais
répondent à une question précise qu'on se pose toujours en optimisant.

---

## 3. Une interface commune avec callback, et la mesure de son coût

*2026-09-18 — proposé*

**Contexte.** `csr` pourrait rendre directement une slice d'adjacence,
`varint-csr` et `bitmatrix` en sont incapables : leurs voisins n'existent pas
en mémoire sous cette forme.

**Décision.** Tout le monde passe par `ForEachNeighbor(u, func(v) bool)`, y
compris ceux qui pourraient faire mieux. En contrepartie,
`BenchmarkCallbackOverhead` chiffre le handicap sur les structures qui savent
rendre une slice.

**Conséquence.** Léger désavantage pour `csr` et les listes d'adjacence dans
les tableaux, mais un désavantage **connu et mesuré**, donc déductible. Sans
cette uniformité, on comparerait des interfaces et non des structures.

---

## 4. Seuil du mode hybride dérivé, pas réglé à la main

*2026-09-18 — proposé*

**Contexte.** `hybrid` doit décider quels sommets passent en bitmap.

**Décision.** Pas de constante arbitraire : un sommet coûte `4·deg` octets en
CSR contre `n/8` en bitmap, donc on promeut **dès que `deg > n/32`**, avec un
plafond global (les bitmaps ne dépassent jamais le tableau de cibles remplacé).

**Conséquence.** Sur un graphe uniforme, aucun sommet ne franchit le seuil :
`hybrid` se réduit à `csr` plus un bitset, et c'est un **résultat**, pas un
échec — la structure ne se déclenche que lorsqu'elle est justifiée. Sur une
loi de puissance, les hubs sont promus et `HasEdge` tombe en O(1).

---

## 5. La matrice dense est plafonnée, pas retirée

*2026-09-18 — proposé*

**Décision.** `bitmatrix` est limitée à 32 768 sommets (et 1 Gio) ; au-delà, le
runner la saute avec un message.

**Conséquence.** On garde la **borne basse en latence** sur les petits graphes,
sans faire exploser les campagnes à grande échelle (12,5 Gio pour un million de
sommets).

---

## 6. Mémoire : delta de tas mesuré, compte analytique en contrôle

*2026-09-18 — proposé*

**Contexte.** Compter les octets à la main ne marche pas pour `adjmap` : la
structure interne des maps Go n'est pas exposée.

**Décision.** La mesure de référence est le **delta de `HeapAlloc`** après GC
forcé de part et d'autre de la construction. Chaque structure déclare en plus
un compte analytique `MemoryBytes()` qui sert de contrôle de cohérence. La
liste d'arêtes source est allouée **avant** la mesure.

**Conséquence.** Les deux colonnes apparaissent côte à côte dans le rapport ;
un écart marqué signale soit un surcoût caché, soit un compte analytique faux.

---

## 7. Les tampons de parcours sont comptés à part

*2026-09-18 — proposé*

**Contexte.** Un BFS a besoin d'un bitset de marquage et d'une file. Sur un
million de sommets, c'est de l'ordre de 8 octets par sommet **et par thread**,
ce qui peut dépasser l'index lui-même.

**Décision.** Un `Scratch` par goroutine, réutilisé entre requêtes, et sa taille
rapportée dans une colonne séparée.

**Conséquence.** Le critère mémoire distingue ce qui se partage (l'index) de ce
qui se multiplie par le nombre de threads. La mesure porte sur le parcours et
non sur l'allocateur. Corollaire : le `reset` n'efface que les bits réellement
posés, sinon chaque BFS paierait un balayage complet du bitset.

---

## 8. Somme de contrôle commutative comme garde-fou permanent

*2026-09-18 — proposé*

**Décision.** Chaque requête renvoie une valeur, sommée par thread puis entre
threads (addition commutative, donc insensible à l'ordonnancement). Toute
divergence entre structures sur un même jeu de requêtes est signalée.

**Conséquence.** Une structure qui irait vite en répondant faux est détectée
pendant la campagne, pas seulement dans les tests unitaires.

---

## 9. Les scénarios balayent sommets × requêtes concurrentes × threads

*2026-09-18 — demandé (message en cours de route)*

**Décision.** Le runner balaye le produit cartésien des trois axes, croisé avec
topologie et charge. L'axe threads borne aussi `GOMAXPROCS` pendant la mesure,
pour que le parallélisme soit réellement limité et non subi.

**Conséquence.** On voit non seulement qui est le plus rapide, mais **qui
passe à l'échelle** : une structure peut gagner à 1 thread et décrocher à 4
(défauts de cache, pression GC). C'est ce que donne la colonne « scaling ».

---

## 10. Volumes de requêtes propres à chaque charge

*2026-09-18 — proposé, après un premier jet raté*

**Contexte.** Premier jet de campagne : 64 et 512 requêtes pour toutes les
charges. Sur `hasedge`, un lot de 512 requêtes de ~50 ns ne chronométrait que
le démarrage des goroutines.

**Décision.** Chaque charge porte ses volumes par défaut (`bfs` : 8 et 64 ;
`hasedge` et `neighbors` : 100 k et 1 M), `-queries` les remplace quand on veut
imposer l'axe. La campagne est découpée en quatre volets aux volumes adaptés.

**Conséquence.** Les lots minuscules restent mesurables si on les demande
explicitement — et ils disent quelque chose de vrai : à ce volume, le
parallélisme ne rentabilise pas son coût de mise en place.

---

## 11. Latence des requêtes courtes mesurée par lots

*2026-09-18 — proposé*

**Contexte.** Chronométrer une requête de 50 ns coûte plus cher que la requête.

**Décision.** Les charges légères sont chronométrées par lots (64 pour
`neighbors`, 256 pour `hasedge`) et la latence est annotée en conséquence :
`p50 = 12 µs/256`.

**Conséquence.** Les p50 et p99 ne sont pas des latences par requête ; le
rapport le dit explicitement plutôt que d'afficher un chiffre faussement précis.

---

## 12. Graphes synthétiques, triés et dédupliqués

*2026-09-18 — proposé*

**Décision.** Trois topologies (`er`, `rmat`, `grid`), arêtes triées et
dédupliquées à la génération, graphe symétrisé par défaut. `rmat` arrondit le
nombre de sommets à la puissance de deux supérieure, `grid` au carré parfait.

**Conséquence.** Toutes les structures voient exactement le même ensemble
d'arêtes, donc `NumEdges` est comparable. Le nombre de sommets demandé n'est
pas toujours celui obtenu : le rapport affiche le nombre réel.

---

## 13. Sorties de campagne hors du dépôt, rapport retenu versionné

*2026-09-18 — proposé*

**Décision.** `results/` est ignoré par git (régénérable par le script) ; le
rapport retenu est versionné dans `RESULTS.md`.

**Conséquence.** Pas de CSV de plusieurs milliers de lignes dans l'historique,
mais un point de comparaison stable dans le temps.

---

## 14. Documentation et commentaires en français

*2026-09-18 — proposé*

**Décision.** Le projet est documenté et commenté en français, les
identifiants restent en anglais (`Graph`, `ForEachNeighbor`, `HasEdge`).

**Conséquence.** Le code reste lisible par un outillage Go standard, les
explications restent dans la langue du projet.

---

## 15. Les fermetures de parcours sont construites une fois par worker

*2026-09-18 — proposé, après constat de mesure*

**Contexte.** La première campagne donnait un passage à l'échelle
**superlinéaire** (×4,8 sur 4 cœurs), ce qui n'a pas de sens physique.
L'analyse d'échappement (`go build -gcflags=-m`) a montré pourquoi : écrite à
l'intérieur de la fonction de parcours, la fermeture passée à
`ForEachNeighbor` s'échappait sur le tas **à chaque appel** — l'analyse
d'échappement ne traverse pas un appel de méthode d'interface. La charge
`neighbors` payait deux allocations par requête, et à 1 thread le GC
concurrent se disputait le seul P disponible.

**Décision.** Les trois fermetures (BFS, DFS, somme) sont construites une fois
dans `NewScratch` et stockées dans la structure ; leur accumulateur est un
champ du `Scratch`.

**Conséquence.** Zéro allocation par requête, vérifié par `-benchmem`. Le
scaling superlinéaire disparaît, et les chiffres de la première campagne sont
caducs. Leçon retenue : un résultat physiquement impossible est un défaut de
protocole, pas une bonne nouvelle.

---

## 16. Deux façons de lire l'adjacence, toutes deux mesurées

*2026-09-18 — proposé, après constat de mesure*

**Contexte.** `BenchmarkCallbackOverhead` a chiffré le prix de
`ForEachNeighbor` une fois les allocations supprimées : **252 ns contre 42 ns**
sur `csr` pour une lecture d'adjacence, soit environ 13 ns par voisin rien
qu'en appel indirect. À ce niveau, la charge `neighbors` mesurait
l'abstraction plus que la structure, et le coût uniforme **comprimait** les
écarts entre structures.

**Décision.** L'interface gagne `AppendNeighbors(dst, u) []uint32` : la
structure remplit un tampon fourni par l'appelant, qui le parcourt ensuite
sans indirection. Chaque structure la spécialise (memmove pour un CSR,
décodage pour le varint, balayage de bits pour les bitmaps). Les charges
`bfs-batch` et `neighbors-batch` mesurent ce second mode, à côté des charges
par callback.

**Alternative écartée.** Garder le seul callback et documenter son coût :
honnête, mais on aurait publié des écarts comprimés par une constante commune.

**Conséquence.** +10 lignes par structure, et le mode d'accès devient une
**dimension mesurée** du banc plutôt qu'un biais. C'est en soi un résultat sur
la conception d'un index : la forme de l'API d'itération pèse autant que la
disposition des données.

---

## 17. Liste d'arêtes dédupliquée en précondition, pas en tolérance

*2026-09-18 — proposé, mis au jour par les tests*

**Contexte.** En écrivant les tests par structure, une divergence est apparue
sur une liste comportant des doublons : un CSR conserve les deux exemplaires,
une représentation par bitmap les fusionne. `NumEdges` et la somme des degrés
ne coïncident alors plus, selon la structure.

**Décision.** La déduplication devient une **précondition explicite** des
constructeurs, outillée par `graph.Dedup` (remontée du paquet `gen`, où elle
était privée), et documentée sur `Builders`. Un test la vérifie et un autre
documente la divergence qu'elle évite.

**Alternative écartée.** Dédupliquer dans chaque constructeur : cela aurait
ajouté un tri à chaque construction, donc faussé la mesure du temps de
construction — un des chiffres du banc.

**Conséquence.** Le contrat est net, et le coût de la déduplication est payé
une fois, du côté de l'appelant.

---

## 18. Mesurer la dispersion plutôt que de supposer la stabilité

*2026-09-22 — proposé, après enquête sur un résultat impossible*

**Contexte.** Malgré la décision 15, la campagne affichait encore des gains
superlinéaires : **299 points sur 804 dépassaient x4,2 sur 4 cœurs**. Quatre
hypothèses ont été testées et écartées par la mesure :

1. `GOMAXPROCS=1` pénaliserait un worker unique → non, ~5 % d'écart ;
2. le surcoût serait hors des lots chronométrés (démarrage des goroutines,
   allocation des tampons) → non, 91 % du temps est bien dans les lots ;
3. un cycle de GC tomberait pendant la mesure → non, zéro cycle observé ;
4. l'ordre de mesure pénaliserait le premier point → non, inverser l'ordre ne
   déplace pas l'anomalie.

La cause est ailleurs : **le point à 1 thread est bruité de ±15 %** (16,5 à
19,1 M req/s d'une répétition à l'autre) quand celui à 4 threads est stable.
Comparer deux « meilleurs de 3 » tirés indépendamment transforme cette variance
en gain apparent.

**Décision.** Trois changements :

- les lots chronométrés passent de 64 à 512 requêtes (et de 256 à 2048 pour le
  test d'arête), pour qu'un lot dure de l'ordre de 10 µs et que le coût des
  deux appels à `time.Now()` sorte de la mesure ;
- le nombre de répétitions passe de 3 à 5 ;
- chaque point porte sa **dispersion** (rapport de la plus lente à la plus
  rapide des répétitions), exportée en CSV et affichée dans les tableaux.

**Conséquence.** Les gains retombent dans le domaine physique, et surtout le
rapport ne prétend plus à une précision qu'il n'a pas : la dispersion est
publiée à côté de chaque comparaison, et le texte engendré dit explicitement
en dessous de quel écart deux structures ne sont pas départageables.

**Leçon.** La première correction (décision 15) avait supprimé une vraie cause
sans supprimer le symptôme. Il a fallu se rendre à l'évidence que le protocole
était encore en cause plutôt que de considérer l'affaire classée.

**Diagnostic incomplet**, complété par la décision 19 : la dispersion est bien
réelle, mais elle n'explique qu'une partie du dépassement.

---

## 19. Le dépassement de x4 est un résultat, pas un artefact

*2026-09-22 — proposé, après mesure du gradient*

**Contexte.** Après la décision 18, les gains restaient au-dessus de x4 sur une
partie des points. Restait à savoir si c'était encore du bruit. Deux mesures
ont tranché :

- le débit **par thread** à 1 thread vaut 1,02 fois celui à 4 threads en
  médiane sur 450 points : le parallélisme rend donc bien à peu près x4, et
  la dispersion ne suffit pas à expliquer les dépassements ;
- le gain suit un **gradient net selon la taille de l'index** : x3,64 quand il
  tient en cache (0,4 Mio), x3,89 à 13 Mio, x4,33 à 65 Mio.

**Décision.** Le dépassement est du **parallélisme mémoire** et il est publié
comme tel, avec le gradient qui l'établit. Un cœur ne peut avoir qu'une dizaine
de défauts de cache en vol ; une lecture d'adjacence aléatoire dans un index de
65 Mio est limitée par cette latence et non par le calcul, si bien que quatre
cœurs multiplient par quatre le nombre de requêtes mémoire en vol et que le
débit agrégé progresse plus que proportionnellement.

**Conséquence.** Le rapport ne présente plus ces valeurs comme une anomalie à
excuser mais comme une observation utile : sur un index qui ne tient pas en
cache, ajouter des threads paie mieux que ne le laisse croire le nombre de
cœurs. La dispersion reste publiée à côté, pour ce qu'elle explique vraiment.

**Leçon.** Deux corrections de protocole (décisions 15 et 18) étaient
justifiées et ont amélioré la mesure ; la troisième explication n'était pas une
correction à faire mais un phénomène à comprendre. Il fallait mesurer le
gradient pour distinguer les deux, et non choisir entre « c'est du bruit » et
« c'est impossible ».

---

## Décisions ouvertes

| sujet | état |
|---|---|
| **Structures dynamiques** (insertion/suppression après construction) | hors périmètre pour l'instant ; demanderait une famille de structures et des charges de mise à jour |
| **Renumérotation des sommets par localité** | avantagerait fortement `varint-csr` ; mérite son propre volet plutôt qu'un biais silencieux |
| **Graphes réels** (SNAP, Graph500) | les générateurs synthétiques suffisent à isoler les effets ; un jeu réel validerait les conclusions |
| **Parcours parallèle d'un même BFS** | le banc parallélise des requêtes indépendantes, pas un parcours unique ; ce serait un autre sujet (frontières, vol de travail) |
| **Sommets sur 64 bits** | `uint32` plafonne à ~4 milliards de sommets, ce qui couvre largement le périmètre ; à rouvrir si besoin |
