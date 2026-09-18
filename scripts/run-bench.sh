#!/usr/bin/env bash
# Campagne de mesure reproductible.
#
# Quatre campagnes, parce qu'un BFS complet et un test d'arête ne se mesurent
# pas avec les mêmes volumes de requêtes ni aux mêmes tailles de graphe. Sur
# chacune, le produit cartésien sommets × requêtes concurrentes × threads est
# balayé en entier, sur les trois topologies.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT=${OUT:-results}
THREADS=${THREADS:-1,2,4}
KINDS=${KINDS:-er,rmat,grid}
mkdir -p "$OUT"
go build -o "$OUT/gsbench" ./cmd/gsbench

run() { # run <nom> <arguments...>
  local name=$1; shift
  echo "== $name"
  "$OUT/gsbench" -kinds "$KINDS" -threads "$THREADS" "$@" \
    -csv "$OUT/$name.csv" -md "$OUT/$name.md" > "$OUT/$name.txt"
}

# Petits graphes : seule échelle où la matrice d'adjacence dense tient en
# mémoire, donc la seule où l'on dispose de la borne basse en latence.
run small-traversal -nodes 20000 -workloads bfs,bfs-batch,dfs -queries 64,512
run small-queries   -nodes 20000 -workloads neighbors,neighbors-batch,hasedge -queries 10000,500000

# Grande échelle : le graphe ne tient plus dans le cache, la localité mémoire
# de la structure devient le facteur dominant.
run large-traversal -nodes 200000,1000000 -workloads bfs,bfs-batch -queries 4,16
run large-queries   -nodes 200000,1000000 -workloads neighbors,neighbors-batch,hasedge -queries 200000,2000000

# Coût des trois façons de lire l'adjacence : callback, bloc, slice directe.
echo "== callback-overhead"
go test ./bench -run '^$' -bench CallbackOverhead -benchtime=500ms > "$OUT/callback-overhead.txt" 2>&1

echo "résultats dans $OUT/"
