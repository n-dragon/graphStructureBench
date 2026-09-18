package bench

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Result est une ligne de mesure : une structure, une charge, un point du
// produit cartésien (sommets × requêtes concurrentes × threads).
type Result struct {
	Kind      string
	N, M      int
	Structure string
	Detail    string
	Workload  string
	Threads   int
	Queries   int

	BuildTime     time.Duration
	IndexBytes    uint64 // delta de tas mesuré
	AnalyticBytes uint64 // compte déclaré par la structure
	ScratchBytes  uint64 // tampons de parcours, cumulés sur les threads

	Wall       time.Duration
	Throughput float64
	P50, P99   time.Duration
	ChunkSize  int
	Checksum   uint64
}

// --- rendu de tableaux, en texte aligné ou en markdown ---

type table struct {
	title string
	head  []string
	rows  [][]string
}

func (t *table) add(cells ...string) { t.rows = append(t.rows, cells) }

func (t *table) render(w io.Writer, markdown bool) {
	widths := make([]int, len(t.head))
	for i, h := range t.head {
		widths[i] = len(h)
	}
	for _, r := range t.rows {
		for i, c := range r {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	pad := func(s string, i int) string {
		if i == 0 {
			return s + strings.Repeat(" ", widths[i]-len(s))
		}
		return strings.Repeat(" ", widths[i]-len(s)) + s // colonnes numériques à droite
	}
	line := func(cells []string) string {
		out := make([]string, len(cells))
		for i, c := range cells {
			out[i] = pad(c, i)
		}
		if markdown {
			return "| " + strings.Join(out, " | ") + " |"
		}
		return "  " + strings.Join(out, "  ")
	}

	if t.title != "" {
		if markdown {
			fmt.Fprintf(w, "\n#### %s\n\n", t.title)
		} else {
			fmt.Fprintf(w, "\n%s\n", t.title)
		}
	}
	fmt.Fprintln(w, line(t.head))
	if markdown {
		sep := make([]string, len(t.head))
		for i := range sep {
			if i == 0 {
				sep[i] = strings.Repeat("-", widths[i])
			} else {
				sep[i] = strings.Repeat("-", widths[i]-1) + ":"
			}
		}
		fmt.Fprintln(w, "| "+strings.Join(sep, " | ")+" |")
	} else {
		total := 2 * len(widths)
		for _, x := range widths {
			total += x
		}
		fmt.Fprintln(w, "  "+strings.Repeat("-", total))
	}
	for _, r := range t.rows {
		fmt.Fprintln(w, line(r))
	}
}

// --- formatage ---

func bytesHuman(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.2f Gio", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f Mio", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f Kio", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d o", b)
	}
}

func rateHuman(r float64) string {
	switch {
	case r >= 1e6:
		return fmt.Sprintf("%.2f M/s", r/1e6)
	case r >= 1e3:
		return fmt.Sprintf("%.1f k/s", r/1e3)
	default:
		return fmt.Sprintf("%.0f /s", r)
	}
}

func durHuman(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2f s", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.2f ms", float64(d)/float64(time.Millisecond))
	case d >= time.Microsecond:
		return fmt.Sprintf("%.1f µs", float64(d)/float64(time.Microsecond))
	default:
		return fmt.Sprintf("%d ns", d.Nanoseconds())
	}
}

// --- rapports ---

type graphKey struct {
	Kind string
	N, M int
}

func groupKeys(rs []Result) []graphKey {
	seen := map[graphKey]bool{}
	var keys []graphKey
	for _, r := range rs {
		k := graphKey{r.Kind, r.N, r.M}
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

func uniqueInts(vals []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, v := range vals {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

// WriteReport écrit le rapport complet : mémoire et construction par graphe,
// puis une matrice débit (threads × requêtes) par charge de travail.
func WriteReport(w io.Writer, rs []Result, markdown bool) {
	for _, gk := range groupKeys(rs) {
		var sub []Result
		for _, r := range rs {
			if r.Kind == gk.Kind && r.N == gk.N && r.M == gk.M {
				sub = append(sub, r)
			}
		}
		title := fmt.Sprintf("Graphe %s — %d sommets, %d arêtes (degré moyen %.1f)",
			gk.Kind, gk.N, gk.M, float64(gk.M)/float64(gk.N))
		if markdown {
			fmt.Fprintf(w, "\n### %s\n", title)
		} else {
			fmt.Fprintf(w, "\n=== %s\n", title)
		}
		writeMemoryTable(w, sub, markdown)
		writeThroughputTables(w, sub, markdown)
	}
}

func writeMemoryTable(w io.Writer, rs []Result, markdown bool) {
	t := &table{
		title: "Construction et empreinte mémoire de l'index",
		head:  []string{"structure", "construction", "mémoire (tas)", "octets/arête", "octets/sommet", "vs csr", "analytique", "note"},
	}
	seen := map[string]bool{}
	var csrBytes float64
	for _, r := range rs {
		if r.Structure == "csr" {
			csrBytes = float64(r.IndexBytes)
		}
	}
	for _, r := range rs {
		if seen[r.Structure] {
			continue
		}
		seen[r.Structure] = true
		ratio := "-"
		if csrBytes > 0 {
			ratio = fmt.Sprintf("%.2fx", float64(r.IndexBytes)/csrBytes)
		}
		t.add(r.Structure,
			durHuman(r.BuildTime),
			bytesHuman(r.IndexBytes),
			fmt.Sprintf("%.2f", float64(r.IndexBytes)/float64(r.M)),
			fmt.Sprintf("%.2f", float64(r.IndexBytes)/float64(r.N)),
			ratio,
			bytesHuman(r.AnalyticBytes),
			r.Detail)
	}
	t.render(w, markdown)
}

func writeThroughputTables(w io.Writer, rs []Result, markdown bool) {
	var workloads []string
	seenW := map[string]bool{}
	var structs []string
	seenS := map[string]bool{}
	var threadVals, queryVals []int
	for _, r := range rs {
		if !seenW[r.Workload] {
			seenW[r.Workload] = true
			workloads = append(workloads, r.Workload)
		}
		if !seenS[r.Structure] {
			seenS[r.Structure] = true
			structs = append(structs, r.Structure)
		}
		threadVals = append(threadVals, r.Threads)
		queryVals = append(queryVals, r.Queries)
	}
	threads := uniqueInts(threadVals)
	queries := uniqueInts(queryVals)

	index := map[string]Result{}
	key := func(s, wl string, th, q int) string {
		return s + "|" + wl + "|" + strconv.Itoa(th) + "|" + strconv.Itoa(q)
	}
	for _, r := range rs {
		index[key(r.Structure, r.Workload, r.Threads, r.Queries)] = r
	}

	for _, wl := range workloads {
		t := &table{title: fmt.Sprintf("Débit — charge « %s » (requêtes/s, plus haut = mieux)", wl)}
		t.head = append(t.head, "structure")
		for _, q := range queries {
			for _, th := range threads {
				t.head = append(t.head, fmt.Sprintf("q=%s t=%d", compactInt(q), th))
			}
		}
		t.head = append(t.head, "p50", "p99", "scaling")
		for _, s := range structs {
			row := []string{s}
			var base, best float64
			var last Result
			for _, q := range queries {
				for _, th := range threads {
					r, ok := index[key(s, wl, th, q)]
					if !ok {
						row = append(row, "-")
						continue
					}
					row = append(row, rateHuman(r.Throughput))
					if th == threads[0] && q == queries[len(queries)-1] {
						base = r.Throughput
					}
					if q == queries[len(queries)-1] && r.Throughput > best {
						best = r.Throughput
						last = r
					}
				}
			}
			scaling := "-"
			if base > 0 && best > 0 {
				scaling = fmt.Sprintf("x%.2f", best/base)
			}
			unit := ""
			if last.ChunkSize > 1 {
				unit = fmt.Sprintf("/%d", last.ChunkSize)
			}
			row = append(row, durHuman(last.P50)+unit, durHuman(last.P99)+unit, scaling)
			t.add(row...)
		}
		t.render(w, markdown)
	}
}

func compactInt(v int) string {
	switch {
	case v >= 1_000_000 && v%1_000_000 == 0:
		return strconv.Itoa(v/1_000_000) + "M"
	case v >= 1000 && v%1000 == 0:
		return strconv.Itoa(v/1000) + "k"
	default:
		return strconv.Itoa(v)
	}
}

// WriteCSV écrit toutes les mesures brutes, une ligne par point de mesure.
func WriteCSV(w io.Writer, rs []Result) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	head := []string{"kind", "n", "m", "structure", "workload", "threads", "queries",
		"build_ns", "index_bytes", "analytic_bytes", "scratch_bytes",
		"wall_ns", "throughput_qps", "p50_ns", "p99_ns", "chunk", "checksum"}
	if err := cw.Write(head); err != nil {
		return err
	}
	for _, r := range rs {
		rec := []string{r.Kind, strconv.Itoa(r.N), strconv.Itoa(r.M), r.Structure, r.Workload,
			strconv.Itoa(r.Threads), strconv.Itoa(r.Queries),
			strconv.FormatInt(r.BuildTime.Nanoseconds(), 10),
			strconv.FormatUint(r.IndexBytes, 10), strconv.FormatUint(r.AnalyticBytes, 10),
			strconv.FormatUint(r.ScratchBytes, 10),
			strconv.FormatInt(r.Wall.Nanoseconds(), 10),
			strconv.FormatFloat(r.Throughput, 'f', 2, 64),
			strconv.FormatInt(r.P50.Nanoseconds(), 10), strconv.FormatInt(r.P99.Nanoseconds(), 10),
			strconv.Itoa(r.ChunkSize), strconv.FormatUint(r.Checksum, 10)}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	return nil
}
