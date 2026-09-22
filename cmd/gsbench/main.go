// Commande gsbench : compare des structures d'index de graphe sur les deux
// critères du banc — rapidité et consommation mémoire — en balayant le produit
// cartésien (nombre de sommets × requêtes concurrentes × threads).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/n-dragon/graphstructurebench/bench"
	"github.com/n-dragon/graphstructurebench/gen"
	"github.com/n-dragon/graphstructurebench/graph"
)

func main() {
	var (
		kindsF     = flag.String("kinds", "er,rmat,grid", "topologies: er, rmat, grid (séparées par des virgules)")
		nodesF     = flag.String("nodes", "100000,1000000", "axe 1 — nombres de sommets")
		queriesF   = flag.String("queries", "", "axe 2 — tailles de lot de requêtes concurrentes (vide = défauts par charge)")
		threadsF   = flag.String("threads", "1,2,4", "axe 3 — nombres de threads")
		degF       = flag.Int("deg", 8, "degré sortant moyen visé")
		workloadsF = flag.String("workloads", "bfs,neighbors,hasedge", "charges: bfs, dfs, neighbors, hasedge")
		structsF   = flag.String("structs", "", "structures à mesurer (vide = toutes)")
		runsF      = flag.Int("runs", 5, "répétitions chronométrées ; le meilleur temps est retenu, leur dispersion est rapportée")
		seedF      = flag.Uint64("seed", 42, "graine du générateur")
		directedF  = flag.Bool("directed", false, "garder le graphe orienté (par défaut il est symétrisé)")
		csvF       = flag.String("csv", "", "écrire les mesures brutes dans ce fichier CSV")
		mdF        = flag.String("md", "", "écrire le rapport en markdown dans ce fichier")
		listF      = flag.Bool("list", false, "lister les structures et les charges puis quitter")
	)
	flag.Parse()

	if *listF {
		listCatalog()
		return
	}

	kinds := splitList(*kindsF)
	nodes, err := parseInts(*nodesF)
	check(err)
	threads, err := parseInts(*threadsF)
	check(err)
	var queryOverride []int
	if *queriesF != "" {
		queryOverride, err = parseInts(*queriesF)
		check(err)
	}

	var workloads []bench.Workload
	for _, name := range splitList(*workloadsF) {
		wl, err := bench.WorkloadByName(name)
		check(err)
		workloads = append(workloads, wl)
	}
	builders := graph.Builders
	if *structsF != "" {
		builders = nil
		for _, name := range splitList(*structsF) {
			b, err := graph.BuilderByName(name)
			check(err)
			builders = append(builders, b)
		}
	}

	fmt.Fprintf(os.Stderr, "gsbench — %s, %d cœurs, %s\n", runtime.Version(), runtime.NumCPU(), time.Now().Format(time.RFC3339))

	var results []bench.Result
	for _, kind := range kinds {
		for _, n := range nodes {
			spec := gen.Spec{Kind: kind, N: n, AvgDegree: *degF, Undirected: !*directedF, Seed: *seedF}
			g, err := gen.Generate(spec)
			check(err)
			fmt.Fprintf(os.Stderr, "\n· graphe %s\n", g.Label())

			// Les jeux de requêtes sont tirés une seule fois : toutes les
			// structures répondent exactement aux mêmes questions, ce qui rend
			// les sommes de contrôle comparables.
			r := rand.New(rand.NewPCG(*seedF+1, *seedF+2))
			type qkey struct {
				wl    string
				count int
			}
			querySets := map[qkey][]bench.Query{}
			for _, wl := range workloads {
				for _, count := range queryCounts(wl, queryOverride) {
					querySets[qkey{wl.Name, count}] = wl.Gen(r, g, count)
				}
			}

			checksums := map[qkey]uint64{}
			for _, b := range builders {
				if b.MaxVertices > 0 && g.N > b.MaxVertices {
					fmt.Fprintf(os.Stderr, "  %-14s sautée (limite %d sommets)\n", b.Name, b.MaxVertices)
					continue
				}
				idx, info, err := bench.Build(b, g)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  %-14s sautée: %v\n", b.Name, err)
					continue
				}
				fmt.Fprintf(os.Stderr, "  %-14s construite en %8v, %10d octets de tas\n", b.Name, info.Duration.Round(time.Millisecond), info.HeapBytes)

				for _, wl := range workloads {
					for _, count := range queryCounts(wl, queryOverride) {
						qs := querySets[qkey{wl.Name, count}]
						for _, th := range threads {
							bench.Run(idx, wl, qs, th) // chauffe : caches et goroutines
							best := bench.Run(idx, wl, qs, th)
							slowest := best.Wall
							for i := 1; i < *runsF; i++ {
								st := bench.Run(idx, wl, qs, th)
								if st.Wall < best.Wall {
									best = st
								}
								if st.Wall > slowest {
									slowest = st.Wall
								}
							}
							// Dispersion entre répétitions : sans elle, on
							// prendrait du bruit pour un écart de structure.
							spread := 1.0
							if best.Wall > 0 {
								spread = float64(slowest) / float64(best.Wall)
							}
							k := qkey{wl.Name, count}
							if prev, ok := checksums[k]; ok && prev != best.Checksum {
								fmt.Fprintf(os.Stderr, "  !! somme de contrôle divergente pour %s/%s: %d != %d\n", b.Name, wl.Name, best.Checksum, prev)
							} else {
								checksums[k] = best.Checksum
							}
							results = append(results, bench.Result{
								Kind: kind, N: g.N, M: len(g.Edges),
								Structure: b.Name, Detail: info.Detail, Workload: wl.Name,
								Threads: th, Queries: count,
								BuildTime: info.Duration, IndexBytes: info.HeapBytes,
								AnalyticBytes: info.AnalyticBytes, ScratchBytes: best.ScratchBytes,
								Wall: best.Wall, Throughput: best.Throughput,
								P50: best.P50, P99: best.P99, ChunkSize: best.ChunkSize,
								Spread: spread, Checksum: best.Checksum,
							})
						}
					}
				}
				idx = nil // libère l'index avant de construire le suivant
				runtime.GC()
			}
		}
	}

	out := bufio.NewWriter(os.Stdout)
	bench.WriteReport(out, results, false)
	writeVerdict(out, results)
	out.Flush()

	if *csvF != "" {
		f, err := os.Create(*csvF)
		check(err)
		check(bench.WriteCSV(f, results))
		f.Close()
		fmt.Fprintf(os.Stderr, "\nmesures brutes: %s\n", *csvF)
	}
	if *mdF != "" {
		f, err := os.Create(*mdF)
		check(err)
		w := bufio.NewWriter(f)
		fmt.Fprintf(w, "# Résultats gsbench\n\n%s, %d cœurs, %s\n",
			runtime.Version(), runtime.NumCPU(), time.Now().Format("2006-01-02"))
		bench.WriteReport(w, results, true)
		writeVerdict(w, results)
		w.Flush()
		f.Close()
		fmt.Fprintf(os.Stderr, "rapport markdown: %s\n", *mdF)
	}
}

// queryCounts choisit l'axe « requêtes » : ce que demande l'utilisateur, ou
// les défauts de la charge (un BFS complet et un test d'arête ne se mesurent
// pas avec les mêmes volumes).
func queryCounts(wl bench.Workload, override []int) []int {
	if len(override) > 0 {
		return override
	}
	return wl.DefaultQueries
}

// writeVerdict désigne, par graphe et par charge, la structure la plus rapide
// et la plus compacte.
func writeVerdict(w *bufio.Writer, rs []bench.Result) {
	if len(rs) == 0 {
		return
	}
	fmt.Fprintf(w, "\n=== Synthèse\n\n")
	type key struct {
		kind string
		n    int
		wl   string
	}
	bestRate := map[key]bench.Result{}
	smallest := map[key]bench.Result{}
	for _, r := range rs {
		k := key{r.Kind, r.N, r.Workload}
		if cur, ok := bestRate[k]; !ok || r.Throughput > cur.Throughput {
			bestRate[k] = r
		}
		if cur, ok := smallest[k]; !ok || r.IndexBytes < cur.IndexBytes {
			smallest[k] = r
		}
	}
	var keys []key
	seen := map[key]bool{}
	for _, r := range rs {
		k := key{r.Kind, r.N, r.Workload}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		f, s := bestRate[k], smallest[k]
		fmt.Fprintf(w, "  %-6s n=%-9d %-10s plus rapide: %-14s (%s, %d threads) | plus compacte: %-14s (%s)\n",
			k.kind, k.n, k.wl, f.Structure, rateHuman(f.Throughput), f.Threads,
			s.Structure, bytesHuman(s.IndexBytes))
	}
}

func rateHuman(r float64) string {
	if r >= 1e6 {
		return fmt.Sprintf("%.2f M req/s", r/1e6)
	}
	if r >= 1e3 {
		return fmt.Sprintf("%.1f k req/s", r/1e3)
	}
	return fmt.Sprintf("%.0f req/s", r)
}

func bytesHuman(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f Gio", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f Mio", float64(b)/(1<<20))
	default:
		return fmt.Sprintf("%.1f Kio", float64(b)/(1<<10))
	}
}

func listCatalog() {
	fmt.Println("Structures d'index:")
	for _, b := range graph.Builders {
		limit := ""
		if b.MaxVertices > 0 {
			limit = fmt.Sprintf(" (max %d sommets)", b.MaxVertices)
		}
		fmt.Printf("  %-14s %s%s\n", b.Name, b.Desc, limit)
	}
	fmt.Println("\nCharges de travail:")
	for _, w := range bench.Workloads {
		fmt.Printf("  %-14s %s\n", w.Name, w.Desc)
	}
	fmt.Println("\nTopologies: er (uniforme), rmat (loi de puissance), grid (grille 2D)")
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseInts(s string) ([]int, error) {
	var out []int
	for _, p := range splitList(s) {
		v, err := strconv.Atoi(strings.ReplaceAll(p, "_", ""))
		if err != nil {
			return nil, fmt.Errorf("entier invalide %q: %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur:", err)
		os.Exit(1)
	}
}
