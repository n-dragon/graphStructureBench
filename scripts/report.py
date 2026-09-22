#!/usr/bin/env python3
"""Extrait de la campagne les tableaux retenus pour RESULTS.md.

Les chiffres du rapport sont produits ici à partir des CSV bruts : aucun
n'est recopié à la main.

    python3 scripts/report.py results > results/tables.md
"""
import csv
import sys
from pathlib import Path

ORDER = ["adjmap", "adjlist", "adjlist-exact", "csr", "csr-slices",
         "edgelist", "varint-csr", "hybrid", "bitmatrix"]


def load(results_dir):
    rows = []
    for path in sorted(Path(results_dir).glob("*.csv")):
        with open(path) as f:
            for r in csv.DictReader(f):
                for k in ("n", "m", "threads", "queries", "build_ns", "index_bytes",
                          "analytic_bytes", "scratch_bytes", "wall_ns", "p50_ns", "p99_ns", "chunk"):
                    r[k] = int(r[k])
                r["throughput_qps"] = float(r["throughput_qps"])
                rows.append(r)
    return rows


def sel(rows, **kw):
    return [r for r in rows if all(r[k] == v for k, v in kw.items())]


def by_structure(rows):
    """Trie selon l'ordre d'affichage du banc."""
    return sorted(rows, key=lambda r: ORDER.index(r["structure"]))


def uniq(rows, key):
    return sorted({r[key] for r in rows})


def table(head, body):
    widths = [len(h) for h in head]
    for row in body:
        for i, c in enumerate(row):
            widths[i] = max(widths[i], len(c))
    def line(cells):
        out = [cells[0].ljust(widths[0])] + [c.rjust(widths[i]) for i, c in enumerate(cells[1:], 1)]
        return "| " + " | ".join(out) + " |"
    sep = "| " + " | ".join(["-" * widths[0]] + ["-" * (w - 1) + ":" for w in widths[1:]]) + " |"
    return "\n".join([line(head), sep] + [line(r) for r in body])


def mib(b):
    return f"{b / (1 << 20):.1f}"


def rate(q):
    """Débit avec son unité : les charges vont du parcours complet (quelques
    requêtes par seconde) au test d'arête (dizaines de millions)."""
    if q >= 1e6:
        return f"{q / 1e6:.1f} M"
    if q >= 1e3:
        return f"{q / 1e3:.1f} k"
    return f"{q:.1f}"


def dur(ns):
    if ns >= 1e9:
        return f"{ns / 1e9:.2f} s"
    if ns >= 1e6:
        return f"{ns / 1e6:.0f} ms"
    if ns >= 1e3:
        return f"{ns / 1e3:.1f} µs"
    return f"{ns} ns"


def memory_table(rows, kind, n):
    """Construction et empreinte de l'index."""
    sub = sel(rows, kind=kind, n=n)
    if not sub:
        return None
    seen, body = set(), []
    csr = next((r for r in sub if r["structure"] == "csr"), None)
    for r in by_structure(sub):
        s = r["structure"]
        if s in seen:
            continue
        seen.add(s)
        ratio = f"{r['index_bytes'] / csr['index_bytes']:.2f}x" if csr else "-"
        body.append([s, dur(r["build_ns"]), mib(r["index_bytes"]),
                     f"{r['index_bytes'] / r['m']:.2f}",
                     f"{r['index_bytes'] / r['n']:.1f}", ratio,
                     mib(r["analytic_bytes"])])
    return table(["structure", "construction", "Mio", "o/arête", "o/sommet", "vs csr", "analytique"], body)


def throughput_table(rows, kind, n, workloads, threads=None, queries=None):
    """Débit par structure pour une liste de charges, à un point du sweep."""
    sub = sel(rows, kind=kind, n=n)
    if not sub:
        return None
    wls = [w for w in workloads if sel(sub, workload=w)]
    if not wls:
        return None
    th = threads if threads is not None else max(uniq(sub, "threads"))
    body, seen = [], set()
    for r in by_structure(sub):
        s = r["structure"]
        if s in seen:
            continue
        seen.add(s)
        row = [s]
        for w in wls:
            cand = sel(sub, structure=s, workload=w, threads=th)
            if queries is not None:
                cand = [c for c in cand if c["queries"] == queries]
            if not cand:
                row.append("-")
                continue
            best = max(cand, key=lambda c: c["throughput_qps"])
            row.append(rate(best["throughput_qps"]))
        body.append(row)
    return table(["structure"] + wls, body), th


def scaling_table(rows, kind, n, workload):
    """Débit en fonction du nombre de threads, au plus gros lot de requêtes."""
    sub = sel(rows, kind=kind, n=n, workload=workload)
    if not sub:
        return None
    q = max(uniq(sub, "queries"))
    sub = [r for r in sub if r["queries"] == q]
    threads = uniq(sub, "threads")
    body, seen = [], set()
    for r in by_structure(sub):
        s = r["structure"]
        if s in seen:
            continue
        seen.add(s)
        vals = []
        for t in threads:
            c = sel(sub, structure=s, threads=t)
            vals.append(max(c, key=lambda x: x["throughput_qps"])["throughput_qps"] if c else 0)
        row = [s] + [rate(v) for v in vals]
        row.append(f"x{vals[-1] / vals[0]:.2f}" if vals[0] else "-")
        body.append(row)
    return table(["structure"] + [f"{t} th." for t in threads] + ["gain"], body), q


def latency_table(rows, kind, n, workload):
    sub = sel(rows, kind=kind, n=n, workload=workload)
    if not sub:
        return None
    t = min(uniq(sub, "threads"))
    sub = [r for r in sub if r["threads"] == t]
    body, seen = [], set()
    for r in by_structure(sub):
        s = r["structure"]
        if s in seen:
            continue
        seen.add(s)
        c = sel(sub, structure=s)
        best = min(c, key=lambda x: x["p50_ns"])
        body.append([s, dur(best["p50_ns"]), dur(best["p99_ns"])])
    return table(["structure", "p50", "p99"], body), t


def main():
    results_dir = sys.argv[1] if len(sys.argv) > 1 else "results"
    rows = load(results_dir)
    if not rows:
        sys.exit(f"aucun CSV dans {results_dir}/")
    out = []
    kinds = [k for k in ("er", "rmat", "grid") if sel(rows, kind=k)]

    for kind in kinds:
        for n in uniq(sel(rows, kind=kind), "n"):
            m = sel(rows, kind=kind, n=n)[0]["m"]
            out.append(f"\n## {kind} — {n} sommets, {m} arêtes\n")
            t = memory_table(rows, kind, n)
            if t:
                out.append("\n### Mémoire et construction\n\n" + t + "\n")
            res = throughput_table(rows, kind, n,
                                   ["bfs", "bfs-batch", "dfs", "neighbors", "neighbors-batch", "hasedge"])
            if res:
                tbl, th = res
                out.append(f"\n### Débit à {th} threads, en requêtes/s\n\n" + tbl + "\n")
            for wl in ("bfs", "neighbors", "hasedge"):
                res = scaling_table(rows, kind, n, wl)
                if res:
                    tbl, q = res
                    out.append(f"\n### Passage à l'échelle — « {wl} », lot de {q} requêtes\n\n" + tbl + "\n")
            for wl in ("bfs", "bfs-batch"):
                res = latency_table(rows, kind, n, wl)
                if res:
                    tbl, th = res
                    out.append(f"\n### Latence d'un parcours complet — « {wl} », {th} thread\n\n" + tbl + "\n")
    print("\n".join(out))


if __name__ == "__main__":
    main()
