#!/usr/bin/env bash
# Campagne de mesure reproductible. Trois campagnes séparées, parce qu'un BFS
# complet et un test d'arête ne se mesurent pas avec les mêmes volumes de
# requêtes : sur chacune, le produit cartésien sommets × requêtes × threads est
# balayé en entier.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT=${OUT:-results}
THREADS=${THREADS:-1,2,4}
mkdir -p "$OUT"
go build -o "$OUT/gsbench" ./cmd/gsbench

echo "== campagne 1/3 : petits graphes, toutes structures (matrice dense comprise)"
"$OUT/gsbench" -kinds er,rmat,grid -nodes 20000 -threads "$THREADS" \
  -workloads bfs,neighbors,hasedge -queries 64,512 \
  -csv "$OUT/small.csv" -md "$OUT/small.md" > "$OUT/small.txt"

echo "== campagne 2/3 : parcours complets à l'échelle"
"$OUT/gsbench" -kinds er,rmat,grid -nodes 200000,1000000 -threads "$THREADS" \
  -workloads bfs -queries 4,16 \
  -csv "$OUT/traversal.csv" -md "$OUT/traversal.md" > "$OUT/traversal.txt"

echo "== campagne 3/3 : requêtes ponctuelles à haut débit"
"$OUT/gsbench" -kinds er,rmat,grid -nodes 200000,1000000 -threads "$THREADS" \
  -workloads neighbors,hasedge -queries 200000,2000000 \
  -csv "$OUT/queries.csv" -md "$OUT/queries.md" > "$OUT/queries.txt"

echo "résultats dans $OUT/"
